/*
Copyright 2024 ZNCDataDev.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package common

import (
	"context"
	"errors"
	"slices"
	"sort"
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// extensionEntry wraps a registered extension with its ordering and fault-tolerance metadata.
type extensionEntry[T Extension] struct {
	extension T
	priority  ExtensionPriority
	// seq is the registration sequence number within the owning registry. It makes the
	// ordering of same-priority extensions total, so execution order never depends on the
	// stability of the sort algorithm.
	seq uint64
	// stopOnError overrides the hook's default fault tolerance when non-nil.
	stopOnError *bool
}

// stopsOnError reports whether a failure of this extension skips the remaining extensions.
func (e extensionEntry[T]) stopsOnError(hookDefault bool) bool {
	if e.stopOnError != nil {
		return *e.stopOnError
	}
	return hookDefault
}

// RegistrationOption configures a single extension registration.
type RegistrationOption func(*registrationOptions)

type registrationOptions struct {
	priority    ExtensionPriority
	stopOnError *bool
}

// WithPriority sets the execution priority of a registration. Extensions execute from
// highest to lowest priority; same-priority extensions execute in registration order.
// Defaults to PriorityNormal.
func WithPriority(priority ExtensionPriority) RegistrationOption {
	return func(o *registrationOptions) {
		o.priority = priority
	}
}

// WithStopOnError controls whether a failure of this extension skips the extensions that
// would run after it in the same hook. The defaults match the fault tolerance each hook
// needs: PreReconcile/PostReconcile stop (reconciliation cannot continue on a broken
// precondition), OnReconcileError continues (every handler must get its chance to clean up).
// A non-stopping PreReconcile/PostReconcile failure is still reported to the caller, so it
// reaches the CR status.
func WithStopOnError(stop bool) RegistrationOption {
	return func(o *registrationOptions) {
		o.stopOnError = &stop
	}
}

// hookPolicy holds the per-hook defaults applied to registrations that did not opt out.
type hookPolicy struct {
	// stopOnError is the default fault tolerance of the hook.
	stopOnError bool
	// aggregate reports whether failures of non-stopping extensions are returned to the
	// caller. Error handlers only log them so the original reconcile error stays authoritative.
	aggregate bool
}

var (
	// reconcileHookPolicy governs PreReconcile/PostReconcile hooks.
	reconcileHookPolicy = hookPolicy{stopOnError: true, aggregate: true}
	// errorHookPolicy governs OnReconcileError hooks.
	errorHookPolicy = hookPolicy{stopOnError: false, aggregate: false}
)

// ExtensionRegistry holds the extensions of a single product CR type. Extensions execute in
// priority order (highest first); same-priority extensions execute in registration order.
//
// The type parameter is load-bearing rather than cosmetic. Go generic types are invariant, so a
// ClusterExtension[*TrinoCluster] does not satisfy ClusterExtension[ClusterInterface]: a registry
// erased to the wide interface can only ever hold extensions written against ClusterInterface,
// forcing every product hook to convert its own CR on entry. Instantiating the registry for the
// product's CR type is what lets an extension declare the CR it operates on and receive exactly
// that.
//
// The type parameter also bounds what a registry can reach. A registry is owned by the reconciler
// it is passed to (reconciler.GenericReconcilerConfig.ExtensionRegistry), so a binary hosting two
// products cannot run one product's hooks against the other's clusters — neither by construction
// (a foreign extension does not typecheck) nor by accident (there is no shared instance). This is
// why the package exposes no process-wide default registry.
type ExtensionRegistry[CR ClusterInterface] struct {
	clusterExtensions   []extensionEntry[ClusterExtension[CR]]
	roleExtensions      []extensionEntry[RoleExtension[CR]]
	roleGroupExtensions []extensionEntry[RoleGroupExtension[CR]]
	// nextSeq hands out registration sequence numbers; guarded by mu.
	nextSeq uint64
	mu      sync.RWMutex
}

// NewExtensionRegistry creates an empty registry for a single CR type. Pass the result to the
// product's GenericReconcilerConfig; a reconciler executes hooks from no other registry.
func NewExtensionRegistry[CR ClusterInterface]() *ExtensionRegistry[CR] {
	return &ExtensionRegistry[CR]{}
}

// RegisterClusterExtension registers a cluster-level extension. See WithPriority and
// WithStopOnError for the available options.
func (r *ExtensionRegistry[CR]) RegisterClusterExtension(extension ClusterExtension[CR], opts ...RegistrationOption) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.clusterExtensions = addEntry(r.clusterExtensions, extension, r.takeSeq(), opts)
}

// RegisterRoleExtension registers a role-level extension. See WithPriority and WithStopOnError
// for the available options.
func (r *ExtensionRegistry[CR]) RegisterRoleExtension(extension RoleExtension[CR], opts ...RegistrationOption) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.roleExtensions = addEntry(r.roleExtensions, extension, r.takeSeq(), opts)
}

// RegisterRoleGroupExtension registers a role group-level extension. See WithPriority and
// WithStopOnError for the available options.
func (r *ExtensionRegistry[CR]) RegisterRoleGroupExtension(extension RoleGroupExtension[CR], opts ...RegistrationOption) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.roleGroupExtensions = addEntry(r.roleGroupExtensions, extension, r.takeSeq(), opts)
}

// takeSeq returns the next registration sequence number. Callers must hold r.mu.
func (r *ExtensionRegistry[CR]) takeSeq() uint64 {
	seq := r.nextSeq
	r.nextSeq++
	return seq
}

// addEntry appends an extension and restores the (priority descending, registration
// ascending) order.
func addEntry[T Extension](entries []extensionEntry[T], extension T, seq uint64, opts []RegistrationOption) []extensionEntry[T] {
	options := registrationOptions{priority: PriorityNormal}
	for _, opt := range opts {
		opt(&options)
	}

	entries = append(entries, extensionEntry[T]{
		extension:   extension,
		priority:    options.priority,
		seq:         seq,
		stopOnError: options.stopOnError,
	})

	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].priority != entries[j].priority {
			return entries[i].priority > entries[j].priority
		}
		return entries[i].seq < entries[j].seq
	})
	return entries
}

// extensionsOf projects registry entries onto the bare extensions.
func extensionsOf[T Extension](entries []extensionEntry[T]) []T {
	extensions := make([]T, len(entries))
	for i, entry := range entries {
		extensions[i] = entry.extension
	}
	return extensions
}

// executeHooks runs hook for every entry in execution order. A failing extension aborts the
// loop when its (possibly overridden) stop-on-error setting says so; otherwise the failure is
// logged and the remaining extensions still run. Every failure the policy aggregates is part
// of the returned error, whether the loop ran to completion or was stopped by a later one.
func executeHooks[T Extension](ctx context.Context, entries []extensionEntry[T], policy hookPolicy, hook func(T) error) error {
	var errs []error
	for _, entry := range entries {
		err := hook(entry.extension)
		if err == nil {
			continue
		}

		extensionErr := NewExtensionError(entry.extension.Name(), err)

		// A wait is not a precondition failure. Without this branch the default
		// hookPolicy{stopOnError: true} let one extension that is merely waiting abort every
		// lower-priority sibling — so a product following AGENTS.md §11b, which puts
		// EnsureGeneratedSecret in this very hook, would stop generating a Secret another
		// extension is waiting for. It is also logged at Info rather than Error: #608's complaint
		// is that a normal first install pages, and an ERROR line every pass forever is the other
		// channel that pages.
		// WaitingErrors, not IsRequeueAfterError: a hook may itself return errors.Join of a wait and
		// a genuine failure, and errors.As would find the wait and launder the failure into one.
		if waiting, _ := WaitingErrors(err); waiting {
			log.FromContext(ctx).Info("Extension is waiting, continuing with remaining extensions",
				"extension", entry.extension.Name(), "reason", err.Error())
			errs = append(errs, extensionErr)
			continue
		}

		if entry.stopsOnError(policy.stopOnError) {
			// Failures already collected from tolerant extensions are reported alongside the
			// stopping one; dropping them would hide from the CR status that those extensions
			// failed too. With nothing collected the *ExtensionError is returned unwrapped so
			// the common single-failure case stays directly inspectable.
			if len(errs) == 0 {
				return extensionErr
			}
			return errors.Join(append(errs, extensionErr)...)
		}

		log.FromContext(ctx).Error(err, "Extension hook failed, continuing with remaining extensions",
			"extension", entry.extension.Name())
		if policy.aggregate {
			errs = append(errs, extensionErr)
		}
	}
	return errors.Join(errs...)
}

// clusterEntries returns a snapshot of the cluster entries in execution order.
func (r *ExtensionRegistry[CR]) clusterEntries() []extensionEntry[ClusterExtension[CR]] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return slices.Clone(r.clusterExtensions)
}

// roleEntries returns a snapshot of the role entries in execution order.
func (r *ExtensionRegistry[CR]) roleEntries() []extensionEntry[RoleExtension[CR]] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return slices.Clone(r.roleExtensions)
}

// roleGroupEntries returns a snapshot of the role group entries in execution order.
func (r *ExtensionRegistry[CR]) roleGroupEntries() []extensionEntry[RoleGroupExtension[CR]] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return slices.Clone(r.roleGroupExtensions)
}

// GetClusterExtensions returns all registered cluster extensions in execution order.
func (r *ExtensionRegistry[CR]) GetClusterExtensions() []ClusterExtension[CR] {
	return extensionsOf(r.clusterEntries())
}

// GetRoleExtensions returns all registered role extensions in execution order.
func (r *ExtensionRegistry[CR]) GetRoleExtensions() []RoleExtension[CR] {
	return extensionsOf(r.roleEntries())
}

// GetRoleGroupExtensions returns all registered role group extensions in execution order.
func (r *ExtensionRegistry[CR]) GetRoleGroupExtensions() []RoleGroupExtension[CR] {
	return extensionsOf(r.roleGroupEntries())
}

// ExecuteClusterPreReconcile executes all cluster PreReconcile hooks.
func (r *ExtensionRegistry[CR]) ExecuteClusterPreReconcile(ctx context.Context, client client.Client, cr CR) error {
	return executeHooks(ctx, r.clusterEntries(), reconcileHookPolicy,
		func(ext ClusterExtension[CR]) error {
			return ext.PreReconcile(ctx, client, cr)
		})
}

// ExecuteClusterPostReconcile executes all cluster PostReconcile hooks.
func (r *ExtensionRegistry[CR]) ExecuteClusterPostReconcile(ctx context.Context, client client.Client, cr CR) error {
	return executeHooks(ctx, r.clusterEntries(), reconcileHookPolicy,
		func(ext ClusterExtension[CR]) error {
			return ext.PostReconcile(ctx, client, cr)
		})
}

// ExecuteClusterOnError executes all cluster OnReconcileError hooks.
// Handler failures are logged instead of returned, so that one broken handler neither hides
// the original reconcile error nor skips the cleanup of the remaining handlers. A handler
// registered with WithStopOnError(true) opts out and aborts the loop.
func (r *ExtensionRegistry[CR]) ExecuteClusterOnError(ctx context.Context, client client.Client, cr CR, reconcileErr error) error {
	return executeHooks(ctx, r.clusterEntries(), errorHookPolicy,
		func(ext ClusterExtension[CR]) error {
			return ext.OnReconcileError(ctx, client, cr, reconcileErr)
		})
}

// ExecuteRolePreReconcile executes all role PreReconcile hooks.
func (r *ExtensionRegistry[CR]) ExecuteRolePreReconcile(ctx context.Context, client client.Client, cr CR, roleName string) error {
	return executeHooks(ctx, r.roleEntries(), reconcileHookPolicy,
		func(ext RoleExtension[CR]) error {
			return ext.PreReconcile(ctx, client, cr, roleName)
		})
}

// ExecuteRolePostReconcile executes all role PostReconcile hooks.
func (r *ExtensionRegistry[CR]) ExecuteRolePostReconcile(ctx context.Context, client client.Client, cr CR, roleName string) error {
	return executeHooks(ctx, r.roleEntries(), reconcileHookPolicy,
		func(ext RoleExtension[CR]) error {
			return ext.PostReconcile(ctx, client, cr, roleName)
		})
}

// ExecuteRoleGroupPreReconcile executes all role group PreReconcile hooks.
func (r *ExtensionRegistry[CR]) ExecuteRoleGroupPreReconcile(ctx context.Context, client client.Client, cr CR, roleName, roleGroupName string) error {
	return executeHooks(ctx, r.roleGroupEntries(), reconcileHookPolicy,
		func(ext RoleGroupExtension[CR]) error {
			return ext.PreReconcile(ctx, client, cr, roleName, roleGroupName)
		})
}

// ExecuteRoleGroupPostReconcile executes all role group PostReconcile hooks.
func (r *ExtensionRegistry[CR]) ExecuteRoleGroupPostReconcile(ctx context.Context, client client.Client, cr CR, roleName, roleGroupName string) error {
	return executeHooks(ctx, r.roleGroupEntries(), reconcileHookPolicy,
		func(ext RoleGroupExtension[CR]) error {
			return ext.PostReconcile(ctx, client, cr, roleName, roleGroupName)
		})
}

// HasClusterExtensions returns true if any cluster extensions are registered.
func (r *ExtensionRegistry[CR]) HasClusterExtensions() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.clusterExtensions) > 0
}

// HasRoleExtensions returns true if any role extensions are registered.
func (r *ExtensionRegistry[CR]) HasRoleExtensions() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.roleExtensions) > 0
}

// HasRoleGroupExtensions returns true if any role group extensions are registered.
func (r *ExtensionRegistry[CR]) HasRoleGroupExtensions() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.roleGroupExtensions) > 0
}

// Clear removes all registered extensions (for testing).
// The registry is emptied in place rather than replaced: a reconciler captures the registry at
// construction time, so handing out a fresh instance would leave it executing a stale one.
func (r *ExtensionRegistry[CR]) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.clusterExtensions = nil
	r.roleExtensions = nil
	r.roleGroupExtensions = nil
	r.nextSeq = 0
}

// Count returns the total number of registered extensions.
func (r *ExtensionRegistry[CR]) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.clusterExtensions) + len(r.roleExtensions) + len(r.roleGroupExtensions)
}
