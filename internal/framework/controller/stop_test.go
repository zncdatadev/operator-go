package controller

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zncdatadev/operator-go/pkg/framework"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func stopNoCreateOrDelete(t *testing.T) interceptor.Funcs {
	t.Helper()
	return interceptor.Funcs{
		Create: func(context.Context, client.WithWatch, client.Object, ...client.CreateOption) error {
			t.Fatal("stopping must not create resources")
			return nil
		},
		Delete: func(context.Context, client.WithWatch, client.Object, ...client.DeleteOption) error {
			t.Fatal("stopping must not delete resources")
			return nil
		},
	}
}

func TestStopWorkloadsUsesLiveInventoryWithoutSourceAndNeverDeletes(t *testing.T) {
	cr, scheme := controllerInput(), retirementScheme(t)
	one, two := retirementObjects(t, cr, "one", 1), retirementObjects(t, cr, "two", 3)
	foreign := retirementObjects(t, cr, "foreign", 2)[0].(*appsv1.StatefulSet)
	foreign.OwnerReferences[0].UID = "another-cr"
	objects := append(append(one, two...), foreign)
	c := retirementClient(scheme, cr, objects, stopNoCreateOrDelete(t))
	r := newTestReconciler(c, scheme, testFacts{})
	pending, failures := r.stopWorkloads(t.Context(), cr, nil)
	if !pending || len(failures) != 0 {
		t.Fatalf("live-only stop failed: pending=%t errors=%v", pending, failures)
	}
	for _, original := range []*appsv1.StatefulSet{one[0].(*appsv1.StatefulSet), two[0].(*appsv1.StatefulSet)} {
		live := original.DeepCopy()
		applyTestGet(t, c, live)
		if *live.Spec.Replicas != 0 || live.UID != original.UID {
			t.Fatalf("live inventory was not stopped in place: %+v", live)
		}
		live.Status = appsv1.StatefulSetStatus{ObservedGeneration: live.Generation}
		if err := c.Status().Update(t.Context(), live); err != nil {
			t.Fatal(err)
		}
	}
	identities := []framework.GroupIdentity{{ClusterIdentity: framework.ClusterIdentity{Name: cr.Name,
		Namespace: cr.Namespace},
		Role: "workers", Name: "one", Replicas: 7}}
	pending, failures = r.stopWorkloads(t.Context(), cr, identities)
	if pending || len(failures) != 0 || identities[0].Replicas != 7 {
		t.Fatalf("complete stop changed declarations or remains pending: %t %v %+v", pending, failures, identities)
	}
	for _, object := range objects {
		applyTestGet(t, c, object.DeepCopyObject().(client.Object))
	}
	applyTestGet(t, c, foreign)
	if *foreign.Spec.Replicas != 2 {
		t.Fatal("another CR's workload was stopped")
	}
}

func TestStopWorkloadsRequiresZeroObservationAndActualPodAbsence(t *testing.T) {
	cases := []string{"complete", "unobserved", "replicas", "ready", "updated", "terminating",
		"pod", "old-owner-pod", "missing-sts-pod", "missing-sts"}
	for _, state := range cases {
		t.Run(state, func(t *testing.T) {
			cr, scheme := controllerInput(), retirementScheme(t)
			objects := retirementObjects(t, cr, "one", 0)
			set := objects[0].(*appsv1.StatefulSet)
			switch state {
			case "unobserved":
				set.Status.ObservedGeneration--
			case "replicas":
				set.Status.Replicas = 1
			case "ready":
				set.Status.ReadyReplicas = 1
			case "updated":
				set.Status.UpdatedReplicas = 1
			case "terminating":
				now := metav1.Now()
				set.DeletionTimestamp, set.Finalizers = &now, []string{"fixture.design/hold"}
			}
			if strings.Contains(state, "pod") {
				pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: set.Name + "-0", Namespace: cr.Namespace}}
				if state == "old-owner-pod" {
					controller, now := true, metav1.Now()
					pod.Name = "old-pod-with-edited-name"
					pod.DeletionTimestamp, pod.Finalizers = &now, []string{"fixture.design/hold"}
					pod.OwnerReferences = []metav1.OwnerReference{{APIVersion: appsv1.SchemeGroupVersion.String(),
						Kind: statefulSetKind, Name: set.Name, UID: "old-set-uid", Controller: &controller}}
				}
				objects = append(objects, pod)
			}
			if strings.HasPrefix(state, "missing-sts") {
				objects = objects[1:]
			}
			intercept := stopNoCreateOrDelete(t)
			intercept.Update = func(context.Context, client.WithWatch, client.Object, ...client.UpdateOption) error {
				t.Fatal("an already stopped or absent StatefulSet must not be rewritten")
				return nil
			}
			c := retirementClient(scheme, cr, objects, intercept)
			r := newTestReconciler(c, scheme, testFacts{})
			pending, failures := r.stopWorkloads(t.Context(), cr, []framework.GroupIdentity{{Role: "workers", Name: "one"}})
			want := state != "complete" && state != "missing-sts"
			if pending != want || len(failures) != 0 {
				t.Fatalf("observation %s: pending=%t errors=%v", state, pending, failures)
			}
		})
	}
}

