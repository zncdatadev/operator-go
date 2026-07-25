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
	stderrors "errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/constant"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// AnnotationPendingDeletion is set on orphaned resources during the gray-delete grace period.
	// Value is an RFC3339 timestamp indicating when the resource was first marked for deletion.
	AnnotationPendingDeletion = "orphan.zncdata.dev/pending-deletion"

	// AnnotationDeletePVCs when set to "true" on the cluster CR, causes the cleaner to also
	// delete PVCs associated with orphaned StatefulSets.
	AnnotationDeletePVCs = "operator.zncdata.dev/delete-pvcs"
)

// LabelRolePodDisruptionBudget marks a PodDisruptionBudget as the framework's role-level slot and
// records the role it covers. Role GROUP orphans are found by diffing Status.RoleGroups, but a
// role that disappears from the spec leaves nothing to diff against: its PDB is applied only while
// the role is declared. The label is what lets the cleaner find that object without guessing from
// the derived name — a product may ship its own PDB through RoleGroupResources.PodDisruptionBudget,
// and that object carries the same controller owner reference.
const LabelRolePodDisruptionBudget = "pdb." + constant.KubedoopDomain + "/role"

// DefaultDrainPollInterval is how long the cleaner asks the caller to wait before re-entering a
// deletion it has already started — a StatefulSet scaling down, a Delete whose effect is not
// observable yet. It paces the orphan state machine, not the pod termination itself, so it is
// short: the cycle it schedules only re-reads the resources it is waiting on.
const DefaultDrainPollInterval = 5 * time.Second

// defaultCleanupRateLimitRetryAfter mirrors GenericReconcilerConfig.RateLimitRetryAfter for a
// cleaner a product builds directly.
const defaultCleanupRateLimitRetryAfter = 10 * time.Second

// RoleGroupCleaner cleans orphaned role group resources.
type RoleGroupCleaner struct {
	Client                client.Client
	Scheme                *runtime.Scheme
	grayDeleteGracePeriod time.Duration
	drainPollInterval     time.Duration
	rateLimitRetryAfter   time.Duration
	eventManager          *EventManager
}

// NewRoleGroupCleaner creates a new RoleGroupCleaner.
func NewRoleGroupCleaner(client client.Client, scheme *runtime.Scheme) *RoleGroupCleaner {
	return &RoleGroupCleaner{
		Client: client,
		Scheme: scheme,
	}
}

// WithGrayDeleteGracePeriod sets the grace period for gray deletion.
// When > 0, orphaned resources are first annotated and only deleted after the grace period.
// When 0 (default), resources are deleted immediately.
func (c *RoleGroupCleaner) WithGrayDeleteGracePeriod(d time.Duration) *RoleGroupCleaner {
	c.grayDeleteGracePeriod = d
	return c
}

// WithDrainPollInterval overrides how long the caller is asked to wait between the phases of an
// orphan deletion (scale to zero → drain → delete → confirm). Products with a long
// terminationGracePeriodSeconds can raise it to avoid polling; a non-positive value keeps
// DefaultDrainPollInterval.
func (c *RoleGroupCleaner) WithDrainPollInterval(d time.Duration) *RoleGroupCleaner {
	c.drainPollInterval = d
	return c
}

// WithRateLimitRetryAfter sets the backoff carried by the *RateLimitError the cleaner returns when
// the API server answers 429. GenericReconciler wires its own RateLimitRetryAfter here so the
// cleanup and apply paths back off identically.
func (c *RoleGroupCleaner) WithRateLimitRetryAfter(d time.Duration) *RoleGroupCleaner {
	c.rateLimitRetryAfter = d
	return c
}

// pollInterval is the wait the cleaner asks for between the phases of a deletion it started.
func (c *RoleGroupCleaner) pollInterval() time.Duration {
	if c.drainPollInterval > 0 {
		return c.drainPollInterval
	}
	return DefaultDrainPollInterval
}

