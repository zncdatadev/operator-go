// Package controller executes the framework resource pipeline against direct API
// observations. Products register through the public operator package.
package controller

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/zncdatadev/operator-go/internal/framework/pipeline"
	"github.com/zncdatadev/operator-go/pkg/framework"
	"github.com/zncdatadev/operator-go/pkg/framework/input"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type Reconciler[CR input.Object, C, S, F any] struct {
	// Client must read directly from the API server, including inside retries.
	Client     client.Client
	Scheme     *runtime.Scheme
	Binding    input.Binding[CR]
	Definition framework.ProductDefinition[C, S, F]
	Assembly   framework.AssemblyOptions
	Facts      F
	// ResolveFacts is the explicit read-only I/O step between CR preparation and
	// pure resource generation. Every group gets an isolated input and reader.
	ResolveFacts func(context.Context, framework.FactsReader, framework.FactInput[C, S,
		F]) (framework.FactResult[F], error)
	// External objects are refreshed even after resolution succeeds. No external
	// object watches or product-owned controller are implied by this interval.
	FactRefreshInterval time.Duration
}

func (r *Reconciler[CR, C, S, F]) SetupWithManager(manager ctrl.Manager) error {
	if r.Client == nil || r.Scheme == nil || r.Binding.NewObject == nil ||
		r.Binding.Status == nil || r.Binding.Project == nil || r.Binding.Operation == nil {
		return fmt.Errorf("controller client, scheme and complete input binding are required")
	}
	// Status writes must not schedule another pass of their own. Child updates
	// include StatefulSet status and metadata, not just generation changes.
	return ctrl.NewControllerManagedBy(manager).
		WithOptions(r.controllerOptions()).
		For(r.Binding.NewObject(), builder.WithPredicates(predicate.Or(
			predicate.GenerationChangedPredicate{}, predicate.LabelChangedPredicate{},
			predicate.AnnotationChangedPredicate{},
		))).
		Owns(&corev1.ConfigMap{}).Owns(&corev1.Service{}).Owns(&appsv1.StatefulSet{}).
		Owns(&policyv1.PodDisruptionBudget{}).
		Complete(r)
}

