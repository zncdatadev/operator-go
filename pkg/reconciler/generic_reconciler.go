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

package reconciler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	stderrors "errors"
	"fmt"
	"maps"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/common"
	"github.com/zncdatadev/operator-go/pkg/config"
	"github.com/zncdatadev/operator-go/pkg/constant"
	"github.com/zncdatadev/operator-go/pkg/productlogging"
	"github.com/zncdatadev/operator-go/pkg/sidecar"
	"github.com/zncdatadev/operator-go/pkg/util"
	"github.com/zncdatadev/operator-go/pkg/vector"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// valueTrue is the canonical string form of a boolean carried by a Kubernetes label or
// annotation, where values are always strings.
const valueTrue = "true"

// Default health check configuration.
const (
	// DefaultHealthCheckInterval is the default interval between health checks.
	DefaultHealthCheckInterval = 120 * time.Second
	// DefaultHealthCheckTimeout is the default timeout for health checks.
	DefaultHealthCheckTimeout = 300 * time.Second
)

// GenericReconcilerConfig contains configuration for creating a GenericReconciler.
type GenericReconcilerConfig[CR common.ClusterInterface] struct {
	// Client is the Kubernetes client.
	Client client.Client

	// APIReader reads directly from the API server, bypassing the manager's informer cache.
	// It is used to refresh the resourceVersion after a conflicting status write: a conflict
	// means the cache has not seen the competing write yet, so refreshing from the cache would
	// retry with the same stale version. Pass mgr.GetAPIReader(). Optional; when unset the
	// regular Client is used, which makes conflict recovery unreliable under a cached client.
	// +optional
	APIReader client.Reader

	// Scheme is the runtime scheme for registering types.
	Scheme *runtime.Scheme

	// Recorder is the event recorder for emitting events.
	Recorder record.EventRecorder

	// RoleGroupHandler is the product-specific handler for building resources.
	RoleGroupHandler RoleGroupHandler[CR]

	// HealthCheckInterval is the interval between health checks. It is the cadence at which a
	// successful reconcile requeues itself (ctrl.Result.RequeueAfter), so state that produces no
	// watch event — a ServiceHealthCheck probe, a StatefulSet that never changes — is still
	// re-evaluated. Defaults to 120s; set a negative value to disable periodic requeue entirely.
	HealthCheckInterval time.Duration

	// HealthCheckTimeout bounds a single health check pass: the product-level ServiceHealthCheck
	// runs under a context with this deadline, so a hanging probe cannot stall the reconcile
	// worker indefinitely. Defaults to 300s; a non-positive value disables the deadline.
	HealthCheckTimeout time.Duration

	// ServiceHealthCheck is an optional product-level health check.
	// When set, it is called after pod-level health verification in each reconciliation cycle.
	// Products use this to verify application readiness (e.g., HDFS SafeMode off).
	// +optional
	ServiceHealthCheck common.ServiceHealthCheck

	// RateLimitRetryAfter is the duration to wait before retrying after a 429 Too Many Requests
	// response from the Kubernetes API. Defaults to 10s if not specified.
	// +optional
	RateLimitRetryAfter time.Duration

	// GrayDeleteGracePeriod sets the grace period for orphaned resource cleanup.
	// When > 0, resources are annotated on first detection and deleted only after this duration.
	// When 0 (default), orphaned resources are deleted immediately.
	// +optional
	GrayDeleteGracePeriod time.Duration

	// ServiceAccountName is the static name of the ServiceAccount to create for the workload.
	// When set (and ServiceAccountNameFunc is nil or returns ""), the GenericReconciler
	// automatically creates (or updates) a ServiceAccount with this name in the CR's namespace
	// at the start of each reconciliation and propagates it to the workload pod template via
	// RoleGroupBuildContext.ServiceAccountName.
	//
	// WARNING: a static name is shared by every CR of the product in a namespace. With two
	// clusters in one namespace, the second cluster can never take controller ownership of the
	// shared SA (its reconcile fails permanently), and deleting the first cluster garbage-collects
	// the SA out from under the second cluster's running pods. Prefer ServiceAccountNameFunc for
	// per-CR naming; keep this field only as a fallback or for single-cluster-per-namespace use.
	// +optional
	ServiceAccountName string

	// ServiceAccountNameFunc computes the ServiceAccount name per CR at reconcile time.
	// When set and returning a non-empty string, its result takes precedence over the static
	// ServiceAccountName. When it returns "", the static ServiceAccountName is used as fallback;
	// if that is also empty, ServiceAccount management is skipped entirely.
	//
	// RECOMMENDED for multi-cluster namespaces: derive the name from the CR, e.g.
	// "<product>-<cr name>" ("kafka-" + cr.GetName()), so each cluster owns its own SA. This
	// avoids the shared-SA failure mode described on ServiceAccountName (permanent ownership
	// conflict for the second cluster, and SA garbage collection when the first cluster is
	// deleted).
	// +optional
	ServiceAccountNameFunc func(cr CR) string

	// ProductConfig, when set, computes the product's configuration contribution for a role
	// group at reconcile time, returned as an *v1alpha1.OverridesSpec (the same shape users
	// write in the CRD). The GenericReconciler merges it as the LOWEST-precedence layer,
	// beneath the role and role group overrides, so a user's configOverrides always win.
	//
	// This is config generation, not defaulting: unlike the webhook ProductDefaulter (which
	// fills static fallbacks into typed spec fields at admission), ProductConfig is recomputed
	// every reconcile and may derive from live cluster state — e.g. a ZooKeeper connection
	// string built from the actual resources, a quorum peer list from pod ordinals, or a JVM
	// heap sized from the role group's resources. Computing here, rather than freezing values
	// into the spec at admission, means operator upgrades propagate config changes to existing
	// clusters. It is a pure function of the CR and the role/role group identity; returning nil
	// contributes nothing for that role group.
	// +optional
	ProductConfig func(cr CR, roleName, roleGroupName string) *v1alpha1.OverridesSpec

	// Dependencies, when set, returns the external objects the CR references (ConfigMaps and
	// Secrets that the product does not create itself, e.g. a Kerberos keytab Secret or an
	// authentication ConfigMap). They are verified to exist before any role is reconciled; a
	// missing one aborts the cycle with a Degraded condition instead of producing pods that
	// crash-loop on a missing mount.
	//
	// Opt-in: nil (the default) performs no checks, and an empty Dependency.Namespace defaults
	// to the CR's namespace. Products that need richer validation call the DependencyResolver
	// helpers (ValidateS3Connection, ValidateDatabaseConnection, ...) from their own code.
	// +optional
	Dependencies func(cr CR) []Dependency

	// ExtensionRegistry is the extension registry this reconciler executes hooks from.
	// Defaults to the global singleton (common.GetExtensionRegistry()) for backward
	// compatibility. Pass a dedicated instance to isolate a product's extensions: the global
	// registry is shared by every GenericReconciler in the process, so an extension written for
	// one CR type also runs — and can fail — for another product's clusters.
	// +optional
	ExtensionRegistry *common.ExtensionRegistry

	// Prototype is a zero-value instance of the CR type used for controller setup.
	// This is required because Go generics don't allow creating new instances.
	Prototype CR
}

