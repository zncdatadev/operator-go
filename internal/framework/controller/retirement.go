package controller

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/zncdatadev/operator-go/pkg/framework"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	kubernetesjson "sigs.k8s.io/json"
)

// GroupSlotAnnotation is issued by the controller for its four fixed group
// slots. It is execution metadata, never a product merge-policy declaration.
const GroupSlotAnnotation = "framework.kubedoop.dev/group-slot"
const retirementPoll = 2 * time.Second

const (
	slotConfigmap   = "configmap"
	slotHeadless    = "headless"
	slotService     = "service"
	slotStatefulset = "statefulset"
)

type groupSlot struct {
	Role  string `json:"role"`
	Group string `json:"group"`
	Slot  string `json:"slot"`
}

func (s groupSlot) key() string { return s.Role + "/" + s.Group }
func (s groupSlot) base(owner client.Object) string {
	return owner.GetName() + "-" + s.Role + "-" + s.Group
}
func (s groupSlot) object(owner client.Object) client.Object {
	name := s.base(owner)
	metadata := metav1.ObjectMeta{Name: name, Namespace: owner.GetNamespace()}
	switch s.Slot {
	case slotStatefulset:
		return &appsv1.StatefulSet{ObjectMeta: metadata}
	case slotConfigmap:
		return &corev1.ConfigMap{ObjectMeta: metadata}
	case slotHeadless:
		metadata.Name += "-headless"
	}
	return &corev1.Service{ObjectMeta: metadata}
}

func decodeSlot(object client.Object) (*groupSlot, error) {
	raw, present := object.GetAnnotations()[GroupSlotAnnotation]
	if !present {
		return nil, nil
	}
	var slot groupSlot
	strict, err := kubernetesjson.UnmarshalStrict([]byte(raw), &slot)
	if err != nil || len(strict) != 0 || len(validation.IsDNS1123Label(slot.Role)) != 0 ||
		len(validation.IsDNS1123Label(slot.Group)) != 0 ||
		!slices.Contains([]string{slotConfigmap, slotService, slotHeadless, slotStatefulset}, slot.Slot) {
		return nil, fmt.Errorf("invalid group slot receipt on %T %s", object, object.GetName())
	}
	return &slot, nil
}

func checkSlot(object client.Object, expected *groupSlot) error {
	actual, err := decodeSlot(object)
	if err != nil {
		return err
	}
	if (actual == nil) != (expected == nil) || (actual != nil && *actual != *expected) {
		return fmt.Errorf("%T %s belongs to a different group slot (actual=%+v expected=%+v)",
			object, object.GetName(), actual, expected)
	}
	return nil
}

func checkSlotObject(owner, object client.Object, kind schema.GroupVersionKind, slot groupSlot) error {
	// Terminating objects remain inventory; checkOwnership additionally rejects
	// them for apply, so use a metadata copy for the ownership portion here.
	copy := object.DeepCopyObject().(client.Object)
	copy.SetDeletionTimestamp(nil)
	if err := checkOwnership(owner, copy, kind); err != nil {
		return err
	}
	if err := checkSlot(object, &slot); err != nil {
		return err
	}
	expected := slot.object(owner)
	if fmt.Sprintf("%T", object) != fmt.Sprintf("%T", expected) ||
		client.ObjectKeyFromObject(object) != client.ObjectKeyFromObject(expected) {
		return fmt.Errorf("group slot %s/%s has a mismatched kind or resource name", slot.key(), slot.Slot)
	}
	labels := object.GetLabels()
	if labels["app.kubernetes.io/instance"] != owner.GetName() ||
		labels["app.kubernetes.io/component"] != slot.Role || labels["role-group"] != slot.Group {
		return fmt.Errorf("group slot %s/%s has damaged identity labels", slot.key(), slot.Slot)
	}
	record, err := decodeManagedMetadata(object)
	if err != nil {
		return err
	}
	if !slices.Contains(record.Object.Annotations, GroupSlotAnnotation) {
		return fmt.Errorf("group slot %s/%s is absent from the managed metadata record", slot.key(), slot.Slot)
	}
	return nil
}