func (r *Reconciler[CR, C, S, F]) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	cr := r.Binding.NewObject()
	if err := r.Client.Get(ctx, request.NamespacedName, cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !cr.GetDeletionTimestamp().IsZero() {
		return ctrl.Result{}, nil
	}
	if r.Binding.Operation == nil {
		return ctrl.Result{}, fmt.Errorf("generated operation binding is required")
	}
	operation := r.Binding.Operation(cr)
	if operation.ReconciliationPaused {
		status := r.Binding.Status(cr).DeepCopy()
		status.ObservedGeneration = cr.GetGeneration()
		setCondition(status, "Paused", true, "ReconciliationPaused", "Resource reconciliation is paused")
		if err := r.writeStatus(ctx, cr, *status); err != nil {
			if errors.Is(err, errSuperseded) {
				return ctrl.Result{RequeueAfter: time.Millisecond}, nil
			}
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	// One private client per pass: the manager's shared reconciler never stores
	// cluster-specific state. Fresh intent reads use the unwrapped client.
	pass := *r
	pass.Client = &operationClient{Client: r.Client, check: func(ctx context.Context) error {
		return r.currentInput(ctx, cr)
	}}
	return pass.reconcileObserved(ctx, cr, operation)
}

func (r *Reconciler[CR, C, S, F]) reconcileObserved(
	ctx context.Context, cr CR, operation framework.ClusterOperation,
) (ctrl.Result, error) {
	status := framework.ReconcileStatus{ObservedGeneration: cr.GetGeneration()}
	// Carry transition times forward. Group observations are rebuilt each pass.
	status.Conditions = slices.Clone(r.Binding.Status(cr).Conditions)
	setCondition(&status, "Paused", false, "ReconciliationActive", "Resource reconciliation is active")
	projection, sourceErr := r.Binding.Project(cr)
	var source pipeline.SourceSnapshot[F]
	if sourceErr == nil {
		source, sourceErr = pipeline.SourceFromProjection(projection, r.Facts, operation)
	}
	if sourceErr == nil && (source.Cluster.Name != cr.GetName() || source.Cluster.Namespace != cr.GetNamespace()) {
		sourceErr = fmt.Errorf("source identity differs from the observed CR")
	}
	if sourceErr == nil && source.Operation != operation {
		sourceErr = fmt.Errorf("source operation differs from the observed CR")
	}
	var identities []framework.GroupIdentity
	var roleIdentities []framework.RoleIdentity
	inventoryErr, roleInventoryErr := sourceErr, sourceErr
	if sourceErr == nil {
		identities, inventoryErr = pipeline.SourceGroupIdentities(source)
		roleIdentities, roleInventoryErr = pipeline.SourceRoleIdentities(source)
	}
	// Role management is built from the complete source independently of group
	// preparation and product callbacks. Facts never remove desired role replicas.
	var roles []pipeline.BuiltRole
	roleBuildErr := sourceErr
	if roleBuildErr == nil {
		roles, roleBuildErr = pipeline.BuildRoleResources(r.Definition, source)
	}
	var failures, stopFailures []error
	pending := false
	if operation.Stopped {
		var stopping bool
		stopping, stopFailures = r.stopWorkloads(ctx, cr, identities)
		pending = pending || stopping
		failures = append(failures, stopFailures...)
	}
	plan, err := r.buildGroupPlan(ctx, source, sourceErr)
	if err != nil {
		failures = append(failures, err)
		setCondition(&status, "Built", false, "BuildFailed", err.Error())
		setCondition(&status, "Applied", false, "NotBuilt", "No resource plan is available")
		setCondition(&status, "WorkloadsReady", false, "NotApplied", "The current generation was not applied")
	} else {
		applying, applyErrors := r.applyPlan(ctx, cr, plan, &status)
		pending = pending || applying
		failures = append(failures, applyErrors...)
	}
	rolePending, roleFailures := r.reconcileRoleResources(ctx, cr, roles,
		roleIdentities, roleBuildErr, roleInventoryErr, &status)
	pending = pending || rolePending
	failures = append(failures, roleFailures...)
	if inventoryErr != nil {
		meta.SetStatusCondition(&status.Conditions, metav1.Condition{Type: "Retired",
			Status: metav1.ConditionUnknown, ObservedGeneration: status.ObservedGeneration,
			Reason:  "InventoryUnavailable",
			Message: "A complete desired identity inventory is unavailable; retirement is not attempted"})
	} else {
		retiring, retirementErrors := r.retireGroups(ctx, cr, identities, &status)
		pending = pending || retiring
		failures = append(failures, retirementErrors...)
	}
	if operation.Stopped {
		// Observe again after apply/retirement: a just-created zero-replica
		// StatefulSet has not yet confirmed its controller observation.
		stopping, observationErrors := r.stopWorkloads(ctx, cr, identities)
		pending = pending || stopping
		stopFailures = append(stopFailures, observationErrors...)
		failures = append(failures, observationErrors...)
		complete := !stopping && len(stopFailures) == 0
		reason, message := "Stopping", "Waiting for zero-replica StatefulSet observations and Pod absence"
		if complete {
			reason, message = "WorkloadsStopped", "Controlled workloads have zero replicas and no remaining Pods"
		} else if len(stopFailures) > 0 {
			reason, message = "StopFailed", errorMessage(stopFailures)
		}
		setCondition(&status, "Stopped", complete, reason, message)
		if inventoryErr != nil && len(stopFailures) == 0 {
			meta.SetStatusCondition(&status.Conditions, metav1.Condition{Type: "Stopped",
				Status: metav1.ConditionUnknown, ObservedGeneration: status.ObservedGeneration,
				Reason:  "InventoryUnavailable",
				Message: "Known live workloads were checked; complete desired inventory is unavailable"})
		}
		meta.SetStatusCondition(&status.Conditions, metav1.Condition{Type: "WorkloadsReady",
			Status: metav1.ConditionUnknown, ObservedGeneration: status.ObservedGeneration,
			Reason: "ClusterStopped", Message: "Stop is requested; application availability is not evaluated"})
	} else {
		setCondition(&status, "Stopped", false, "NotRequested", "Stop is not requested")
	}
	if errors.Is(errors.Join(failures...), errSuperseded) {
		return ctrl.Result{RequeueAfter: time.Millisecond}, nil
	}
	if err := r.writeStatus(ctx, cr, status); err != nil {
		if errors.Is(err, errSuperseded) {
			return ctrl.Result{RequeueAfter: time.Millisecond}, nil
		}
		failures = append(failures, err)
	}
	if err := errors.Join(failures...); err != nil {
		return ctrl.Result{}, err
	}
	// Pod drain/deletion may not emit a directly owned-object event. Pending
	// retirement and facts use the shorter cadence; resolved facts still refresh.
	return ctrl.Result{RequeueAfter: r.nextRefresh(pending)}, nil
}

func (r *Reconciler[CR, C, S, F]) buildGroupPlan(ctx context.Context, source pipeline.SourceSnapshot[F],
	sourceErr error,
) (pipeline.ResourcePlan[C, S, F], error) {
	if sourceErr != nil {
		return pipeline.ResourcePlan[C, S, F]{}, sourceErr
	}
	if err := checkOperation(ctx, r.Client); err != nil {
		return pipeline.ResourcePlan[C, S, F]{}, err
	}
	prepared, err := pipeline.PrepareInputs(r.Definition, source)
	if err != nil {
		return pipeline.ResourcePlan[C, S, F]{}, err
	}
	facts := r.resolvePreparedFacts(ctx, prepared)
	generated, err := pipeline.GeneratePreparedGroups(r.Definition, prepared, facts)
	if err != nil {
		return pipeline.ResourcePlan[C, S, F]{}, err
	}
	assembly := r.resolvePlatformInputs(ctx, prepared, generated, r.Assembly)
	if err := checkOperation(ctx, r.Client); err != nil {
		return pipeline.ResourcePlan[C, S, F]{}, err
	}
	return pipeline.AssemblePreparedGroups(r.Definition, prepared, generated, assembly)
}

func (r *Reconciler[CR, C, S, F]) applyPlan(
	ctx context.Context, cr CR, plan pipeline.ResourcePlan[C, S, F], status *framework.ReconcileStatus,
) (bool, []error) {
	var failures []error
	pending := false
	built, applied, ready := true, true, true
	for i := range plan.Groups {
		group := &plan.Groups[i]
		observation := framework.GroupReconcileStatus{
			Role: group.Outcome.Group.Role, Name: group.Outcome.Group.Name, DesiredReplicas: group.Outcome.Group.Replicas,
			Checks: slices.Clone(group.Checks), Facts: cloneFactDiagnostic(group.Outcome.Facts),
			Platform: group.Outcome.Platform,
		}
		if blocked, waiting := factsBlockGroup(&observation); blocked {
			built, applied, ready = false, false, false
			pending = pending || waiting
		} else if group.Resources == nil || group.Outcome.Error != "" {
			observation.Message = group.Outcome.Error
			if observation.Message == "" {
				observation.Message = "Group did not produce a resource set"
			}
			built, applied, ready = false, false, false
			failures = append(failures, fmt.Errorf("%s/%s: %s", observation.Role, observation.Name, observation.Message))
		} else {
			waiting, groupReady, errs := r.applyPreparedGroup(ctx, cr, group, &observation)
			pending = pending || waiting
			applied = applied && observation.Applied
			ready = ready && groupReady
			failures = append(failures, errs...)
		}
		group.Outcome.Platform = observation.Platform
		status.Groups = append(status.Groups, observation)
	}
	if plan.Prepared != nil {
		pipeline.RefreshClusterOutput(r.Definition, &plan)
	}
	dependenciesReady := true
	for _, g := range status.Groups {
		dependenciesReady = dependenciesReady && (g.Platform == nil || g.Platform.Diagnostic.State == framework.FactsResolved)
	}
	setCondition(status, "PlatformReady", dependenciesReady, choose(dependenciesReady, "PlatformReady", "PlatformPending"),
		choose(dependenciesReady, "Platform observations are ready", "Waiting for platform producers and addresses"))
	if plan.ClusterError != "" {
		built, applied = false, false
		failures = append(failures, fmt.Errorf("cluster output: %s", plan.ClusterError))
	} else {
		waiting, errs := r.reconcileShared(ctx, cr, plan.ClusterOutput)
		pending = pending || waiting
		applied = applied && !waiting && len(errs) == 0
		failures = append(failures, errs...)
	}
	recordPlanConditions(plan, status, built, applied, ready, failures)
	return pending, failures
}

func (r *Reconciler[CR, C, S, F]) applyPreparedGroup(ctx context.Context, cr CR,
	group *pipeline.BuiltGroup[C, S, F], observation *framework.GroupReconcileStatus,
) (bool, bool, []error) {
	resources := group.Resources
	observation.ExecutionReplicas = input.Clone(resources.StatefulSet.Spec.Replicas)
	terminating, err := r.groupTerminating(ctx, cr, group.Outcome.Group)
	if err != nil {
		observation.Message = err.Error()
		return false, false, []error{err}
	}
	if terminating {
		observation.Message = "Waiting for terminating group slots before re-adding resources"
		return true, false, nil
	}
	platform, stamp, err := r.preparePlatformVolumes(ctx, cr.GetNamespace(), group.Runtime,
		&resources.StatefulSet.Spec.Template.Spec)
	observation.Platform = platform
	if err != nil {
		observation.Message = safeFactError(err)
		return false, false, []error{err}
	}
	if platform != nil && platform.Diagnostic.State != framework.FactsResolved {
		observation.Message = platform.Diagnostic.Message
		return true, false, nil
	}
	if platform != nil {
		observation.Platform = &framework.PlatformObservation{Phase: platformObserving, Diagnostic: framework.FactDiagnostic{
			State: framework.FactsPending, Reason: "ProducerNotApplied",
			Message: "Waiting for producer apply and current Pod observation"}}
	}
	if stamp != "" {
		if resources.StatefulSet.Spec.Template.Annotations == nil {
			resources.StatefulSet.Spec.Template.Annotations = map[string]string{}
		}
		resources.StatefulSet.Spec.Template.Annotations["framework.kubedoop.dev/platform-inputs"] = stamp
	}
	slot := groupSlot{Role: observation.Role, Group: observation.Name, Slot: slotStatefulset}
	err = r.preflightStorage(ctx, cr, slot, &resources.StatefulSet, resources.RetainedData, group.Runtime)
	pending := errors.Is(err, errStorageUnbound)
	if err != nil && !pending {
		observation.Message = err.Error()
		if errors.Is(err, errStoragePending) {
			return true, false, nil
		}
		return false, false, []error{err}
	}
	waiting, err := r.applyGroupObjects(ctx, cr, resources, observation, group.Runtime)
	pending = pending || waiting
	var failures []error
	if err != nil {
		failures = append(failures, err)
	}
	groupReady, err := r.observeWorkload(ctx, cr, &resources.StatefulSet, observation)
	if err != nil {
		failures = append(failures, err)
		observation.Message = err.Error()
	}
	pending = pending || (resources.Coordination != nil && !groupReady)
	if observation.Applied {
		platform, err = r.observePlatform(ctx, cr, group.Outcome.Group, resources, group.Runtime)
		observation.Platform = platform
		if err != nil {
			failures = append(failures, err)
			observation.Message = safeFactError(err)
		}
		pending = pending || (platform != nil && platform.Diagnostic.State != framework.FactsResolved)
	}
	return pending, observation.Applied && groupReady && len(failures) == 0, failures
}

func (r *Reconciler[CR, C, S, F]) applyGroupObjects(ctx context.Context, cr CR,
	resources *pipeline.GroupResources, observation *framework.GroupReconcileStatus,
	runtime ...*framework.RuntimeDescription,
) (bool, error) {
	objects := []client.Object{&resources.ConfigMap, &resources.HeadlessService,
		&resources.Service, &resources.StatefulSet}
	slots := []string{slotConfigmap, slotHeadless, slotService, slotStatefulset}
	observation.Applied = true
	for index, object := range objects {
		slot := &groupSlot{Role: observation.Role, Group: observation.Name, Slot: slots[index]}
		if slots[index] == slotStatefulset && resources.Coordination != nil && r.Binding.Operation(cr).Stopped {
			live, err := r.readSlot(ctx, cr, *slot)
			if err != nil {
				observation.Applied = false
				observation.Message = err.Error()
				return false, err
			}
			if live != nil && replicas(live.(*appsv1.StatefulSet)) > 0 {
				observation.Applied = false
				return true, nil
			}
		}
		var platformRuntime *framework.RuntimeDescription
		if len(runtime) > 0 {
			platformRuntime = runtime[0]
		}
		_, err := applyScopedObject(ctx, r.Client, cr, object, r.Scheme, slot, resources.RetainedData,
			nil, nil, platformRuntime, resources.Coordination)
		if err != nil {
			observation.Applied = false
			observation.Message = err.Error()
			if errors.Is(err, errStoragePending) {
				return true, nil
			}
			return false, fmt.Errorf("%s/%s: %w", observation.Role, observation.Name, err)
		}
	}
	return false, nil
}

func (r *Reconciler[CR, C, S, F]) observeWorkload(
	ctx context.Context, cr CR, desired *appsv1.StatefulSet, status *framework.GroupReconcileStatus,
) (bool, error) {
	live := &appsv1.StatefulSet{}
	if err := r.Client.Get(ctx, client.ObjectKeyFromObject(desired), live); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("observe StatefulSet %s: %w", desired.Name, err)
	}
	if !live.DeletionTimestamp.IsZero() {
		return false, nil
	}
	ownerKind, err := apiutil.GVKForObject(cr, r.Scheme)
	if err != nil {
		return false, err
	}
	if err := checkOwnership(cr, live, ownerKind); err != nil {
		return false, fmt.Errorf("observe StatefulSet %s: %w", desired.Name, err)
	}
	status.ReadyReplicas = live.Status.ReadyReplicas
	want := status.DesiredReplicas
	if desired.Spec.Replicas != nil {
		want = *desired.Spec.Replicas
	}
	ready := live.Status.ObservedGeneration >= live.Generation && live.Spec.Replicas != nil &&
		*live.Spec.Replicas == want && live.Status.Replicas == want &&
		live.Status.ReadyReplicas == want && live.Status.UpdatedReplicas == want &&
		(want == 0 || (live.Status.CurrentRevision != "" && live.Status.CurrentRevision == live.Status.UpdateRevision))
	if err := r.observeCoordination(ctx, cr, live, want, ready); err != nil {
		return false, err
	}
	return ready, nil
}

var errSuperseded = errors.New("input generation or identity changed during reconciliation")

func (r *Reconciler[CR, C, S, F]) writeStatus(ctx context.Context, observed CR,
	status framework.ReconcileStatus) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current := r.Binding.NewObject()
		if err := r.Client.Get(ctx, client.ObjectKeyFromObject(observed), current); err != nil {
			return client.IgnoreNotFound(err)
		}
		if current.GetUID() != observed.GetUID() || current.GetGeneration() != observed.GetGeneration() ||
			r.Binding.Operation(current) != r.Binding.Operation(observed) {
			return errSuperseded
		}
		if !current.GetDeletionTimestamp().IsZero() {
			return nil
		}
		if apiequality.Semantic.DeepEqual(r.Binding.Status(current), &status) {
			return nil
		}
		status.DeepCopyInto(r.Binding.Status(current))
		return client.IgnoreNotFound(r.Client.Status().Update(ctx, current))
	})
}