// apiError maps a Kubernetes API failure onto the framework's typed errors. A 429 becomes a
// *RateLimitError, which the reconcile loop turns into a plain backoff: throttling says nothing
// about the cluster's state, so reporting it as a cleanup failure would mark a healthy cluster
// Degraded and the next pass would push the API server further over its budget.
func (c *RoleGroupCleaner) apiError(err error) error {
	if err == nil {
		return nil
	}
	if errors.IsTooManyRequests(err) {
		retryAfter := c.rateLimitRetryAfter
		if retryAfter <= 0 {
			retryAfter = defaultCleanupRateLimitRetryAfter
		}
		return NewRateLimitError(retryAfter, err)
	}
	return err
}

// WithEventManager sets the EventManager used to emit a Normal "Deleted" event for every
// resource the cleaner actually removes. Without it, deletions are silent: only the operator
// log records that a role group's resources disappeared.
func (c *RoleGroupCleaner) WithEventManager(em *EventManager) *RoleGroupCleaner {
	c.eventManager = em
	return c
}

// emitDeleted emits a Normal "Deleted" event for a resource the cleaner removed. It is a no-op
// when no EventManager is wired (WithEventManager is optional, so a cleaner built directly by a
// product keeps working).
func (c *RoleGroupCleaner) emitDeleted(clusterName string, obj client.Object) {
	if c.eventManager == nil {
		return
	}
	c.eventManager.EmitDeleteEvent(clusterName, obj)
}