func (r *Reconciler[CR, C, S, F]) currentInput(ctx context.Context, observed CR) error {
	current := r.Binding.NewObject()
	if err := r.Client.Get(ctx, client.ObjectKeyFromObject(observed), current); err != nil {
		if apierrors.IsNotFound(err) {
			return errSuperseded
		}
		return err
	}
	if current.GetUID() != observed.GetUID() || current.GetGeneration() != observed.GetGeneration() ||
		!current.GetDeletionTimestamp().IsZero() ||
		r.Binding.Operation(current) != r.Binding.Operation(observed) {
		return errSuperseded
	}
	return nil
}

// Inventory is reconstructed from live fixed slots, not status or process memory.
// Damaged controlled receipts fail visibly; labels alone never authorize deletion.
func (r *Reconciler[CR, C, S, F]) retirementInventory(ctx context.Context, cr CR) (map[string]groupSlot, []error) {
	groups := map[string]groupSlot{}
	var failures []error
	kind, err := apiutil.GVKForObject(cr, r.Scheme)
	if err != nil {
		return groups, []error{err}
	}
	lists := []client.ObjectList{&appsv1.StatefulSetList{}, &corev1.ServiceList{}, &corev1.ConfigMapList{}}
	for _, list := range lists {
		if err := r.Client.List(ctx, list, client.InNamespace(cr.GetNamespace())); err != nil {
			failures = append(failures, err)
			continue
		}
		var objects []client.Object
		switch items := list.(type) {
		case *appsv1.StatefulSetList:
			for i := range items.Items {
				objects = append(objects, &items.Items[i])
			}
		case *corev1.ServiceList:
			for i := range items.Items {
				objects = append(objects, &items.Items[i])
			}
		case *corev1.ConfigMapList:
			for i := range items.Items {
				objects = append(objects, &items.Items[i])
			}
		}
		for _, object := range objects {
			controller := metav1.GetControllerOf(object)
			if controller == nil || controller.UID != cr.GetUID() {
				continue
			}
			if shared, err := sharedInventoryObject(cr, object, kind); shared {
				if err != nil {
					failures = append(failures, err)
				}
				continue
			}
			slot, err := decodeSlot(object)
			if err == nil && slot == nil {
				record, recordErr := decodeManagedMetadata(object)
				_, isSet := object.(*appsv1.StatefulSet)
				labels := object.GetLabels()
				prefix := cr.GetName() + "-" + labels["app.kubernetes.io/component"] + "-" + labels["role-group"]
				looksLikeGroup := labels["role-group"] != "" && labels["app.kubernetes.io/component"] != "" &&
					strings.HasPrefix(object.GetName(), prefix)
				recordedSlot := recordErr == nil && slices.Contains(record.Object.Annotations, GroupSlotAnnotation)
				if isSet || looksLikeGroup || recordedSlot {
					err = fmt.Errorf("controlled group candidate %T %s is missing its group slot receipt", object, object.GetName())
				}
			}
			if err == nil && slot != nil {
				err = checkSlotObject(cr, object, kind, *slot)
			}
			if err != nil {
				failures = append(failures, err)
				continue
			}
			if slot != nil {
				groups[slot.key()] = *slot
			}
		}
	}
	return groups, failures
}

func (r *Reconciler[CR, C, S, F]) retireGroups(ctx context.Context, cr CR,
	desired []framework.GroupIdentity, status *framework.ReconcileStatus,
) (bool, []error) {
	groups, failures := r.retirementInventory(ctx, cr)
	wanted := map[string]bool{}
	for _, group := range desired {
		wanted[group.Role+"/"+group.Name] = true
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		if !wanted[key] {
			keys = append(keys, key)
		}
	}
	priorities, priorityErr := r.shutdownPriorities(ctx, cr, groups)
	if priorityErr != nil {
		return true, append(failures, priorityErr)
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
	pending := []string{}
	var blockedPriority *int32
	for _, key := range keys {
		if blockedPriority != nil && priorities[key] > *blockedPriority {
			pending = append(pending, key+": waiting for lower shutdown priorities")
			continue
		}
		phase, err := r.retireGroup(ctx, cr, groups[key])
		if phase != "" || err != nil {
			priority := priorities[key]
			blockedPriority = &priority
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("retire %s: %w", key, err))
		}
		if phase != "" {
			pending = append(pending, key+": "+phase)
		}
		if errors.Is(err, errSuperseded) {
			break
		}
	}
	switch {
	case len(failures) > 0:
		setCondition(status, "Retired", false, "RetirementFailed", errorMessage(failures))
	case len(pending) > 0:
		setCondition(status, "Retired", false, "RetirementPending", strings.Join(pending, "; "))
	default:
		setCondition(status, "Retired", true, "RetirementComplete",
			"No removed group slots remain; desired workload readiness is separate")
	}
	return len(pending) > 0, failures
}

