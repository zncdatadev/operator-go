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
	"strings"
	"time"

	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/common"
	"github.com/zncdatadev/operator-go/pkg/config"
	"github.com/zncdatadev/operator-go/pkg/productlogging"
	"github.com/zncdatadev/operator-go/pkg/sidecar"
	"github.com/zncdatadev/operator-go/pkg/util"
	"github.com/zncdatadev/operator-go/pkg/vector"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

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

	// Scheme is the runtime scheme for registering types.
	Scheme *runtime.Scheme

	// Recorder is the event recorder for emitting events.
	Recorder record.EventRecorder

	// RoleGroupHandler is the product-specific handler for building resources.
	RoleGroupHandler RoleGroupHandler[CR]

	// HealthCheckInterval is the interval between health checks.
	// Defaults to 120s if not specified.
	HealthCheckInterval time.Duration

	// HealthCheckTimeout is the timeout for health checks.
	// Defaults to 300s if not specified.
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

	// ServiceAccountName is the name of the ServiceAccount to create for the workload.
	// When set, the GenericReconciler automatically creates (or updates) a ServiceAccount
	// with this name in the CR's namespace at the start of each reconciliation.
	// Products can then reference this SA in their StatefulSetBuilder via WithServiceAccount().
	// +optional
	ServiceAccountName string

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
//     a. PreReconcile Extensions (Hook)
//     b. Validate Dependencies
//     - Handle ReconciliationPaused -> return early
//     - Handle Stopped -> scale to 0
//     c. For Each Role:
//     - Role PreReconcile Extensions
//     - For Each RoleGroup:
//     - RoleGroup PreReconcile Extensions
//     - Build RoleGroupBuildContext
//     - Delegate to RoleGroupHandler.BuildResources()
//     - Apply Resources (CM -> HeadlessSvc -> Service -> STS -> PDB -> MetricsSvc)
//     - Track in Status
//     - RoleGroup PostReconcile Extensions
//     - Role PostReconcile Extensions
//     d. Cleanup Orphaned Resources
//     e. Update Health Status
//     f. PostReconcile Extensions
//     g. Final Status Update
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
	roleGroupHandler    RoleGroupHandler[CR]
	extensionRegistry   *common.ExtensionRegistry
	prototype           CR
	rateLimitRetryAfter time.Duration
	serviceAccountName  string
	productConfig       func(cr CR, roleName, roleGroupName string) *v1alpha1.OverridesSpec
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

	cleaner := NewRoleGroupCleaner(cfg.Client, cfg.Scheme)
	if cfg.GrayDeleteGracePeriod > 0 {
		cleaner.WithGrayDeleteGracePeriod(cfg.GrayDeleteGracePeriod)
	}

	rateLimitRetryAfter := cfg.RateLimitRetryAfter
	if rateLimitRetryAfter == 0 {
		rateLimitRetryAfter = 10 * time.Second
	}

	return &GenericReconciler[CR]{
		client:              cfg.Client,
		scheme:              cfg.Scheme,
		k8sUtil:             util.NewK8sUtil(cfg.Client, cfg.Scheme),
		healthManager:       healthManager,
		dependencyResolver:  NewDependencyResolver(cfg.Client),
		cleaner:             cleaner,
		eventManager:        NewEventManager(cfg.Recorder),
		configMerger:        config.NewConfigMerger(),
		roleGroupHandler:    cfg.RoleGroupHandler,
		extensionRegistry:   common.GetExtensionRegistry(),
		prototype:           cfg.Prototype,
		rateLimitRetryAfter: rateLimitRetryAfter,
		serviceAccountName:  cfg.ServiceAccountName,
		productConfig:       cfg.ProductConfig,
	}, nil
}