// GenericReconciler provides a reusable reconciliation template for product operators.
// It implements the Template Method Pattern where the reconciliation flow is fixed,
// but product-specific behavior is delegated to the RoleGroupHandler.
//
// Reconciliation Flow:
//  1. Fetch CR
//  2. Panic recovery wrapper
//  3. Execute reconcile():
//     a. ClusterOperation gate (evaluated first, before any mutation)
//     - Handle ReconciliationPaused -> return early (full freeze)
//     - Stopped is NOT short-circuited here: it falls through to the normal reconcile
//     so all resources are created/preserved, with StatefulSet replicas forced to 0
//     downstream (see BaseRoleGroupHandler.buildStatefulSet)
//     b. PreReconcile Extensions (Hook)
//     c. Validate Dependencies
//     d. For Each Role:
//     - Role PreReconcile Extensions
//     - For Each RoleGroup:
//     - RoleGroup PreReconcile Extensions
//     - Build RoleGroupBuildContext
//     - Delegate to RoleGroupHandler.BuildResources()
//     - Apply Resources (CM -> HeadlessSvc -> Service -> Extras -> STS -> PDB -> MetricsSvc)
//     - Track in Status
//     - RoleGroup PostReconcile Extensions
//     - Role PostReconcile Extensions
//     e. Cleanup Orphaned Resources
//     f. Update Health Status
//     g. PostReconcile Extensions
//     h. Final Status Update
//  4. Error handling: OnReconcileError hooks, set Degraded condition
type GenericReconciler[CR common.ClusterInterface] struct {
	client              client.Client
	scheme              *runtime.Scheme
	k8sUtil             *util.K8sUtil
	healthManager       *HealthManager
	dependencyResolver  *DependencyResolver
	cleaner             *RoleGroupCleaner
	eventManager        *EventManager
	configMerger        *config.ConfigMerger
	apiReader           client.Reader
	roleGroupHandler    RoleGroupHandler[CR]
	extensionRegistry   *common.ExtensionRegistry
	prototype           CR
	rateLimitRetryAfter time.Duration
	// healthCheckInterval is the cadence at which a successful reconcile requeues itself; <= 0
	// disables periodic requeue (see reconcile's wakeup aggregation).
	healthCheckInterval time.Duration
	serviceAccountName  string
	// serviceAccountNameFunc, when set, resolves a per-CR SA name that takes precedence over
	// the static serviceAccountName (see resolveServiceAccountName).
	serviceAccountNameFunc func(cr CR) string
	productConfig          func(cr CR, roleName, roleGroupName string) *v1alpha1.OverridesSpec
	dependencies           func(cr CR) []Dependency
}

// NewGenericReconciler creates a new GenericReconciler.
func NewGenericReconciler[CR common.ClusterInterface](cfg *GenericReconcilerConfig[CR]) (*GenericReconciler[CR], error) {
	if cfg.Client == nil {
		return nil, fmt.Errorf("client is required")
	}
	if cfg.Scheme == nil {
		return nil, fmt.Errorf("scheme is required")
	}
	if cfg.Recorder == nil {
		return nil, fmt.Errorf("recorder is required")
	}
	if cfg.RoleGroupHandler == nil {
		return nil, fmt.Errorf("roleGroupHandler is required")
	}

	healthCheckInterval := cfg.HealthCheckInterval
	if healthCheckInterval == 0 {
		healthCheckInterval = DefaultHealthCheckInterval
	}

	healthCheckTimeout := cfg.HealthCheckTimeout
	if healthCheckTimeout == 0 {
		healthCheckTimeout = DefaultHealthCheckTimeout
	}

	healthManager := NewHealthManager(cfg.Client)
	healthManager.CheckInterval = healthCheckInterval
	healthManager.Timeout = healthCheckTimeout
	if cfg.ServiceHealthCheck != nil {
		healthManager.WithServiceHealthCheck(cfg.ServiceHealthCheck)
	}

	eventManager := NewEventManager(cfg.Recorder)

	cleaner := NewRoleGroupCleaner(cfg.Client, cfg.Scheme)
	cleaner.WithEventManager(eventManager)
	if cfg.GrayDeleteGracePeriod > 0 {
		cleaner.WithGrayDeleteGracePeriod(cfg.GrayDeleteGracePeriod)
	}

	rateLimitRetryAfter := cfg.RateLimitRetryAfter
	if rateLimitRetryAfter == 0 {
		rateLimitRetryAfter = 10 * time.Second
	}

	extensionRegistry := cfg.ExtensionRegistry
	if extensionRegistry == nil {
		extensionRegistry = common.GetExtensionRegistry()
	}

	return &GenericReconciler[CR]{
		client:                 cfg.Client,
		apiReader:              cfg.APIReader,
		scheme:                 cfg.Scheme,
		k8sUtil:                util.NewK8sUtil(cfg.Client, cfg.Scheme),
		healthManager:          healthManager,
		dependencyResolver:     NewDependencyResolver(cfg.Client),
		cleaner:                cleaner,
		eventManager:           eventManager,
		configMerger:           config.NewConfigMerger(),
		roleGroupHandler:       cfg.RoleGroupHandler,
		extensionRegistry:      extensionRegistry,
		prototype:              cfg.Prototype,
		rateLimitRetryAfter:    rateLimitRetryAfter,
		healthCheckInterval:    healthCheckInterval,
		serviceAccountName:     cfg.ServiceAccountName,
		serviceAccountNameFunc: cfg.ServiceAccountNameFunc,
		productConfig:          cfg.ProductConfig,
		dependencies:           cfg.Dependencies,
	}, nil
}

// Reconcile implements controller-runtime's Reconciler interface.
// It is the entry point for reconciliation requests.
//
// The results are named so the deferred panic recovery can turn a recovered panic into a
// returned error: swallowing it would report the cycle as successful and the workqueue would
// neither requeue nor back off. The status is deliberately left untouched on this path — an
// internal error says nothing about the cluster's actual state (docs/architecture.md §4.8.2).
func (r *GenericReconciler[CR]) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	logger := log.FromContext(ctx)

	// panicSubject is the object the Warning event is attached to. It is only known once the CR
	// has been fetched; before that a panic is reported through the returned error alone.
	var panicSubject client.Object

	// Panic recovery
	defer func() {
		if recovered := recover(); recovered != nil {
			result = ctrl.Result{}
			err = fmt.Errorf("panic in reconciliation: %v", recovered)
			logger.Error(err, "Panic recovered in reconciliation", "stack", string(debug.Stack()))
			if panicSubject != nil {
				r.eventManager.EmitWarningEvent(panicSubject, "ReconcilePanic", err.Error())
			}
		}
	}()

	// Fetch the CR
	cr, err := r.fetchCR(ctx, req.NamespacedName)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Cluster resource not found, assuming deleted")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	panicSubject = r.getAsClientObject(cr)

	// Snapshot the object exactly as stored, before any step mutates its status. Every status
	// write this cycle is compared against it, so a cycle that computes nothing new costs a read
	// instead of a write (and, since the controller watches its own CR, does not schedule itself
	// again).
	stored := panicSubject.DeepCopyObject().(client.Object)

	// Perform reconciliation
	result, err = r.reconcile(ctx, cr, stored)
	if err != nil {
		// Handle 429 rate limit: back off without setting Degraded or emitting an error event
		var rateLimitErr *RateLimitError
		if stderrors.As(err, &rateLimitErr) {
			logger.Info("Rate limited by Kubernetes API, backing off", "retryAfter", rateLimitErr.RetryAfter)
			return ctrl.Result{RequeueAfter: rateLimitErr.RetryAfter}, nil
		}

		// Execute error hooks
		r.executeErrorHooks(ctx, cr, err)

		// Set degraded condition. ReconcileComplete has to be falsified here too: it is otherwise
		// only ever set to True, so a cluster that succeeded once would keep advertising a
		// completed reconcile next to Degraded=True for every subsequent failure.
		status := cr.GetStatus()
		status.SetDegraded(true, v1alpha1.ReasonReconcileError, err.Error())
		status.SetReconcileComplete(false, v1alpha1.ReasonReconcileError, err.Error())

		// Update status
		if updateErr := r.updateStatus(ctx, cr, stored); updateErr != nil {
			logger.Error(updateErr, "Failed to update status after reconciliation error")
		}

		// Emit error event
		r.eventManager.EmitErrorEvent(cr.GetName(), r.getAsClientObject(cr), err)

		return ctrl.Result{}, err
	}

	return result, nil
}