func (r *Reconciler[CR, C, S, F]) readSlot(ctx context.Context, cr CR, slot groupSlot) (client.Object, error) {
	object := slot.object(cr)
	if err := r.Client.Get(ctx, client.ObjectKeyFromObject(object), object); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	kind, err := apiutil.GVKForObject(cr, r.Scheme)
	if err != nil {
		return nil, err
	}
	if err := checkSlotObject(cr, object, kind, slot); err != nil {
		return nil, err
	}
	return object, nil
}

// Pod absence is observed directly, including terminating Pods, old StatefulSet
// UIDs and missing/edited labels. No Pod is force-deleted by this controller.
func (r *Reconciler[CR, C, S, F]) groupPodsRemain(ctx context.Context, cr CR, slot groupSlot) (bool, error) {
	var pods corev1.PodList
	if err := r.Client.List(ctx, &pods, client.InNamespace(cr.GetNamespace())); err != nil {
		return false, err
	}
	name := slot.base(cr)
	for _, pod := range pods.Items {
		owner := metav1.GetControllerOf(&pod)
		if owner != nil && owner.Kind == statefulSetKind &&
			owner.APIVersion == appsv1.SchemeGroupVersion.String() && owner.Name == name {
			return true, nil
		}
		// A Pod with the canonical ordinal name is also a conservative stop barrier,
		// even if someone removed its controller reference.
		if suffix, ok := strings.CutPrefix(pod.Name, name+"-"); ok {
			if ordinal, err := strconv.ParseUint(suffix, 10, 32); err == nil && strconv.FormatUint(ordinal, 10) == suffix {
				return true, nil
			}
		}
	}
	return false, nil
}

func storageRetirementUnsupported(sts *appsv1.StatefulSet) bool {
	platformNames, err := platformClaimNames(sts)
	if err != nil {
		return true
	}
	if len(sts.Spec.VolumeClaimTemplates) > 0 {
		return true
	}
	if policy := sts.Spec.PersistentVolumeClaimRetentionPolicy; policy != nil &&
		(policy.WhenDeleted == appsv1.DeletePersistentVolumeClaimRetentionPolicyType ||
			policy.WhenScaled == appsv1.DeletePersistentVolumeClaimRetentionPolicyType) {
		return true
	}
	for _, volume := range sts.Spec.Template.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil || (volume.Ephemeral != nil && !platformNames[volume.Name]) {
			return true
		}
	}
	return false
}