func setCondition(status *framework.ReconcileStatus, name string, success bool, reason, message string) {
	value := metav1.ConditionFalse
	if success {
		value = metav1.ConditionTrue
	}
	// Kubernetes condition messages are limited to 32768 bytes; keep aggregated
	// errors bounded without losing the individual group diagnostics.
	if len(message) > 32000 {
		message = string([]rune(message)[:min(len([]rune(message)), 8000)])
	}
	meta.SetStatusCondition(&status.Conditions, metav1.Condition{Type: name, Status: value,
		ObservedGeneration: status.ObservedGeneration, Reason: reason, Message: message})
}

func choose[T any](condition bool, yes, no T) T {
	if condition {
		return yes
	}
	return no
}

func errorMessage(failures []error) string {
	if err := errors.Join(failures...); err != nil {
		return err.Error()
	}
	return "Some resources were not applied"
}

func recordPlanConditions[C, S, F any](plan pipeline.ResourcePlan[C, S, F], status *framework.ReconcileStatus,
	built, applied, ready bool, failures []error,
) {
	setCondition(status, "Built", built, choose(built, "ResourcesBuilt", "BuildFailed"),
		choose(built, "Resources built; per-group Unknown checks remain diagnostic", "Some resources could not be built"))
	setCondition(status, "Applied", applied, choose(applied, "ResourcesApplied", "ApplyIncomplete"),
		choose(applied, "All planned resources match the current generation", errorMessage(failures)))
	if plan.ClusterError == "" && plan.ClusterOutput.State == framework.ClusterOutputPending {
		setCondition(status, "Built", false, "SharedOutputPending", plan.ClusterOutput.Reason)
		setCondition(status, "Applied", false, "SharedOutputPending", plan.ClusterOutput.Reason)
	}
	setCondition(status, "WorkloadsReady", ready && applied,
		choose(ready && applied, "StatefulSetsReady", "WaitingForWorkloads"),
		choose(ready && applied, "StatefulSet replicas are ready; application health and file reload are not checked",
			"The current resource plan is not fully applied or its StatefulSets have not reported updated replicas ready"))
	if len(plan.Groups) == 0 && applied {
		meta.SetStatusCondition(&status.Conditions, metav1.Condition{Type: "WorkloadsReady",
			Status: metav1.ConditionUnknown, ObservedGeneration: status.ObservedGeneration,
			Reason: "NoWorkloads", Message: "There are no desired workloads to observe"})
	}
}