// fetchCR fetches the cluster resource by name.
func (r *GenericReconciler[CR]) fetchCR(ctx context.Context, key types.NamespacedName) (CR, error) {
	var zero CR
	// Create a new instance of the CR type by deep copying the prototype.
	// Using r.prototype instead of a zero value prevents a nil pointer panic
	// when CR is a pointer type (e.g. *TrinoCluster), where var zero CR yields nil.
	cr := r.prototype.DeepCopyCluster().(CR)

	// Get the client.Object for the fetch
	obj := r.getAsClientObject(cr)
	if err := r.client.Get(ctx, key, obj); err != nil {
		return zero, err
	}

	return cr, nil
}

// getAsClientObject converts CR to client.Object using the runtime.Object interface.
func (r *GenericReconciler[CR]) getAsClientObject(cr CR) client.Object {
	return cr.GetRuntimeObject().(client.Object)
}

// reconcile performs the main reconciliation logic.
func (r *GenericReconciler[CR]) reconcile(ctx context.Context, cr CR, stored client.Object) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	spec := cr.GetSpec()
	status := cr.GetStatus()

	// Record the generation this cycle is computed from before any condition is written, so the
	// conditions and the top-level observedGeneration always agree. The field means "observed",
	// not "successfully reconciled" — that distinction is carried by ReconcileComplete, which the
	// error path sets to False.
	status.SetObservedGeneration(r.getAsClientObject(cr).GetGeneration())

	// ClusterOperation gate — evaluated FIRST, before any resource mutation (ServiceAccount
	// provisioning, PreReconcile extensions, role reconciliation). reconciliationPaused must fully
	// freeze reconciliation per the documented contract (docs/architecture.md), so it returns here
	// before any mutating step.
	//
	// Note: `stopped` is deliberately NOT gated here. Stopping a cluster means "keep every resource
	// (ConfigMap, Service, StatefulSet, PDB, ServiceAccount, PVCs) created and up to date, but run
	// zero pods", so it must fall through to the full normal reconcile. The StatefulSet replica
	// count is forced to 0 downstream (see BaseRoleGroupHandler.buildStatefulSet), which is what
	// scales the workload down while all resources are still reconciled/preserved and any spec or
	// config change applied while stopped is honored. The stopped status is reported by the health
	// step at the end of the normal reconcile (see health.go).
	if op := spec.ClusterOperation; op != nil {
		if op.ReconciliationPaused {
			logger.Info("Reconciliation is paused")
			status.SetDegraded(true, v1alpha1.ReasonReconciliationPaused, "Reconciliation is paused")
			// Surfacing the pause is the only thing this cycle does, so a failed write must be
			// retried rather than dropped — otherwise the CR keeps advertising the last running
			// cycle's status and nothing reschedules it.
			if err := r.updateStatus(ctx, cr, stored); err != nil {
				return ctrl.Result{}, NewReconcileError("StatusUpdate", "failed to record the paused state", err)
			}
			return ctrl.Result{}, nil
		}
	}

	// 0. Auto-create ServiceAccount if configured. The name is resolved per CR (per-CR func
	// wins over the static name; empty means SA management is disabled) so that two clusters
	// of the same product in one namespace each own their own SA.
	if saName := r.resolveServiceAccountName(cr); saName != "" {
		if err := r.ensureServiceAccount(ctx, cr, saName); err != nil {
			return ctrl.Result{}, NewReconcileError("ServiceAccount", "failed to ensure service account", err)
		}
	}

	// 1. Execute PreReconcile extensions
	if err := r.extensionRegistry.ExecuteClusterPreReconcile(ctx, r.client, cr); err != nil {
		return ctrl.Result{}, NewReconcileError("PreReconcile", "extension hook failed", err)
	}

	// 2. Validate dependencies declared by the product (no-op when the hook is unset)
	if err := r.validateDependencies(ctx, cr); err != nil {
		return ctrl.Result{}, NewReconcileError("DependencyValidation", "dependency validation failed", err)
	}

	// 3. Process each role
	r.warnOnUnknownConfiguredRoles(ctx, cr, spec)
	// Sorted, not map order: the first failing role aborts the cycle and its error text becomes
	// the Degraded message. With map order, a cluster with several broken role groups reports a
	// different one every cycle, so the status never settles and each write schedules the next
	// reconcile.
	for _, roleName := range slices.Sorted(maps.Keys(spec.Roles)) {
		roleSpec := spec.Roles[roleName]
		if err := r.reconcileRole(ctx, cr, roleName, &roleSpec); err != nil {
			return ctrl.Result{}, err
		}
	}

	// 4. Cleanup orphaned resources. The returned duration is the earliest pending gray-delete
	// deadline; it feeds the wakeup aggregation below so a deferred deletion runs on time.
	owner := r.getAsClientObject(cr)
	cleanupRequeue, err := r.cleaner.Cleanup(ctx, cr.GetNamespace(), cr.GetName(), spec, status, owner.GetUID(), owner.GetAnnotations())
	if err != nil {
		logger.Error(err, "Failed to cleanup orphaned resources")
		// Don't fail reconciliation for cleanup errors
	}

	// 5. Update health status
	if err := r.healthManager.Check(ctx, cr.GetNamespace(), cr.GetName(), spec, status); err != nil {
		logger.Error(err, "Failed to update health status")
		// Don't fail reconciliation for health check errors
	}

	// 6. Execute PostReconcile extensions
	if err := r.extensionRegistry.ExecuteClusterPostReconcile(ctx, r.client, cr); err != nil {
		return ctrl.Result{}, NewReconcileError("PostReconcile", "extension hook failed", err)
	}

	// 7. Final status update
	status.SetReconcileComplete(true, v1alpha1.ReasonReconcileComplete, "Reconciliation completed successfully")

	if err := r.updateStatus(ctx, cr, stored); err != nil {
		return ctrl.Result{}, err
	}

	// 8. Schedule the next wakeup. Watches only cover the resources the framework owns, so
	// anything that changes without producing an event — a ServiceHealthCheck probe, a
	// gray-delete grace period running out — needs a timed requeue. Both sources collapse into
	// a single RequeueAfter (the earliest one); zero means "no periodic wakeup".
	requeueAfter := earliestRequeue(r.healthCheckInterval, cleanupRequeue)

	logger.Info("Reconciliation completed successfully")
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// earliestRequeue returns the smallest strictly positive duration, or 0 when there is none.
// Non-positive candidates mean "this source has nothing pending" and are ignored.
func earliestRequeue(candidates ...time.Duration) time.Duration {
	next := time.Duration(0)
	for _, d := range candidates {
		if d <= 0 {
			continue
		}
		if next == 0 || d < next {
			next = d
		}
	}
	return next
}