// Cleanup removes orphaned resources for a cluster.
// Resources are deleted in order: PDB → StatefulSet → ConfigMap → Service → headless Service →
// metrics Service, and the role-level PDB of any role that disappeared from the spec entirely.
// PVCs are intentionally preserved to protect data unless AnnotationDeletePVCs is set in crAnnotations.
// Only resources with an ownerReference pointing to ownerUID (with controller=true) are deleted.
// If GrayDeleteGracePeriod > 0, resources are annotated on first detection and only deleted
// after the grace period has elapsed. Resources that are no longer orphaned have the annotation cleared.
//
// Deletion is a state machine, not a single pass: each step waits for the previous one to be
// observably gone before the next is issued, and an orphaned StatefulSet is scaled to zero and
// left to the StatefulSet controller's ordered drain before it is deleted. A step that is still
// in flight ends the pass for that role group and is resumed on a later reconcile.
//
// A role group is removed from status.RoleGroups only once its resources were really deleted, so
// a deferred (gray-delete) or failed pass is retried on the next reconcile instead of silently
// dropping the group from the status snapshot. A group that fails does not abort the pass for the
// others: its error is collected and the remaining groups still make progress, so one wedged role
// group cannot keep every other orphan in the status snapshot forever.
//
// The returned duration is the earliest wakeup the cleanup needs (a remaining gray-delete grace
// period, or the poll interval of a deletion in flight; 0 when nothing is pending). The caller
// turns it into a RequeueAfter so the pending work resumes on time instead of waiting for an
// unrelated event.
func (c *RoleGroupCleaner) Cleanup(
	ctx context.Context,
	namespace, clusterName string,
	spec *v1alpha1.GenericClusterSpec,
	status *v1alpha1.GenericClusterStatus,
	ownerUID types.UID,
	crAnnotations map[string]string,
) (time.Duration, error) {
	logger := log.FromContext(ctx)

	deletePVCs := crAnnotations[AnnotationDeletePVCs] == valueTrue

	// Failures are collected instead of returned on the spot, so one unreclaimable resource never
	// stops the cleanup of the rest.
	var errs []error

	// Get orphaned role groups
	orphanedGroups := status.GetOrphanedRoleGroups(spec.Roles)

	// LOW-3: If gray-delete is enabled, clear AnnotationPendingDeletion from resources
	// that are no longer orphaned (re-added to spec). This ensures the grace period is
	// respected correctly on any future re-orphaning.
	if c.grayDeleteGracePeriod > 0 {
		for roleName, roleSpec := range spec.Roles {
			for groupName := range roleSpec.RoleGroups {
				resourceName := RoleGroupResourceName(clusterName, roleName, groupName)
				if err := c.clearGrayDeleteAnnotation(ctx, namespace, resourceName, ownerUID); err != nil {
					logger.V(1).Info("Failed to clear gray-delete annotation from active resource",
						"resource", resourceName, "error", err)
				}
			}
		}
	}

	// Reclaim the role-level PDBs of vanished roles before the group loop: it must also run when
	// no role group is orphaned any more, because the groups are pruned from the status snapshot
	// as they are deleted while the role PDB may still be waiting for a retry.
	var nextRequeue time.Duration
	rolePDBState, err := c.cleanupOrphanedRolePDBs(ctx, namespace, clusterName, spec, ownerUID)
	if err != nil {
		if IsRateLimitError(err) {
			return 0, err
		}
		errs = append(errs, err)
	}
	if rolePDBState == deletionInFlight {
		nextRequeue = c.pollInterval()
	}

	if len(orphanedGroups) == 0 {
		return nextRequeue, stderrors.Join(errs...)
	}

	liveResourceNames := make(map[string]struct{})
	for roleName, roleSpec := range spec.Roles {
		for groupName := range roleSpec.RoleGroups {
			liveResourceNames[RoleGroupResourceName(clusterName, roleName, groupName)] = struct{}{}
		}
	}

	logger.Info("Cleaning up orphaned role groups", "count", countOrphanedGroups(orphanedGroups))

	// Sorted, not map order: the deletion pass is spread over several reconciles, so a stable
	// order makes the sequence of events (and the logs) reproducible.
	for _, roleName := range slices.Sorted(maps.Keys(orphanedGroups)) {
		for _, groupName := range orphanedGroups[roleName] {
			resourceName := RoleGroupResourceName(clusterName, roleName, groupName)

			deleted, retryAfter, err := c.cleanupRoleGroup(ctx, namespace, resourceName, ownerUID, deletePVCs, clusterName, liveResourceNames)
			if err != nil {
				// A 429 is the API server throttling this operator as a whole; the remaining
				// groups would only add to the requests it is rejecting.
				if IsRateLimitError(err) {
					return nextRequeue, err
				}
				// Every other failure is confined to its own role group: aborting the whole pass
				// here would let one wedged group keep every other orphan alive indefinitely.
				errs = append(errs, fmt.Errorf("failed to cleanup role group %s/%s: %w", roleName, groupName, err))
				nextRequeue = earliestRequeue(nextRequeue, c.pollInterval())
				continue
			}
			if !deleted {
				nextRequeue = earliestRequeue(nextRequeue, retryAfter)
				continue
			}

			// Prune the status snapshot only for a role group whose resources were really
			// deleted, so Status.RoleGroups converges to the desired set (docs/architecture.md
			// §4.4.2 step 5) without dropping groups whose deletion is still pending.
			status.RemoveRoleGroup(roleName, groupName)
			logger.Info("Cleaned up orphaned role group", "role", roleName, "group", groupName)
		}
	}

	return nextRequeue, stderrors.Join(errs...)
}