func (r *Reconciler[CR, C, S, F]) retireGroup(ctx context.Context, cr CR, group groupSlot) (string, error) {
	group.Slot = slotStatefulset
	object, err := r.readSlot(ctx, cr, group)
	if err != nil {
		return "", err
	}
	drained := object
	if object != nil {
		sts := object.(*appsv1.StatefulSet)
		if err := checkRetiringStorage(ctx, r.Client, cr, group, sts); err != nil {
			if errors.Is(err, errStoragePending) {
				return err.Error(), nil
			}
			return "", err
		}
		if err := r.observeCoordination(ctx, cr, sts, 0, false); err != nil {
			return "coordination blocked", err
		}
		if !sts.DeletionTimestamp.IsZero() {
			return "waiting for StatefulSet deletion", nil
		}
		if sts.Spec.Replicas == nil || *sts.Spec.Replicas != 0 {
			return r.stopRetainedGroup(ctx, cr, group, sts)
		}
		if sts.Status.ObservedGeneration < sts.Generation || sts.Status.Replicas != 0 || sts.Status.ReadyReplicas != 0 ||
			sts.Status.UpdatedReplicas != 0 {
			return "waiting for StatefulSet drain observation", nil
		}
	}
	remain, err := r.groupPodsRemain(ctx, cr, group)
	if err != nil || remain {
		return "waiting for Pods to disappear", err
	}
	// Issue at most one delete per pass, and confirm its absence on a later pass.
	for _, kind := range []string{slotStatefulset, slotService, slotHeadless, slotConfigmap} {
		group.Slot = kind
		object, err := r.readSlot(ctx, cr, group)
		if err != nil {
			return "", err
		}
		if object == nil {
			continue
		}
		if kind == slotStatefulset && (drained == nil || object.GetUID() != drained.GetUID() ||
			object.GetResourceVersion() != drained.GetResourceVersion()) {
			return "StatefulSet changed; drain must be observed again", nil
		}
		if !object.GetDeletionTimestamp().IsZero() {
			return "waiting for " + kind + " deletion", nil
		}
		if kind == slotStatefulset {
			if err := checkRetiringStorage(ctx, r.Client, cr, group, object.(*appsv1.StatefulSet)); err != nil {
				if errors.Is(err, errStoragePending) {
					return err.Error(), nil
				}
				return "", err
			}
		}
		if err := r.currentInput(ctx, cr); err != nil {
			return "", err
		}
		uid, version := object.GetUID(), object.GetResourceVersion()
		err = r.Client.Delete(ctx, object, &client.DeleteOptions{Preconditions: &metav1.Preconditions{
			UID: &uid, ResourceVersion: &version}, PropagationPolicy: deletionPropagation()})
		return "deleting " + kind, client.IgnoreNotFound(err)
	}
	return "", nil
}

func deletionPropagation() *metav1.DeletionPropagation {
	policy := metav1.DeletePropagationBackground
	return &policy
}

// Re-adding a group whose delete was already accepted cannot undo that delete.
// Wait for terminating fixed slots before applying any part of the new plan.
func (r *Reconciler[CR, C, S, F]) groupTerminating(
	ctx context.Context, cr CR, identity framework.GroupIdentity,
) (bool, error) {
	for _, kind := range []string{slotConfigmap, slotHeadless, slotService, slotStatefulset} {
		object, err := r.readSlot(ctx, cr, groupSlot{Role: identity.Role, Group: identity.Name, Slot: kind})
		if err != nil {
			return false, err
		}
		if object != nil && !object.GetDeletionTimestamp().IsZero() {
			return true, nil
		}
	}
	return false, nil
}

func (r *Reconciler[CR, C, S, F]) stopRetainedGroup(ctx context.Context, cr CR, group groupSlot,
	sts *appsv1.StatefulSet,
) (string, error) {
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		current, err := r.readSlot(ctx, cr, group)
		if err != nil || current == nil {
			return err
		}
		live := current.(*appsv1.StatefulSet)
		if live.UID != sts.UID {
			return fmt.Errorf("StatefulSet identity changed while stopping")
		}
		if !live.DeletionTimestamp.IsZero() {
			return nil
		}
		if err := checkRetiringStorage(ctx, r.Client, cr, group, live); err != nil {
			return err
		}
		if err := r.currentInput(ctx, cr); err != nil {
			return err
		}
		count, err := nextScaleDown(ctx, r.Client, live, 0)
		if err != nil {
			return err
		}
		if err := r.observeCoordination(ctx, cr, live, 0, false); err != nil {
			return err
		}
		if replicas(live) == count {
			return nil
		}
		live.Spec.Replicas = &count
		return r.Client.Update(ctx, live)
	})
	if errors.Is(err, errStoragePending) {
		return err.Error(), nil
	}
	return "stopping StatefulSet", err
}

// The apply path requires the same complete receipt as inventory, including a
// damaged slot whose annotation was removed while its managed record survived.
func checkApplyGroupSlot(owner, live client.Object, kind schema.GroupVersionKind, expected *groupSlot) error {
	if expected != nil {
		return checkSlotObject(owner, live, kind, *expected)
	}
	if err := checkSlot(live, nil); err != nil {
		return err
	}
	record, err := decodeManagedMetadata(live)
	if err != nil {
		return err
	}
	if slices.Contains(record.Object.Annotations, GroupSlotAnnotation) {
		return fmt.Errorf("%T %s is missing its managed group receipt", live, live.GetName())
	}
	return nil
}