// validateDependencies verifies that every external object the product declares for this CR
// exists. Returns a *DependencyError (which the caller turns into a Degraded condition and a
// requeue) for the first missing one. No-op when the Dependencies hook is unset.
func (r *GenericReconciler[CR]) validateDependencies(ctx context.Context, cr CR) error {
	if r.dependencies == nil {
		return nil
	}

	for _, dep := range r.dependencies(cr) {
		namespace := dep.Namespace
		if namespace == "" {
			namespace = cr.GetNamespace()
		}
		if dep.Name == "" {
			return &DependencyError{
				Type:    "InvalidDependency",
				Message: fmt.Sprintf("%s dependency declared with an empty name", dep.Kind),
			}
		}

		switch dep.Kind {
		case DependencyConfigMap:
			if err := r.dependencyResolver.ValidateConfigMap(ctx, namespace, dep.Name); err != nil {
				return err
			}
		case DependencySecret:
			if err := r.dependencyResolver.ValidateSecret(ctx, namespace, dep.Name); err != nil {
				return err
			}
		default:
			return &DependencyError{
				Type:    "UnsupportedDependencyKind",
				Message: fmt.Sprintf("dependency %s/%s has unsupported kind %q", namespace, dep.Name, dep.Kind),
			}
		}
	}

	return nil
}

// reconcileRole reconciles a single role.
func (r *GenericReconciler[CR]) reconcileRole(ctx context.Context, cr CR, roleName string, roleSpec *v1alpha1.RoleSpec) error {
	logger := log.FromContext(ctx)

	// Execute role PreReconcile extensions
	if err := r.extensionRegistry.ExecuteRolePreReconcile(ctx, r.client, cr, roleName); err != nil {
		return NewReconcileError("RolePreReconcile", fmt.Sprintf("role %s extension hook failed", roleName), err)
	}

	// Process each role group
	// Note: groupSpec is deep copied because it may be modified during reconciliation.
	// roleSpec is passed directly as read-only (used for configuration lookup only);
	// it originates from spec.Roles which is re-fetched from the API server each reconcile,
	// so any accidental modifications would not persist and would be corrected on next reconcile.
	for groupName, groupSpec := range roleSpec.GetRoleGroups() {
		groupSpecCopy := *groupSpec.DeepCopy()
		if err := r.reconcileRoleGroup(ctx, cr, roleName, roleSpec, groupName, &groupSpecCopy); err != nil {
			return err
		}
	}

	// Reconcile the role-level PodDisruptionBudget (one per role, covering all its role groups).
	if err := r.reconcileRolePodDisruptionBudget(ctx, cr, roleName, roleSpec); err != nil {
		return err
	}

	// Execute role PostReconcile extensions
	if err := r.extensionRegistry.ExecuteRolePostReconcile(ctx, r.client, cr, roleName); err != nil {
		return NewReconcileError("RolePostReconcile", fmt.Sprintf("role %s extension hook failed", roleName), err)
	}

	logger.V(1).Info("Role reconciled", "role", roleName)
	return nil
}

// rolePodDisruptionBudgetBuilder is satisfied by BaseRoleGroupHandler (and any handler that
// embeds it, via the promoted method). Asserting on this method-set interface — rather than the
// concrete *BaseRoleGroupHandler[CR] — is what lets product handlers that embed the base handler
// (e.g. *ZkRoleGroupHandler) still get a role-level PDB. The method signature does not depend on
// CR, so the interface is non-generic.
type rolePodDisruptionBudgetBuilder interface {
	BuildRolePodDisruptionBudget(clusterName, namespace, roleName string, clusterLabels map[string]string, roleSpec *v1alpha1.RoleSpec) *policyv1.PodDisruptionBudget
}

// reconcileRolePodDisruptionBudget builds and applies the role's single PodDisruptionBudget.
// The PDB is a role-level resource (roleConfig.podDisruptionBudget covers all pods of a role
// across every role group), so it is reconciled here rather than per role group. When the PDB
// is unset or disabled, any previously-created role PDB is deleted so toggling it off takes
// effect. Handlers that neither are nor embed BaseRoleGroupHandler manage their own PDBs.
func (r *GenericReconciler[CR]) reconcileRolePodDisruptionBudget(ctx context.Context, cr CR, roleName string, roleSpec *v1alpha1.RoleSpec) error {
	handler, ok := r.roleGroupHandler.(rolePodDisruptionBudgetBuilder)
	if !ok {
		return nil
	}

	owner := r.getAsClientObject(cr)
	name := RoleResourceName(cr.GetName(), roleName)

	pdb := handler.BuildRolePodDisruptionBudget(cr.GetName(), cr.GetNamespace(), roleName, cr.GetLabels(), roleSpec)
	if pdb == nil {
		// PDB unset or disabled: remove any role PDB we previously created (ownership-checked).
		if err := r.cleaner.deletePDB(ctx, cr.GetNamespace(), name, owner.GetUID(), cr.GetName()); err != nil {
			return NewResourceApplyError("PodDisruptionBudget", cr.GetNamespace(), name, "failed to delete disabled PDB", err)
		}
		return nil
	}

	if err := r.applyResource(ctx, owner, pdb); err != nil {
		return NewResourceApplyError("PodDisruptionBudget", cr.GetNamespace(), name, "failed to apply", err)
	}
	return nil
}

// reconcileRoleGroup reconciles a single role group.
func (r *GenericReconciler[CR]) reconcileRoleGroup(ctx context.Context, cr CR, roleName string, roleSpec *v1alpha1.RoleSpec, groupName string, groupSpec *v1alpha1.RoleGroupSpec) error {
	logger := log.FromContext(ctx)

	// Execute role group PreReconcile extensions
	if err := r.extensionRegistry.ExecuteRoleGroupPreReconcile(ctx, r.client, cr, roleName, groupName); err != nil {
		return NewReconcileError("RoleGroupPreReconcile", fmt.Sprintf("role %s group %s extension hook failed", roleName, groupName), err)
	}

	// Build context
	buildCtx := r.buildRoleGroupContext(cr, roleName, roleSpec, groupName, groupSpec)

	// A podOverrides layer that fails to decode is dropped by the merger so the rest of the
	// configuration still applies, but dropping a user's override silently is worse than a
	// partially applied one: surface it on the CR where the author will see it.
	for _, overrideErr := range buildCtx.MergedConfig.PodOverrideErrors {
		logger.Error(overrideErr, "Ignoring undecodable podOverrides layer", "role", roleName, "roleGroup", groupName)
		r.eventManager.EmitWarningEvent(r.getAsClientObject(cr), "PodOverrideIgnored",
			fmt.Sprintf("role %s group %s: %v", roleName, groupName, overrideErr))
	}

	// Resolve the Vector aggregator address (if the CR exposes it) so the framework can own
	// vector.yaml generation. Must run before building resources / the ConfigMap.
	if err := r.resolveVectorAggregatorAddress(ctx, cr, buildCtx); err != nil {
		return NewResourceBuildError("resources", roleName, groupName, "failed to resolve vector aggregator address", err)
	}

	// Auto-create SidecarManager based on CRD configuration. The product image is propagated to
	// the registered sidecars by BaseRoleGroupHandler.BuildResources — after the product's
	// BuildResources override has resolved the CR-driven image — so both plain and embedding
	// handlers are covered without a concrete-type assertion here.
	buildCtx.SidecarManager = r.buildSidecarManager(ctx, buildCtx)

	// Delegate to handler for resource building
	resources, err := r.roleGroupHandler.BuildResources(ctx, r.client, cr, buildCtx)
	if err != nil {
		return NewResourceBuildError("resources", roleName, groupName, "failed to build resources", err)
	}

	// Apply resources in dependency order
	if err := r.applyResources(ctx, cr, resources, buildCtx); err != nil {
		return err
	}

	// Track role group in status
	cr.GetStatus().SetRoleGroup(roleName, groupName)

	// Execute role group PostReconcile extensions
	if err := r.extensionRegistry.ExecuteRoleGroupPostReconcile(ctx, r.client, cr, roleName, groupName); err != nil {
		return NewReconcileError("RoleGroupPostReconcile", fmt.Sprintf("role %s group %s extension hook failed", roleName, groupName), err)
	}

	logger.V(1).Info("Role group reconciled", "role", roleName, "group", groupName)
	return nil
}