func TestStopWorkloadsRejectsDamagedReceiptsAndContinuesOtherGroups(t *testing.T) {
	cases := []string{"missing", "duplicate", "wrong-slot", "wrong-label", "missing-record", "wrong-owner-kind"}
	for _, damage := range cases {
		t.Run(damage, func(t *testing.T) {
			cr, scheme := controllerInput(), retirementScheme(t)
			bad := retirementObjects(t, cr, "bad", 1)[0].(*appsv1.StatefulSet)
			good := retirementObjects(t, cr, "good", 1)[0].(*appsv1.StatefulSet)
			switch damage {
			case "missing":
				delete(bad.Annotations, GroupSlotAnnotation)
			case "duplicate":
				bad.Annotations[GroupSlotAnnotation] = `{"role":"workers","role":"workers","group":"bad","slot":"statefulset"}`
			case "wrong-slot":
				bad.Annotations[GroupSlotAnnotation] = storageJSON(t, groupSlot{Role: "workers", Group: "bad", Slot: slotService})
			case "wrong-label":
				bad.Labels["role-group"] = "other"
			case "missing-record":
				delete(bad.Annotations, ManagedMetadataAnnotation)
			case "wrong-owner-kind":
				bad.OwnerReferences[0].Kind = "AnotherKind"
			}
			c := retirementClient(scheme, cr, []client.Object{bad, good}, stopNoCreateOrDelete(t))
			r := newTestReconciler(c, scheme, testFacts{})
			pending, failures := r.stopWorkloads(t.Context(), cr, nil)
			if !pending || len(failures) == 0 {
				t.Fatalf("damaged inventory was hidden or stopped other work: %t %v", pending, failures)
			}
			applyTestGet(t, c, bad)
			applyTestGet(t, c, good)
			if *bad.Spec.Replicas != 1 || *good.Spec.Replicas != 0 {
				t.Fatal("damaged workload was changed or independent workload was blocked")
			}
		})
	}
}

