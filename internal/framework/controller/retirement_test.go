package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/zncdatadev/operator-go/pkg/framework"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	generatedtrino "github.com/zncdatadev/operator-go/internal/framework/pipeline/testinput"
)

// These are observations supplied to the controller, not simulated evidence of
// a StatefulSet controller, kubelet or garbage collector having run.
func retirementObjects(t *testing.T, cr *generatedtrino.TrinoCluster,
	group string, replicas int32,
) []client.Object {
	t.Helper()
	objects := make([]client.Object, 0, 4)
	for _, kind := range []string{"statefulset", "service", "headless", "configmap"} {
		slot := groupSlot{Role: "workers", Group: group, Slot: kind}
		object := slot.object(cr)
		data, err := json.Marshal(slot)
		if err != nil {
			t.Fatal(err)
		}
		controller := true
		object.SetOwnerReferences([]metav1.OwnerReference{{APIVersion: generatedtrino.GroupVersion.String(),
			Kind: "TrinoCluster", Name: cr.Name, UID: cr.UID, Controller: &controller}})
		object.SetUID(types.UID(group + "-" + kind + "-uid"))
		object.SetResourceVersion("1")
		object.SetGeneration(3)
		object.SetLabels(map[string]string{"app.kubernetes.io/instance": cr.Name,
			"app.kubernetes.io/component": "workers", "role-group": group})
		object.SetAnnotations(map[string]string{GroupSlotAnnotation: string(data),
			ManagedMetadataAnnotation: `{"object":{"labels":["app.kubernetes.io/instance",` +
				`"app.kubernetes.io/component","role-group"],"annotations":[` +
				`"framework.kubedoop.dev/group-slot"]},"template":{}}`})
		if sts, ok := object.(*appsv1.StatefulSet); ok {
			sts.Spec.Replicas = &replicas
			sts.Status = appsv1.StatefulSetStatus{ObservedGeneration: 3, Replicas: replicas,
				ReadyReplicas: replicas, UpdatedReplicas: replicas}
		}
		objects = append(objects, object)
	}
	return objects
}

func retirementScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := controllerScheme(t)
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return scheme
}

func retirementClient(scheme *runtime.Scheme, cr *generatedtrino.TrinoCluster,
	objects []client.Object, intercept interceptor.Funcs,
) client.Client {
	all := append([]client.Object{cr.DeepCopy()}, objects...)
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(all...).
		WithStatusSubresource(cr, &appsv1.StatefulSet{}).WithInterceptorFuncs(intercept).Build()
}

func retirementCondition(t *testing.T, status *framework.ReconcileStatus, want metav1.ConditionStatus, reason string) {
	t.Helper()
	condition := meta.FindStatusCondition(status.Conditions, "Retired")
	if condition == nil || condition.Status != want || condition.Reason != reason {
		t.Fatalf("unexpected retirement condition: %+v", condition)
	}
}