// cleanupOrphanedRolePDBs deletes the framework's role-level PodDisruptionBudgets
// ("<cluster>-<role>") of roles that are no longer declared in the spec. The apply path only ever
// writes a role PDB for a role the spec still declares, so removing a whole role leaves its PDB
// behind — with a selector matching pods that no longer exist, blocking nothing but showing up in
// every disruption budget review.
//
// The live objects are listed by LabelRolePodDisruptionBudget rather than by derived name: a
// product's own PDB carries the same controller owner reference, so ownership alone cannot tell
// the framework's slot apart from it. An empty ownerUID disables the reclaim entirely — without an
// owner to match, every labelled PDB in the namespace (including a sibling cluster's) would look
// like this cluster's.
func (c *RoleGroupCleaner) cleanupOrphanedRolePDBs(
	ctx context.Context,
	namespace, clusterName string,
	spec *v1alpha1.GenericClusterSpec,
	ownerUID types.UID,
) (deletionState, error) {
	if ownerUID == "" {
		return deletionSettled, nil
	}

	pdbs := &policyv1.PodDisruptionBudgetList{}
	if err := c.Client.List(ctx, pdbs, client.InNamespace(namespace), client.HasLabels{LabelRolePodDisruptionBudget}); err != nil {
		return deletionInFlight, fmt.Errorf("failed to list role PodDisruptionBudgets: %w", c.apiError(err))
	}

	state := deletionSettled
	for i := range pdbs.Items {
		pdb := &pdbs.Items[i]
		if !isOwnedByCluster(pdb, ownerUID) {
			continue
		}
		roleName := pdb.Labels[LabelRolePodDisruptionBudget]
		if _, declared := spec.Roles[roleName]; declared {
			continue
		}

		stepState, err := deleteOwned[policyv1.PodDisruptionBudget](ctx, c, namespace, pdb.Name, ownerUID, clusterName)
		if err != nil {
			return stepState, fmt.Errorf("failed to cleanup PodDisruptionBudget of removed role %q: %w", roleName, err)
		}
		if stepState == deletionInFlight {
			state = deletionInFlight
			continue
		}
		log.FromContext(ctx).Info("Cleaned up the PodDisruptionBudget of a removed role", "role", roleName, "name", pdb.Name)
	}
	return state, nil
}

// countOrphanedGroups counts total orphaned groups.
func countOrphanedGroups(orphaned map[string][]string) int {
	count := 0
	for _, groups := range orphaned {
		count += len(groups)
	}
	return count
}

// cleanupRoleGroup cleans up all resources for a single role group.
// When GrayDeleteGracePeriod is set, resources are first annotated and deleted only after
// the grace period has elapsed.
//
// The deletion runs as a state machine across reconciles rather than as one pass: a step that is
// still in flight (a StatefulSet draining, a resource that still answers Get after its Delete)
// ends the pass here, and the next reconcile resumes from the first step that has not settled.
// Every step is a Get-then-act, so re-entering is idempotent.
//
// It reports whether the role group is fully reclaimed: deleted is false while anything is still
// pending — the gray-delete grace period has not elapsed (retryAfter carries the time left) or a
// deletion is in flight (retryAfter is the poll interval). Callers must not prune the status
// snapshot unless deleted is true. A resource that belongs to another cluster counts as settled:
// this cluster will never delete it, so waiting for it would keep the role group in the status
// snapshot forever.
func (c *RoleGroupCleaner) cleanupRoleGroup(
	ctx context.Context,
	namespace, resourceName string,
	ownerUID types.UID,
	deletePVCs bool,
	clusterName string,
	liveResourceNames map[string]struct{},
) (deleted bool, retryAfter time.Duration, err error) {
	if c.grayDeleteGracePeriod > 0 {
		// Gray delete: check if the primary resource (StatefulSet or ConfigMap) is already
		// annotated. If not, annotate and defer; if yes and grace period elapsed, proceed.
		ready, remaining, err := c.checkOrMarkGrayDelete(ctx, namespace, resourceName, ownerUID)
		if err != nil {
			return false, 0, err
		}
		if !ready {
			// Grace period not yet elapsed; skip deletion this cycle
			return false, remaining, nil
		}
	}

	// Delete in order: PDB → StatefulSet → ConfigMap → Service → headless Service → metrics Service.
	// The order only means something because each step is confirmed gone before the next is issued:
	// the PDB goes first so it cannot block the eviction of the pods that follow, and the Services
	// go last so the pods still resolve each other while they terminate.
	steps := []func() (deletionState, error){
		func() (deletionState, error) {
			return deleteOwned[policyv1.PodDisruptionBudget](ctx, c, namespace, resourceName, ownerUID, clusterName)
		},
		func() (deletionState, error) {
			return c.deleteStatefulSet(ctx, namespace, resourceName, ownerUID, deletePVCs, clusterName)
		},
		func() (deletionState, error) {
			return deleteOwned[corev1.ConfigMap](ctx, c, namespace, resourceName, ownerUID, clusterName)
		},
		func() (deletionState, error) {
			return deleteOwned[corev1.Service](ctx, c, namespace, resourceName, ownerUID, clusterName)
		},
	}

	// The headless and metrics names are derived by suffix, and a role group may legitimately be
	// named "<group>-headless" or "<group>-metrics", making its own client Service collide with
	// this orphan's derived name. Both objects carry the same controller owner reference, so the
	// ownership check cannot separate them: skip any derived name that belongs to a role group
	// the spec still declares.
	for _, suffix := range []string{"-headless", "-metrics"} {
		derived := resourceName + suffix
		if _, live := liveResourceNames[derived]; live {
			log.FromContext(ctx).V(1).Info("Skipping derived Service deletion: name belongs to a live role group",
				"name", derived)
			continue
		}
		steps = append(steps, func() (deletionState, error) {
			return deleteOwned[corev1.Service](ctx, c, namespace, derived, ownerUID, clusterName)
		})
	}

	for _, step := range steps {
		state, err := step()
		if err != nil {
			return false, 0, err
		}
		if state == deletionInFlight {
			return false, c.pollInterval(), nil
		}
	}

	return true, 0, nil
}