func TestStopWorkloadsRequiresSafeRetainedStorage(t *testing.T) {
	for _, damage := range []string{"none", "when-scaled", "when-deleted", "claim-owner", "missing-source"} {
		t.Run(damage, func(t *testing.T) {
			f := retainedFixture(t)
			switch damage {
			case "when-scaled":
				f.sts.Spec.PersistentVolumeClaimRetentionPolicy.WhenScaled = appsv1.DeletePersistentVolumeClaimRetentionPolicyType
			case "when-deleted":
				f.sts.Spec.PersistentVolumeClaimRetentionPolicy.WhenDeleted = appsv1.DeletePersistentVolumeClaimRetentionPolicyType
			case "claim-owner":
				f.claim.OwnerReferences = []metav1.OwnerReference{{UID: f.sts.UID}}
			case "missing-source":
				delete(f.sts.Spec.VolumeClaimTemplates[0].Annotations, retainedSourceAnnotation)
			}
			c := fake.NewClientBuilder().WithScheme(controllerScheme(t)).
				WithObjects(f.cr, f.class, f.sts, f.claim, f.pv).WithStatusSubresource(&appsv1.StatefulSet{}).
				WithInterceptorFuncs(stopNoCreateOrDelete(t)).Build()
			r := newTestReconciler(c, c.Scheme(), testFacts{})
			pending, failures := r.stopWorkloads(t.Context(), f.cr, nil)
			live := f.sts.DeepCopy()
			applyTestGet(t, c, live)
			if damage != "none" {
				if len(failures) == 0 || *live.Spec.Replicas != 1 {
					t.Fatalf("unsafe storage was stopped: %v %+v", failures, live.Spec)
				}
				return
			}
			if !pending || len(failures) != 0 || *live.Spec.Replicas != 0 {
				t.Fatalf("same-source retained workload failed to stop: %t %v", pending, failures)
			}
			claim := f.claim.DeepCopy()
			applyTestGet(t, c, claim)
			if claim.UID != f.claim.UID || claim.Annotations[retainedBindingAnnotation] == "" ||
				len(claim.OwnerReferences) != 0 {
				t.Fatalf("stop lost binding evidence or altered claim identity: %+v", claim)
			}
			live.Status = appsv1.StatefulSetStatus{ObservedGeneration: live.Generation}
			if err := c.Status().Update(t.Context(), live); err != nil {
				t.Fatal(err)
			}
			if pending, failures = r.stopWorkloads(t.Context(), f.cr, nil); pending || len(failures) != 0 {
				t.Fatalf("retained stop never completed: %t %v", pending, failures)
			}
			applyTestGet(t, c, f.sts.DeepCopy())
			applyTestGet(t, c, f.pv.DeepCopy())
		})
	}
}

func TestStopWorkloadsSupersededConflictStopsThePass(t *testing.T) {
	cr, scheme := controllerInput(), retirementScheme(t)
	first := retirementObjects(t, cr, "first", 1)[0].(*appsv1.StatefulSet)
	second := retirementObjects(t, cr, "second", 1)[0].(*appsv1.StatefulSet)
	writes := 0
	intercept := stopNoCreateOrDelete(t)
	intercept.Update = func(ctx context.Context, c client.WithWatch, object client.Object,
		_ ...client.UpdateOption) error {
		writes++
		current := cr.DeepCopy()
		if err := c.Get(ctx, client.ObjectKeyFromObject(cr), current); err != nil {
			return err
		}
		current.Generation++
		if err := c.Update(ctx, current); err != nil {
			return err
		}
		return apierrors.NewConflict(schema.GroupResource{Resource: "statefulsets"}, object.GetName(),
			errors.New("CR changed during stop"))
	}
	c := retirementClient(scheme, cr, []client.Object{first, second}, intercept)
	r := newTestReconciler(c, scheme, testFacts{})
	_, failures := r.stopWorkloads(t.Context(), cr, nil)
	if !errors.Is(errors.Join(failures...), errSuperseded) || writes != 1 {
		t.Fatalf("superseded stop continued: writes=%d errors=%v", writes, failures)
	}
	for _, set := range []*appsv1.StatefulSet{first, second} {
		applyTestGet(t, c, set)
		if *set.Spec.Replicas != 1 {
			t.Fatal("stale stop was persisted")
		}
	}
}

func TestStopWorkloadsEmptyInventoryAndFailedInventoryDiffer(t *testing.T) {
	for _, failList := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty", true: "list-failed"}[failList], func(t *testing.T) {
			cr, scheme := controllerInput(), retirementScheme(t)
			intercept := stopNoCreateOrDelete(t)
			if failList {
				intercept.List = func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error {
					return errors.New("inventory unavailable")
				}
			}
			c := retirementClient(scheme, cr, nil, intercept)
			r := newTestReconciler(c, scheme, testFacts{})
			pending, failures := r.stopWorkloads(t.Context(), cr, nil)
			if pending || (len(failures) != 0) != failList {
				t.Fatalf("inventory absence and failure were conflated: %t %v", pending, failures)
			}
		})
	}
}