func TestRetirementDamagedIdentityCannotDisappearAsSuccess(t *testing.T) {
	for _, damage := range []string{"missing-receipt", "invalid-receipt", "duplicate-key", "missing-record",
		"invalid-record", "receipt-not-recorded", "wrong-label", "wrong-owner-kind", "wrong-name"} {
		t.Run(damage, func(t *testing.T) {
			cr, scheme := controllerInput(), retirementScheme(t)
			object := retirementObjects(t, cr, "removed", 0)[1]
			switch damage {
			case "missing-receipt":
				delete(object.GetAnnotations(), GroupSlotAnnotation)
			case "invalid-receipt":
				object.GetAnnotations()[GroupSlotAnnotation] = "null"
			case "duplicate-key":
				object.GetAnnotations()[GroupSlotAnnotation] =
					`{"role":"workers","role":"workers","group":"removed","slot":"service"}`
			case "missing-record":
				delete(object.GetAnnotations(), ManagedMetadataAnnotation)
			case "invalid-record":
				object.GetAnnotations()[ManagedMetadataAnnotation] = "null"
			case "receipt-not-recorded":
				object.GetAnnotations()[ManagedMetadataAnnotation] = `{"object":{},"template":{}}`
			case "wrong-label":
				object.GetLabels()["role-group"] = "other"
			case "wrong-owner-kind":
				owners := object.GetOwnerReferences()
				owners[0].Kind = "WrongKind"
				object.SetOwnerReferences(owners)
			case "wrong-name":
				object.SetName(object.GetName() + "-unexpected")
			}
			c := retirementClient(scheme, cr, []client.Object{object}, interceptor.Funcs{})
			r := newTestReconciler(c, scheme, testFacts{})
			status := framework.ReconcileStatus{ObservedGeneration: cr.Generation}
			_, failures := r.retireGroups(t.Context(), cr, nil, &status)
			if len(failures) == 0 {
				t.Fatal("damaged controlled object vanished from retirement diagnostics")
			}
			retirementCondition(t, &status, metav1.ConditionFalse, "RetirementFailed")
			applyTestGet(t, c, object.DeepCopyObject().(client.Object))
		})
	}
}

func TestRetirementWaitsForActualPodsRegardlessOfLabelsAndUID(t *testing.T) {
	for _, shape := range []string{"no-labels", "old-sts-uid", "terminating", "no-owner-canonical-name"} {
		t.Run(shape, func(t *testing.T) {
			cr, scheme := controllerInput(), retirementScheme(t)
			objects := retirementObjects(t, cr, "removed", 0)
			sts := objects[0].(*appsv1.StatefulSet)
			controller := true
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: sts.Name + "-0", Namespace: cr.Namespace,
				OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "StatefulSet",
					Name: sts.Name, UID: sts.UID, Controller: &controller}}}}
			switch shape {
			case "old-sts-uid":
				pod.Name = "noncanonical-but-owned"
				pod.OwnerReferences[0].UID = "an-older-sts-uid"
			case "terminating":
				now := metav1.Now()
				pod.DeletionTimestamp, pod.Finalizers = &now, []string{"test.design/hold"}
			case "no-owner-canonical-name":
				pod.OwnerReferences = nil
			}
			c := retirementClient(scheme, cr, append(objects, pod), interceptor.Funcs{})
			r := newTestReconciler(c, scheme, testFacts{})
			phase, err := r.retireGroup(t.Context(), cr, groupSlot{Role: "workers", Group: "removed"})
			if err != nil || phase != "waiting for Pods to disappear" {
				t.Fatalf("existing pod did not block retirement: phase=%q err=%v", phase, err)
			}
			for _, object := range objects {
				applyTestGet(t, c, object.DeepCopyObject().(client.Object))
			}
		})
	}
}

func TestRetirementRequiresCurrentDrainObservation(t *testing.T) {
	for _, state := range []string{"old-generation", "replicas", "ready-replicas", "updated-replicas"} {
		t.Run(state, func(t *testing.T) {
			cr, scheme := controllerInput(), retirementScheme(t)
			objects := retirementObjects(t, cr, "removed", 0)
			sts := objects[0].(*appsv1.StatefulSet)
			switch state {
			case "old-generation":
				sts.Status.ObservedGeneration--
			case "replicas":
				sts.Status.Replicas = 1
			case "ready-replicas":
				sts.Status.ReadyReplicas = 1
			case "updated-replicas":
				sts.Status.UpdatedReplicas = 1
			}
			c := retirementClient(scheme, cr, objects, interceptor.Funcs{})
			r := newTestReconciler(c, scheme, testFacts{})
			phase, err := r.retireGroup(t.Context(), cr, groupSlot{Role: "workers", Group: "removed"})
			if err != nil || phase != "waiting for StatefulSet drain observation" {
				t.Fatalf("unobserved drain was treated as complete: %q %v", phase, err)
			}
			applyTestGet(t, c, sts.DeepCopy())
		})
	}
}

