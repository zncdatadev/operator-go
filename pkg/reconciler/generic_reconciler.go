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
	rbacv1 "k8s.io/api/rbac/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
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
type GenericReconcilerConfig[CR common.ClusterResource[CR]] struct {
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

	// Recorder is the event recorder for emitting events. Required — the framework emits on every
	// apply and on several conditions the status does not carry.
	//
	// It obliges the OPERATOR's own ClusterRole to hold
	// `+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch` (`patch` because a repeat
	// event is aggregated onto the existing object). This is the one framework permission whose
	// absence does not announce itself: client-go treats a 403 on an event as permanent, logs
	// "Server rejected event (will not retry!)" and DISCARDS it, and emission is fire-and-forget, so
	// no error reaches Reconcile, the status is untouched, and the pass reports success. What is
	// lost includes ImmutableFieldIgnored — the only framework Warning with no paired log line and
	// no status condition. See docs/security.md §3.3 for the full operator-side set.
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

	// DrainPollInterval is how long a reconcile waits before resuming an orphan deletion it has
	// already started (scale to zero -> drain -> delete -> confirm). It paces the state machine,
	// not the pod termination itself. Defaults to DefaultDrainPollInterval (5s); products with a
	// long terminationGracePeriodSeconds can raise it to poll less often.
	// +optional
	DrainPollInterval time.Duration

	// DrainTimeout bounds how long an orphaned StatefulSet's ordered drain may block the rest of
	// its role group's teardown. Once it elapses the StatefulSet is deleted regardless of the pods
	// still reported, so a pod that cannot terminate (a stuck finalizer, an unreachable node)
	// cannot strand the group's ConfigMap, Services and status entry indefinitely. Defaults to
	// DefaultDrainTimeout (10m).
	// +optional
	DrainTimeout time.Duration

	// WorkloadRBACRules, when set, declares the Kubernetes API permissions this cluster's WORKLOAD
	// pods need — the product's own processes calling the API, not the operator's permissions,
	// which come from its ClusterRole and are a separate axis entirely.
	//
	// It is the other half of the workload's ServiceAccount. That gives the workload an IDENTITY;
	// this gives that identity its PERMISSIONS, and the framework maintains both at the same point
	// in the same way: once per CR, at cluster level, before any role is built. A role group merely
	// consumes the result — the ServiceAccount name reaches the pod template through
	// RoleGroupBuildContext.ServiceAccountName, and the Role is already bound to it.
	//
	//	WorkloadRBACRules: func(cr *v1alpha1.NifiCluster) []rbacv1.PolicyRule {
	//	    return []rbacv1.PolicyRule{{
	//	        APIGroups: []string{"coordination.k8s.io"},
	//	        Resources: []string{"leases"},
	//	        Verbs:     []string{"get", "list", "watch", "create", "update"},
	//	    }}
	//	},
	//
	// The Role and RoleBinding are namespaced, named after the derived ServiceAccount and
	// controller-owned by the CR, so they are garbage-collected with it. There is deliberately no
	// ClusterRole path: a namespaced CR cannot controller-own a cluster-scoped object, so the
	// framework would have no lifecycle for one.
	//
	// There are two distinct "nothing" cases, and they mean OPPOSITE things:
	//
	//	WorkloadRBACRules == nil            this product does not use workload RBAC. No Role or
	//	                                    RoleBinding is read, written or deleted, and the
	//	                                    operator needs no RBAC permissions at all.
	//
	//	WorkloadRBACRules returns nil       this cluster is granted nothing. The Role and
	//	  (or an empty slice)               RoleBinding are DELETED if this CR controller-owns
	//	                                    them, so the pods stop holding the permissions.
	//
	// The second is the same reading every other optional slot in this framework gets — a nil
	// MetricsService reclaims the Service — and it is the only one under which a rule set that
	// shrinks to nothing actually stops granting. The first exists so that operators which never
	// opted in pay nothing: running a revoke every reconcile would cost two API reads per cluster
	// and demand RBAC read permission from every operator built on this SDK.
	//
	// A hook that returns rules conditionally therefore revokes on the passes where its condition
	// is false, which is usually what a product wants; leaving the hook unset is how a product says
	// "not my concern" rather than "not right now".
	//
	// The framework passes the ServiceAccount name it derived itself, so the identity and the
	// permissions cannot drift apart. That is why this is a config field and not only the exported
	// EnsureWorkloadRBAC helper: the helper takes the name as a parameter, so a caller can pass one
	// that is not the workload's, producing a Role that grants to a ServiceAccount no pod uses —
	// two objects that both exist, pods that start fine, and a 403 on the first API call.
	//
	// Setting this REQUIRES the operator to hold these permissions itself — Kubernetes forbids
	// granting what the granter lacks — plus write access to the RBAC API. Both are kubebuilder
	// markers on the operator; see EnsureWorkloadRBAC for the exact failure messages.
	// +optional
	WorkloadRBACRules func(cr CR) []rbacv1.PolicyRule

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
	// clusters. Returning a nil spec contributes nothing for that role group.
	//
	// The ctx and client are what make "may derive from live cluster state" true rather than
	// aspirational. Without them the hook could only be a pure function of the CR, so a product
	// needing an API lookup — resolving an S3Connection reference to an endpoint — could not use
	// it at all, and had no way to report a failed lookup either: a Get error could only be
	// swallowed, rendering a silently wrong config, or panicked. Zero operators used the hook,
	// which was the symptom; the products that needed a product-config layer were exactly the
	// ones it could not serve.
	//
	// A returned error fails the role group. For a product that already performs its lookup
	// inside BuildResources and does not want to repeat it here, the imperative counterpart is
	// RoleGroupBuildContext.ApplyProductDefaults.
	// +optional
	ProductConfig func(
		ctx context.Context, c client.Client, cr CR, roleName, roleGroupName string,
	) (*v1alpha1.OverridesSpec, error)

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

	// ExtensionRegistry is the extension registry this reconciler executes hooks from. It is
	// typed for this reconciler's CR, so its extensions receive the product CR directly and no
	// registry can be shared with a reconciler of another product. Build it with
	// common.NewExtensionRegistry[CR]() and register the product's extensions on it.
	//
	// Optional: when unset the reconciler runs with an empty registry and every hook is a no-op.
	// +optional
	ExtensionRegistry *common.ExtensionRegistry[CR]

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
type GenericReconciler[CR common.ClusterResource[CR]] struct {
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
	extensionRegistry   *common.ExtensionRegistry[CR]
	prototype           CR
	rateLimitRetryAfter time.Duration
	// healthCheckInterval is the cadence at which a successful reconcile requeues itself; <= 0
	// disables periodic requeue (see reconcile's wakeup aggregation).
	healthCheckInterval time.Duration
	// workloadRBACRules, when set, declares the workload's API permissions (see the config field).
	workloadRBACRules func(cr CR) []rbacv1.PolicyRule
	productConfig     func(ctx context.Context, c client.Client, cr CR, roleName, roleGroupName string) (*v1alpha1.OverridesSpec, error)
	dependencies      func(cr CR) []Dependency
}

// NewGenericReconciler creates a new GenericReconciler.
func NewGenericReconciler[CR common.ClusterResource[CR]](cfg *GenericReconcilerConfig[CR]) (*GenericReconciler[CR], error) {
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

	eventManager := NewEventManager(cfg.Recorder, cfg.Scheme)

	rateLimitRetryAfter := cfg.RateLimitRetryAfter
	if rateLimitRetryAfter == 0 {
		rateLimitRetryAfter = 10 * time.Second
	}

	cleaner := NewRoleGroupCleaner(cfg.Client, cfg.Scheme)
	cleaner.WithEventManager(eventManager)
	// The cleanup path issues API writes like the apply path does, so it backs off on a 429 with
	// the same delay instead of inventing its own.
	cleaner.WithRateLimitRetryAfter(rateLimitRetryAfter)
	// Pruning a role group from the status snapshot is irreversible — nothing enumerates the
	// group afterwards — so that verdict is confirmed against the API server rather than a cache
	// that may answer NotFound for an object that exists.
	cleaner.WithAPIReader(cfg.APIReader)
	cleaner.WithDrainPollInterval(cfg.DrainPollInterval)
	cleaner.WithDrainTimeout(cfg.DrainTimeout)
	if cfg.GrayDeleteGracePeriod > 0 {
		cleaner.WithGrayDeleteGracePeriod(cfg.GrayDeleteGracePeriod)
	}

	// An empty registry rather than nil keeps the hook call sites unconditional; a product that
	// registers no extensions pays an empty loop per hook.
	extensionRegistry := cfg.ExtensionRegistry
	if extensionRegistry == nil {
		extensionRegistry = common.NewExtensionRegistry[CR]()
	}

	return &GenericReconciler[CR]{
		client:              cfg.Client,
		apiReader:           cfg.APIReader,
		scheme:              cfg.Scheme,
		k8sUtil:             util.NewK8sUtil(cfg.Client, cfg.Scheme),
		healthManager:       healthManager,
		dependencyResolver:  NewDependencyResolver(cfg.Client),
		cleaner:             cleaner,
		eventManager:        eventManager,
		configMerger:        config.NewConfigMerger(),
		roleGroupHandler:    cfg.RoleGroupHandler,
		extensionRegistry:   extensionRegistry,
		prototype:           cfg.Prototype,
		rateLimitRetryAfter: rateLimitRetryAfter,
		healthCheckInterval: healthCheckInterval,
		workloadRBACRules:   cfg.WorkloadRBACRules,
		productConfig:       cfg.ProductConfig,
		dependencies:        cfg.Dependencies,
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
			// Drop this cluster's series. A gauge left behind keeps publishing its last value for a
			// cluster that no longer exists, so an alert on "cleanup pending" would fire forever for
			// something nobody can fix.
			forgetClusterMetrics(req.Namespace, req.Name)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	panicSubject = cr

	// The CR is being deleted. Stop here — before the ClusterOperation gate and before any
	// mutating step.
	//
	// A deleted CR is not necessarily an absent one. Background propagation (the `kubectl delete`
	// default) removes it from etcd immediately and the next event is the IsNotFound above, but
	// FOREGROUND propagation (`kubectl delete --cascade=foreground`, and any product that registers
	// its own finalizer) leaves the object readable with a deletionTimestamp set until its
	// dependents are gone. Without this guard `fetchCR` succeeds and a full normal reconcile runs:
	// every resource is re-created with `SetControllerReference`, which sets
	// `BlockOwnerDeletion: true`, so foreground deletion can never retire the CR while each GC
	// delete fires an Owns() watch event that schedules the recreate again. The CR stays
	// Terminating forever, pods churn, and because the cycle returns no error the workqueue rate
	// limiter is Forgotten every pass — the loop hammers the API server with no backoff.
	//
	// Returning nil (not an error) is deliberate: deletion is not a failure, and the object is
	// about to disappear, so there is nothing to requeue for. The SDK registers no finalizer of its
	// own; a product that needs teardown work registers its own finalizer and observes the same
	// deletionTimestamp from its own controller.
	if !cr.GetDeletionTimestamp().IsZero() {
		logger.Info("Cluster resource is being deleted, skipping reconciliation",
			"deletionTimestamp", cr.GetDeletionTimestamp())
		return ctrl.Result{}, nil
	}

	// Snapshot the object exactly as stored, before any step mutates its status. Every status
	// write this cycle is compared against it, so a cycle that computes nothing new costs a read
	// instead of a write (and, since the controller watches its own CR, does not schedule itself
	// again).
	stored := cr.DeepCopy()

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
		r.eventManager.EmitErrorEvent(cr.GetName(), cr, err)

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
	cr := r.prototype.DeepCopy()

	if err := r.client.Get(ctx, key, cr); err != nil {
		return zero, err
	}

	return cr, nil
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
	status.SetObservedGeneration(cr.GetGeneration())

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
			// Observe before returning. The pause freezes the RESOURCES, not the reporting: reads
			// mutate nothing, and skipping this left every other condition at whatever the last
			// running cycle wrote — a cluster paused mid-rollout advertised Progressing=True for as
			// long as the pause lasted, and a cluster paused after a failure kept claiming
			// Available. The health pass also owns the Paused condition and clears Degraded, so a
			// maintenance window stops looking like a fault (see HealthManager.Check).
			if err := r.healthManager.Check(ctx, cr.GetNamespace(), cr.GetName(), spec, status); err != nil {
				logger.Error(err, "Failed to update health status while paused")
			}
			// Surfacing the pause is the only thing this cycle does, so a failed write must be
			// retried rather than dropped — otherwise the CR keeps advertising the last running
			// cycle's status and nothing reschedules it.
			if err := r.updateStatus(ctx, cr, stored); err != nil {
				return ctrl.Result{}, NewReconcileError("StatusUpdate", "failed to record the paused state", err)
			}
			// Re-observe on the health cadence: a paused cluster's pods keep changing state, and
			// nothing else will wake this CR up while its resources are frozen.
			return ctrl.Result{RequeueAfter: r.healthCheckInterval}, nil
		}
	}

	// 0. The workload's identity. EVERY cluster gets one, with no way to turn it off, which is
	// what makes docs/security.md's claim true for every product rather than only the ones that
	// opted in: "audit logs reflect the specific application identity rather than a generic
	// default account". A ServiceAccount is pure metadata, so the cost of always having one does
	// not buy a switch whose main effect was clusters running as `default` by accident.
	//
	// The name is derived from the CR (kind + name), so there is nothing to configure here and
	// nothing for two clusters in one namespace to collide over.
	saName := r.resolveServiceAccountName(cr)
	if err := r.ensureServiceAccount(ctx, cr, saName); err != nil {
		return ctrl.Result{}, NewReconcileError("ServiceAccount", "failed to ensure service account", err)
	}

	// 0b. The workload's PERMISSIONS, immediately after its IDENTITY and from the same derived
	// name, so the two cannot drift apart. Both are per-CR, both are settled before any role is
	// built, and the role group only consumes the result (the SA name reaches the pod template
	// through RoleGroupBuildContext.ServiceAccountName, already bound to the Role).
	if err := r.ensureWorkloadRBAC(ctx, cr, saName); err != nil {
		return ctrl.Result{}, NewReconcileError("WorkloadRBAC", "failed to ensure workload RBAC", err)
	}

	// 1. Execute PreReconcile extensions.
	//
	// A wait does NOT return here. The pass continues so cleanup and health still run, which is what
	// keeps Degraded and Available current; the wait itself is reported at step 8.
	var waitFor *waitState
	if err := r.extensionRegistry.ExecuteClusterPreReconcile(ctx, r.client, cr); err != nil {
		waitErr, waiting := common.WaitingErrors(err)
		if !waiting {
			return ctrl.Result{}, NewReconcileError("PreReconcile", "extension hook failed", err)
		}
		waitFor = mergeWait(waitFor, newWaitState(waitErr))
	}

	// 2. Validate dependencies declared by the product (no-op when the hook is unset)
	if err := r.validateDependencies(ctx, cr); err != nil {
		waitErr, waiting := common.WaitingErrors(err)
		if !waiting {
			return ctrl.Result{}, NewReconcileError("DependencyValidation", "dependency validation failed", err)
		}
		waitFor = mergeWait(waitFor, newWaitState(waitErr))
	}

	// 3. Process each role, BEST EFFORT: a role that fails does not stop the others, and does not
	// stop the cleanup/health/PostReconcile steps below.
	//
	// Roles and role groups are independent workloads. Aborting the pass at the first failure meant
	// one unparsable value on the alphabetically-first role indefinitely blocked the DELETION of an
	// unrelated role group, the health of every other role, and the discovery ConfigMap a product
	// publishes from PostReconcile — none of which had anything to do with the broken role.
	// Sorting made that starvation deterministic rather than fixing it: the same later roles were
	// starved every cycle.
	//
	// The cleaner already reasoned its way to this policy for the same situation (see
	// cleaner.go, "aborting the whole pass here would let one wedged group keep every other orphan
	// alive"). The framework held both policies and applied the wrong one to the hot path.
	//
	// The iteration stays sorted so the aggregated error — and therefore the Degraded message —
	// is byte-stable across cycles; an unstable message would defeat the no-op guard in
	// updateStatus and make the controller reschedule itself forever.
	r.warnOnUnknownConfiguredRoles(ctx, cr, spec)
	var roleErrs []error
	// A CLUSTER-level wait blocks the roles. That is the whole point of #608's case — a schema
	// migration Job that must finish before any workload starts — and applying the StatefulSets
	// anyway would start the product against a database that is not initialised. The pass still
	// continues below: cleanup, health and PostReconcile run, so the wait does not freeze the
	// cluster's own observations the way an early return would.
	//
	// A ROLE-level wait does not do this. It is raised while iterating, so the roles before it have
	// already been reconciled, and skipping the rest would make the outcome depend on map ordering.
	for _, roleName := range slices.Sorted(maps.Keys(spec.Roles)) {
		if waitFor != nil {
			break
		}
		roleSpec := spec.Roles[roleName]
		if err := r.reconcileRole(ctx, cr, roleName, &roleSpec); err != nil {
			// Throttling is the one failure that must stop the pass: the API server is rejecting
			// this operator's requests, so pushing the remaining roles through would only deepen
			// the backlog.
			if IsRateLimitError(err) {
				return ctrl.Result{}, err
			}
			roleErrs = append(roleErrs, err)
		}
	}

	// 4. Cleanup orphaned resources. The returned duration is the earliest wakeup the cleanup needs
	// (a pending gray-delete deadline, or the next poll of a deletion in flight); it feeds the
	// wakeup aggregation below so the deletion state machine advances on time.
	cleanupRequeue, err := r.cleaner.Cleanup(ctx, cr.GetNamespace(), cr.GetName(), spec, status, cr.GetUID(), cr.GetAnnotations())
	if err != nil {
		// Throttling is not a cleanup problem: the API server is rejecting this operator's
		// requests, so the whole cycle has to back off rather than push the remaining writes
		// through. Every other cleanup failure stays non-fatal — the cluster's own resources are
		// reconciled, only the leftovers of a removed role group are not.
		if IsRateLimitError(err) {
			return ctrl.Result{}, err
		}
		logger.Error(err, "Failed to cleanup orphaned resources")
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

	// 7. Report role failures, if any. Returning here — after cleanup, health and PostReconcile
	// have all run — is what makes the iteration best-effort rather than merely tolerant: the
	// caller sets Degraded and writes the status once, and the workqueue backs off. The status
	// write is left to that path so a failing cycle does not write twice.
	if len(roleErrs) > 0 {
		joined := stderrors.Join(roleErrs...)
		// A genuine failure anywhere outranks a wait: Degraded has to be set and the workqueue has
		// to back off, and the joined message carries the waiting role's own text alongside the
		// failure so nothing is hidden. Only an aggregate that is waits ALL THE WAY DOWN is a wait.
		waitErr, waiting := common.WaitingErrors(joined)
		if !waiting {
			return ctrl.Result{}, joined
		}
		waitFor = mergeWait(waitFor, newWaitState(waitErr))
	}

	// 8. Final status update.
	//
	// A wait is reported HERE, after cleanup, health and PostReconcile have all run, rather than by
	// returning early where it was raised. That position is the whole design: SetDegraded(false) has
	// exactly one writer, HealthManager.Check, so an early return leaves Degraded LATCHED at whatever
	// the last completed pass wrote — a cluster whose pods recovered while an extension waits would
	// keep paging with a message naming a pod that no longer exists, and observedGeneration would
	// match, so no staleness heuristic catches it. The framework already diagnosed exactly this on
	// the reconciliationPaused gate and fixed it the same way: observe, then return.
	if waitFor != nil {
		status.SetCondition(metav1.Condition{
			Type:    string(ConditionWaiting),
			Status:  metav1.ConditionTrue,
			Reason:  waitFor.Reason,
			Message: waitFor.Message,
		})
		// ReconcileComplete is falsified because it is the documented generation gate: leaving it
		// True next to the bumped observedGeneration reports a user's edit as applied while the
		// framework is still waiting to apply it.
		status.SetReconcileComplete(false, ReasonWaiting, waitFor.Message)
	} else {
		// Cleared only when this framework raised it, so a product that never waits gets no noise —
		// the rule ConditionOrphanCleanupPending already follows.
		if status.GetCondition(ConditionWaiting) != nil {
			status.SetCondition(metav1.Condition{
				Type:    string(ConditionWaiting),
				Status:  metav1.ConditionFalse,
				Reason:  ReasonWaitSatisfied,
				Message: "Nothing is waiting",
			})
		}
		status.SetReconcileComplete(true, v1alpha1.ReasonReconcileComplete, "Reconciliation completed successfully")
	}

	if err := r.updateStatus(ctx, cr, stored); err != nil {
		return ctrl.Result{}, err
	}

	// 9. Schedule the next wakeup. Watches only cover the resources the framework owns, so
	// anything that changes without producing an event — a ServiceHealthCheck probe, a
	// gray-delete grace period running out — needs a timed requeue. Both sources collapse into
	// a single RequeueAfter (the earliest one); zero means "no periodic wakeup".
	requeueAfter := earliestRequeue(r.healthCheckInterval, cleanupRequeue)
	if waitFor != nil {
		requeueAfter = earliestRequeue(requeueAfter, waitFor.After)
		logger.Info("Reconciliation is waiting", "reason", waitFor.Reason, "requeueAfter", waitFor.After)
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

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

	// Process each role group, BEST EFFORT: role groups are independent StatefulSets, so one
	// failing to build must not stop its siblings — nor the role-level PDB that covers all of
	// them, nor the role's PostReconcile hook.
	//
	// Sorted, not map order, so the aggregated error text is byte-stable across cycles: an
	// unstable Degraded message would defeat the no-op guard in updateStatus and make the
	// controller reschedule itself forever.
	//
	// Note: groupSpec is deep copied because it may be modified during reconciliation.
	// roleSpec is passed directly as read-only (used for configuration lookup only);
	// it originates from spec.Roles which is re-fetched from the API server each reconcile,
	// so any accidental modifications would not persist and would be corrected on next reconcile.
	roleGroups := roleSpec.GetRoleGroups()
	var errs []error
	for _, groupName := range slices.Sorted(maps.Keys(roleGroups)) {
		groupSpec := roleGroups[groupName]
		groupSpecCopy := *groupSpec.DeepCopy()
		if err := r.reconcileRoleGroup(ctx, cr, roleName, roleSpec, groupName, &groupSpecCopy); err != nil {
			// A 429 stops everything: see the role loop in reconcile.
			if IsRateLimitError(err) {
				return err
			}
			errs = append(errs, err)
		}
	}

	// Reconcile the role-level PodDisruptionBudget (one per role, covering all its role groups).
	// Still attempted when a group failed: the PDB protects the groups that ARE running, and it is
	// derived from roleConfig, not from anything a failed group produced.
	if err := r.reconcileRolePodDisruptionBudget(ctx, cr, roleName, roleSpec); err != nil {
		if IsRateLimitError(err) {
			return err
		}
		errs = append(errs, err)
	}

	// Execute role PostReconcile extensions
	if err := r.extensionRegistry.ExecuteRolePostReconcile(ctx, r.client, cr, roleName); err != nil {
		errs = append(errs, NewReconcileError("RolePostReconcile", fmt.Sprintf("role %s extension hook failed", roleName), err))
	}

	if len(errs) > 0 {
		return stderrors.Join(errs...)
	}

	logger.V(1).Info("Role reconciled", "role", roleName)
	return nil
}

// reservedSlotLabelKeys are the framework's slot markers: labels whose presence (or value) makes
// the framework SELECT an object for deletion. They are excluded from the CR labels that reach a
// handler, because nothing downstream overwrites them.
//
// Every other CR label is deliberately propagated — that is the documented channel for platform
// opt-ins the framework does not own, restarter.kubedoop.dev/enable above all — and the
// app.kubernetes.io/* set is safe to propagate because recommendedLabels and
// frameworkSelectorLabels are applied over it. These three have no such backstop: BaseRoleGroupHandler
// stamps them only on the objects that really are the slot, so a CR label using one of these keys
// would travel to every built resource and make each of them answer to a reclaim aimed at the slot.
//
//   - LabelMetricsService on the client Service of a role group named "<x>-metrics" makes the
//     reclaim of role group "<x>" delete it, every reconcile.
//   - LabelRolePodDisruptionBudget carries the ROLE NAME in its value, and cleanupOrphanedRolePDBs
//     deletes every PDB whose value names a role the spec does not declare — so one CR label with a
//     bogus value reaps the per-group PDBs a product ships through the escape hatch.
//
// Filtering here rather than at each List keeps the rule in one place and makes it true for any
// future reclaim that keys off a marker.
var reservedSlotLabelKeys = []string{
	LabelMetricsService,
	LabelRolePodDisruptionBudget,
	LabelRoleGroupPodDisruptionBudget,
}

// handlerWritableLabels returns the CR's labels as a map a handler may both read and write, minus
// the framework's own slot markers (see reservedSlotLabelKeys).
//
// It is cloned because cr.GetLabels() hands out the live object's map, and a handler that adds an
// entry to ClusterLabels would otherwise mutate the object this cycle's status write is computed
// from. It is materialized because maps.Clone preserves nil and a CR carrying no labels is
// perfectly ordinary: RoleGroupBuildContext.ClusterLabels and RoleBuildContext.ClusterLabels are
// documented as the handler's to build on, and writing to a nil map panics.
func handlerWritableLabels(ctx context.Context, labels map[string]string) map[string]string {
	cloned := maps.Clone(labels)
	if cloned == nil {
		cloned = make(map[string]string)
	}
	for _, key := range reservedSlotLabelKeys {
		if _, present := cloned[key]; !present {
			continue
		}
		delete(cloned, key)
		log.FromContext(ctx).V(1).Info(
			"Dropped a reserved framework label from the CR's labels: it marks a resource slot for reclaim and is not propagated to built resources",
			"label", key)
	}
	return cloned
}

// rolePodDisruptionBudgetBuilder is satisfied by BaseRoleGroupHandler (and any handler that
// embeds it, via the promoted method). Asserting on this method-set interface — rather than the
// concrete *BaseRoleGroupHandler[CR] — is what lets product handlers that embed the base handler
// (e.g. *ZkRoleGroupHandler) still get a role-level PDB. The method signature does not depend on
// CR, so the interface is non-generic.
type rolePodDisruptionBudgetBuilder interface {
	BuildRolePodDisruptionBudget(buildCtx *RoleBuildContext) *policyv1.PodDisruptionBudget
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

	name := RoleResourceName(cr.GetName(), roleName)

	pdb := handler.BuildRolePodDisruptionBudget(&RoleBuildContext{
		ClusterName:      cr.GetName(),
		ClusterNamespace: cr.GetNamespace(),
		ClusterLabels:    handlerWritableLabels(ctx, cr.GetLabels()),
		ClusterSpec:      cr.GetSpec(),
		RoleName:         roleName,
		RoleSpec:         roleSpec,
	})
	if pdb == nil {
		// PDB unset or disabled: remove the role PDB we previously created. Gated on the slot
		// label, not on ownership: a product's own PDB may legitimately be named "<cluster>-<role>"
		// and carries the same controller owner reference, so ownership cannot tell them apart.
		if err := r.reclaimRolePDB(ctx, cr.GetNamespace(), name, roleName, cr.GetUID(), cr.GetName()); err != nil {
			return NewResourceApplyError("PodDisruptionBudget", cr.GetNamespace(), name, "failed to delete disabled PDB", err)
		}
		return nil
	}

	// Stamp the role slot before applying. This branch only runs for roles the spec declares, so
	// nothing here reclaims the PDB of a role that was deleted outright; the label is what lets
	// the cleaner find it afterwards without confusing it with a product's own PDB.
	markRolePodDisruptionBudget(pdb, roleName)

	if err := r.applyResource(ctx, cr, pdb); err != nil {
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
	buildCtx, err := r.buildRoleGroupContext(ctx, cr, roleName, roleSpec, groupName, groupSpec)
	if err != nil {
		return WrapConfigError(fmt.Sprintf("role %s group %s", roleName, groupName), err)
	}

	// A podOverrides layer that fails to decode is dropped by the merger so the rest of the
	// configuration still applies, but dropping a user's override silently is worse than a
	// partially applied one: surface it on the CR where the author will see it.
	for _, overrideErr := range buildCtx.MergedConfig.PodOverrideErrors {
		logger.Error(overrideErr, "Ignoring undecodable podOverrides layer", "role", roleName, "roleGroup", groupName)
		r.eventManager.EmitWarningEvent(cr, "PodOverrideIgnored",
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
	buildCtx.SidecarManager = r.buildSidecarManager(ctx, cr, buildCtx)

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
// "<cluster>-<role>". Used for resources that span all of a role's role groups — today only the
// role's PodDisruptionBudget.
//
// Unlike RoleGroupResourceName this is deliberately NOT truncated, because the limit that applies
// to it is a different one. A PodDisruptionBudget name is validated as a DNS *subdomain* (253
// bytes), not as the 63-byte DNS label that bounds a Service name and a StatefulSet's
// .spec.serviceName; "<cluster>-<role>" cannot realistically reach 253 when the cluster name is
// itself capped at 253. Truncating here would only invent a hash suffix nobody needs and change
// the name of every existing role PDB.
func RoleResourceName(clusterName, roleName string) string {
	return fmt.Sprintf("%s-%s", clusterName, roleName)
}

// maxServiceAccountNameLen bounds ServiceAccountResourceName. A ServiceAccount name is validated
// as a DNS subdomain, so 253 is the real limit; the CR name feeding it is itself capped at 253,
// which is why truncation here only ever guards the pathological case.
const maxServiceAccountNameLen = 253

// ServiceAccountResourceName returns the canonical name of the ServiceAccount a cluster's workload
// pods run as: "<lowercased kind>-<cluster>", e.g. "hdfscluster-prod".
//
// There is deliberately no config field for this name, for the same reason every other
// framework-owned name is derived (AGENTS.md §3, "the framework owns the NAME of every fixed
// slot"): the framework creates the object, controller-owns it, garbage-collects it with the CR
// and binds WorkloadRBACRules to it, so nothing needs to address it by a name the product chose.
// Letting the product choose is what produced the failure class this replaced — a static name is a
// constant, so every CR of a product in one namespace resolved to the SAME ServiceAccount:
// whichever cluster reconciled second failed with AlreadyOwnedError forever, and deleting the
// first garbage-collected the SA out from under the second's running pods.
//
// The Kind is part of it because the CR name alone is not unique within a namespace: an
// HdfsCluster and a TrinoCluster both named "prod" would otherwise derive the same ServiceAccount,
// and whichever controller reconciled second could never take ownership of it — reintroducing that
// same collision one level down.
//
// It is exported because it is a CONTRACT, not an implementation detail: anything outside the
// operator that has to name this ServiceAccount — a hand-written RoleBinding, an admission policy,
// an audit query — should derive it from the formula rather than read it out of the operator's
// source.
//
// Like RoleGroupResourceName it is bounded with a deterministic hash suffix, but against a much
// looser limit: a ServiceAccount name is a DNS subdomain (253), not the 63-byte DNS label that
// constrains a Service name.
func ServiceAccountResourceName(kind, clusterName string) string {
	name := strings.ToLower(kind) + "-" + clusterName
	if len(name) <= maxServiceAccountNameLen {
		return name
	}
	sum := sha256.Sum256([]byte(name))
	suffix := hex.EncodeToString(sum[:])[:8]
	head := strings.TrimRight(name[:maxServiceAccountNameLen-len(suffix)-1], "-")
	return head + "-" + suffix
}

// RoleGroupMarkerLabelKey returns the label key that fingerprints a resource as one this framework
// built for a role group: "<cluster>-<group>", or a bounded substitute when that is not a legal
// label key.
//
// It is NOT unique per role group, and nothing may select on it alone. The natural form omits the
// ROLE, so a product whose roles all use the conventional group name "default" — four roles in
// DolphinScheduler, three in HDFS — stamps the identical key "<cluster>-default" on all of them.
// The one consumer, isFrameworkRoleGroupPDB, is safe because it is reached only after a Get by the
// role group's derived name, which already pins the role; the key merely answers "did we build
// this?". Anything that starts from a List instead must key on the descriptive labels
// (app.kubernetes.io/component + the role-group label), as orphan discovery does.
//
// The role is missing for a reason that cannot be undone: the key lands in the StatefulSet's
// .spec.selector when LabelDomain is unset, and a selector is IMMUTABLE — adding the role would
// break every running cluster on upgrade. The substitute below IS role-scoped, so the two forms
// deliberately differ in what they identify. That asymmetry is safe for the same reason (see the
// paragraph on legality), and is another reason not to treat the key as an identifier.
//
// A label key's name part is limited to 63 bytes and to a restricted character set, and this key is
// derived from two free-form user strings. Entirely ordinary big-data names blow through it — a
// 43-character cluster plus a 21-character role group is 65 bytes — and because the key lands in
// the StatefulSet's .spec.selector, in both Services' selectors and in every pod template label
// set, the API server then rejects the whole role group, naming a label key the user never wrote.
// The resource NAME built from the same two strings was already bounded (RoleGroupResourceName);
// this second derivation was not, so the framework applied the limit it knew about to one of the
// two places it applies.
//
// The natural form is kept whenever it is legal, and that is load-bearing rather than cosmetic:
// .spec.selector is IMMUTABLE, so changing this key for a role group that already has a
// StatefulSet would leave the pod template no longer matching the frozen selector and every
// subsequent update would be rejected. Only combinations that could never have produced a
// StatefulSet in the first place get the substitute — for those there is nothing to stay
// compatible with, which is what makes this safe to roll out to running clusters.
//
// The substitute is RoleGroupResourceName: already bounded to maxRoleGroupNameLen, already unique
// per role group, and already the framework's canonical identifier for one. Reusing it keeps a
// single truncation algorithm in the codebase instead of a second one that has to agree with it.
func RoleGroupMarkerLabelKey(clusterName, roleName, roleGroupName string) string {
	key := clusterName + "-" + roleGroupName
	if len(validation.IsQualifiedName(key)) == 0 {
		return key
	}
	return RoleGroupResourceName(clusterName, roleName, roleGroupName)
}

// buildRoleGroupContext creates the build context for a role group.
func (r *GenericReconciler[CR]) buildRoleGroupContext(ctx context.Context, cr CR, roleName string, roleSpec *v1alpha1.RoleSpec, groupName string, groupSpec *v1alpha1.RoleGroupSpec) (*RoleGroupBuildContext, error) {
	// Merge configurations in increasing precedence: product config (lowest) < role < role
	// group (highest). The product's computed config flows through the same merge pipeline as
	// CRD overrides, so a value set anywhere in the CRD always wins over it.
	var productConfig *v1alpha1.OverridesSpec
	if r.productConfig != nil {
		var err error
		productConfig, err = r.productConfig(ctx, r.client, cr, roleName, groupName)
		if err != nil {
			return nil, fmt.Errorf("computing the product config for role %s group %s: %w",
				roleName, groupName, err)
		}
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
		ClusterLabels:    handlerWritableLabels(ctx, cr.GetLabels()),
		ClusterSpec:      cr.GetSpec(),
		RoleName:         roleName,
		RoleSpec:         roleSpec,
		RoleGroupName:    groupName,
		RoleGroupSpec:    *mergedGroupSpec,
		MergedConfig:     mergedConfig,
		ResourceName:     resourceName,
		// Propagate the reconciler-managed ServiceAccount so the workload pods actually run as
		// the SA the reconciler creates. Derived from the CR (kind + name), never configured and
		// never empty — this is the consumption half of the identity settled at step 0.
		ServiceAccountName: r.resolveServiceAccountName(cr),
	}, nil
}

// buildSidecarManager creates a SidecarManager based on CRD configuration.
// It reads Logging configuration from Role and RoleGroup specs, merging them
// to determine which built-in sidecar providers should be registered.
//
// It ALWAYS returns a non-nil manager (possibly empty). This guarantees products always
// have a SidecarManager to register their own containers with (e.g. init containers via
// StaticContainerProvider), so pod container injection always flows through the manager
// rather than being mutated directly.
//
// It also records the resolved Vector decision on buildCtx (VectorLogPipelineActive), so the
// logging renderers gate the rolling file appender on the sidecar that is really injected rather
// than on the enablement flag they cannot fully evaluate.
func (r *GenericReconciler[CR]) buildSidecarManager(ctx context.Context, cr CR, buildCtx *RoleGroupBuildContext) *sidecar.SidecarManager {
	mgr := sidecar.NewSidecarManager()
	// Every early return below leaves the shared log volume unbuilt, so the pipeline is inactive
	// unless the registration at the end of this function is reached.
	buildCtx.VectorLogPipelineActive = ptr.To(false)

	// Logging was deep-merged once in buildRoleGroupContext.
	logging := buildCtx.MergedConfig.Logging
	if !vector.IsAgentEnabled(logging) {
		return mgr
	}

	// The handler declares which containers produce logs and the shared log volume size. The
	// Vector provider is the single owner of the shared log pipeline: it creates the volume,
	// RW-mounts it on the producer containers, mounts it on itself (pre-creating the per-container
	// log dirs, as it starts first), and adds the sidecar.
	//
	// This asserts on r.roleGroupHandler — the OUTER handler — so a product overriding
	// LoggingProducers is honored here, while BaseRoleGroupHandler renders config files from its own
	// LoggingContainers field and cannot see that override (Go has no virtual dispatch). Do not
	// "simplify" this to read the rendered list: the two being separately addressable is the
	// documented way to join the Vector pipeline without a framework-rendered config file, which is
	// what a product owning its own logging config (Airflow's log_config.py) needs.
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

	// The sidecar runs "vector --config <mount>/vector.yaml", so it is only injected when
	// something actually writes that key into the role group ConfigMap: the framework does it for
	// a CR implementing VectorAggregatorProvider, otherwise the handler must claim the file
	// through VectorConfigProvider. With neither, registering the provider would fail sidecar
	// validation on every cycle and abort the whole cluster's reconcile over a product that is
	// simply not wired for Vector — so this is reported as the product-configuration mistake it
	// is, and the rest of the cluster keeps converging.
	if !r.vectorConfigIsProvided(cr, buildCtx.RoleName) {
		message := fmt.Sprintf(
			"role %s group %s enables the vector agent, but neither the cluster resource implements VectorAggregatorProvider "+
				"nor the role group handler implements VectorConfigProvider; no vector.yaml would be generated, so the sidecar is skipped",
			buildCtx.RoleName, buildCtx.RoleGroupName)
		log.FromContext(ctx).Info("Skipping vector sidecar: no source for vector.yaml",
			"role", buildCtx.RoleName, "roleGroup", buildCtx.RoleGroupName)
		r.eventManager.EmitWarningEvent(cr, "VectorSidecarSkipped", message)
		return mgr
	}

	// The role-group ConfigMap name (buildCtx.ResourceName) is passed at construction so the
	// Vector container mounts the right config. The image is propagated later via
	// SidecarManager.SetProductImage (Vector ships inside the product image), so an empty image
	// here is intentional.
	opts := []vector.ProviderOption{
		vector.WithConfigMapName(buildCtx.ResourceName),
		vector.WithProducers(producers),
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
	buildCtx.VectorLogPipelineActive = ptr.To(true)

	return mgr
}

// vectorConfigIsProvided reports whether anything will write vector.yaml into the role group
// ConfigMap: the framework generates it for a CR exposing an aggregator ConfigMap
// (VectorAggregatorProvider, see resolveVectorAggregatorAddress), and a product that builds the
// file itself says so through VectorConfigProvider on its handler. It gates Vector sidecar
// registration, so the two sides of the contract cannot drift apart.
func (r *GenericReconciler[CR]) vectorConfigIsProvided(cr CR, roleName string) bool {
	if _, ok := any(cr).(VectorAggregatorProvider); ok {
		return true
	}
	if provider, ok := r.roleGroupHandler.(VectorConfigProvider); ok {
		return provider.ProvidesVectorConfig(roleName)
	}
	return false
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

	// 0. Reject a declaration the lifecycle cannot honour, BEFORE anything is applied — a role
	// group that half-converged and then failed is worse than one that did not start.
	if err := validateRoleGroupResources(r.scheme, resources, buildCtx); err != nil {
		return err
	}

	// 1. Apply ConfigMap
	if resources.ConfigMap != nil {
		if err := r.applyResource(ctx, cr, resources.ConfigMap); err != nil {
			return NewResourceApplyError("ConfigMap", buildCtx.ClusterNamespace, buildCtx.ResourceName, "failed to apply", err)
		}
	}

	// 2. Apply Headless Service
	if resources.HeadlessService != nil {
		if err := r.applyResource(ctx, cr, resources.HeadlessService); err != nil {
			return NewResourceApplyError("Service", buildCtx.ClusterNamespace, buildCtx.ResourceName+"-headless", "failed to apply headless service", err)
		}
	}

	// 3. Apply Service
	if resources.Service != nil {
		if err := r.applyResource(ctx, cr, resources.Service); err != nil {
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
		if err := r.applyResource(ctx, cr, extra); err != nil {
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
		if err := r.applyResource(ctx, cr, resources.StatefulSet); err != nil {
			return NewResourceApplyError("StatefulSet", buildCtx.ClusterNamespace, buildCtx.ResourceName, "failed to apply", err)
		}
	}

	// 6. Apply the custom per-group PodDisruptionBudget (escape hatch), or reclaim the legacy
	// per-role-group PDB. The framework's own PDB is now role-level (reconcileRolePodDisruptionBudget);
	// a product may still ship a custom per-group PDB via RoleGroupResources.PodDisruptionBudget.
	// When it does not, delete the PDB named "<cluster>-<role>-<group>" left behind by older
	// framework versions (no-op if absent) so upgraded clusters converge to exactly one role-level
	// PDB instead of retaining stale per-group constraints.
	if resources.PodDisruptionBudget != nil {
		// The reclaim below deletes by derived name; stamping the slot marks this object as the
		// framework's per-group PDB, so turning the escape hatch off later actually removes it.
		markRoleGroupPodDisruptionBudget(resources.PodDisruptionBudget, buildCtx.RoleGroupName)
		if err := r.applyResource(ctx, cr, resources.PodDisruptionBudget); err != nil {
			return NewResourceApplyError("PodDisruptionBudget", buildCtx.ClusterNamespace, buildCtx.ResourceName, "failed to apply", err)
		}
	} else if err := r.reclaimRoleGroupPDB(ctx, buildCtx, cr.GetUID()); err != nil {
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
		if err := r.applyResource(ctx, cr, resources.MetricsService); err != nil {
			return NewResourceApplyError("Service", buildCtx.ClusterNamespace, metricsName, "failed to apply metrics service", err)
		}
	} else if err := r.reclaimMetricsService(ctx, buildCtx.ClusterNamespace, metricsName, cr.GetUID(), buildCtx.ClusterName); err != nil {
		return NewResourceApplyError("Service", buildCtx.ClusterNamespace, metricsName, "failed to delete disabled metrics service", err)
	}

	return nil
}

// apiError maps an API-server error the framework cannot act on into the framework's own type. A
// 429 becomes a *RateLimitError, which aborts the pass and backs off rather than marking the
// cluster Degraded: pushing the remaining roles through a throttled API server only deepens the
// backlog, and a throttled cluster is not a broken one. Everything else is returned unchanged.
//
// It is a method rather than an inline check because the mapping used to live only inside
// applyResource, so every reclaim path added afterwards reported a throttled Get as a resource
// failure — and the reclaims run on EVERY role group of EVERY cluster, being the branch taken when
// a handler ships no metrics Service or no per-group PDB. RoleGroupCleaner carries its own copy
// (RoleGroupCleaner.apiError) for the teardown side.
func (r *GenericReconciler[CR]) apiError(err error) error {
	if errors.IsTooManyRequests(err) {
		return NewRateLimitError(r.rateLimitRetryAfter, err)
	}
	return err
}

// validateRoleGroupResources rejects a RoleGroupResources the lifecycle cannot honour.
//
// The six fixed slots are addressed BY DERIVED NAME on every path that removes them: the in-spec
// reclaims here in applyResources, and the orphan teardown in RoleGroupCleaner when the role group
// leaves the spec. Nothing recovers a slot filled under a different name — discoverLiveOrphans
// rejects any object whose name is not what RoleGroupResourceName produces, and confirmRoleGroupReclaimed
// re-checks the same fixed list before the group's status entry is pruned. Such an object is
// applied, owner-referenced, reported healthy, and then survives every teardown until the cluster
// CR itself is deleted: a Service that keeps a Prometheus target with no endpoints, a ConfigMap and
// a StatefulSet nothing will ever look at again.
//
// So the framework owns these names and says so at build time, where the answer is checkable
// without a live cluster and costs one reconcile instead of a silent leak. A product that needs to
// choose its own name ships the object through RoleGroupResources.ExtraResources, whose reclaim is
// label-based precisely because its names are the product's (see SetupWithManagerOptions.ExtraOwns).
//
// The namespace is checked for the same reason and one more: a cross-namespace owner reference is
// not honoured by Kubernetes garbage collection, so the object would outlive even the CR.
//
// ExtraResources take the other branch of that trade — the product owns those names, so there is no
// derived name to check them against — but they are checked for the three things the apply path
// cannot survive whatever the name is: a missing name, a namespace that is not the cluster's, and an
// identity (GVK + name) some other entry of the same declaration already claims. The first two would
// otherwise fail inside applyResource at step 4, with the ConfigMap and Services of the same role
// group already applied; the third fails nowhere at all, which is worse (see validateExtraResources).
func validateRoleGroupResources(scheme *runtime.Scheme, resources *RoleGroupResources, buildCtx *RoleGroupBuildContext) error {
	slots := []struct {
		field string
		want  string
		obj   client.Object
	}{
		{"ConfigMap", buildCtx.ResourceName, objectOrNil(resources.ConfigMap)},
		{"HeadlessService", buildCtx.ResourceName + "-headless", objectOrNil(resources.HeadlessService)},
		{"Service", buildCtx.ResourceName, objectOrNil(resources.Service)},
		{"StatefulSet", buildCtx.ResourceName, objectOrNil(resources.StatefulSet)},
		{"PodDisruptionBudget", buildCtx.ResourceName, objectOrNil(resources.PodDisruptionBudget)},
		{"MetricsService", buildCtx.ResourceName + "-metrics", objectOrNil(resources.MetricsService)},
	}

	for _, slot := range slots {
		if slot.obj == nil {
			continue
		}
		if name := slot.obj.GetName(); name != slot.want {
			return NewValidationError("RoleGroupResources."+slot.field, buildCtx.RoleName, buildCtx.RoleGroupName,
				fmt.Errorf("the framework owns the name of this slot: got %q, want %q. "+
					"A slot filled under any other name is applied but reclaimed by neither the in-spec "+
					"reclaim nor the orphan teardown, both of which address it by that name. Ship a "+
					"differently named object through RoleGroupResources.ExtraResources instead, and "+
					"register its kind in SetupWithManagerOptions.ExtraOwns so it is reclaimed by label",
					name, slot.want))
		}
		// An UNSET namespace is rejected along with a wrong one. Nothing downstream fills it in:
		// applyResource hands the object straight to CreateOrUpdate, and SetControllerReference
		// refuses any namespace that is not the owner's — including the empty one. Letting it
		// through would fail at this slot's own apply step, which for a late slot means the
		// earlier ones are already applied: exactly the half-converged state this check prevents.
		if ns := slot.obj.GetNamespace(); ns != buildCtx.ClusterNamespace {
			return NewValidationError("RoleGroupResources."+slot.field, buildCtx.RoleName, buildCtx.RoleGroupName,
				fmt.Errorf("the slot must live in the cluster's namespace: got %q, want %q. "+
					"Kubernetes disallows a cross-namespace owner reference, and an unset namespace is "+
					"not filled in by the framework", ns, buildCtx.ClusterNamespace))
		}
	}

	// The fixed slots are the first claimants of an identity, because they are applied around the
	// extras and their names are the framework's. Their own identities cannot collide with each
	// other: the three Services are told apart by the -headless and -metrics suffixes, and the rest
	// are distinct kinds.
	claimed := make(map[string]string, len(slots)+len(resources.ExtraResources))
	for _, slot := range slots {
		if slot.obj == nil {
			continue
		}
		claimed[objectIdentity(scheme, slot.obj)] = "RoleGroupResources." + slot.field
	}

	return validateExtraResources(scheme, resources.ExtraResources, buildCtx, claimed)
}

// validateExtraResources rejects the ExtraResources entries the apply path cannot honour.
//
// The name is the product's here, so there is nothing to compare it against — but three properties
// are still the framework's business, because it is the framework that writes these objects.
//
// A missing name and a namespace that is not the cluster's both fail inside applyResource:
// CreateOrUpdate Gets by key and the API server rejects an empty name, and SetControllerReference
// refuses a namespace that is not the owner's — including the empty one a cluster-scoped object
// carries, which Kubernetes would not honour as a dependent of a namespaced CR anyway. Left to
// apply, they fail at step 4, after the ConfigMap and both Services of the same role group have
// landed: the half-converged role group validateRoleGroupResources exists to prevent.
//
// A DUPLICATE identity is the one that fails nowhere. Two entries with the same GVK and name — two
// extras, or an extra that lands on a fixed slot's name — are two writers for one object in a single
// pass. The last write wins, so the other entry is silently discarded, and if the two desired states
// differ the object is rewritten twice per reconcile: each write bumps its resourceVersion, the
// framework's own Owns() watch fires, and the reconcile that follows does it again. Nothing errors
// and nothing backs off — the same self-sustaining loop two writers produced for the restarter's
// pod-template annotation. A product cannot mean this, so it is rejected rather than resolved.
func validateExtraResources(
	scheme *runtime.Scheme,
	extras []client.Object,
	buildCtx *RoleGroupBuildContext,
	claimed map[string]string,
) error {
	for i, extra := range extras {
		// Skipped by the apply path too, so an optional extra may be left nil without the product
		// having to filter the slice it builds.
		if extra == nil {
			continue
		}
		field := fmt.Sprintf("RoleGroupResources.ExtraResources[%d]", i)

		if extra.GetName() == "" {
			return NewValidationError(field, buildCtx.RoleName, buildCtx.RoleGroupName,
				fmt.Errorf("an extra %s carries no name. The framework applies extras with "+
					"CreateOrUpdate, which addresses the object by name every reconcile, so "+
					"metadata.generateName cannot serve here: a generated name would produce a new "+
					"object on every pass instead of converging one", resolveKind(scheme, extra)))
		}

		if ns := extra.GetNamespace(); ns != buildCtx.ClusterNamespace {
			return NewValidationError(field, buildCtx.RoleName, buildCtx.RoleGroupName,
				fmt.Errorf("extra %s %q must live in the cluster's namespace: got %q, want %q. The "+
					"framework sets a controller owner reference on every extra, and Kubernetes "+
					"honours neither a cross-namespace one nor a namespaced owner for a "+
					"cluster-scoped object. A cluster-scoped resource has no owner the framework can "+
					"give it, so the product owns its lifecycle, cleanup included",
					resolveKind(scheme, extra), extra.GetName(), ns, buildCtx.ClusterNamespace))
		}

		identity := objectIdentity(scheme, extra)
		if owner, taken := claimed[identity]; taken {
			return NewValidationError(field, buildCtx.RoleName, buildCtx.RoleGroupName,
				fmt.Errorf("extra %s %q is already declared by %s. One object cannot have two "+
					"writers in one pass: the later apply wins, discarding the other silently, and "+
					"if the two differ the object is rewritten on every reconcile — each write wakes "+
					"the framework's own watch, which reconciles again, with nothing failing to back "+
					"the loop off",
					resolveKind(scheme, extra), extra.GetName(), owner))
		}
		claimed[identity] = field
	}

	return nil
}

// objectIdentity renders the object the apply path will write to: its GVK and name, within the one
// namespace validation already pins. It is a comparison key, never shown to a user.
//
// TypeMeta is preferred when it is populated (an unstructured object carries nothing else), and the
// scheme answers for the typed objects that leave it empty — the same ladder resolveKind climbs. A
// type that is in neither falls back to its Go type name, which is a coarser key: it cannot tell two
// GVKs backed by one Go type apart. That direction is safe here, because such an object cannot be
// applied at all — the client has no way to resolve its GVK either — and no false duplicate follows
// from it, since one Go type is one type.
func objectIdentity(scheme *runtime.Scheme, obj client.Object) string {
	gvk := obj.GetObjectKind().GroupVersionKind()
	if (gvk.Kind == "" || gvk.Version == "") && scheme != nil {
		if gvks, _, err := scheme.ObjectKinds(obj); err == nil && len(gvks) > 0 {
			gvk = gvks[0]
		}
	}
	if gvk.Kind == "" {
		return goTypeName(obj) + "/" + obj.GetName()
	}
	return gvk.GroupVersion().String() + ", Kind=" + gvk.Kind + "/" + obj.GetName()
}

// objectOrNil converts a typed slot pointer to client.Object without the typed-nil trap: a nil
// *corev1.Service assigned to a client.Object interface is non-nil, so `obj == nil` on the
// interface would be false and every unset slot would be validated as an object with an empty name.
func objectOrNil[T interface {
	client.Object
	comparable
}](obj T) client.Object {
	var zero T
	if obj == zero {
		return nil
	}
	return obj
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

// markRolePodDisruptionBudget stamps the role slot label, carrying the role the PDB covers, on a
// handler-built role PodDisruptionBudget. See LabelRolePodDisruptionBudget and the cleaner's
// cleanupOrphanedRolePDBs for why the role name has to travel on the object.
func markRolePodDisruptionBudget(pdb *policyv1.PodDisruptionBudget, roleName string) {
	if pdb.Labels == nil {
		pdb.Labels = map[string]string{}
	}
	pdb.Labels[LabelRolePodDisruptionBudget] = roleName
}

// markRoleGroupPodDisruptionBudget stamps the role group slot label on the per-group PDB a product
// ships through RoleGroupResources.PodDisruptionBudget. See LabelRoleGroupPodDisruptionBudget.
func markRoleGroupPodDisruptionBudget(pdb *policyv1.PodDisruptionBudget, roleGroupName string) {
	if pdb.Labels == nil {
		pdb.Labels = map[string]string{}
	}
	pdb.Labels[LabelRoleGroupPodDisruptionBudget] = roleGroupName
}

// reclaimRolePDB deletes the role's PodDisruptionBudget when its config is unset or disabled, but
// only when the live object is the one this framework applied as the role slot. Deleting purely by
// derived name would also hit a product's own PDB named "<cluster>-<role>" (shipped through
// RoleGroupResources.ExtraResources, say), which carries this CR's controller owner reference too —
// ownership does not distinguish them, which is exactly why the slot label exists.
func (r *GenericReconciler[CR]) reclaimRolePDB(ctx context.Context, namespace, name, roleName string, ownerUID types.UID, clusterName string) error {
	pdb := &policyv1.PodDisruptionBudget{}
	if err := r.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, pdb); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return r.apiError(err)
	}
	if pdb.Labels[LabelRolePodDisruptionBudget] != roleName {
		return nil
	}
	return r.cleaner.deletePDB(ctx, namespace, name, ownerUID, clusterName)
}

// reclaimRoleGroupPDB deletes the per-role-group PodDisruptionBudget named "<cluster>-<role>-<group>"
// when the handler stops shipping one, but only when the live object is the framework's per-group
// slot: the label this apply path stamps, or — for a cluster created before the PDB became a
// role-level resource (issue #530), when no slot label existed — the pre-#530 framework
// fingerprint. Without that check the reclaim would delete any owned PDB a product happens to ship
// under the role group's own name.
func (r *GenericReconciler[CR]) reclaimRoleGroupPDB(ctx context.Context, buildCtx *RoleGroupBuildContext, ownerUID types.UID) error {
	pdb := &policyv1.PodDisruptionBudget{}
	key := types.NamespacedName{Namespace: buildCtx.ClusterNamespace, Name: buildCtx.ResourceName}
	if err := r.client.Get(ctx, key, pdb); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return r.apiError(err)
	}
	if !isFrameworkRoleGroupPDB(pdb, buildCtx.ClusterName, buildCtx.RoleName, buildCtx.RoleGroupName) {
		return nil
	}
	return r.cleaner.deletePDB(ctx, key.Namespace, key.Name, ownerUID, buildCtx.ClusterName)
}

// isFrameworkRoleGroupPDB reports whether a live PodDisruptionBudget under a role group's own name
// is one the framework put there. Pre-#530 versions built it from the role group's descriptive
// labels, which is the only fingerprint those objects carry: the framework's managed-by value plus
// the role group marker label. Both are required — managed-by alone would also match the role-level
// PDB of a differently named role group.
func isFrameworkRoleGroupPDB(pdb *policyv1.PodDisruptionBudget, clusterName, roleName, roleGroupName string) bool {
	if pdb.Labels[LabelRoleGroupPodDisruptionBudget] == roleGroupName {
		return true
	}
	return pdb.Labels["app.kubernetes.io/managed-by"] == managedByValue &&
		pdb.Labels[RoleGroupMarkerLabelKey(clusterName, roleName, roleGroupName)] == valueTrue
}

// reclaimMetricsService deletes the role group's metrics Service, but only when the live object
// is one this framework applied as the metrics slot. Deleting purely by derived name would also
// hit the client Service of a role group named "<group>-metrics", and any object a product ships
// under that name through RoleGroupResources.ExtraResources — both of which carry this CR's
// controller owner reference, so ownership alone does not distinguish them.
//
// That separation holds only because LabelMetricsService cannot arrive on an object by any route
// other than this framework stamping it: it is in reservedSlotLabelKeys, so a CR carrying it does
// not propagate it to the resources a handler builds. Without that filter one `kubectl label` on
// the cluster CR puts the slot label on every built object, and this Get finds the neighbouring
// role group's client Service wearing it.
func (r *GenericReconciler[CR]) reclaimMetricsService(ctx context.Context, namespace, name string, ownerUID types.UID, clusterName string) error {
	svc := &corev1.Service{}
	if err := r.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, svc); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return r.apiError(err)
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
	return resolveKind(r.scheme, obj)
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
	// The resourceVersion the live object carried before the write. CreateOrUpdate Gets into obj
	// and then runs this mutate func, so this is the only point where it can be read; after the
	// call obj holds whatever the API server returned.
	var liveResourceVersion string
	// Field paths whose desired value the API server will not let the framework change. Collected
	// inside the mutate func because that is the only place both states are in hand.
	var ignoredImmutable []string
	result, err := r.k8sUtil.CreateOrUpdate(ctx, obj, func() error {
		liveResourceVersion = obj.GetResourceVersion()
		// Set ownership
		if err := controllerutil.SetControllerReference(owner, obj, r.scheme); err != nil {
			return err
		}
		// Copy the desired state onto the live object; without this the apply path is
		// create-only (see the doc comment above and copyDesiredState).
		var copyErr error
		ignoredImmutable, copyErr = copyDesiredState(desired, obj)
		return copyErr
	})
	// Tell the user their change was dropped. The framework preserving an immutable field is
	// correct — the alternative is an Update the API server rejects on every reconcile — but
	// doing it silently is what let a storage resize be accepted, reported as
	// ReconcileComplete=True, and never applied. Kubernetes aggregates repeated identical events
	// into one entry with a count, so this stays visible in `kubectl describe` for as long as the
	// spec and the live object disagree, without flooding.
	//
	// Emitted BEFORE the error is returned, and deliberately: the mutate func has already run, so
	// what the framework refused to change is known even when the write then failed — and a
	// rejected Update is precisely when the user most needs to be told which of their changes the
	// framework had dropped. Returning first meant the one event that explains the situation was
	// the one event that never fired.
	if len(ignoredImmutable) > 0 {
		r.eventManager.EmitWarningEvent(owner, "ImmutableFieldIgnored", fmt.Sprintf(
			"%s %q: %s cannot be changed after creation, so the live value is kept and the spec has no effect. Recreate the resource to apply it.",
			r.resourceKind(obj), obj.GetName(), strings.Join(ignoredImmutable, ", ")))
	}

	if err != nil {
		return r.apiError(err)
	}

	switch result {
	case controllerutil.OperationResultCreated:
		r.eventManager.EmitCreateEvent(owner.GetName(), obj)
	case controllerutil.OperationResultUpdated:
		// An Update is issued whenever the desired object differs from the live one in Go terms,
		// which it always does once the API server has defaulted fields the handler omits. The
		// server short-circuits such a write and returns the stored object unchanged, so the
		// resourceVersion is what separates a real change from the steady state — without it a
		// settled cluster emits a Normal "Updated" event per resource on every reconcile, forever.
		if obj.GetResourceVersion() != liveResourceVersion {
			r.eventManager.EmitUpdateEvent(owner.GetName(), obj)
		}
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
	// The controller watches its own CR, so an unconditional write would deliver a watch event
	// and schedule another reconcile that writes again.
	if stored != nil && apiequality.Semantic.DeepEqual(stored, cr) {
		return nil
	}

	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		err := r.client.Status().Update(ctx, cr)
		if !errors.IsConflict(err) {
			return err
		}
		// Read the fresh resourceVersion through the uncached reader: a conflict means the
		// informer has not observed the competing write yet, so the cache would hand back the
		// same stale version and every retry would conflict again.
		fresh := cr.DeepCopy()
		if getErr := r.statusReader().Get(ctx, client.ObjectKeyFromObject(cr), fresh); getErr != nil {
			return getErr
		}
		cr.SetResourceVersion(fresh.GetResourceVersion())
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
	r.eventManager.EmitWarningEvent(cr, "UnknownConfiguredRole", message)
}

// SetupWithManagerOptions extends the controller's watch set with resources the framework does
// not know about. RoleGroupResources.ExtraResources have arbitrary GVKs, and a GVK that is not
// watched produces no reconcile event: out-of-band edits to such a resource are only repaired by
// the informer resync or an unrelated CR event. Products that emit extras register them here.
type SetupWithManagerOptions struct {
	// ExtraOwns adds an Owns() watch for each object's GVK — the right choice for resources the
	// reconciler creates with a controller owner reference (which is how ExtraResources are
	// applied).
	//
	// It ALSO tells the orphan cleaner which kinds to reclaim when a role group leaves the spec
	// (see RoleGroupCleaner.WithExtraResourceKinds). The two are the same list by construction —
	// the kinds a product ships as ExtraResources — so deriving cleanup from it means a product
	// cannot register a kind for watches and forget to register it for cleanup. Only resources
	// carrying the departing role group's identity labels AND this CR's controller owner reference
	// are deleted, so a kind listed here that is not per-role-group is simply never matched.
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
	// ExtraOwns is the product's declaration of "these GVKs are mine, per role group". The cleaner
	// needs exactly that list to reclaim a removed role group's extras, and taking it from here
	// rather than from a second field means the two cannot drift: a product that adds an extra kind
	// has to register it for watches anyway, and its cleanup follows.
	r.cleaner.WithExtraResourceKinds(opts.ExtraOwns...)
	return r.ControllerBuilder(mgr, opts).Complete(r)
}

// ControllerBuilder returns the controller builder configured with the framework's watch set
// (the CR plus the resource kinds the reconciler owns) and the caller's extra watches, without
// completing it. Products needing full control over the controller — predicates, options,
// a custom Reconciler wrapper — build on top of this and call Complete themselves.
func (r *GenericReconciler[CR]) ControllerBuilder(mgr ctrl.Manager, opts SetupWithManagerOptions) *builder.Builder {
	b := ctrl.NewControllerManagedBy(mgr).
		For(r.prototype).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Service{}).
		Owns(&policyv1.PodDisruptionBudget{}).
		Owns(&corev1.ServiceAccount{})

	// The RBAC watches are registered ONLY when the product declares workload rules. An
	// unconditional Owns() would force every operator built on this SDK to grant itself cluster-wide
	// list;watch on roles and rolebindings — and a forbidden informer fails WaitForCacheSync for ALL
	// sources, so manager.Start returns and the process exits. Charging that to operators that never
	// use the feature is a cost that has to stay at zero.
	if r.workloadRBACRules != nil {
		b = b.Owns(&rbacv1.Role{}).Owns(&rbacv1.RoleBinding{})
	}

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

// resolveServiceAccountName returns the name of the ServiceAccount this cluster's workload runs
// as. It is DERIVED from the CR — never configured, never empty — so the identity a role group
// consumes and the object step 0 ensured are the same string by construction rather than by two
// call sites agreeing.
//
// The Kind comes from the scheme (resolveKind), which resolves it from the Go type, so it is
// stable across a fetched CR and a prototype even though the typed client strips TypeMeta.
func (r *GenericReconciler[CR]) resolveServiceAccountName(cr CR) string {
	return ServiceAccountResourceName(r.resourceKind(cr), cr.GetName())
}

// ensureServiceAccount creates or updates the ServiceAccount for the cluster workload and sets
// the CR as its controller owner.
//
// Guard rail: if an SA with this name already exists but is controlled by a DIFFERENT owner,
// SetControllerReference refuses to steal it and the raw AlreadyOwnedError is wrapped in an error
// naming both owners. With a derived name this no longer means "two clusters were configured onto
// one SA" — that is now unrepresentable — so it means something ELSE occupies the derived name,
// which the framework must not adopt: adopting it would hand these pods whatever identity that
// object already carries.
func (r *GenericReconciler[CR]) ensureServiceAccount(ctx context.Context, cr CR, name string) error {
	sa := &corev1.ServiceAccount{}
	sa.Name = name
	sa.Namespace = cr.GetNamespace()

	_, err := r.k8sUtil.CreateOrUpdate(ctx, sa, func() error {
		// Copied, not aliased: cr.GetLabels() hands out the live object's map, and assigning it
		// here would let anything that later writes to sa.Labels mutate the object this cycle's
		// status write is computed against.
		//
		// The canonical identity labels are applied on top, the same pair EnsureDiscoveryConfigMap
		// stamps and the same two every other framework-built resource carries. The ServiceAccount
		// was the one object that carried whatever the CR happened to have and nothing else, so a
		// cluster with no labels produced an SA that no `app.kubernetes.io/instance` query could
		// find.
		labels := maps.Clone(cr.GetLabels())
		if labels == nil {
			labels = map[string]string{}
		}
		labels[constant.LabelKubernetesInstance] = cr.GetName()
		labels[constant.LabelKubernetesManagedBy] = managedByValue
		sa.Labels = labels

		if err := controllerutil.SetControllerReference(cr, sa, r.scheme); err != nil {
			var alreadyOwned *controllerutil.AlreadyOwnedError
			if stderrors.As(err, &alreadyOwned) {
				return fmt.Errorf(
					"ServiceAccount %s/%s is already controlled by %s %q and cannot be adopted by %s %q. "+
						"This name is derived from the CR (kind + name), so it is not a naming collision "+
						"between two clusters: something else occupies it. Delete that object, or rename "+
						"the cluster: %w",
					sa.Namespace, sa.Name,
					alreadyOwned.Owner.Kind, alreadyOwned.Owner.Name,
					r.resourceKind(cr), cr.GetName(),
					err,
				)
			}
			return err
		}
		return nil
	})
	// Route the API error through the same translation applyResource uses. A 429 here is the API
	// server asking for a pause, not a broken cluster, and this step is now on EVERY cluster's
	// critical path — returning the raw error would run the product's error hooks and flip Degraded
	// on a cluster nobody was able to read yet. While the SA was opt-in this branch was reachable
	// only by the products that configured one, which is why the gap survived.
	return r.apiError(err)
}

// ensureWorkloadRBAC maintains the workload's namespaced Role and RoleBinding from the rules the
// product declared, bound to the ServiceAccount name this cycle already derived and ensured.
//
// A nil WorkloadRBACRules hook means the product never opted in, and does NOT reach
// EnsureWorkloadRBAC: an empty rule set there means REVOKE, and running a revoke every reconcile
// for every operator that has no workload RBAC would issue two pointless Gets per cluster per pass
// and, worse, require the RBAC read permission from operators that never asked for the feature.
// A hook that is set and RETURNS empty is a different statement — that is the product revoking —
// and does go through.
func (r *GenericReconciler[CR]) ensureWorkloadRBAC(ctx context.Context, cr CR, serviceAccountName string) error {
	if r.workloadRBACRules == nil {
		return nil
	}
	if err := EnsureWorkloadRBAC(ctx, r.client, r.scheme, cr, serviceAccountName, r.workloadRBACRules(cr)); err != nil {
		// Same reasoning as ensureServiceAccount: a 429 is a request to back off, not a fault.
		return r.apiError(err)
	}
	return nil
}
