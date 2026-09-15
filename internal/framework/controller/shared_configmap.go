package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/zncdatadev/operator-go/pkg/framework"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// SharedConfigMapAnnotation records one slot in the complete shared output set.
// A matching owner alone does not authorize adoption or withdrawal of custom data.
const SharedConfigMapAnnotation = "framework.kubedoop.dev/shared-configmap"

var errSharedTerminating = errors.New("waiting for terminating shared ConfigMap")

type sharedConfigMapSlot struct {
	Version int       `json:"version"`
	CRUID   types.UID `json:"crUID"`
	Name    string    `json:"name"`
}

func sharedSlot(owner client.Object, name string) sharedConfigMapSlot {
	return sharedConfigMapSlot{Version: 1, CRUID: owner.GetUID(), Name: name}
}

func decodeSharedSlot(object client.Object) (*sharedConfigMapSlot, error) {
	raw, present := object.GetAnnotations()[SharedConfigMapAnnotation]
	if !present {
		return nil, nil
	}
	var slot sharedConfigMapSlot
	if err := strictReceipt(raw, &slot); err != nil || slot.Version != 1 || slot.CRUID == "" ||
		len(validation.IsDNS1123Subdomain(slot.Name)) != 0 {
		return nil, fmt.Errorf("invalid shared ConfigMap receipt on %s", object.GetName())
	}
	return &slot, nil
}

func stampSharedSlot(owner, desired client.Object, slot sharedConfigMapSlot) (client.Object, error) {
	if _, ok := desired.(*corev1.ConfigMap); !ok || slot != sharedSlot(owner, desired.GetName()) ||
		len(validation.IsDNS1123Subdomain(slot.Name)) != 0 || desired.GetNamespace() != owner.GetNamespace() {
		return nil, fmt.Errorf("shared ConfigMap slot differs from the desired object")
	}
	next := desired.DeepCopyObject().(client.Object)
	annotations := maps.Clone(next.GetAnnotations())
	if annotations == nil {
		annotations = map[string]string{}
	}
	data, err := json.Marshal(slot)
	if err != nil {
		return nil, err
	}
	annotations[SharedConfigMapAnnotation] = string(data)
	next.SetAnnotations(annotations)
	return next, nil
}

func checkSharedObject(owner, live client.Object, kind schema.GroupVersionKind, expected sharedConfigMapSlot) error {
	copy := live.DeepCopyObject().(client.Object)
	copy.SetDeletionTimestamp(nil)
	if err := checkOwnership(owner, copy, kind); err != nil {
		return err
	}
	actual, err := decodeSharedSlot(live)
	if err != nil {
		return err
	}
	if _, ok := live.(*corev1.ConfigMap); !ok || actual == nil || *actual != expected ||
		expected != sharedSlot(owner, live.GetName()) || live.GetNamespace() != owner.GetNamespace() {
		return fmt.Errorf("shared ConfigMap %s has a missing or mismatched source receipt", live.GetName())
	}
	if live.GetLabels()["app.kubernetes.io/instance"] != owner.GetName() {
		return fmt.Errorf("shared ConfigMap %s has damaged identity labels", live.GetName())
	}
	record, err := decodeManagedMetadata(live)
	if err != nil {
		return err
	}
	if !slices.Contains(record.Object.Annotations, SharedConfigMapAnnotation) ||
		!slices.Contains(record.Object.Labels, "app.kubernetes.io/instance") {
		return fmt.Errorf("shared ConfigMap %s is absent from managed metadata", live.GetName())
	}
	if err := checkApplyGroupSlot(owner, live, kind, nil); err != nil {
		return err
	}
	if _, present := live.GetAnnotations()[RolePDBAnnotation]; present {
		return fmt.Errorf("shared ConfigMap %s carries a conflicting role receipt", live.GetName())
	}
	return nil
}

func checkApplyShared(owner, live client.Object, kind schema.GroupVersionKind, expected *sharedConfigMapSlot) error {
	actual, err := decodeSharedSlot(live)
	if err != nil {
		return err
	}
	if expected == nil {
		record, err := decodeManagedMetadata(live)
		if err != nil {
			return err
		}
		if actual != nil || slices.Contains(record.Object.Annotations, SharedConfigMapAnnotation) {
			return fmt.Errorf("%T %s belongs to a shared ConfigMap slot", live, live.GetName())
		}
		return nil
	}
	if err := checkSharedObject(owner, live, kind, *expected); err != nil {
		return err
	}
	if !live.GetDeletionTimestamp().IsZero() {
		return errSharedTerminating
	}
	return nil
}