func TestRetirementStorageRequiresPolicyBeforeScaleDown(t *testing.T) {
	for _, storage := range []string{
		"claim-templates", "delete-when-scaled", "delete-when-deleted", "pod-pvc", "ephemeral",
	} {
		t.Run(storage, func(t *testing.T) {
			cr, scheme := controllerInput(), retirementScheme(t)
			objects := retirementObjects(t, cr, "removed", 1)
			sts := objects[0].(*appsv1.StatefulSet)
			switch storage {
			case "claim-templates":
				sts.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{{ObjectMeta: metav1.ObjectMeta{Name: "data"}}}
			case "delete-when-scaled":
				sts.Spec.PersistentVolumeClaimRetentionPolicy = &appsv1.StatefulSetPersistentVolumeClaimRetentionPolicy{
					WhenScaled: appsv1.DeletePersistentVolumeClaimRetentionPolicyType}
			case "delete-when-deleted":
				sts.Spec.PersistentVolumeClaimRetentionPolicy = &appsv1.StatefulSetPersistentVolumeClaimRetentionPolicy{
					WhenDeleted: appsv1.DeletePersistentVolumeClaimRetentionPolicyType}
			case "pod-pvc":
				sts.Spec.Template.Spec.Volumes = []corev1.Volume{{Name: "data", VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "data"}}}}
			case "ephemeral":
				sts.Spec.Template.Spec.Volumes = []corev1.Volume{{Name: "data", VolumeSource: corev1.VolumeSource{
					Ephemeral: &corev1.EphemeralVolumeSource{}}}}
			}
			c := retirementClient(scheme, cr, objects, interceptor.Funcs{})
			r := newTestReconciler(c, scheme, testFacts{})
			_, err := r.retireGroup(t.Context(), cr, groupSlot{Role: "workers", Group: "removed"})
			if err == nil || !strings.Contains(err.Error(), "separate data policy") {
				t.Fatalf("storage retirement was accepted: %v", err)
			}
			live := sts.DeepCopy()
			applyTestGet(t, c, live)
			if *live.Spec.Replicas != 1 || live.ResourceVersion != sts.ResourceVersion {
				t.Fatal("unsupported storage was modified before policy validation")
			}
		})
	}
}

func TestRetirementDeleteFailureRecoversFromLiveSlotsAcrossNewReconcilers(t *testing.T) {
	cr, scheme := controllerInput(), retirementScheme(t)
	objects := retirementObjects(t, cr, "removed", 0)
	failed := false
	var deleted []string
	c := retirementClient(scheme, cr, objects, interceptor.Funcs{Delete: func(ctx context.Context, c client.WithWatch,
		object client.Object, options ...client.DeleteOption) error {
		settings := &client.DeleteOptions{}
		settings.ApplyOptions(options)
		if settings.Preconditions == nil || settings.Preconditions.UID == nil ||
			settings.Preconditions.ResourceVersion == nil || *settings.Preconditions.UID != object.GetUID() ||
			*settings.Preconditions.ResourceVersion != object.GetResourceVersion() {
			t.Fatal("delete did not fence the exact observed UID and resourceVersion")
		}
		if !failed {
			failed = true
			return apierrors.NewForbidden(schema.GroupResource{Group: "apps", Resource: "statefulsets"},
				object.GetName(), errors.New("injected failure"))
		}
		slot, err := decodeSlot(object)
		if err != nil {
			return err
		}
		deleted = append(deleted, slot.Slot)
		return c.Delete(ctx, object, options...)
	}})
	r := newTestReconciler(c, scheme, testFacts{})
	status := framework.ReconcileStatus{ObservedGeneration: cr.Generation}
	if _, failures := r.retireGroups(t.Context(), cr, nil, &status); len(failures) == 0 {
		t.Fatal("injected delete failure was hidden")
	}
	retirementCondition(t, &status, metav1.ConditionFalse, "RetirementFailed")
	for _, object := range objects {
		applyTestGet(t, c, object.DeepCopyObject().(client.Object))
	}
	for i := 0; i < 5; i++ {
		// No previous status or controller object is carried into the next pass.
		r = newTestReconciler(c, scheme, testFacts{})
		status = framework.ReconcileStatus{ObservedGeneration: cr.Generation}
		if _, failures := r.retireGroups(t.Context(), cr, nil, &status); len(failures) != 0 {
			t.Fatalf("restart did not recover: %v", failures)
		}
	}
	if !reflect.DeepEqual(deleted, []string{"statefulset", "service", "headless", "configmap"}) {
		t.Fatalf("wrong deletion order or repeated delete: %v", deleted)
	}
	retirementCondition(t, &status, metav1.ConditionTrue, "RetirementComplete")
}

