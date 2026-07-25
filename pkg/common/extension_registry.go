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

// ExtensionRegistry manages all registered extensions.
// Extensions are executed in priority order (highest first), same-priority extensions in
// registration order.
type ExtensionRegistry struct {
	clusterExtensions   []extensionEntry[ClusterExtension[ClusterInterface]]
	roleExtensions      []extensionEntry[RoleExtension[ClusterInterface]]
	roleGroupExtensions []extensionEntry[RoleGroupExtension[ClusterInterface]]
	// nextSeq hands out registration sequence numbers; guarded by mu.
	nextSeq uint64
	mu      sync.RWMutex
}

// NewExtensionRegistry creates an empty, isolated registry. Use it when extensions must not
// leak into the process-wide singleton — a controller that owns its extensions, or tests
// that run in parallel.
func NewExtensionRegistry() *ExtensionRegistry {
	return &ExtensionRegistry{}
}

// globalRegistry is the singleton instance.
// Thread-safety: All access to globalRegistry is protected by sync.RWMutex.
// For testing scenarios, use ResetExtensionRegistry() to reset state between tests,
// or create isolated instances with NewExtensionRegistry() for parallel tests.
var globalRegistry = NewExtensionRegistry()

// GetExtensionRegistry returns the global registry singleton.
func GetExtensionRegistry() *ExtensionRegistry {
	return globalRegistry
}

// ResetExtensionRegistry empties the global registry (for testing).
// The singleton pointer is deliberately left untouched: reconcilers capture the registry at
// construction time, so replacing the pointer would leave them executing a stale registry.
func ResetExtensionRegistry() {
	globalRegistry.Clear()
}

// RegisterClusterExtension registers a cluster-level extension with default priority.
func (r *ExtensionRegistry) RegisterClusterExtension(extension ClusterExtension[ClusterInterface]) {
	r.RegisterClusterExtensionWithOptions(extension)
}

// RegisterClusterExtensionWithPriority registers a cluster-level extension with specific priority.
func (r *ExtensionRegistry) RegisterClusterExtensionWithPriority(extension ClusterExtension[ClusterInterface], priority ExtensionPriority) {
	r.RegisterClusterExtensionWithOptions(extension, WithPriority(priority))
}

// RegisterClusterExtensionWithOptions registers a cluster-level extension, see WithPriority
// and WithStopOnError for the available options.
func (r *ExtensionRegistry) RegisterClusterExtensionWithOptions(extension ClusterExtension[ClusterInterface], opts ...RegistrationOption) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.clusterExtensions = addEntry(r.clusterExtensions, extension, r.takeSeq(), opts)
}

// RegisterRoleExtension registers a role-level extension with default priority.
func (r *ExtensionRegistry) RegisterRoleExtension(extension RoleExtension[ClusterInterface]) {
	r.RegisterRoleExtensionWithOptions(extension)
}

// RegisterRoleExtensionWithPriority registers a role-level extension with specific priority.
func (r *ExtensionRegistry) RegisterRoleExtensionWithPriority(extension RoleExtension[ClusterInterface], priority ExtensionPriority) {
	r.RegisterRoleExtensionWithOptions(extension, WithPriority(priority))
}

// RegisterRoleExtensionWithOptions registers a role-level extension, see WithPriority and
// WithStopOnError for the available options.
func (r *ExtensionRegistry) RegisterRoleExtensionWithOptions(extension RoleExtension[ClusterInterface], opts ...RegistrationOption) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.roleExtensions = addEntry(r.roleExtensions, extension, r.takeSeq(), opts)
}

// RegisterRoleGroupExtension registers a role group-level extension with default priority.
func (r *ExtensionRegistry) RegisterRoleGroupExtension(extension RoleGroupExtension[ClusterInterface]) {
	r.RegisterRoleGroupExtensionWithOptions(extension)
}

// RegisterRoleGroupExtensionWithPriority registers a role group-level extension with specific priority.
func (r *ExtensionRegistry) RegisterRoleGroupExtensionWithPriority(extension RoleGroupExtension[ClusterInterface], priority ExtensionPriority) {
	r.RegisterRoleGroupExtensionWithOptions(extension, WithPriority(priority))
}

// RegisterRoleGroupExtensionWithOptions registers a role group-level extension, see
// WithPriority and WithStopOnError for the available options.
func (r *ExtensionRegistry) RegisterRoleGroupExtensionWithOptions(extension RoleGroupExtension[ClusterInterface], opts ...RegistrationOption) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.roleGroupExtensions = addEntry(r.roleGroupExtensions, extension, r.takeSeq(), opts)
}