// maxRoleGroupNameLen bounds the role group resource name so that, even with the longest
// suffix the framework appends ("-headless" = 9 chars), the result stays within the 63-char
// DNS label limit that applies to Service names and StatefulSet .spec.serviceName.
const maxRoleGroupNameLen = 54

// RoleGroupResourceName returns the canonical resource name for a role group:
// "<cluster>-<role>-<group>". Including the role prevents collisions between role groups of
// different roles that share a group name (e.g. namenode/default vs datanode/default).
//
// If the name would exceed maxRoleGroupNameLen, it is deterministically truncated and a short
// hash suffix is appended to preserve uniqueness while staying within the 63-char DNS limit.
func RoleGroupResourceName(clusterName, roleName, groupName string) string {
	name := fmt.Sprintf("%s-%s-%s", clusterName, roleName, groupName)
	if len(name) <= maxRoleGroupNameLen {
		return name
	}
	sum := sha256.Sum256([]byte(name))
	suffix := hex.EncodeToString(sum[:])[:8]
	head := strings.TrimRight(name[:maxRoleGroupNameLen-len(suffix)-1], "-")
	return head + "-" + suffix
}

// RoleResourceName returns the canonical resource name for a role-level resource:
// "<cluster>-<role>". Used for resources that span all of a role's role groups, e.g. the
// role's PodDisruptionBudget. It is always shorter than the corresponding role group name,
// so no truncation is needed to stay within DNS limits.
func RoleResourceName(clusterName, roleName string) string {
	return fmt.Sprintf("%s-%s", clusterName, roleName)
}

// buildRoleGroupContext creates the build context for a role group.
func (r *GenericReconciler[CR]) buildRoleGroupContext(cr CR, roleName string, roleSpec *v1alpha1.RoleSpec, groupName string, groupSpec *v1alpha1.RoleGroupSpec) *RoleGroupBuildContext {
	// Merge configurations in increasing precedence: product config (lowest) < role < role
	// group (highest). The product's computed config flows through the same merge pipeline as
	// CRD overrides, so a value set anywhere in the CRD always wins over it.
	var productConfig *v1alpha1.OverridesSpec
	if r.productConfig != nil {
		productConfig = r.productConfig(cr, roleName, groupName)
	}
	mergedConfig := r.configMerger.Merge(productConfig, roleSpec.GetOverrides(), groupSpec.GetOverrides())
	// Deep-merge logging (role + role group) once, so both Vector enablement and per-container
	// logging config file generation read from a single merged source.
	mergedConfig.Logging = productlogging.MergeLoggingSpec(roleSpec.GetConfig().Logging, groupSpec.GetConfig().Logging)

	// Merge the role-level config into the role group config (group wins per field), so
	// role-wide defaults for resources/affinity/gracefulShutdownTimeout reach every group —
	// previously only logging and overrides were merged and role-level config was silently
	// dropped. The merge works on copies; the CR's spec objects are never mutated.
	mergedGroupSpec := groupSpec.DeepCopy()
	mergedGroupSpec.Config = MergeRoleGroupConfig(roleSpec.GetConfig(), groupSpec.GetConfig())

	resourceName := RoleGroupResourceName(cr.GetName(), roleName, groupName)

	return &RoleGroupBuildContext{
		ClusterName:      cr.GetName(),
		ClusterNamespace: cr.GetNamespace(),
		ClusterLabels:    cr.GetLabels(),
		ClusterSpec:      cr.GetSpec(),
		RoleName:         roleName,
		RoleSpec:         roleSpec,
		RoleGroupName:    groupName,
		RoleGroupSpec:    *mergedGroupSpec,
		MergedConfig:     mergedConfig,
		ResourceName:     resourceName,
		// Propagate the reconciler-managed ServiceAccount so the workload pods actually run as
		// the SA the reconciler creates. Resolved per CR (per-CR func over static name), and
		// empty when no SA is configured (backward compatible).
		ServiceAccountName: r.resolveServiceAccountName(cr),
	}
}

// buildSidecarManager creates a SidecarManager based on CRD configuration.
// It reads Logging configuration from Role and RoleGroup specs, merging them
// to determine which built-in sidecar providers should be registered.
//
// It ALWAYS returns a non-nil manager (possibly empty). This guarantees products always
// have a SidecarManager to register their own containers with (e.g. init containers via
// StaticContainerProvider), so pod container injection always flows through the manager
// rather than being mutated directly.
func (r *GenericReconciler[CR]) buildSidecarManager(ctx context.Context, buildCtx *RoleGroupBuildContext) *sidecar.SidecarManager {
	mgr := sidecar.NewSidecarManager()

	// Logging was deep-merged once in buildRoleGroupContext.
	logging := buildCtx.MergedConfig.Logging
	if !vector.IsAgentEnabled(logging) {
		return mgr
	}

	// The handler declares which containers produce logs and the shared log volume size. The
	// Vector provider is the single owner of the shared log pipeline: it creates the volume,
	// RW-mounts it on the producer containers, mounts it on itself (pre-creating the per-container
	// log dirs, as it starts first), and adds the sidecar.
	var producers []productlogging.ContainerLogging
	var logVolumeSize string
	if lp, ok := r.roleGroupHandler.(LoggingProducerProvider); ok {
		producers = lp.LoggingProducers(buildCtx.RoleName)
		logVolumeSize = lp.LogVolumeSizeLimit()
	}

	// Only register Vector when there is at least one producer to collect from. A role group that
	// enables the Vector agent but declares no producers has nothing to ship, so skip (and warn)
	// rather than add a sidecar mounting an empty pipeline. This keeps the enablement decision and
	// the producer declaration consistent in one place.
	if len(producers) == 0 {
		log.FromContext(ctx).Info(
			"vector agent is enabled but no logging producers are declared; skipping vector sidecar",
			"role", buildCtx.RoleName, "roleGroup", buildCtx.RoleGroupName)
		return mgr
	}

	// The role-group ConfigMap name (buildCtx.ResourceName) is passed at construction so the
	// Vector container mounts the right config. The image is propagated later via
	// SidecarManager.SetProductImage (Vector ships inside the product image), so an empty image
	// here is intentional.
	opts := []vector.ProviderOption{
		vector.WithConfigMapName(buildCtx.ResourceName),
		vector.WithProducers(producerContainerNames(producers)),
	}
	if logVolumeSize != "" {
		if q, err := resource.ParseQuantity(logVolumeSize); err != nil {
			log.FromContext(ctx).Error(err, "invalid LogVolumeSize; using vector default",
				"logVolumeSize", logVolumeSize, "role", buildCtx.RoleName, "roleGroup", buildCtx.RoleGroupName)
		} else {
			opts = append(opts, vector.WithLogVolumeSize(q))
		}
	}
	mgr.Register(vector.NewVectorSidecarProvider("", opts...), &sidecar.SidecarConfig{Enabled: true})

	return mgr
}