func TestRetirementWaitsForFinalizersAndReaddWaitsForEverySlot(t *testing.T) {
	for _, kind := range []string{"statefulset", "service", "headless", "configmap"} {
		t.Run(kind, func(t *testing.T) {
			cr, scheme := controllerInput(), retirementScheme(t)
			objects := retirementObjects(t, cr, "removed", 0)
			var remaining []client.Object
			seen := false
			for _, object := range objects {
				slot, _ := decodeSlot(object)
				if slot.Slot == kind {
					object.SetFinalizers([]string{"test.design/hold"})
					seen = true
				}
				if seen {
					remaining = append(remaining, object)
				}
			}
			c := retirementClient(scheme, cr, remaining, interceptor.Funcs{})
			r := newTestReconciler(c, scheme, testFacts{})
			group := groupSlot{Role: "workers", Group: "removed"}
			if _, err := r.retireGroup(t.Context(), cr, group); err != nil {
				t.Fatal(err)
			}
			phase, err := r.retireGroup(t.Context(), cr, group)
			if err != nil || !strings.Contains(phase, "waiting for") || !strings.Contains(phase, "deletion") {
				t.Fatalf("accepted Delete was confused with absence: %q %v", phase, err)
			}
			waiting, err := r.groupTerminating(t.Context(), cr, framework.GroupIdentity{Role: "workers", Name: "removed"})
			if err != nil || !waiting {
				t.Fatalf("readd ignored terminating %s: %t %v", kind, waiting, err)
			}
			for _, object := range remaining {
				applyTestGet(t, c, object.DeepCopyObject().(client.Object))
			}
		})
	}
}

func TestRetirementConflictRechecksInputAndObjectIdentity(t *testing.T) {
	for _, change := range []string{"generation", "cr-uid", "sts-uid", "owner"} {
		t.Run(change, func(t *testing.T) {
			cr, scheme := controllerInput(), retirementScheme(t)
			objects := retirementObjects(t, cr, "removed", 1)
			writes := 0
			c := retirementClient(scheme, cr, objects, interceptor.Funcs{Update: func(ctx context.Context, c client.WithWatch,
				object client.Object, options ...client.UpdateOption) error {
				writes++
				if change == "generation" || change == "cr-uid" {
					current := cr.DeepCopy()
					if err := c.Get(ctx, client.ObjectKeyFromObject(current), current); err != nil {
						return err
					}
					if change == "generation" {
						current.Generation++
					} else {
						current.UID = "replacement-cr"
					}
					if err := c.Update(ctx, current); err != nil {
						return err
					}
				} else {
					live := &appsv1.StatefulSet{}
					if err := c.Get(ctx, client.ObjectKeyFromObject(object), live); err != nil {
						return err
					}
					if change == "sts-uid" {
						live.UID = "replacement-workload"
					} else {
						live.OwnerReferences[0].UID = "another-owner"
					}
					if err := c.Update(ctx, live, options...); err != nil {
						return err
					}
				}
				return apierrors.NewConflict(schema.GroupResource{Group: "apps", Resource: "statefulsets"},
					object.GetName(), errors.New("changed while stopping"))
			}})
			r := newTestReconciler(c, scheme, testFacts{})
			_, err := r.retireGroup(t.Context(), cr, groupSlot{Role: "workers", Group: "removed"})
			if err == nil || writes != 1 {
				t.Fatalf("conflict retried a stale mutation: writes=%d err=%v", writes, err)
			}
			if (change == "generation" || change == "cr-uid") && !errors.Is(err, errSuperseded) {
				t.Fatalf("input replacement was not recognized: %v", err)
			}
			live := objects[0].(*appsv1.StatefulSet).DeepCopy()
			applyTestGet(t, c, live)
			if *live.Spec.Replicas != 1 {
				t.Fatal("stale stop was persisted")
			}
		})
	}
}