// deletionState reports how far a single resource's deletion got.
type deletionState int

const (
	// deletionSettled means nothing of this cluster's is left under that name: the object is gone,
	// or it belongs to someone else and must not be touched.
	deletionSettled deletionState = iota

	// deletionInFlight means a write was issued whose effect is not observable yet — the
	// StatefulSet is still draining, the object is terminating behind a finalizer, or a cached read
	// has not caught up. The pass must stop and resume on a later reconcile.
	deletionInFlight
)

// ptrObject constrains PT to a pointer to the API struct T that also satisfies client.Object, so
// the deletion helpers can allocate the concrete type themselves instead of taking a client.Object
// and type-asserting it back.
type ptrObject[T any] interface {
	*T
	client.Object
}

// deleteOwned deletes one framework-owned resource and confirms the deletion took effect.
// A resource that is absent, or that belongs to another cluster, settles without a write.
func deleteOwned[T any, PT ptrObject[T]](
	ctx context.Context,
	c *RoleGroupCleaner,
	namespace, name string,
	ownerUID types.UID,
	clusterName string,
) (deletionState, error) {
	key := types.NamespacedName{Namespace: namespace, Name: name}
	obj := PT(new(T))

	if err := c.Client.Get(ctx, key, obj); err != nil {
		if errors.IsNotFound(err) {
			return deletionSettled, nil
		}
		return deletionInFlight, c.apiError(err)
	}

	if !isOwnedByCluster(obj, ownerUID) {
		log.FromContext(ctx).Info("Skipping deletion: resource not owned by this cluster",
			"name", name, "type", fmt.Sprintf("%T", obj))
		return deletionSettled, nil
	}

	if err := c.Client.Delete(ctx, obj); err != nil {
		if errors.IsNotFound(err) {
			return deletionSettled, nil
		}
		return deletionInFlight, c.apiError(err)
	}
	c.emitDeleted(clusterName, obj)

	return confirmDeleted[T, PT](ctx, c, key)
}

// confirmDeleted re-reads a resource whose Delete the API server accepted. Acceptance is not
// removal: an object held by a finalizer keeps answering Get until the finalizer clears, and a
// cached client lags behind its own writes. Treating "Delete returned nil" as "gone" is what would
// make the deletion order meaningless — the next resource type would be deleted while this one is
// still there.
func confirmDeleted[T any, PT ptrObject[T]](ctx context.Context, c *RoleGroupCleaner, key types.NamespacedName) (deletionState, error) {
	obj := PT(new(T))
	err := c.Client.Get(ctx, key, obj)
	switch {
	case errors.IsNotFound(err):
		return deletionSettled, nil
	case err != nil:
		return deletionInFlight, c.apiError(err)
	default:
		log.FromContext(ctx).V(1).Info("Deletion accepted but the resource is still present; re-checking on the next pass",
			"name", key.Name, "type", fmt.Sprintf("%T", obj))
		return deletionInFlight, nil
	}
}