// producerContainerNames extracts the container names from a log-producer declaration list.
func producerContainerNames(producers []productlogging.ContainerLogging) []string {
	names := make([]string, 0, len(producers))
	for _, p := range producers {
		names = append(names, p.Container)
	}
	return names
}

// loggingProducers returns the handler's declared log-producer containers when it implements
// LoggingProducerProvider (nil otherwise). Both Vector sidecar registration and aggregator-address
// resolution gate on "≥1 producer" so they stay consistent: the framework only wires Vector — and
// only resolves/generates its config — when there is actually something to collect.
func (r *GenericReconciler[CR]) loggingProducers(roleName string) []productlogging.ContainerLogging {
	if lp, ok := r.roleGroupHandler.(LoggingProducerProvider); ok {
		return lp.LoggingProducers(roleName)
	}
	return nil
}

// resolveVectorAggregatorAddress resolves the Vector aggregator discovery address for a role group
// and stores it on buildCtx, enabling framework-owned vector.yaml generation
// (RenderLoggingConfigMapData). It is a no-op unless Vector will actually be injected — the agent
// is enabled AND at least one producer is declared (mirroring buildSidecarManager's gating) — and
// the CR implements VectorAggregatorProvider. In that active case the aggregator ConfigMap name
// must be non-empty and resolvable; an unset name or a discovery failure is returned as an error,
// failing loudly rather than shipping a Vector sidecar with no aggregator.
func (r *GenericReconciler[CR]) resolveVectorAggregatorAddress(ctx context.Context, cr CR, buildCtx *RoleGroupBuildContext) error {
	if !vectorEnabledFor(buildCtx) || len(r.loggingProducers(buildCtx.RoleName)) == 0 {
		return nil
	}
	provider, ok := any(cr).(VectorAggregatorProvider)
	if !ok {
		return nil
	}
	name := provider.VectorAggregatorConfigMapName()
	if name == "" {
		return fmt.Errorf("vector agent is enabled but vectorAggregatorConfigMapName is not configured (role %q, group %q)",
			buildCtx.RoleName, buildCtx.RoleGroupName)
	}
	address, err := vector.DiscoverAggregatorAddress(ctx, r.client, buildCtx.ClusterNamespace, name)
	if err != nil {
		return fmt.Errorf("failed to discover vector aggregator address from ConfigMap %q: %w", name, err)
	}
	buildCtx.VectorAggregatorAddress = address
	return nil
}

// applyResources applies all resources in the correct dependency order.
// Order: ConfigMap -> Headless Service -> Service -> ExtraResources -> StatefulSet -> PDB -> MetricsService
// ExtraResources are applied before the StatefulSet because they are typically prerequisites
// for pod scheduling (e.g. a Listener CR referenced by an ephemeral CSI volume).
// Each resource is created when absent and updated to the handler-built desired state when it
// already exists (see applyResource / copyDesiredState for the exact update semantics).
func (r *GenericReconciler[CR]) applyResources(ctx context.Context, cr CR, resources *RoleGroupResources, buildCtx *RoleGroupBuildContext) error {
	owner := r.getAsClientObject(cr)

	// 1. Apply ConfigMap
	if resources.ConfigMap != nil {
		if err := r.applyResource(ctx, owner, resources.ConfigMap); err != nil {
			return NewResourceApplyError("ConfigMap", buildCtx.ClusterNamespace, buildCtx.ResourceName, "failed to apply", err)
		}
	}

	// 2. Apply Headless Service
	if resources.HeadlessService != nil {
		if err := r.applyResource(ctx, owner, resources.HeadlessService); err != nil {
			return NewResourceApplyError("Service", buildCtx.ClusterNamespace, buildCtx.ResourceName+"-headless", "failed to apply headless service", err)
		}
	}

	// 3. Apply Service
	if resources.Service != nil {
		if err := r.applyResource(ctx, owner, resources.Service); err != nil {
			return NewResourceApplyError("Service", buildCtx.ClusterNamespace, buildCtx.ResourceName, "failed to apply", err)
		}
	}

	// 4. Apply extra product resources BEFORE the StatefulSet: extras are typically
	// prerequisites for pod scheduling (e.g. a Listener CR that pods reference through an
	// ephemeral CSI volume — without it the pods hang in ContainerCreating). They go through
	// the same applyResource path as the fixed fields, so they get the same controller owner
	// reference and are GC'd with the CR. Nil entries are skipped (see RoleGroupResources).
	for _, extra := range resources.ExtraResources {
		if extra == nil {
			continue
		}
		if err := r.applyResource(ctx, owner, extra); err != nil {
			return NewResourceApplyError(r.resourceKind(extra), extra.GetNamespace(), extra.GetName(), "failed to apply extra resource", err)
		}
	}

	// 4b. Validate sidecar dependencies before the workload is applied, so a missing external
	// object fails the reconcile instead of producing pods that crash-loop on a broken mount.
	// It runs here, not before step 1: the built-in providers (Vector, JMX exporter) point at
	// the role group ConfigMap, and products may ship a provider's dependency as an extra
	// resource — both are in place by now, on the very first reconcile too.
	if err := r.validateSidecars(ctx, buildCtx); err != nil {
		return err
	}

	// 5. Apply StatefulSet
	if resources.StatefulSet != nil {
		if err := r.applyResource(ctx, owner, resources.StatefulSet); err != nil {
			return NewResourceApplyError("StatefulSet", buildCtx.ClusterNamespace, buildCtx.ResourceName, "failed to apply", err)
		}
	}

	// 6. Apply the custom per-group PodDisruptionBudget (escape hatch), or reclaim the legacy
	// per-role-group PDB. The framework's own PDB is now role-level (reconcileRolePodDisruptionBudget);
	// a product may still ship a custom per-group PDB via RoleGroupResources.PodDisruptionBudget.
	// When it does not, delete any PDB named "<cluster>-<role>-<group>" left behind by older
	// framework versions (ownership-checked, no-op if absent) so upgraded clusters converge to
	// exactly one role-level PDB instead of retaining stale per-group constraints.
	if resources.PodDisruptionBudget != nil {
		if err := r.applyResource(ctx, owner, resources.PodDisruptionBudget); err != nil {
			return NewResourceApplyError("PodDisruptionBudget", buildCtx.ClusterNamespace, buildCtx.ResourceName, "failed to apply", err)
		}
	} else if err := r.cleaner.deletePDB(ctx, buildCtx.ClusterNamespace, buildCtx.ResourceName, owner.GetUID(), buildCtx.ClusterName); err != nil {
		return NewResourceApplyError("PodDisruptionBudget", buildCtx.ClusterNamespace, buildCtx.ResourceName, "failed to delete legacy per-group PDB", err)
	}

	// 7. Apply the metrics Service, or reclaim it when metrics are turned off — mirroring the
	// PDB branch above, so disabling metrics actually removes the stale Service instead of
	// leaving a scrape target pointing at pods nobody exports metrics from.
	metricsName := buildCtx.ResourceName + "-metrics"
	if resources.MetricsService != nil {
		// The reclaim below deletes by derived name, and "<resource>-metrics" is also a legal
		// resource name for a role group literally called "<group>-metrics". Stamping the slot
		// here is what lets the reclaim tell "the metrics Service this framework applied" apart
		// from a sibling role group's Service, or a product's own object of the same name.
		markMetricsService(resources.MetricsService)
		if err := r.applyResource(ctx, owner, resources.MetricsService); err != nil {
			return NewResourceApplyError("Service", buildCtx.ClusterNamespace, metricsName, "failed to apply metrics service", err)
		}
	} else if err := r.reclaimMetricsService(ctx, buildCtx.ClusterNamespace, metricsName, owner.GetUID(), buildCtx.ClusterName); err != nil {
		return NewResourceApplyError("Service", buildCtx.ClusterNamespace, metricsName, "failed to delete disabled metrics service", err)
	}

	return nil
}