func TestStopWorkloadsReobservesStatefulSetAfterPodList(t *testing.T) {
	for _, change := range []string{"scaled-up", "replacement", "appeared"} {
		t.Run(change, func(t *testing.T) {
			cr, scheme := controllerInput(), retirementScheme(t)
			set := retirementObjects(t, cr, "one", 0)[0].(*appsv1.StatefulSet)
			var objects []client.Object
			if change != "appeared" {
				objects = append(objects, set)
			}
			intercept := stopNoCreateOrDelete(t)
			intercept.List = func(ctx context.Context, c client.WithWatch, list client.ObjectList,
				options ...client.ListOption) error {
				if _, pods := list.(*corev1.PodList); pods {
					fresh := set.DeepCopy()
					if change == "appeared" {
						fresh.ResourceVersion = ""
						if err := c.Create(ctx, fresh); err != nil {
							return err
						}
					} else {
						if err := c.Get(ctx, client.ObjectKeyFromObject(set), fresh); err != nil {
							return err
						}
						if change == "scaled-up" {
							one := int32(1)
							fresh.Spec.Replicas = &one
						} else {
							fresh.UID = "replacement-set"
						}
						if err := c.Update(ctx, fresh); err != nil {
							return err
						}
					}
				}
				return c.List(ctx, list, options...)
			}
			c := retirementClient(scheme, cr, objects, intercept)
			r := newTestReconciler(c, scheme, testFacts{})
			pending, failures := r.stopWorkloads(t.Context(), cr, []framework.GroupIdentity{{Role: "workers", Name: "one"}})
			if !pending || len(failures) != 0 {
				t.Fatalf("StatefulSet change during Pod observation was missed: pending=%t errors=%v", pending, failures)
			}
		})
	}
}

func TestStopWorkloadsInventoryFailureStillStopsDeclaredAuthenticatedTarget(t *testing.T) {
	cr, scheme := controllerInput(), retirementScheme(t)
	set := retirementObjects(t, cr, "one", 1)[0].(*appsv1.StatefulSet)
	intercept := stopNoCreateOrDelete(t)
	intercept.List = func(ctx context.Context, c client.WithWatch, list client.ObjectList,
		options ...client.ListOption) error {
		if _, sets := list.(*appsv1.StatefulSetList); sets {
			return errors.New("StatefulSet inventory unavailable")
		}
		return c.List(ctx, list, options...)
	}
	c := retirementClient(scheme, cr, []client.Object{set}, intercept)
	r := newTestReconciler(c, scheme, testFacts{})
	pending, failures := r.stopWorkloads(t.Context(), cr, []framework.GroupIdentity{{Role: "workers", Name: "one"}})
	if !pending || len(failures) != 1 || !strings.Contains(failures[0].Error(), "inventory unavailable") {
		t.Fatalf("inventory failure or independent progress was lost: %t %v", pending, failures)
	}
	applyTestGet(t, c, set)
	if *set.Spec.Replicas != 0 {
		t.Fatal("an inventory list failure blocked the authenticated declared target")
	}
}

func TestStopWorkloadsObservesPodsFromSurvivingSlotsWithoutStatefulSet(t *testing.T) {
	for name, index := range map[string]int{"service": 1, "configmap": 3} {
		t.Run(name, func(t *testing.T) {
			cr, scheme := controllerInput(), retirementScheme(t)
			survivor := retirementObjects(t, cr, "removed", 0)[index]
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
				Name: cr.Name + "-workers-removed-0", Namespace: cr.Namespace}}
			raw := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cr, survivor, pod).Build()
			c := interceptor.NewClient(raw, stopNoCreateOrDelete(t))
			r := newTestReconciler(c, scheme, testFacts{})
			// This group has left the declared identities and its StatefulSet is
			// absent. The surviving authenticated slot must retain Pod observation.
			pending, failures := r.stopWorkloads(t.Context(), cr, nil)
			if !pending || len(failures) != 0 {
				t.Fatalf("removed group's residual Pod was not observed: pending=%t errors=%v", pending, failures)
			}
			// Simulate another actor completing Pod deletion, outside the guarded
			// stop client. The stop path itself must never issue a delete.
			if err := raw.Delete(t.Context(), pod); err != nil {
				t.Fatal(err)
			}
			pending, failures = r.stopWorkloads(t.Context(), cr, nil)
			if pending || len(failures) != 0 {
				t.Fatalf("remaining non-workload slot blocked a completed stop: pending=%t errors=%v", pending, failures)
			}
			applyTestGet(t, c, survivor.DeepCopyObject().(client.Object))
		})
	}
}