// checkOrMarkGrayDelete checks whether the grace period for a gray-deleted resource has elapsed.
// Uses the StatefulSet (falling back to ConfigMap) as the primary resource to annotate.
// Only resources owned by ownerUID are annotated; a foreign primary carries no grace period at all
// (see below).
// Returns true if the resource should be deleted now, false if still within grace period; in the
// latter case remaining is the time left until the resource becomes deletable, so the caller can
// requeue precisely instead of waiting for an unrelated event.
func (c *RoleGroupCleaner) checkOrMarkGrayDelete(ctx context.Context, namespace, name string, ownerUID types.UID) (ready bool, remaining time.Duration, err error) {
	logger := log.FromContext(ctx)

	// Try StatefulSet first, then ConfigMap as fallback
	var primaryObj client.Object
	sts := &appsv1.StatefulSet{}
	if err := c.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, sts); err == nil {
		primaryObj = sts
	} else if !errors.IsNotFound(err) {
		return false, 0, err
	} else {
		cm := &corev1.ConfigMap{}
		if err := c.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, cm); err == nil {
			primaryObj = cm
		} else if errors.IsNotFound(err) {
			// Resource already gone — allow deletion pass-through
			return true, 0, nil
		} else {
			return false, 0, err
		}
	}

	// A foreign primary must not be annotated (that would mutate an unrelated object on a name
	// collision), which also means there is no timestamp to run a grace period from. Let the pass
	// proceed: every deletion is ownership-checked on its own, so the foreign objects are skipped
	// and whatever this cluster does own under that name is reclaimed. Deferring instead would
	// leave the role group in Status.RoleGroups for as long as the foreign object exists.
	if !isOwnedByCluster(primaryObj.(metav1.Object), ownerUID) {
		logger.V(1).Info("Gray-delete grace period skipped: primary resource not owned by this cluster", "name", name)
		return true, 0, nil
	}

	annotations := primaryObj.(metav1.Object).GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	markedAt, exists := annotations[AnnotationPendingDeletion]
	if !exists {
		// First detection: annotate and defer
		annotations[AnnotationPendingDeletion] = time.Now().UTC().Format(time.RFC3339)
		primaryObj.(metav1.Object).SetAnnotations(annotations)
		if err := c.Client.Update(ctx, primaryObj); err != nil {
			return false, 0, fmt.Errorf("failed to mark resource for gray deletion: %w", err)
		}
		logger.Info("Marked orphaned resource for gray deletion", "name", name, "gracePeriod", c.grayDeleteGracePeriod)
		return false, c.grayDeleteGracePeriod, nil
	}

	// Check if grace period has elapsed
	markedTime, err := time.Parse(time.RFC3339, markedAt)
	if err != nil {
		// Invalid timestamp — proceed with deletion
		return true, 0, nil
	}

	if time.Since(markedTime) >= c.grayDeleteGracePeriod {
		logger.Info("Gray deletion grace period elapsed, proceeding with deletion", "name", name)
		return true, 0, nil
	}

	logger.Info("Gray deletion grace period not yet elapsed", "name", name,
		"markedAt", markedAt, "gracePeriod", c.grayDeleteGracePeriod)
	// The annotation timestamp has second granularity, so round up to the next whole second to
	// avoid requeuing a hair too early and burning a no-op reconcile.
	return false, time.Until(markedTime.Add(c.grayDeleteGracePeriod)).Round(time.Second) + time.Second, nil
}

// isOwnedByCluster returns true if the object's ownerReferences contain an entry
// matching ownerUID with controller=true.
// If ownerUID is empty, all resources are considered owned (backward compatible).
func isOwnedByCluster(obj metav1.Object, ownerUID types.UID) bool {
	if ownerUID == "" {
		return true
	}
	for _, ref := range obj.GetOwnerReferences() {
		if ref.UID == ownerUID && ref.Controller != nil && *ref.Controller {
			return true
		}
	}
	return false
}

