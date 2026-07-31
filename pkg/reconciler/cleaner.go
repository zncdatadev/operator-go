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
	"strings"
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

	// AnnotationDrainStarted records, as an RFC3339 timestamp, when an orphaned StatefulSet was
	// first observed draining. It lives on the object rather than in memory so the drain deadline
	// survives operator restarts and leader changes.
	AnnotationDrainStarted = "orphan.zncdata.dev/drain-started"
)

// ConditionOrphanCleanupPending reports that the framework has not finished reclaiming the
// resources of role groups that left the spec. The teardown runs as a state machine across
// reconciles and can wait on a StatefulSet that refuses to drain; without this condition that
// wait is only visible in the operator log, while the CR advertises a perfectly healthy cluster.
const ConditionOrphanCleanupPending v1alpha1.ConditionType = "OrphanCleanupPending"

const (
	// ReasonOrphanCleanupPending means at least one orphaned role group is not reclaimed yet.
	ReasonOrphanCleanupPending = "OrphanCleanupPending"
	// ReasonOrphanCleanupComplete means nothing is left to reclaim.
	ReasonOrphanCleanupComplete = "OrphanCleanupComplete"
)

// LabelRolePodDisruptionBudget marks a PodDisruptionBudget as the framework's role-level slot and
// records the role it covers. Role GROUP orphans are discovered from their own resources (see
// discoverOrphans), but a role-level PDB has no role group whose ConfigMap or StatefulSet would
// lead the cleaner to it. The label is what lets it find that object without guessing from the
// derived name — a product may ship its own PDB through RoleGroupResources.PodDisruptionBudget,
// and that object carries the same controller owner reference.
const LabelRolePodDisruptionBudget = "pdb." + constant.KubedoopDomain + "/role"

// LabelRoleGroupPodDisruptionBudget marks a PodDisruptionBudget as the framework's per-role-group
// slot (RoleGroupResources.PodDisruptionBudget) and records the role group it covers. Only objects
// carrying it — or the pre-#530 framework fingerprint, see isFrameworkRoleGroupPDB — are reclaimed
// when a role group stops shipping a per-group PDB, because "<cluster>-<role>-<group>" is also a
// name a product may legitimately use for a PDB of its own.
const LabelRoleGroupPodDisruptionBudget = "pdb." + constant.KubedoopDomain + "/role-group"

// DefaultDrainPollInterval is how long the cleaner asks the caller to wait before re-entering a
// deletion it has already started — a StatefulSet scaling down, a Delete whose effect is not
// observable yet. It paces the orphan state machine, not the pod termination itself, so it is
// short: the cycle it schedules only re-reads the resources it is waiting on.
const DefaultDrainPollInterval = 5 * time.Second

// DefaultDrainTimeout bounds the ordered drain of an orphaned StatefulSet. The drain is a
// courtesy — it lets a stateful product shut down the way its own rolling update would — but it
// gates every following step, so a pod that can never terminate (a stuck finalizer, an
// unreachable node) would otherwise strand the role group's ConfigMap, Services and status entry
// forever. Once this elapses the StatefulSet is deleted anyway and the remaining pods are left to
// cascade garbage collection, which is what deleting it outright always did.
const DefaultDrainTimeout = 10 * time.Minute

// defaultCleanupRateLimitRetryAfter mirrors GenericReconcilerConfig.RateLimitRetryAfter for a
// cleaner a product builds directly.
const defaultCleanupRateLimitRetryAfter = 10 * time.Second