// takeSeq returns the next registration sequence number. Callers must hold r.mu.
func (r *ExtensionRegistry) takeSeq() uint64 {
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
// logged and the remaining extensions still run.
func executeHooks[T Extension](ctx context.Context, entries []extensionEntry[T], policy hookPolicy, hook func(T) error) error {
	var errs []error
	for _, entry := range entries {
		err := hook(entry.extension)
		if err == nil {
			continue
		}

		extensionErr := NewExtensionError(entry.extension.Name(), err)
		if entry.stopsOnError(policy.stopOnError) {
			return extensionErr
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
func (r *ExtensionRegistry) clusterEntries() []extensionEntry[ClusterExtension[ClusterInterface]] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return slices.Clone(r.clusterExtensions)
}

// roleEntries returns a snapshot of the role entries in execution order.
func (r *ExtensionRegistry) roleEntries() []extensionEntry[RoleExtension[ClusterInterface]] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return slices.Clone(r.roleExtensions)
}

// roleGroupEntries returns a snapshot of the role group entries in execution order.
func (r *ExtensionRegistry) roleGroupEntries() []extensionEntry[RoleGroupExtension[ClusterInterface]] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return slices.Clone(r.roleGroupExtensions)
}

// GetClusterExtensions returns all registered cluster extensions in execution order.
func (r *ExtensionRegistry) GetClusterExtensions() []ClusterExtension[ClusterInterface] {
	return extensionsOf(r.clusterEntries())
}

// GetRoleExtensions returns all registered role extensions in execution order.
func (r *ExtensionRegistry) GetRoleExtensions() []RoleExtension[ClusterInterface] {
	return extensionsOf(r.roleEntries())
}

// GetRoleGroupExtensions returns all registered role group extensions in execution order.
func (r *ExtensionRegistry) GetRoleGroupExtensions() []RoleGroupExtension[ClusterInterface] {
	return extensionsOf(r.roleGroupEntries())
}

// ExecuteClusterPreReconcile executes all cluster PreReconcile hooks.
func (r *ExtensionRegistry) ExecuteClusterPreReconcile(ctx context.Context, client client.Client, cr ClusterInterface) error {
	return executeHooks(ctx, r.clusterEntries(), reconcileHookPolicy,
		func(ext ClusterExtension[ClusterInterface]) error {
			return ext.PreReconcile(ctx, client, cr)
		})
}

// ExecuteClusterPostReconcile executes all cluster PostReconcile hooks.
func (r *ExtensionRegistry) ExecuteClusterPostReconcile(ctx context.Context, client client.Client, cr ClusterInterface) error {
	return executeHooks(ctx, r.clusterEntries(), reconcileHookPolicy,
		func(ext ClusterExtension[ClusterInterface]) error {
			return ext.PostReconcile(ctx, client, cr)
		})
}

// ExecuteClusterOnError executes all cluster OnReconcileError hooks.
// Handler failures are logged instead of returned, so that one broken handler neither hides
// the original reconcile error nor skips the cleanup of the remaining handlers. A handler
// registered with WithStopOnError(true) opts out and aborts the loop.
func (r *ExtensionRegistry) ExecuteClusterOnError(ctx context.Context, client client.Client, cr ClusterInterface, reconcileErr error) error {
	return executeHooks(ctx, r.clusterEntries(), errorHookPolicy,
		func(ext ClusterExtension[ClusterInterface]) error {
			return ext.OnReconcileError(ctx, client, cr, reconcileErr)
		})
}

// ExecuteRolePreReconcile executes all role PreReconcile hooks.
func (r *ExtensionRegistry) ExecuteRolePreReconcile(ctx context.Context, client client.Client, cr ClusterInterface, roleName string) error {
	return executeHooks(ctx, r.roleEntries(), reconcileHookPolicy,
		func(ext RoleExtension[ClusterInterface]) error {
			return ext.PreReconcile(ctx, client, cr, roleName)
		})
}

// ExecuteRolePostReconcile executes all role PostReconcile hooks.
func (r *ExtensionRegistry) ExecuteRolePostReconcile(ctx context.Context, client client.Client, cr ClusterInterface, roleName string) error {
	return executeHooks(ctx, r.roleEntries(), reconcileHookPolicy,
		func(ext RoleExtension[ClusterInterface]) error {
			return ext.PostReconcile(ctx, client, cr, roleName)
		})
}

// ExecuteRoleGroupPreReconcile executes all role group PreReconcile hooks.
func (r *ExtensionRegistry) ExecuteRoleGroupPreReconcile(ctx context.Context, client client.Client, cr ClusterInterface, roleName, roleGroupName string) error {
	return executeHooks(ctx, r.roleGroupEntries(), reconcileHookPolicy,
		func(ext RoleGroupExtension[ClusterInterface]) error {
			return ext.PreReconcile(ctx, client, cr, roleName, roleGroupName)
		})
}

// ExecuteRoleGroupPostReconcile executes all role group PostReconcile hooks.
func (r *ExtensionRegistry) ExecuteRoleGroupPostReconcile(ctx context.Context, client client.Client, cr ClusterInterface, roleName, roleGroupName string) error {
	return executeHooks(ctx, r.roleGroupEntries(), reconcileHookPolicy,
		func(ext RoleGroupExtension[ClusterInterface]) error {
			return ext.PostReconcile(ctx, client, cr, roleName, roleGroupName)
		})
}

// HasClusterExtensions returns true if any cluster extensions are registered.
func (r *ExtensionRegistry) HasClusterExtensions() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.clusterExtensions) > 0
}

// HasRoleExtensions returns true if any role extensions are registered.
func (r *ExtensionRegistry) HasRoleExtensions() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.roleExtensions) > 0
}

// HasRoleGroupExtensions returns true if any role group extensions are registered.
func (r *ExtensionRegistry) HasRoleGroupExtensions() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.roleGroupExtensions) > 0
}

// Clear removes all registered extensions (for testing).
func (r *ExtensionRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.clusterExtensions = nil
	r.roleExtensions = nil
	r.roleGroupExtensions = nil
	r.nextSeq = 0
}

// Count returns the total number of registered extensions.
func (r *ExtensionRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.clusterExtensions) + len(r.roleExtensions) + len(r.roleGroupExtensions)
}