func TestRetirementReobservesWorkloadChangedAfterDrainCheck(t *testing.T) {
	for _, change := range []string{"scaled-up", "replacement", "appeared"} {
		t.Run(change, func(t *testing.T) {
			cr, scheme := controllerInput(), retirementScheme(t)
			objects := retirementObjects(t, cr, "removed", 0)
			sts := objects[0].(*appsv1.StatefulSet).DeepCopy()
			seed := objects
			if change == "appeared" {
				seed = objects[1:]
			}
			reads, deletes := 0, 0
			c := retirementClient(scheme, cr, seed, interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey,
					object client.Object, opts ...client.GetOption,
				) error {
					if _, ok := object.(*appsv1.StatefulSet); ok {
						reads++
						if reads == 2 {
							live := sts.DeepCopy()
							if change == "appeared" {
								live.ResourceVersion = ""
								if err := c.Create(ctx, live); err != nil {
									return err
								}
							} else {
								if err := c.Get(ctx, key, live); err != nil {
									return err
								}
								if change == "scaled-up" {
									one := int32(1)
									live.Spec.Replicas = &one
								} else {
									live.UID = "replacement-uid"
								}
								if err := c.Update(ctx, live); err != nil {
									return err
								}
							}
						}
					}
					return c.Get(ctx, key, object, opts...)
				},
				Delete: func(context.Context, client.WithWatch, client.Object, ...client.DeleteOption) error {
					deletes++
					return errors.New("changed workload must be observed again")
				},
			})
			r := newTestReconciler(c, scheme, testFacts{})
			phase, err := r.retireGroup(t.Context(), cr, groupSlot{Role: "workers", Group: "removed"})
			if err != nil || phase == "" || deletes != 0 {
				t.Fatalf("unverified workload reached delete: phase=%q deletes=%d err=%v", phase, deletes, err)
			}
		})
	}
}

func TestApplyRefusesSameOwnerDifferentSlotAndReservedUserReceipt(t *testing.T) {
	cr, scheme := controllerInput(), retirementScheme(t)
	objects := retirementObjects(t, cr, "a", 0)
	headless := objects[2].(*corev1.Service)
	c := retirementClient(scheme, cr, objects, interceptor.Funcs{})
	desired := headless.DeepCopy()
	desired.Annotations, desired.OwnerReferences = nil, nil
	other := groupSlot{Role: "workers", Group: "a-headless", Slot: "service"}
	_, err := applyObject(t.Context(), c, cr, desired, scheme, &other, nil)
	if err == nil || !strings.Contains(err.Error(), "different group slot") {
		t.Fatalf("same-owner resource name collision was adopted: %v", err)
	}
	cm := objects[3].(*corev1.ConfigMap).DeepCopy()
	cm.Annotations, cm.OwnerReferences = nil, nil
	_, err = ApplyObject(t.Context(), c, cr, cm, scheme)
	if err == nil || !strings.Contains(err.Error(), "different group slot") {
		t.Fatalf("cluster output adopted group ConfigMap: %v", err)
	}
	for _, location := range []string{"object-annotation", "object-label", "pod-annotation", "pod-label"} {
		t.Run(location, func(t *testing.T) {
			desired := objects[0].(*appsv1.StatefulSet).DeepCopy()
			desired.Annotations = nil
			switch location {
			case "object-annotation":
				desired.Annotations = map[string]string{GroupSlotAnnotation: "user"}
			case "object-label":
				desired.Labels[GroupSlotAnnotation] = "user"
			case "pod-annotation":
				desired.Spec.Template.Annotations = map[string]string{GroupSlotAnnotation: "user"}
			case "pod-label":
				desired.Spec.Template.Labels = map[string]string{GroupSlotAnnotation: "user"}
			}
			_, err := ApplyObject(t.Context(), c, cr, desired, scheme)
			if err == nil || !strings.Contains(err.Error(), "reserved key") {
				t.Fatalf("user wrote a controller-issued receipt at %s: %v", location, err)
			}
		})
	}
}