// deletePDB deletes a PodDisruptionBudget if it exists and is owned by the cluster.
// clusterName names the owning cluster in the emitted Deleted event. It is the apply path's
// reclaim entry point (a disabled role PDB, a legacy per-group PDB), where a deletion that is
// still in flight simply resolves on the next reconcile — the orphan state machine calls
// deleteOwned directly, because there the pending state gates the following steps.
func (c *RoleGroupCleaner) deletePDB(ctx context.Context, namespace, name string, ownerUID types.UID, clusterName string) error {
	_, err := deleteOwned[policyv1.PodDisruptionBudget](ctx, c, namespace, name, ownerUID, clusterName)
	return err
}

// deleteStatefulSet drives an orphaned StatefulSet through an ordered drain: scale to zero, let
// the StatefulSet controller retire the pods in reverse-ordinal order, and only then delete the
// object. Deleting it outright leaves the pods to cascade garbage collection, which removes them
// in arbitrary order and gives a stateful product no chance to shut down the way its own rolling
// update would — the whole point of scaling to zero first.
//
// Each phase returns deletionInFlight so the caller requeues instead of blocking a reconcile
// worker: a drain outlives many reconcile cycles. If deletePVCs is true the PVCs are deleted
// first, while the replica count still describes which ones exist; otherwise they are preserved.
// clusterName names the owning cluster in the emitted Deleted event.
func (c *RoleGroupCleaner) deleteStatefulSet(
	ctx context.Context,
	namespace, name string,
	ownerUID types.UID,
	deletePVCs bool,
	clusterName string,
) (deletionState, error) {
	logger := log.FromContext(ctx)
	sts := &appsv1.StatefulSet{}
	key := types.NamespacedName{Namespace: namespace, Name: name}

	if err := c.Client.Get(ctx, key, sts); err != nil {
		if errors.IsNotFound(err) {
			return deletionSettled, nil
		}
		return deletionInFlight, c.apiError(err)
	}

	if !isOwnedByCluster(sts, ownerUID) {
		logger.Info("Skipping StatefulSet deletion: not owned by this cluster", "name", name)
		return deletionSettled, nil
	}

	// Delete PVCs BEFORE scaling to 0 (replica count is still valid at this point)
	if deletePVCs {
		if err := c.deletePVCsForStatefulSet(ctx, sts); err != nil {
			return deletionInFlight, err
		}
	}

	// A nil replica count means the API server default of 1, so it is a scale-down like any other.
	if sts.Spec.Replicas == nil || *sts.Spec.Replicas > 0 {
		if err := c.scaleToZero(ctx, key); err != nil {
			return deletionInFlight, err
		}
		logger.Info("Scaled orphaned StatefulSet to zero, waiting for the drain", "name", name)
		return deletionInFlight, nil
	}

	// The scale-down is only a request: the pods leave one by one, honouring their
	// terminationGracePeriodSeconds. Deleting the StatefulSet before that finishes cancels the
	// ordered shutdown it was scaled down for.
	if sts.Status.Replicas > 0 {
		logger.V(1).Info("Waiting for the orphaned StatefulSet to finish draining",
			"name", name, "remainingReplicas", sts.Status.Replicas)
		return deletionInFlight, nil
	}

	if err := c.Client.Delete(ctx, sts); err != nil {
		if errors.IsNotFound(err) {
			return deletionSettled, nil
		}
		return deletionInFlight, c.apiError(err)
	}
	c.emitDeleted(clusterName, sts)

	return confirmDeleted[appsv1.StatefulSet](ctx, c, key)
}