func (r *Reconciler[CR, C, S, F]) sharedInventory(ctx context.Context, cr CR) ([]sharedConfigMapSlot, []error) {
	var list corev1.ConfigMapList
	if err := r.Client.List(ctx, &list, client.InNamespace(cr.GetNamespace())); err != nil {
		return nil, []error{err}
	}
	kind, err := apiutil.GVKForObject(cr, r.Scheme)
	if err != nil {
		return nil, []error{err}
	}
	slices.SortFunc(list.Items, func(a, b corev1.ConfigMap) int { return strings.Compare(a.Name, b.Name) })
	var slots []sharedConfigMapSlot
	var failures []error
	for i := range list.Items {
		object := &list.Items[i]
		owner := metav1.GetControllerOf(object)
		if owner == nil || owner.UID != cr.GetUID() {
			continue
		}
		slot, err := decodeSharedSlot(object)
		record, recordErr := decodeManagedMetadata(object)
		if err == nil && slot == nil && recordErr == nil &&
			slices.Contains(record.Object.Annotations, SharedConfigMapAnnotation) {
			err = fmt.Errorf("shared ConfigMap %s is missing its source receipt", object.Name)
		}
		if err == nil && slot != nil {
			err = checkSharedObject(cr, object, kind, *slot)
		}
		if err != nil {
			failures = append(failures, err)
		} else if slot != nil {
			slots = append(slots, *slot)
		}
	}
	return slots, failures
}

func (r *Reconciler[CR, C, S, F]) deleteShared(ctx context.Context, cr CR, slot sharedConfigMapSlot) (bool, error) {
	pending := false
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := r.currentInput(ctx, cr); err != nil {
			return err
		}
		live := &corev1.ConfigMap{}
		if err := r.Client.Get(ctx, client.ObjectKey{Namespace: cr.GetNamespace(), Name: slot.Name}, live); err != nil {
			pending = false
			return client.IgnoreNotFound(err)
		}
		kind, err := apiutil.GVKForObject(cr, r.Scheme)
		if err != nil {
			return err
		}
		if err := checkSharedObject(cr, live, kind, slot); err != nil {
			return err
		}
		pending = true
		if !live.DeletionTimestamp.IsZero() {
			return nil
		}
		if err := r.currentInput(ctx, cr); err != nil {
			return err
		}
		uid, version := live.UID, live.ResourceVersion
		return client.IgnoreNotFound(r.Client.Delete(ctx, live, &client.DeleteOptions{
			Preconditions:     &metav1.Preconditions{UID: &uid, ResourceVersion: &version},
			PropagationPolicy: deletionPropagation(),
		}))
	})
	return pending, err
}

// Ready is a complete set, including empty withdrawal. Pending never applies a
// partial set or retires previous output. Live inventory survives status loss and
// controller restarts; every mutation checks fresh intent and exact object source.
func (r *Reconciler[CR, C, S, F]) reconcileShared(ctx context.Context, cr CR,
	output framework.ClusterOutput,
) (bool, []error) {
	if err := framework.ValidateClusterOutput(output); err != nil {
		return false, []error{err}
	}
	if output.State == framework.ClusterOutputPending {
		return true, nil
	}
	desired := make(map[string]bool, len(output.ConfigMaps))
	var failures []error
	pending := false
	for i := range output.ConfigMaps {
		cm := &output.ConfigMaps[i]
		desired[cm.Name] = true
		slot := sharedSlot(cr, cm.Name)
		if err := r.currentInput(ctx, cr); err != nil {
			failures = append(failures, err)
			continue
		}
		_, err := applyScopedObject(ctx, r.Client, cr, cm, r.Scheme, nil, nil, nil, &slot, nil)
		if errors.Is(err, errSharedTerminating) {
			pending = true
		} else if err != nil {
			failures = append(failures, err)
		}
	}
	slots, errs := r.sharedInventory(ctx, cr)
	failures = append(failures, errs...)
	for _, slot := range slots {
		if desired[slot.Name] {
			continue
		}
		waiting, err := r.deleteShared(ctx, cr, slot)
		pending = pending || waiting
		if err != nil {
			failures = append(failures, err)
		}
	}
	return pending, failures
}

// A receipt chooses the validator, not an exemption from source checks. A valid
// shared ConfigMap may carry group-looking user labels and a derived-name prefix;
// only complete shared identity validation excludes it from workload inventory.
func sharedInventoryObject(owner, object client.Object, kind schema.GroupVersionKind) (bool, error) {
	slot, err := decodeSharedSlot(object)
	if err != nil {
		return true, err
	}
	if slot == nil {
		return false, nil
	}
	return true, checkSharedObject(owner, object, kind, *slot)
}
