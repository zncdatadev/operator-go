package controller

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/zncdatadev/operator-go/pkg/framework"

	appsv1 "k8s.io/api/apps/v1"
)

// stopWorkloads operates on authenticated live slots even when product input
// cannot build. Declared identities add absent workloads to Pod observation;
// their replica counts remain unchanged for topology, status and role budgets.
// Unlike retirement, this path never creates or deletes workload resources.
func (r *Reconciler[CR, C, S, F]) stopWorkloads(ctx context.Context, cr CR,
	identities []framework.GroupIdentity,
) (bool, []error) {
	if cr.GetUID() == "" {
		return false, []error{fmt.Errorf("stopping workloads requires an observed CR UID")}
	}
	if err := r.currentInput(ctx, cr); err != nil {
		return false, []error{err}
	}
	// A surviving authenticated ConfigMap or Service still identifies a group
	// whose StatefulSet is absent but whose Pods may not have disappeared.
	groups, failures := r.retirementInventory(ctx, cr)
	if errors.Is(errors.Join(failures...), errSuperseded) {
		return false, failures
	}
	for _, identity := range identities {
		slot := groupSlot{Role: identity.Role, Group: identity.Name, Slot: slotStatefulset}
		groups[slot.key()] = slot
	}
	keys := make([]string, 0, len(groups))
	for key, group := range groups {
		// Inventory records whichever fixed slot survived; stopping always reads
		// the StatefulSet slot and then observes this group's actual Pods.
		group.Slot = slotStatefulset
		groups[key] = group
		keys = append(keys, key)
	}
	priorities, err := r.shutdownPriorities(ctx, cr, groups)
	if err != nil {
		return false, append(failures, err)
	}
	slices.SortFunc(keys, func(a, b string) int {
		if priorities[a] < priorities[b] {
			return -1
		}
		if priorities[a] > priorities[b] {
			return 1
		}
		return strings.Compare(a, b)
	})
	pending := false
	var blockedPriority *int32
	for _, key := range keys {
		if blockedPriority != nil && priorities[key] > *blockedPriority {
			pending = true
			continue
		}
		waiting, err := r.stopWorkload(ctx, cr, groups[key])
		pending = pending || waiting
		if waiting || err != nil {
			priority := priorities[key]
			blockedPriority = &priority
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("stop %s: %w", key, err))
		}
		if errors.Is(err, errSuperseded) {
			return pending, failures
		}
	}
	return pending, failures
}

func (r *Reconciler[CR, C, S, F]) stopWorkload(ctx context.Context, cr CR, group groupSlot) (bool, error) {
	object, err := r.readSlot(ctx, cr, group)
	if err != nil {
		return false, err
	}
	if object != nil {
		set := object.(*appsv1.StatefulSet)
		if err := checkRetiringStorage(ctx, r.Client, cr, group, set); err != nil {
			if errors.Is(err, errStoragePending) {
				return true, nil
			}
			return false, err
		}
		if replicas(set) > 0 || set.Status.ObservedGeneration < set.Generation ||
			set.Status.Replicas != 0 || set.Status.ReadyReplicas != 0 || set.Status.UpdatedReplicas != 0 {
			if err := r.observeCoordination(ctx, cr, set, 0, false); err != nil {
				return true, err
			}
		}
		if !set.DeletionTimestamp.IsZero() {
			return true, nil
		}
		if set.Spec.Replicas == nil || *set.Spec.Replicas != 0 {
			_, err := r.stopRetainedGroup(ctx, cr, group, set)
			return true, err
		}
		if set.Status.ObservedGeneration < set.Generation || set.Status.Replicas != 0 ||
			set.Status.ReadyReplicas != 0 || set.Status.UpdatedReplicas != 0 {
			return true, nil
		}
	}
	remain, err := r.groupPodsRemain(ctx, cr, group)
	if err != nil || remain {
		if err == nil && object != nil {
			err = r.observeCoordination(ctx, cr, object.(*appsv1.StatefulSet), 0, false)
		}
		return true, err
	}
	// A replacement or scale change during Pod observation needs a new pass.
	// This is an observed stop condition, not a lock against external writers.
	fresh, err := r.readSlot(ctx, cr, group)
	if err != nil {
		return false, err
	}
	if (object == nil) != (fresh == nil) || (object != nil && fresh != nil &&
		(object.GetUID() != fresh.GetUID() || object.GetResourceVersion() != fresh.GetResourceVersion())) {
		return true, nil
	}
	if fresh != nil {
		if err := r.observeCoordination(ctx, cr, fresh.(*appsv1.StatefulSet), 0, true); err != nil {
			return false, err
		}
	}
	return false, nil
}