// scaleToZero sets .spec.replicas to 0 on a live StatefulSet, re-reading it on conflict. The same
// object is written by the apply path and by any autoscaler pointed at it, so a routine 409 must
// not turn into a failed cleanup pass that leaves the role group half-deleted.
func (c *RoleGroupCleaner) scaleToZero(ctx context.Context, key types.NamespacedName) error {
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		live := &appsv1.StatefulSet{}
		if err := c.Client.Get(ctx, key, live); err != nil {
			return err
		}
		if live.Spec.Replicas != nil && *live.Spec.Replicas == 0 {
			return nil
		}
		live.Spec.Replicas = ptr.To(int32(0))
		return c.Client.Update(ctx, live)
	})
	// The StatefulSet disappeared while it was being scaled down: there is nothing left to drain.
	if errors.IsNotFound(err) {
		return nil
	}
	return c.apiError(err)
}

// deletePVCsForStatefulSet deletes PVCs associated with a StatefulSet by listing existing PVCs
// using the StatefulSet's pod selector labels. This is more reliable than deriving names from
// replica count, as it handles scaled-down StatefulSets and catches all existing PVCs regardless
// of current replica count.
func (c *RoleGroupCleaner) deletePVCsForStatefulSet(ctx context.Context, sts *appsv1.StatefulSet) error {
	if len(sts.Spec.VolumeClaimTemplates) == 0 {
		return nil
	}

	logger := log.FromContext(ctx)
	namespace := sts.Namespace

	// List PVCs matching the StatefulSet's pod selector
	pvcList := &corev1.PersistentVolumeClaimList{}
	if err := c.Client.List(ctx, pvcList,
		client.InNamespace(namespace),
		client.MatchingLabels(sts.Spec.Selector.MatchLabels),
	); err != nil {
		return fmt.Errorf("failed to list PVCs for StatefulSet %s/%s: %w", namespace, sts.Name, err)
	}

	for i := range pvcList.Items {
		pvc := &pvcList.Items[i]
		if err := c.Client.Delete(ctx, pvc); err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete PVC %s/%s: %w", namespace, pvc.Name, err)
		}
		logger.Info("Deleted PVC", "name", pvc.Name, "namespace", namespace)
	}
	return nil
}

// deleteService deletes a Service if it exists and is owned by the cluster.
// clusterName names the owning cluster in the emitted Deleted event. Like deletePDB, it serves the
// apply path's reclaim (the metrics Service slot), where a deletion still in flight is retried by
// the next reconcile rather than gating a sequence.
func (c *RoleGroupCleaner) deleteService(ctx context.Context, namespace, name string, ownerUID types.UID, clusterName string) error {
	_, err := deleteOwned[corev1.Service](ctx, c, namespace, name, ownerUID, clusterName)
	return err
}

// clearGrayDeleteAnnotation removes the AnnotationPendingDeletion annotation from a resource
// (StatefulSet or ConfigMap) if it is present. Only modifies resources owned by ownerUID
// to avoid mutating unrelated objects on name collision.
func (c *RoleGroupCleaner) clearGrayDeleteAnnotation(ctx context.Context, namespace, name string, ownerUID types.UID) error {
	// Try StatefulSet first
	sts := &appsv1.StatefulSet{}
	if err := c.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, sts); err == nil {
		if !isOwnedByCluster(sts, ownerUID) {
			return nil
		}
		if _, ok := sts.GetAnnotations()[AnnotationPendingDeletion]; ok {
			annotations := sts.GetAnnotations()
			delete(annotations, AnnotationPendingDeletion)
			sts.SetAnnotations(annotations)
			return c.Client.Update(ctx, sts)
		}
		return nil
	} else if !errors.IsNotFound(err) {
		return err
	}

	// Try ConfigMap as fallback
	cm := &corev1.ConfigMap{}
	if err := c.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, cm); err == nil {
		if !isOwnedByCluster(cm, ownerUID) {
			return nil
		}
		if _, ok := cm.GetAnnotations()[AnnotationPendingDeletion]; ok {
			annotations := cm.GetAnnotations()
			delete(annotations, AnnotationPendingDeletion)
			cm.SetAnnotations(annotations)
			return c.Client.Update(ctx, cm)
		}
	}
	return nil
}