// LabelMetricsService marks a Service as the framework's per-role-group metrics slot. Only
// Services carrying it are reclaimed when a role group stops producing a MetricsService.
const LabelMetricsService = "metrics." + constant.KubedoopDomain + "/service"

// markMetricsService stamps the metrics slot label on a handler-built metrics Service.
func markMetricsService(svc *corev1.Service) {
	if svc.Labels == nil {
		svc.Labels = map[string]string{}
	}
	svc.Labels[LabelMetricsService] = valueTrue
}

// reclaimMetricsService deletes the role group's metrics Service, but only when the live object
// is one this framework applied as the metrics slot. Deleting purely by derived name would also
// hit the client Service of a role group named "<group>-metrics", and any object a product ships
// under that name through RoleGroupResources.ExtraResources — both of which carry this CR's
// controller owner reference, so ownership alone does not distinguish them.
func (r *GenericReconciler[CR]) reclaimMetricsService(ctx context.Context, namespace, name string, ownerUID types.UID, clusterName string) error {
	svc := &corev1.Service{}
	if err := r.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, svc); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if svc.Labels[LabelMetricsService] != valueTrue {
		return nil
	}
	return r.cleaner.deleteService(ctx, namespace, name, ownerUID, clusterName)
}

// validateSidecars runs the registered sidecar providers' dependency checks for a role group.
// The manager only validates once a client and namespace are wired (both are no-ops otherwise),
// which is why the seam is closed here rather than at construction: the namespace is per CR.
func (r *GenericReconciler[CR]) validateSidecars(ctx context.Context, buildCtx *RoleGroupBuildContext) error {
	mgr := buildCtx.SidecarManager
	if mgr == nil || !mgr.HasSidecars() {
		return nil
	}

	mgr.WithClient(r.client, buildCtx.ClusterNamespace)
	if err := mgr.ValidateAll(ctx); err != nil {
		return NewValidationError("sidecar", buildCtx.RoleName, buildCtx.RoleGroupName, err)
	}
	return nil
}

// resourceKind resolves a human-readable kind for an arbitrary resource (used in error
// messages for ExtraResources, whose GVK is not fixed). Typed objects usually carry an empty
// TypeMeta, so the scheme registration is preferred; the Go type name is the fallback.
func (r *GenericReconciler[CR]) resourceKind(obj client.Object) string {
	if gvks, _, err := r.scheme.ObjectKinds(obj); err == nil && len(gvks) > 0 {
		return gvks[0].Kind
	}
	return fmt.Sprintf("%T", obj)
}

// applyResource applies a single resource using CreateOrUpdate: it creates the object when
// absent and otherwise UPDATES the live object to the handler-built desired state, so CR spec
// changes (replicas, config, ports, ...) propagate to existing resources on every reconcile
// (issue #526).
//
// controllerutil.CreateOrUpdate overwrites the passed object with live cluster state on Get
// before running the mutate func, so the desired state is deep-copied up front and copied
// back onto the live object inside the mutate func. The copy semantics — wholesale labels,
// merged annotations, per-kind spec/data rules that respect Kubernetes immutable fields
// (StatefulSet selector/serviceName/volumeClaimTemplates/podManagementPolicy, Service
// clusterIP/allocated NodePorts), and a generic top-level field copy for arbitrary GVKs —
// are documented on copyDesiredState in apply.go.
//
// After the operation, emits a Create or Update event based on the result.
// If the Kubernetes API returns 429 Too Many Requests, a RateLimitError is returned.
//
// Note: handler-built objects usually omit server-defaulted fields, so a steady-state
// reconcile may still issue an Update whose server-side result is identical to the stored
// object; the API server short-circuits such writes (no resourceVersion bump, no watch
// event), so this cannot cause a reconcile loop.
func (r *GenericReconciler[CR]) applyResource(ctx context.Context, owner client.Object, obj client.Object) error {
	// Capture the desired state before CreateOrUpdate clobbers obj with live state on Get.
	desired, ok := obj.DeepCopyObject().(client.Object)
	if !ok {
		return fmt.Errorf("failed to deep copy desired object %T: copy is not a client.Object", obj)
	}
	result, err := r.k8sUtil.CreateOrUpdate(ctx, obj, func() error {
		// Set ownership
		if err := controllerutil.SetControllerReference(owner, obj, r.scheme); err != nil {
			return err
		}
		// Copy the desired state onto the live object; without this the apply path is
		// create-only (see the doc comment above and copyDesiredState).
		return copyDesiredState(desired, obj)
	})
	if err != nil {
		if errors.IsTooManyRequests(err) {
			return NewRateLimitError(r.rateLimitRetryAfter, err)
		}
		return err
	}

	switch result {
	case controllerutil.OperationResultCreated:
		r.eventManager.EmitCreateEvent(owner.GetName(), obj)
	case controllerutil.OperationResultUpdated:
		r.eventManager.EmitUpdateEvent(owner.GetName(), obj)
	}
	return nil
}

// executeErrorHooks executes error hooks when reconciliation fails.
func (r *GenericReconciler[CR]) executeErrorHooks(ctx context.Context, cr CR, reconcileErr error) {
	logger := log.FromContext(ctx)
	if err := r.extensionRegistry.ExecuteClusterOnError(ctx, r.client, cr, reconcileErr); err != nil {
		logger.Error(err, "Failed to execute error hooks")
	}
}