// RoleGroupCleaner cleans orphaned role group resources.
type RoleGroupCleaner struct {
	Client                client.Client
	Scheme                *runtime.Scheme
	apiReader             client.Reader
	grayDeleteGracePeriod time.Duration
	drainPollInterval     time.Duration
	drainTimeout          time.Duration
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

// WithDrainTimeout bounds how long an orphaned StatefulSet's ordered drain may block the rest of
// its role group's teardown; once it elapses the StatefulSet is deleted with pods still
// terminating. A non-positive value keeps DefaultDrainTimeout — the wait is always bounded,
// because everything that follows it (the ConfigMap, the Services, the status entry) is blocked
// until it ends.
func (c *RoleGroupCleaner) WithDrainTimeout(d time.Duration) *RoleGroupCleaner {
	c.drainTimeout = d
	return c
}

// WithAPIReader wires the uncached reader used to confirm that a role group's resources are
// really gone before its entry is pruned from the status snapshot. That verdict is terminal —
// the snapshot is the orphan detector's only record of the group — and a cached read can answer
// NotFound for an object that exists. Optional; without it the confirmation falls back to the
// regular (cached) client, which is the behavior a product building a cleaner by hand gets.
func (c *RoleGroupCleaner) WithAPIReader(reader client.Reader) *RoleGroupCleaner {
	c.apiReader = reader
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

// drainDeadline is how long an orphaned StatefulSet may keep the rest of its role group waiting.
func (c *RoleGroupCleaner) drainDeadline() time.Duration {
	if c.drainTimeout > 0 {
		return c.drainTimeout
	}
	return DefaultDrainTimeout
}

// confirmReader is the reader used for the terminal "nothing of this role group is left" check.
// It is the uncached APIReader when one is wired, falling back to the regular client.
func (c *RoleGroupCleaner) confirmReader() client.Reader {
	if c.apiReader != nil {
		return c.apiReader
	}
	return c.Client
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
// Orphans are discovered from the live cluster and from status.roleGroups (see discoverOrphans),
// so a lost status entry no longer hides a role group's resources from this cleaner. A role group
// is removed from status.RoleGroups only once its resources were really deleted, so a deferred
// (gray-delete) or failed pass is retried on the next reconcile instead of silently dropping the
// group from the status snapshot. A group that fails does not abort the pass for the others: its
// error is collected and the remaining groups still make progress, so one wedged role group cannot
// keep every other orphan alive forever.
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

	// Reset the teardown state machine's progress on every role group that IS in the spec, so a
	// role group which was orphaned and then re-added starts its next teardown from scratch.
	//
	// Deliberately NOT gated on grayDeleteGracePeriod: AnnotationDrainStarted is stamped whether or
	// not gray-delete is configured, so gating the reset on it — as this loop used to be — meant the
	// drain deadline was never reset in the default configuration. Clearing a mark that was never
	// written is a no-op, so running unconditionally costs a cached read per role group.
	for roleName, roleSpec := range spec.Roles {
		for groupName := range roleSpec.RoleGroups {
			resourceName := RoleGroupResourceName(clusterName, roleName, groupName)
			if err := c.clearTeardownProgress(ctx, namespace, resourceName, ownerUID); err != nil {
				logger.V(1).Info("Failed to reset teardown progress on an active resource",
					"resource", resourceName, "error", err)
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

	orphans, err := c.discoverOrphans(ctx, namespace, clusterName, spec, status, ownerUID)
	if err != nil {
		if IsRateLimitError(err) {
			return nextRequeue, err
		}
		// Deliberately NOT falling through to "no orphans": with no reliable inventory this pass
		// cannot honestly clear ConditionOrphanCleanupPending, and reporting a converged cluster
		// on the strength of a failed list is how an orphan stops being looked for.
		errs = append(errs, err)
		return earliestRequeue(nextRequeue, c.pollInterval()), stderrors.Join(errs...)
	}

	if len(orphans) == 0 {
		setOrphanCleanupCondition(status, nil)
		return nextRequeue, stderrors.Join(errs...)
	}

	liveResourceNames := make(map[string]struct{})
	for roleName, roleSpec := range spec.Roles {
		for groupName := range roleSpec.RoleGroups {
			liveResourceNames[RoleGroupResourceName(clusterName, roleName, groupName)] = struct{}{}
		}
	}

	logger.Info("Cleaning up orphaned role groups", "count", len(orphans))

	// Role groups whose teardown has not finished this pass. They are reported on the CR so a
	// deletion that cannot make progress is visible where an operator looks first.
	var pending []string

	// discoverOrphans returns them sorted by resource name, so the sequence of events across the
	// several reconciles a teardown spans is reproducible.
	for _, orphan := range orphans {
		deleted, retryAfter, err := c.cleanupRoleGroup(ctx, namespace, orphan.resourceName, ownerUID, deletePVCs, clusterName, liveResourceNames)
		if err != nil {
			// A 429 is the API server throttling this operator as a whole; the remaining
			// groups would only add to the requests it is rejecting.
			if IsRateLimitError(err) {
				return nextRequeue, err
			}
			// Every other failure is confined to its own role group: aborting the whole pass
			// here would let one wedged group keep every other orphan alive indefinitely.
			errs = append(errs, fmt.Errorf("failed to cleanup role group %s: %w", orphan.describe(), err))
			nextRequeue = earliestRequeue(nextRequeue, c.pollInterval())
			pending = append(pending, orphan.describe())
			continue
		}
		if !deleted {
			nextRequeue = earliestRequeue(nextRequeue, retryAfter)
			pending = append(pending, orphan.describe())
			continue
		}

		// Prune the status snapshot only for a role group whose resources were really deleted, so
		// Status.RoleGroups converges to the desired set (docs/architecture.md §4.4.2 step 5)
		// without dropping groups whose deletion is still pending. A live orphan found without
		// role-group labels has no ledger entry to prune — that is precisely the case the ledger
		// had already lost.
		if orphan.roleName != "" {
			status.RemoveRoleGroup(orphan.roleName, orphan.groupName)
		}
		logger.Info("Cleaned up orphaned role group",
			"role", orphan.roleName, "group", orphan.groupName, "resource", orphan.resourceName)
	}

	setOrphanCleanupCondition(status, pending)

	return nextRequeue, stderrors.Join(errs...)
}

// setOrphanCleanupCondition reports the teardown still in flight on the CR. The message lists the
// role groups in sorted order, so a teardown that stays stuck writes an identical status on every
// pass: the reconciler's no-op guard then skips the write instead of scheduling itself again.
func setOrphanCleanupCondition(status *v1alpha1.GenericClusterStatus, pending []string) {
	if len(pending) == 0 {
		// Only clear a condition that was actually raised. Stamping a permanently-False condition
		// on every cluster that never had an orphan is noise on every product's CR.
		if status.GetCondition(ConditionOrphanCleanupPending) == nil {
			return
		}
		status.SetCondition(metav1.Condition{
			Type:    string(ConditionOrphanCleanupPending),
			Status:  metav1.ConditionFalse,
			Reason:  ReasonOrphanCleanupComplete,
			Message: "No orphaned role group is waiting to be reclaimed",
		})
		return
	}

	slices.Sort(pending)
	status.SetCondition(metav1.Condition{
		Type:    string(ConditionOrphanCleanupPending),
		Status:  metav1.ConditionTrue,
		Reason:  ReasonOrphanCleanupPending,
		Message: fmt.Sprintf("Waiting to reclaim orphaned role groups: %s", strings.Join(pending, ", ")),
	})
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

// orphanRef identifies one orphaned role group's resource slot.
//
// resourceName is the base name every resource of the group derives from, and is the only field
// the teardown itself needs. roleName/groupName are the ledger coordinates: known when the orphan
// came from status.roleGroups or when the live objects carry the role-group labels, empty for a
// live object created by a framework version that predates them — in which case there is no
// status entry to prune either.
type orphanRef struct {
	resourceName string
	roleName     string
	groupName    string
}

// describe names the orphan the way an operator would look for it: by role group when that is
// known, else by the resource name, which is all the live objects reveal.
func (r orphanRef) describe() string {
	if r.roleName == "" {
		return r.resourceName
	}
	return r.roleName + "/" + r.groupName
}

// discoverOrphans returns the role group slots this cluster owns that its spec no longer declares,
// unioning two sources: the LIVE cluster, and status.roleGroups.
//
// The ledger used to be the only source, and it is a record the operator must first have
// successfully written. Anything that loses it makes the resources it named invisible to this
// cleaner *forever* — nothing else ever enumerates them, so they hold their PVCs, their PDB and
// their pods until someone deletes them by hand. It is lost when the process dies between
// applying a role group's resources and writing the CR status, when a backup tool restores the CR
// without its status subresource, and whenever a `kubectl replace` drops it.
//
// Reading the live cluster removes that dependency for every resource created by a framework
// version that stamps app.kubernetes.io/role-group. The ledger stays in the union to cover the
// ones created before it, whose (role, role group) cannot be recovered from their labels — and
// because it costs nothing: a union can only widen what gets reclaimed.
//
// A live object is claimed only when it is unambiguously the framework's own role group slot: the
// full instance/managed-by/component/role-group label set, a controller owner reference to this
// CR, AND a name equal to what RoleGroupResourceName would produce for those coordinates.
// Ownership and labels alone are not enough — a discovery ConfigMap carries the same
// instance/managed-by pair, and a product's RoleGroupResources.ExtraResources may carry the
// handler's entire label set.
func (c *RoleGroupCleaner) discoverOrphans(
	ctx context.Context,
	namespace, clusterName string,
	spec *v1alpha1.GenericClusterSpec,
	status *v1alpha1.GenericClusterStatus,
	ownerUID types.UID,
) ([]orphanRef, error) {
	refs := make(map[string]orphanRef)

	// The ledger goes in first: it carries role/group coordinates, which a pre-labels live object
	// cannot supply. Both sources derive the name identically, so they can never disagree.
	for roleName, groups := range status.GetOrphanedRoleGroups(spec.Roles) {
		for _, groupName := range groups {
			name := RoleGroupResourceName(clusterName, roleName, groupName)
			refs[name] = orphanRef{resourceName: name, roleName: roleName, groupName: groupName}
		}
	}

	live, err := c.discoverLiveOrphans(ctx, namespace, clusterName, spec, ownerUID)
	if err != nil {
		return nil, err
	}
	for _, ref := range live {
		if _, alreadyKnown := refs[ref.resourceName]; !alreadyKnown {
			refs[ref.resourceName] = ref
		}
	}

	orphans := slices.Collect(maps.Values(refs))
	// Sorted, not map order: the deletion pass is spread over several reconciles, so a stable
	// order makes the sequence of events (and the logs) reproducible, and keeps the pending
	// condition's message byte-stable across cycles.
	slices.SortFunc(orphans, func(a, b orphanRef) int { return strings.Compare(a.resourceName, b.resourceName) })
	return orphans, nil
}

// discoverLiveOrphans lists the role group ConfigMaps and StatefulSets this cluster owns and
// returns the slots the spec no longer declares.
//
// Both kinds are listed because the teardown deletes the StatefulSet before the ConfigMap: a pass
// interrupted in between leaves a ConfigMap whose StatefulSet is already gone, and looking only at
// StatefulSets would never see it again. Services are derived by suffix from the same base name,
// so they add no slot these two do not already cover.
func (c *RoleGroupCleaner) discoverLiveOrphans(
	ctx context.Context,
	namespace, clusterName string,
	spec *v1alpha1.GenericClusterSpec,
	ownerUID types.UID,
) ([]orphanRef, error) {
	// Without an owner to match, every labelled object in the namespace — including a sibling
	// cluster's, which shares this cluster's name only by accident but not its UID — would look
	// like this cluster's. Same rule as the role PDB reclaim.
	if ownerUID == "" {
		return nil, nil
	}

	listOpts := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels{
			constant.LabelKubernetesInstance:  clusterName,
			constant.LabelKubernetesManagedBy: managedByValue,
		},
	}

	stsList := &appsv1.StatefulSetList{}
	if err := c.Client.List(ctx, stsList, listOpts...); err != nil {
		return nil, c.apiError(fmt.Errorf("failed to list StatefulSets for orphan discovery: %w", err))
	}
	cmList := &corev1.ConfigMapList{}
	if err := c.Client.List(ctx, cmList, listOpts...); err != nil {
		return nil, c.apiError(fmt.Errorf("failed to list ConfigMaps for orphan discovery: %w", err))
	}

	candidates := make([]metav1.Object, 0, len(stsList.Items)+len(cmList.Items))
	for i := range stsList.Items {
		candidates = append(candidates, &stsList.Items[i])
	}
	for i := range cmList.Items {
		candidates = append(candidates, &cmList.Items[i])
	}

	var orphans []orphanRef
	seen := make(map[string]struct{})
	for _, obj := range candidates {
		ref, isSlot := roleGroupSlotOf(obj, clusterName, ownerUID)
		if !isSlot {
			continue
		}
		if roleSpec, declared := spec.Roles[ref.roleName]; declared {
			if _, stillLive := roleSpec.GetRoleGroups()[ref.groupName]; stillLive {
				continue
			}
		}
		if _, duplicate := seen[ref.resourceName]; duplicate {
			continue
		}
		seen[ref.resourceName] = struct{}{}
		orphans = append(orphans, ref)
	}
	return orphans, nil
}

// roleGroupSlotOf reports which role group slot a live object occupies, and whether it occupies
// one at all. Every condition is load-bearing — see discoverOrphans.
func roleGroupSlotOf(obj metav1.Object, clusterName string, ownerUID types.UID) (orphanRef, bool) {
	// Defence in depth rather than the sole guard: every deletion step re-checks ownership on its
	// own, and checkOrMarkGrayDelete refuses to annotate a foreign primary. Filtering here keeps
	// the *inventory* honest — a foreign object is not this cluster's orphan — and saves the six
	// per-resource reads a full teardown pass would spend discovering that on every reconcile.
	if !isOwnedByCluster(obj, ownerUID) {
		return orphanRef{}, false
	}
	labels := obj.GetLabels()
	roleName := labels[constant.LabelKubernetesComponent]
	groupName := labels[constant.LabelKubernetesRoleGroup]
	if roleName == "" || groupName == "" {
		return orphanRef{}, false
	}
	// The decisive check. A discovery ConfigMap is named "<cluster>" and carries no component or
	// role-group label; a product's extra resource may carry the full label set but is named
	// whatever the product chose. Only an object the framework itself would have named this way
	// for these coordinates is one of its slots.
	if obj.GetName() != RoleGroupResourceName(clusterName, roleName, groupName) {
		return orphanRef{}, false
	}
	return orphanRef{resourceName: obj.GetName(), roleName: roleName, groupName: groupName}, true
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
	derivedServices := make([]string, 0, 2)
	for _, suffix := range []string{"-headless", "-metrics"} {
		derived := resourceName + suffix
		if _, live := liveResourceNames[derived]; live {
			log.FromContext(ctx).V(1).Info("Skipping derived Service deletion: name belongs to a live role group",
				"name", derived)
			continue
		}
		derivedServices = append(derivedServices, derived)
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

	// Every step settled — but each of those reads went through the manager's cache, which may
	// report NotFound for an object that exists (a stale or not-yet-synced informer). The caller
	// prunes the role group from Status.RoleGroups on the strength of this verdict, and that
	// snapshot is the ONLY record the orphan detector has: a wrong "nothing left" answer leaves
	// live resources that nothing will ever look at again. Confirm it against the API server.
	state, err := c.confirmRoleGroupReclaimed(ctx, namespace, resourceName, ownerUID, derivedServices)
	if err != nil {
		return false, 0, err
	}
	if state == deletionInFlight {
		return false, c.pollInterval(), nil
	}

	return true, 0, nil
}

// confirmRoleGroupReclaimed re-reads every resource of a role group through the uncached reader
// (see WithAPIReader). It runs once, at the end of a teardown whose every step already settled —
// not on every poll — so the extra direct API reads are paid once per reclaimed role group.
// A resource that belongs to another cluster does not count: this cluster will never delete it,
// and waiting for it would keep the role group in the status snapshot forever.
func (c *RoleGroupCleaner) confirmRoleGroupReclaimed(
	ctx context.Context,
	namespace, resourceName string,
	ownerUID types.UID,
	derivedServices []string,
) (deletionState, error) {
	key := types.NamespacedName{Namespace: namespace, Name: resourceName}

	checks := make([]func() (bool, error), 0, 4+len(derivedServices))
	checks = append(checks,
		func() (bool, error) { return stillOwned[policyv1.PodDisruptionBudget](ctx, c, key, ownerUID) },
		func() (bool, error) { return stillOwned[appsv1.StatefulSet](ctx, c, key, ownerUID) },
		func() (bool, error) { return stillOwned[corev1.ConfigMap](ctx, c, key, ownerUID) },
		func() (bool, error) { return stillOwned[corev1.Service](ctx, c, key, ownerUID) },
	)
	for _, derived := range derivedServices {
		derivedKey := types.NamespacedName{Namespace: namespace, Name: derived}
		checks = append(checks, func() (bool, error) {
			return stillOwned[corev1.Service](ctx, c, derivedKey, ownerUID)
		})
	}

	for _, check := range checks {
		present, err := check()
		if err != nil {
			return deletionInFlight, err
		}
		if present {
			log.FromContext(ctx).Info(
				"Cached reads reported the role group gone but the API server still has resources; resuming the teardown",
				"name", resourceName)
			return deletionInFlight, nil
		}
	}
	return deletionSettled, nil
}

// stillOwned reports whether a resource this cluster owns is still present, read through the
// uncached confirmation reader.
func stillOwned[T any, PT ptrObject[T]](ctx context.Context, c *RoleGroupCleaner, key types.NamespacedName, ownerUID types.UID) (bool, error) {
	obj := PT(new(T))
	if err := c.confirmReader().Get(ctx, key, obj); err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, c.apiError(err)
	}
	return isOwnedByCluster(obj, ownerUID), nil
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

// checkOrMarkGrayDelete checks whether the grace period for a gray-deleted role group has elapsed.
//
// The mark is written to EVERY primary resource that exists (the StatefulSet and the ConfigMap),
// carrying the same timestamp, so the grace gate is evaluated ONCE per teardown. Annotating only
// the preferred primary would restart the clock as the state machine progresses: the StatefulSet
// is deleted first, the next pass finds a never-annotated ConfigMap, stamps a fresh timestamp, and
// the remaining resources wait another whole grace period.
//
// Only resources owned by ownerUID are annotated; a foreign primary carries no grace period at all
// (see below). Returns true if the resources should be deleted now, false if still within the
// grace period; in the latter case remaining is the time left, so the caller can requeue precisely
// instead of waiting for an unrelated event.
func (c *RoleGroupCleaner) checkOrMarkGrayDelete(ctx context.Context, namespace, name string, ownerUID types.UID) (ready bool, remaining time.Duration, err error) {
	logger := log.FromContext(ctx)
	key := types.NamespacedName{Namespace: namespace, Name: name}

	var primaries []client.Object
	sts := &appsv1.StatefulSet{}
	switch err := c.Client.Get(ctx, key, sts); {
	case err == nil:
		primaries = append(primaries, sts)
	case !errors.IsNotFound(err):
		return false, 0, c.apiError(err)
	}
	cm := &corev1.ConfigMap{}
	switch err := c.Client.Get(ctx, key, cm); {
	case err == nil:
		primaries = append(primaries, cm)
	case !errors.IsNotFound(err):
		return false, 0, c.apiError(err)
	}

	// Resources already gone — allow deletion pass-through.
	if len(primaries) == 0 {
		return true, 0, nil
	}

	// A foreign primary must not be annotated (that would mutate an unrelated object on a name
	// collision), which also means there is no timestamp to run a grace period from. Let the pass
	// proceed: every deletion is ownership-checked on its own, so the foreign objects are skipped
	// and whatever this cluster does own under that name is reclaimed. Deferring instead would
	// leave the role group in Status.RoleGroups for as long as the foreign object exists.
	for _, primary := range primaries {
		if !isOwnedByCluster(primary, ownerUID) {
			logger.V(1).Info("Gray-delete grace period skipped: primary resource not owned by this cluster", "name", name)
			return true, 0, nil
		}
	}

	// The mark of whichever primary already carries one is authoritative: it is when this teardown
	// began. An unparsable timestamp is treated as no mark and re-stamped below, rather than
	// short-circuiting the grace period the operator asked for.
	markedTime, marked := grayDeleteMark(primaries)
	if !marked {
		markedTime = time.Now().UTC()
	}

	for _, primary := range primaries {
		annotations := primary.GetAnnotations()
		if annotations[AnnotationPendingDeletion] == markedTime.Format(time.RFC3339) {
			continue
		}
		if annotations == nil {
			annotations = make(map[string]string)
		}
		annotations[AnnotationPendingDeletion] = markedTime.Format(time.RFC3339)
		primary.SetAnnotations(annotations)
		if err := c.Client.Update(ctx, primary); err != nil {
			return false, 0, fmt.Errorf("failed to mark resource for gray deletion: %w", c.apiError(err))
		}
	}

	if !marked {
		logger.Info("Marked orphaned resource for gray deletion", "name", name, "gracePeriod", c.grayDeleteGracePeriod)
		return false, c.grayDeleteGracePeriod, nil
	}

	if time.Since(markedTime) >= c.grayDeleteGracePeriod {
		logger.Info("Gray deletion grace period elapsed, proceeding with deletion", "name", name)
		return true, 0, nil
	}

	logger.Info("Gray deletion grace period not yet elapsed", "name", name,
		"markedAt", markedTime.Format(time.RFC3339), "gracePeriod", c.grayDeleteGracePeriod)
	// The annotation timestamp has second granularity, so round up to the next whole second to
	// avoid requeuing a hair too early and burning a no-op reconcile.
	return false, time.Until(markedTime.Add(c.grayDeleteGracePeriod)).Round(time.Second) + time.Second, nil
}

// grayDeleteMark returns the timestamp this teardown was first marked with, taken from the first
// primary carrying a parsable one.
func grayDeleteMark(primaries []client.Object) (time.Time, bool) {
	for _, primary := range primaries {
		markedAt, exists := primary.GetAnnotations()[AnnotationPendingDeletion]
		if !exists {
			continue
		}
		if marked, err := time.Parse(time.RFC3339, markedAt); err == nil {
			return marked, true
		}
	}
	return time.Time{}, false
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
// worker: a drain outlives many reconcile cycles.
//
// When deletePVCs is true the PVCs are deleted at the END of that sequence — after the drain,
// immediately before the StatefulSet — and not at the start. See the comment at that call site for
// why the irreversible step goes last, why it nevertheless precedes the StatefulSet, and why the
// drain-timeout path falls through to it. Without the annotation they are preserved.
//
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
	// ordered shutdown it was scaled down for — but the wait gates every following step, so it is
	// bounded: a pod that can never terminate must not strand the ConfigMap, the Services and the
	// status entry of this role group indefinitely.
	if sts.Status.Replicas > 0 {
		expired, err := c.drainDeadlineExpired(ctx, sts, key)
		if err != nil {
			return deletionInFlight, err
		}
		if !expired {
			logger.V(1).Info("Waiting for the orphaned StatefulSet to finish draining",
				"name", name, "remainingReplicas", sts.Status.Replicas)
			return deletionInFlight, nil
		}
		logger.Info("Orphaned StatefulSet drain timed out; deleting it with pods still terminating",
			"name", name, "remainingReplicas", sts.Status.Replicas, "drainTimeout", c.drainDeadline())
	}

	// Delete the PVCs here — after the drain, immediately before the StatefulSet — and not earlier.
	//
	// This used to run first, before the scale-to-zero, justified by a comment claiming "replica
	// count is still valid at this point". That was never true: deletePVCsForStatefulSet has listed
	// by the pod selector since it was written, precisely so it does not depend on the replica
	// count. So the most destructive and least reversible step in the whole teardown was being
	// issued first, for a reason the code never had.
	//
	// It matters because deleting a role group is undoable right up until the data goes. A user who
	// removes a role group by mistake and re-adds it a minute later — a `git revert` of a bad CR
	// edit — used to find the volumes already reclaimed while the StatefulSet was still there
	// draining. Running after the drain means the pods are provably gone before anything
	// irreversible happens, and re-adding the group during the drain costs a restart rather than
	// the data.
	//
	// PVCs before the StatefulSet, not after: if the process dies between the two, the next pass
	// still finds the StatefulSet, re-enters here and finishes the job. The other order would leave
	// PVCs nothing ever looks at again — the cleaner finds them through the StatefulSet's selector,
	// so once it is gone they are unreachable.
	//
	// The drain-timeout path above falls through to here deliberately: the user asked for the PVCs
	// to go, and skipping them because a pod would not terminate would leak the volumes silently.
	if deletePVCs {
		if err := c.deletePVCsForStatefulSet(ctx, sts); err != nil {
			return deletionInFlight, err
		}
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

// drainDeadlineExpired reports whether the ordered drain of an orphaned StatefulSet has been
// waiting longer than the configured timeout. The start of the drain is recorded on the object
// itself (AnnotationDrainStarted) rather than in memory, so the deadline survives an operator
// restart or a leader change — with in-memory state, a controller that restarts every few minutes
// would reset the clock and the wait would be unbounded again.
func (c *RoleGroupCleaner) drainDeadlineExpired(ctx context.Context, sts *appsv1.StatefulSet, key types.NamespacedName) (bool, error) {
	if startedAt, ok := sts.GetAnnotations()[AnnotationDrainStarted]; ok {
		if started, err := time.Parse(time.RFC3339, startedAt); err == nil {
			return time.Since(started) >= c.drainDeadline(), nil
		}
		// An unparsable timestamp is re-stamped below: it carries no deadline, and treating it as
		// expired would delete a StatefulSet that may have started draining seconds ago.
	}

	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		live := &appsv1.StatefulSet{}
		if err := c.Client.Get(ctx, key, live); err != nil {
			return err
		}
		annotations := live.GetAnnotations()
		if annotations == nil {
			annotations = make(map[string]string)
		}
		annotations[AnnotationDrainStarted] = time.Now().UTC().Format(time.RFC3339)
		live.SetAnnotations(annotations)
		return c.Client.Update(ctx, live)
	})
	// The StatefulSet disappeared while the drain was being timestamped: the next step re-reads it.
	if errors.IsNotFound(err) {
		return false, nil
	}
	return false, c.apiError(err)
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
		return fmt.Errorf("failed to list PVCs for StatefulSet %s/%s: %w", namespace, sts.Name, c.apiError(err))
	}

	for i := range pvcList.Items {
		pvc := &pvcList.Items[i]
		if err := c.Client.Delete(ctx, pvc); err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete PVC %s/%s: %w", namespace, pvc.Name, c.apiError(err))
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

// clearTeardownProgress removes the cleaner's per-teardown progress annotations from a role
// group's primary resources, so a role group that comes BACK into the spec starts its next
// teardown from scratch.
//
// It must visit BOTH primaries and BOTH annotations. The previous version cleared only
// AnnotationPendingDeletion, and only on whichever primary it found first — returning as soon as
// the StatefulSet was handled. Both halves of that were bugs, and both failed toward FASTER
// destruction:
//
//   - checkOrMarkGrayDelete deliberately stamps the mark on every primary that exists, and treats
//     "the mark of whichever primary already carries one" as authoritative. Clearing only the
//     StatefulSet therefore left the ConfigMap holding the timestamp of the FIRST teardown. Remove
//     a role group, re-add it, remove it again, and the grace period the operator asked for was
//     evaluated against the original timestamp — usually already elapsed, so the resources were
//     deleted immediately with no grace window at all.
//
//   - AnnotationDrainStarted was never cleared anywhere in the tree. It survives re-apply because
//     copyDesiredState MERGES annotations (the framework's desired StatefulSet does not carry the
//     key, so the live value wins), so the same re-add/re-remove sequence made drainDeadlineExpired
//     read a timestamp from the previous teardown and force-delete the StatefulSet without waiting
//     for the ordered drain.
//
// Only resources owned by ownerUID are modified, so a name collision with a foreign object cannot
// make the cleaner mutate it. Per-primary failures are joined rather than returned on the first,
// so one unwritable object does not leave the other still carrying stale progress.
func (c *RoleGroupCleaner) clearTeardownProgress(ctx context.Context, namespace, name string, ownerUID types.UID) error {
	key := types.NamespacedName{Namespace: namespace, Name: name}
	primaries := []client.Object{&appsv1.StatefulSet{}, &corev1.ConfigMap{}}

	var errs []error
	for _, primary := range primaries {
		switch err := c.Client.Get(ctx, key, primary); {
		case errors.IsNotFound(err):
			continue
		case err != nil:
			errs = append(errs, err)
			continue
		}
		if !isOwnedByCluster(primary, ownerUID) {
			continue
		}

		annotations := primary.GetAnnotations()
		changed := false
		for _, a := range []string{AnnotationPendingDeletion, AnnotationDrainStarted} {
			if _, ok := annotations[a]; ok {
				delete(annotations, a)
				changed = true
			}
		}
		if !changed {
			continue
		}
		primary.SetAnnotations(annotations)
		if err := c.Client.Update(ctx, primary); err != nil {
			errs = append(errs, err)
		}
	}

	return stderrors.Join(errs...)
}