func TestRetirementReaddingStoppedGroupKeepsStatefulSetIdentity(t *testing.T) {
	cr, scheme := controllerInput(), retirementScheme(t)
	objects := retirementObjects(t, cr, "returning", 1)
	c := retirementClient(scheme, cr, objects, interceptor.Funcs{})
	r := newTestReconciler(c, scheme, testFacts{})
	group := groupSlot{Role: "workers", Group: "returning", Slot: "statefulset"}
	if phase, err := r.retireGroup(t.Context(), cr, group); err != nil || phase != "stopping StatefulSet" {
		t.Fatalf("first stop failed: %s %v", phase, err)
	}
	want := objects[0].(*appsv1.StatefulSet).DeepCopy()
	want.Annotations, want.OwnerReferences = nil, nil
	if _, err := applyObject(t.Context(), c, cr, want, scheme, &group, nil); err != nil {
		t.Fatalf("readding stopped workload failed: %v", err)
	}
	status := framework.ReconcileStatus{ObservedGeneration: cr.Generation}
	desired := []framework.GroupIdentity{{Role: "workers", Name: "returning"}}
	if pending, failures := r.retireGroups(t.Context(), cr, desired, &status); pending || len(failures) != 0 {
		t.Fatalf("readded group was still retired: %t %v", pending, failures)
	}
	live := want.DeepCopy()
	applyTestGet(t, c, live)
	if live.UID != want.UID || *live.Spec.Replicas != 1 || !live.DeletionTimestamp.IsZero() {
		t.Fatalf("readded group lost original workload identity: %+v", live.ObjectMeta)
	}
	retirementCondition(t, &status, metav1.ConditionTrue, "RetirementComplete")
}

func TestRetirementFailureIsIsolatedAndForeignSlotsStayUntouched(t *testing.T) {
	cr, scheme := controllerInput(), retirementScheme(t)
	broken := retirementObjects(t, cr, "a-failing", 0)
	healthy := retirementObjects(t, cr, "b-retiring", 0)
	foreign := retirementObjects(t, cr, "c-foreign", 0)
	for _, object := range foreign {
		owners := object.GetOwnerReferences()
		owners[0].UID = "foreign-cr"
		object.SetOwnerReferences(owners)
	}
	objects := append(append(broken, healthy...), foreign...)
	c := retirementClient(scheme, cr, objects, interceptor.Funcs{Delete: func(ctx context.Context, c client.WithWatch,
		object client.Object, opts ...client.DeleteOption) error {
		if strings.Contains(object.GetName(), "a-failing") {
			return fmt.Errorf("injected first group failure")
		}
		return c.Delete(ctx, object, opts...)
	}})
	r := newTestReconciler(c, scheme, testFacts{})
	status := framework.ReconcileStatus{ObservedGeneration: cr.Generation}
	if _, failures := r.retireGroups(t.Context(), cr, nil, &status); len(failures) != 1 {
		t.Fatalf("wrong isolated failure count: %v", failures)
	}
	err := c.Get(t.Context(), client.ObjectKeyFromObject(healthy[0]), &appsv1.StatefulSet{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("first failure blocked second group retirement: %v", err)
	}
	for _, object := range append(broken, foreign...) {
		applyTestGet(t, c, object.DeepCopyObject().(client.Object))
	}
	retirementCondition(t, &status, metav1.ConditionFalse, "RetirementFailed")
}