// updateStatus writes the in-memory status to the API server, retrying on conflict and skipping
// the write entirely when this cycle computed nothing new.
//
// stored is the object as it was read at the start of the cycle. Comparing against the whole
// object — rather than only the embedded GenericClusterStatus — is what lets a product's own
// status fields participate in the decision: a CR's status typically embeds the generic struct
// alongside product-specific fields that an extension hook filled in during this cycle, and
// ClusterInterface exposes only the generic part.
//
// The write is issued from the in-memory object without re-fetching first, for the same reason:
// a re-fetch replaces the whole status, discarding exactly those product-specific fields. On
// conflict only the resourceVersion is refreshed and the write is retried, which is safe because
// the controller is the sole writer of its CR's status.
func (r *GenericReconciler[CR]) updateStatus(ctx context.Context, cr CR, stored client.Object) error {
	obj := r.getAsClientObject(cr)

	// The controller watches its own CR, so an unconditional write would deliver a watch event
	// and schedule another reconcile that writes again.
	if stored != nil && apiequality.Semantic.DeepEqual(stored, obj) {
		return nil
	}

	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		err := r.client.Status().Update(ctx, obj)
		if !errors.IsConflict(err) {
			return err
		}
		// Read the fresh resourceVersion through the uncached reader: a conflict means the
		// informer has not observed the competing write yet, so the cache would hand back the
		// same stale version and every retry would conflict again.
		fresh := obj.DeepCopyObject().(client.Object)
		if getErr := r.statusReader().Get(ctx, client.ObjectKeyFromObject(obj), fresh); getErr != nil {
			return getErr
		}
		obj.SetResourceVersion(fresh.GetResourceVersion())
		return err
	})
	// The CR was deleted while this cycle was running. There is nothing left to report on, and
	// treating it as a failure would run the product's error hooks and emit a Warning event
	// against an object that no longer exists.
	if errors.IsNotFound(err) {
		return nil
	}
	return err
}

// statusReader returns the reader used to refresh a conflicting status write. It is the
// configured uncached APIReader when available, falling back to the regular client.
func (r *GenericReconciler[CR]) statusReader() client.Reader {
	if r.apiReader != nil {
		return r.apiReader
	}
	return r.client
}

// warnOnUnknownConfiguredRoles reports handler role configuration that no role in the CR claims.
// Role names are bare strings that must match a key of spec.roles; a typo makes every per-role
// lookup return nil, so the role group silently comes up with no ports, no image override and no
// Service. It is only a warning, not a failure: a handler may legitimately be configured for
// optional roles a given CR does not declare (an HA-only role, say).
func (r *GenericReconciler[CR]) warnOnUnknownConfiguredRoles(ctx context.Context, cr CR, spec *v1alpha1.GenericClusterSpec) {
	provider, ok := r.roleGroupHandler.(RoleNameProvider)
	if !ok {
		return
	}
	var unknown []string
	for _, name := range provider.ConfiguredRoleNames() {
		if _, declared := spec.Roles[name]; !declared {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) == 0 {
		return
	}
	message := fmt.Sprintf("handler configured for role(s) %s, which the cluster does not declare; their settings are ignored",
		strings.Join(unknown, ", "))
	log.FromContext(ctx).Info("Unknown configured role names", "roles", unknown)
	r.eventManager.EmitWarningEvent(r.getAsClientObject(cr), "UnknownConfiguredRole", message)
}

// SetupWithManagerOptions extends the controller's watch set with resources the framework does
// not know about. RoleGroupResources.ExtraResources have arbitrary GVKs, and a GVK that is not
// watched produces no reconcile event: out-of-band edits to such a resource are only repaired by
// the informer resync or an unrelated CR event. Products that emit extras register them here.
type SetupWithManagerOptions struct {
	// ExtraOwns adds an Owns() watch for each object's GVK — the right choice for resources the
	// reconciler creates with a controller owner reference (which is how ExtraResources are
	// applied).
	ExtraOwns []client.Object

	// Watches registers arbitrary additional watches on the controller builder, for sources
	// that are not owned by the CR (e.g. a shared ConfigMap mapped back to several clusters).
	Watches []func(*builder.Builder) *builder.Builder
}

// SetupWithManager sets up the controller with the Manager.
// The prototype must be set in the config during construction.
func (r *GenericReconciler[CR]) SetupWithManager(mgr ctrl.Manager) error {
	return r.SetupWithManagerOpts(mgr, SetupWithManagerOptions{})
}

// SetupWithManagerOpts sets up the controller with the Manager, adding the product's own
// watches to the framework's fixed set. It is the extension point for ExtraResources GVKs;
// SetupWithManager is the zero-options form.
func (r *GenericReconciler[CR]) SetupWithManagerOpts(mgr ctrl.Manager, opts SetupWithManagerOptions) error {
	return r.ControllerBuilder(mgr, opts).Complete(r)
}

// ControllerBuilder returns the controller builder configured with the framework's watch set
// (the CR plus the resource kinds the reconciler owns) and the caller's extra watches, without
// completing it. Products needing full control over the controller — predicates, options,
// a custom Reconciler wrapper — build on top of this and call Complete themselves.
func (r *GenericReconciler[CR]) ControllerBuilder(mgr ctrl.Manager, opts SetupWithManagerOptions) *builder.Builder {
	// Use the prototype for controller setup
	prototype := r.getAsClientObject(r.prototype)

	b := ctrl.NewControllerManagedBy(mgr).
		For(prototype).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Service{}).
		Owns(&policyv1.PodDisruptionBudget{}).
		Owns(&corev1.ServiceAccount{})

	for _, obj := range opts.ExtraOwns {
		if obj == nil {
			continue
		}
		b = b.Owns(obj)
	}
	for _, with := range opts.Watches {
		if with == nil {
			continue
		}
		b = with(b)
	}
	return b
}

// resolveServiceAccountName resolves the ServiceAccount name for a CR.
// Precedence: ServiceAccountNameFunc (when set and returning non-empty) > static
// ServiceAccountName > "" (SA management skipped). Per-CR naming is what keeps two clusters
// of the same product in one namespace from fighting over a single shared SA.
func (r *GenericReconciler[CR]) resolveServiceAccountName(cr CR) string {
	if r.serviceAccountNameFunc != nil {
		if name := r.serviceAccountNameFunc(cr); name != "" {
			return name
		}
	}
	return r.serviceAccountName
}

// ensureServiceAccount creates or updates the ServiceAccount for the cluster workload and sets
// the CR as its controller owner.
//
// Guard rail: if an SA with this name already exists but is controlled by a DIFFERENT owner,
// SetControllerReference refuses to steal it; that raw AlreadyOwnedError is wrapped in an
// explicit error naming both owners, because it almost always means two CRs were configured to
// share one static ServiceAccountName — the fix is per-CR naming via ServiceAccountNameFunc.
func (r *GenericReconciler[CR]) ensureServiceAccount(ctx context.Context, cr CR, name string) error {
	owner := r.getAsClientObject(cr)
	sa := &corev1.ServiceAccount{}
	sa.Name = name
	sa.Namespace = cr.GetNamespace()

	_, err := r.k8sUtil.CreateOrUpdate(ctx, sa, func() error {
		sa.Labels = cr.GetLabels()
		if err := controllerutil.SetControllerReference(owner, sa, r.scheme); err != nil {
			var alreadyOwned *controllerutil.AlreadyOwnedError
			if stderrors.As(err, &alreadyOwned) {
				return fmt.Errorf(
					"ServiceAccount %s/%s is already controlled by %s %q and cannot be adopted by %s %q: "+
						"multiple clusters appear to share one ServiceAccount name; "+
						"use per-CR naming (GenericReconcilerConfig.ServiceAccountNameFunc, e.g. \"<product>-<cluster>\"): %w",
					sa.Namespace, sa.Name,
					alreadyOwned.Owner.Kind, alreadyOwned.Owner.Name,
					r.resourceKind(owner), owner.GetName(),
					err,
				)
			}
			return err
		}
		return nil
	})
	return err
}