// Reconcile implements controller-runtime's Reconciler interface.
// It is the entry point for reconciliation requests.
func (r *GenericReconciler[CR]) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Panic recovery
	defer func() {
		if recovered := recover(); recovered != nil {
			logger.Error(fmt.Errorf("panic: %v", recovered), "Panic recovered in reconciliation")
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

	// Perform reconciliation
	result, err := r.reconcile(ctx, cr)
	if err != nil {
		// Handle 429 rate limit: back off without setting Degraded or emitting an error event
		var rateLimitErr *RateLimitError
		if stderrors.As(err, &rateLimitErr) {
			logger.Info("Rate limited by Kubernetes API, backing off", "retryAfter", rateLimitErr.RetryAfter)
			return ctrl.Result{RequeueAfter: rateLimitErr.RetryAfter}, nil
		}

		// Execute error hooks
		r.executeErrorHooks(ctx, cr, err)

		// Set degraded condition
		status := cr.GetStatus()
		status.SetDegraded(true, v1alpha1.ReasonReconcileError, err.Error())

		// Update status
		if updateErr := r.updateStatus(ctx, cr); updateErr != nil {
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
func (r *GenericReconciler[CR]) reconcile(ctx context.Context, cr CR) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	spec := cr.GetSpec()
	status := cr.GetStatus()

	// 0. Auto-create ServiceAccount if configured
	if r.serviceAccountName != "" {
		if err := r.ensureServiceAccount(ctx, cr); err != nil {
			return ctrl.Result{}, NewReconcileError("ServiceAccount", "failed to ensure service account", err)
		}
	}

	// 1. Execute PreReconcile extensions
	if err := r.extensionRegistry.ExecuteClusterPreReconcile(ctx, r.client, cr); err != nil {
		return ctrl.Result{}, NewReconcileError("PreReconcile", "extension hook failed", err)
	}

	// 2. Validate dependencies
	if err := r.dependencyResolver.Validate(ctx, spec); err != nil {
		// Handle special dependency errors
		if depErr, ok := err.(*DependencyError); ok {
			switch depErr.Type {
			case "ReconciliationPaused":
				logger.Info("Reconciliation is paused")
				status.SetDegraded(true, v1alpha1.ReasonReconciliationPaused, "Reconciliation is paused")
				_ = r.updateStatus(ctx, cr) // nolint:errcheck
				return ctrl.Result{}, nil
			case "Stopped":
				logger.Info("Cluster is stopped, scaling to zero")
				return r.handleStoppedCluster(ctx, cr)
			}
		}
		return ctrl.Result{}, NewReconcileError("DependencyValidation", "dependency validation failed", err)
	}

	// 3. Process each role
	for roleName, roleSpec := range spec.Roles {
		if err := r.reconcileRole(ctx, cr, roleName, &roleSpec); err != nil {
			return ctrl.Result{}, err
		}
	}

	// 4. Cleanup orphaned resources
	owner := r.getAsClientObject(cr)
	if err := r.cleaner.Cleanup(ctx, cr.GetNamespace(), cr.GetName(), spec, status, owner.GetUID(), owner.GetAnnotations()); err != nil {
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
	status.ObservedGeneration = r.getAsClientObject(cr).GetGeneration()

	if err := r.updateStatus(ctx, cr); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("Reconciliation completed successfully")
	return ctrl.Result{}, nil
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

	// Execute role PostReconcile extensions
	if err := r.extensionRegistry.ExecuteRolePostReconcile(ctx, r.client, cr, roleName); err != nil {
		return NewReconcileError("RolePostReconcile", fmt.Sprintf("role %s extension hook failed", roleName), err)
	}

	logger.V(1).Info("Role reconciled", "role", roleName)
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

	// Auto-create SidecarManager based on CRD configuration
	buildCtx.SidecarManager = r.buildSidecarManager(ctx, buildCtx)

	// Set product image on sidecar manager so sidecars use the product image
	if buildCtx.SidecarManager != nil {
		if handler, ok := r.roleGroupHandler.(*BaseRoleGroupHandler[CR]); ok {
			image := handler.containerImage(buildCtx.RoleName)
			if err := buildCtx.SidecarManager.SetProductImage(image, handler.ImagePullPolicy); err != nil {
				return NewResourceBuildError("sidecar", roleName, groupName, "failed to set product image", err)
			}
		}
	}

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

	resourceName := RoleGroupResourceName(cr.GetName(), roleName, groupName)

	return &RoleGroupBuildContext{
		ClusterName:      cr.GetName(),
		ClusterNamespace: cr.GetNamespace(),
		ClusterLabels:    cr.GetLabels(),
		ClusterSpec:      cr.GetSpec(),
		RoleName:         roleName,
		RoleSpec:         roleSpec,
		RoleGroupName:    groupName,
		RoleGroupSpec:    *groupSpec,
		MergedConfig:     mergedConfig,
		ResourceName:     resourceName,
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

	// Register the canonical Vector sidecar provider (pkg/vector) if enabled.
	//
	// The role-group ConfigMap name (buildCtx.ResourceName) is passed at construction so the
	// Vector container mounts the right config without products having to cast the provider and
	// set it after the fact. The image is propagated later via SidecarManager.SetProductImage
	// (Vector ships inside the product image), so an empty image here is intentional.
	if vector.IsAgentEnabled(logging) {
		mgr.Register(
			vector.NewVectorSidecarProvider("", vector.WithConfigMapName(buildCtx.ResourceName)),
			&sidecar.SidecarConfig{Enabled: true},
		)
	}

	return mgr
}

// applyResources applies all resources in the correct dependency order.
// Order: ConfigMap -> Headless Service -> Service -> StatefulSet -> PDB -> MetricsService
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

	// 4. Apply StatefulSet
	if resources.StatefulSet != nil {
		if err := r.applyResource(ctx, owner, resources.StatefulSet); err != nil {
			return NewResourceApplyError("StatefulSet", buildCtx.ClusterNamespace, buildCtx.ResourceName, "failed to apply", err)
		}
	}

	// 5. Apply PodDisruptionBudget
	if resources.PodDisruptionBudget != nil {
		if err := r.applyResource(ctx, owner, resources.PodDisruptionBudget); err != nil {
			return NewResourceApplyError("PodDisruptionBudget", buildCtx.ClusterNamespace, buildCtx.ResourceName, "failed to apply", err)
		}
	}

	// 6. Apply MetricsService
	if resources.MetricsService != nil {
		if err := r.applyResource(ctx, owner, resources.MetricsService); err != nil {
			return NewResourceApplyError("Service", buildCtx.ClusterNamespace, buildCtx.ResourceName+"-metrics", "failed to apply metrics service", err)
		}
	}

	return nil
}

// applyResource applies a single resource using CreateOrUpdate.
// After the operation, emits a Create or Update event based on the result.
// If the Kubernetes API returns 429 Too Many Requests, a RateLimitError is returned.
func (r *GenericReconciler[CR]) applyResource(ctx context.Context, owner client.Object, obj client.Object) error {
	result, err := r.k8sUtil.CreateOrUpdate(ctx, obj, func() error {
		// Set ownership
		if err := controllerutil.SetControllerReference(owner, obj, r.scheme); err != nil {
			return err
		}
		return nil
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

// handleStoppedCluster handles the case when a cluster is stopped.
func (r *GenericReconciler[CR]) handleStoppedCluster(ctx context.Context, cr CR) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	spec := cr.GetSpec()
	status := cr.GetStatus()

	// Scale all role groups to 0
	for roleName, roleSpec := range spec.Roles {
		for groupName := range roleSpec.GetRoleGroups() {
			resourceName := RoleGroupResourceName(cr.GetName(), roleName, groupName)
			if err := r.scaleToZero(ctx, cr.GetNamespace(), resourceName); err != nil {
				logger.Error(err, "Failed to scale role group to zero", "role", roleName, "group", groupName)
				// Continue with other groups
			}
		}
	}

	status.SetUnavailable(v1alpha1.ReasonStopped, "Cluster is stopped")
	status.SetDegraded(false, v1alpha1.ReasonStopped, "Cluster is intentionally stopped")

	if err := r.updateStatus(ctx, cr); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// scaleToZero scales a StatefulSet to zero replicas.
func (r *GenericReconciler[CR]) scaleToZero(ctx context.Context, namespace, name string) error {
	sts := &appsv1.StatefulSet{}
	key := types.NamespacedName{Namespace: namespace, Name: name}

	if err := r.client.Get(ctx, key, sts); err != nil {
		if errors.IsNotFound(err) {
			return nil // StatefulSet doesn't exist, nothing to scale
		}
		return err
	}

	zero := int32(0)
	sts.Spec.Replicas = &zero

	return r.client.Update(ctx, sts)
}

// executeErrorHooks executes error hooks when reconciliation fails.
func (r *GenericReconciler[CR]) executeErrorHooks(ctx context.Context, cr CR, reconcileErr error) {
	logger := log.FromContext(ctx)
	if err := r.extensionRegistry.ExecuteClusterOnError(ctx, r.client, cr, reconcileErr); err != nil {
		logger.Error(err, "Failed to execute error hooks")
	}
}

// updateStatus updates the cluster status.
func (r *GenericReconciler[CR]) updateStatus(ctx context.Context, cr CR) error {
	return r.k8sUtil.UpdateStatus(ctx, r.getAsClientObject(cr))
}

// SetupWithManager sets up the controller with the Manager.
// The prototype must be set in the config during construction.
func (r *GenericReconciler[CR]) SetupWithManager(mgr ctrl.Manager) error {
	// Use the prototype for controller setup
	prototype := r.getAsClientObject(r.prototype)

	return ctrl.NewControllerManagedBy(mgr).
		For(prototype).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Service{}).
		Owns(&policyv1.PodDisruptionBudget{}).
		Owns(&corev1.ServiceAccount{}).
		Complete(r)
}

// ensureServiceAccount creates or updates the ServiceAccount for the cluster workload.
func (r *GenericReconciler[CR]) ensureServiceAccount(ctx context.Context, cr CR) error {
	owner := r.getAsClientObject(cr)
	sa := &corev1.ServiceAccount{}
	sa.Name = r.serviceAccountName
	sa.Namespace = cr.GetNamespace()

	_, err := r.k8sUtil.CreateOrUpdate(ctx, sa, func() error {
		sa.Labels = cr.GetLabels()
		return controllerutil.SetControllerReference(owner, sa, r.scheme)
	})
	return err
}
