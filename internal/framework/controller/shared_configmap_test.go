package controller

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/zncdatadev/operator-go/internal/framework/pipeline"
	"github.com/zncdatadev/operator-go/pkg/framework"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func sharedOutput(cr client.Object, names ...string) framework.ClusterOutput {
	output := framework.ClusterOutput{State: framework.ClusterOutputReady}
	for _, name := range names {
		output.ConfigMaps = append(output.ConfigMaps, corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: cr.GetNamespace(), Labels: map[string]string{"app.kubernetes.io/instance": cr.GetName()},
		}, Data: map[string]string{"value": "original"}})
	}
	return output
}
func sharedRead(t *testing.T, c client.Client, cr client.Object, name string) *corev1.ConfigMap {
	t.Helper()
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: cr.GetNamespace()}}
	applyTestGet(t, c, cm)
	return cm
}
func sharedGone(t *testing.T, c client.Client, cr client.Object, name string) {
	t.Helper()
	err := c.Get(t.Context(), client.ObjectKey{Name: name, Namespace: cr.GetNamespace()}, &corev1.ConfigMap{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected shared %s gone: %v", name, err)
	}
}

func TestSharedCompleteSetWithdrawalSurvivesControllerRestart(t *testing.T) {
	r, cr := factsTestReconciler(t, nil)
	if pending, errs := r.reconcileShared(t.Context(), cr, sharedOutput(cr, "one", "two")); pending || len(errs) != 0 {
		t.Fatalf("create shared set: %t %v", pending, errs)
	}
	live := sharedRead(t, r.Client, cr, "one")
	version := live.ResourceVersion
	if live.Annotations[SharedConfigMapAnnotation] == "" || metav1.GetControllerOf(live).UID != cr.UID {
		t.Fatal("source/owner receipt missing")
	}
	if pending, errs := r.reconcileShared(t.Context(), cr, sharedOutput(cr, "one", "two")); pending || len(errs) != 0 {
		t.Fatalf("no-op shared set: %t %v", pending, errs)
	}
	if got := sharedRead(t, r.Client, cr, "one"); got.ResourceVersion != version {
		t.Fatal("steady shared output rewrote object")
	}
	r = newTestReconciler(r.Client, r.Scheme, testFacts{})
	if pending, errs := r.reconcileShared(t.Context(), cr, sharedOutput(cr, "two")); !pending || len(errs) != 0 {
		t.Fatalf("withdrawal must await observed absence: %t %v", pending, errs)
	}
	sharedGone(t, r.Client, cr, "one")
	if pending, errs := r.reconcileShared(t.Context(), cr, sharedOutput(cr)); !pending || len(errs) != 0 {
		t.Fatalf("empty Ready did not withdraw remaining output: %t %v", pending, errs)
	}
	sharedGone(t, r.Client, cr, "two")
	if pending, errs := r.reconcileShared(t.Context(), cr, sharedOutput(cr)); pending || len(errs) != 0 {
		t.Fatalf("withdrawn set did not settle: %t %v", pending, errs)
	}
}

func TestSharedPendingInvalidAndErrorKeepExistingOutputs(t *testing.T) {
	for _, mode := range []string{"pending", "invalid-state", "invalid-partial", "error"} {
		t.Run(mode, func(t *testing.T) {
			r, cr := factsTestReconciler(t, nil)
			if _, errs := r.reconcileShared(t.Context(), cr, sharedOutput(cr, "old")); len(errs) != 0 {
				t.Fatal(errs)
			}
			before := sharedRead(t, r.Client, cr, "old")
			plan := pipeline.ResourcePlan[testConfig, testClusterConfig, testFacts]{ClusterOutput: sharedOutput(cr)}
			switch mode {
			case "pending":
				plan.ClusterOutput = framework.ClusterOutput{State: framework.ClusterOutputPending, Reason: "waiting"}
			case "invalid-state":
				plan.ClusterOutput.State = ""
			case "invalid-partial":
				plan.ClusterOutput = sharedOutput(cr, "new")
				plan.ClusterOutput.State = framework.ClusterOutputPending
				plan.ClusterOutput.Reason = "waiting"
			case "error":
				plan.ClusterError = "product failed"
			}
			status := framework.ReconcileStatus{ObservedGeneration: cr.Generation}
			waiting, errs := r.applyPlan(t.Context(), cr, plan, &status)
			if (mode == "pending") != waiting || (mode != "pending") != (len(errs) > 0) {
				t.Fatalf("%t %v", waiting, errs)
			}
			after := sharedRead(t, r.Client, cr, "old")
			if before.ResourceVersion != after.ResourceVersion || before.UID != after.UID {
				t.Fatal("unavailable output changed old set")
			}
			if mode == "pending" && meta.FindStatusCondition(status.Conditions, "Applied").Message != "waiting" {
				t.Fatal("pending output reason lost")
			}
			if meta.IsStatusConditionTrue(status.Conditions, "Applied") {
				t.Fatal("pending/error reported applied")
			}
			sharedGone(t, r.Client, cr, "new")
		})
	}
}

func TestSharedRejectsForeignDamagedAndCrossSlotObjects(t *testing.T) {
	for _, damage := range []string{"foreign-owner", "owner-kind", "missing-receipt", "missing-record", "copied-name",
		"wrong-cruid", "identity-label", "group-slot", "duplicate-key", "null-receipt"} {
		t.Run(damage, func(t *testing.T) {
			r, cr := factsTestReconciler(t, nil)
			if _, errs := r.reconcileShared(t.Context(), cr, sharedOutput(cr, "old")); len(errs) != 0 {
				t.Fatal(errs)
			}
			live := sharedRead(t, r.Client, cr, "old")
			switch damage {
			case "foreign-owner":
				live.OwnerReferences[0].UID = "foreign"
			case "owner-kind":
				live.OwnerReferences[0].Kind = "Other"
			case "missing-receipt":
				delete(live.Annotations, SharedConfigMapAnnotation)
			case "missing-record":
				delete(live.Annotations, ManagedMetadataAnnotation)
			case "copied-name":
				live.Annotations[SharedConfigMapAnnotation] = storageJSON(t, sharedSlot(cr, "different"))
			case "wrong-cruid":
				slot := sharedSlot(cr, "old")
				slot.CRUID = "different"
				live.Annotations[SharedConfigMapAnnotation] = storageJSON(t, slot)
			case "identity-label":
				live.Labels["app.kubernetes.io/instance"] = "other"
			case "group-slot":
				live.Annotations[GroupSlotAnnotation] = storageJSON(t, groupSlot{Role: "workers", Group: "old",
					Slot: slotConfigmap})
			case "duplicate-key":
				live.Annotations[SharedConfigMapAnnotation] = `{"version":1,"version":1,"crUID":"original-uid","name":"old"}`
			case "null-receipt":
				live.Annotations[SharedConfigMapAnnotation] = "null"
			}
			if err := r.Client.Update(t.Context(), live); err != nil {
				t.Fatal(err)
			}
			version := sharedRead(t, r.Client, cr, "old").ResourceVersion
			if _, errs := r.reconcileShared(t.Context(), cr, sharedOutput(cr, "old", "independent")); len(errs) == 0 {
				t.Fatal("damaged shared slot was adopted")
			}
			sharedRead(t, r.Client, cr, "independent")
			if _, errs := r.reconcileShared(t.Context(), cr, sharedOutput(cr)); len(errs) == 0 && damage != "foreign-owner" {
				t.Fatal("damaged own slot was silently accepted")
			}
			if got := sharedRead(t, r.Client, cr, "old"); got.ResourceVersion != version {
				t.Fatal("damaged/foreign source was changed")
			}
		})
	}
}

func TestSharedNeverAdoptsOrDeletesCustomOwnerConfigMap(t *testing.T) {
	r, cr := factsTestReconciler(t, nil)
	custom := sharedOutput(cr, "custom").ConfigMaps[0]
	if _, err := ApplyObject(t.Context(), r.Client, cr, &custom, r.Scheme); err != nil {
		t.Fatal(err)
	}
	before := sharedRead(t, r.Client, cr, "custom")
	if _, errs := r.reconcileShared(t.Context(), cr, sharedOutput(cr, "custom")); len(errs) == 0 {
		t.Fatal("custom CM adopted as shared")
	}
	if pending, errs := r.reconcileShared(t.Context(), cr, sharedOutput(cr)); pending || len(errs) != 0 {
		t.Fatalf("custom was inventory: %t %v", pending, errs)
	}
	after := sharedRead(t, r.Client, cr, "custom")
	if before.ResourceVersion != after.ResourceVersion {
		t.Fatal("custom changed")
	}
	if _, errs := r.reconcileShared(t.Context(), cr, sharedOutput(cr, "owned")); len(errs) != 0 {
		t.Fatal(errs)
	}
	live := sharedRead(t, r.Client, cr, "owned")
	delete(live.Annotations, SharedConfigMapAnnotation)
	if err := r.Client.Update(t.Context(), live); err != nil {
		t.Fatal(err)
	}
	desired := sharedOutput(cr, "owned").ConfigMaps[0]
	if _, err := ApplyObject(t.Context(), r.Client, cr, &desired, r.Scheme); err == nil {
		t.Fatal("unscoped apply overwrote damaged shared slot")
	}
}

func TestSharedTerminatingReaddWaitsAndChecksSource(t *testing.T) {
	r, cr := factsTestReconciler(t, nil)
	if _, errs := r.reconcileShared(t.Context(), cr, sharedOutput(cr, "one")); len(errs) != 0 {
		t.Fatal(errs)
	}
	live := sharedRead(t, r.Client, cr, "one")
	live.Finalizers = []string{"test.framework/hold"}
	if err := r.Client.Update(t.Context(), live); err != nil {
		t.Fatal(err)
	}
	if _, errs := r.reconcileShared(t.Context(), cr, sharedOutput(cr)); len(errs) != 0 {
		t.Fatal(errs)
	}
	live = sharedRead(t, r.Client, cr, "one")
	if live.DeletionTimestamp.IsZero() {
		t.Fatal("test did not start deletion")
	}
	if pending, errs := r.reconcileShared(t.Context(), cr, sharedOutput(cr, "one")); !pending || len(errs) != 0 {
		t.Fatalf("readd stole terminating source: %t %v", pending, errs)
	}
	if got := sharedRead(t, r.Client, cr, "one"); got.UID != live.UID || got.ResourceVersion != live.ResourceVersion {
		t.Fatal("terminating source changed")
	}
}

func TestSharedDeleteRetryRechecksIntentAndSource(t *testing.T) {
	for _, change := range []string{"generation", "pause", "replacement"} {
		t.Run(change, func(t *testing.T) {
			cr, scheme := controllerInput(), controllerScheme(t)
			deletes := 0
			c := retirementClient(scheme, cr, nil, interceptor.Funcs{Delete: func(ctx context.Context, c client.WithWatch,
				object client.Object, options ...client.DeleteOption) error {
				deletes++
				opts := (&client.DeleteOptions{}).ApplyOptions(options)
				if opts.Preconditions == nil || opts.Preconditions.UID == nil || opts.Preconditions.ResourceVersion == nil {
					t.Fatal("delete lacks exact identity preconditions")
				}
				if change == "replacement" {
					fresh := object.DeepCopyObject().(client.Object)
					fresh.SetUID("replacement")
					fresh.SetOwnerReferences(nil)
					if err := c.Update(ctx, fresh); err != nil {
						return err
					}
				} else {
					current := cr.DeepCopy()
					if err := c.Get(ctx, client.ObjectKeyFromObject(cr), current); err != nil {
						return err
					}
					if change == "generation" {
						current.Generation++
					} else {
						if err := pauseOperationWithoutGenerationChange(ctx, c, cr); err != nil {
							return err
						}
						return apierrors.NewConflict(schema.GroupResource{Resource: "configmaps"}, object.GetName(),
							fmt.Errorf("pause race"))
					}
					if err := c.Update(ctx, current); err != nil {
						return err
					}
				}
				return apierrors.NewConflict(schema.GroupResource{Resource: "configmaps"}, object.GetName(), fmt.Errorf("race"))
			}})
			r := newTestReconciler(c, scheme, testFacts{})
			if _, errs := r.reconcileShared(t.Context(), cr, sharedOutput(cr, "one")); len(errs) != 0 {
				t.Fatal(errs)
			}
			_, errs := r.reconcileShared(t.Context(), cr, sharedOutput(cr))
			if len(errs) == 0 || deletes != 1 {
				t.Fatalf("retry bypassed changed authority: calls=%d errs=%v", deletes, errs)
			}
			if change == "generation" && !errors.Is(errors.Join(errs...), errSuperseded) {
				t.Fatal(errs)
			}
			sharedRead(t, c, cr, "one")
		})
	}
}

func TestSharedGroupLookingMetadataRemainsOutsideWorkloadInventory(t *testing.T) {
	r, cr := factsTestReconciler(t, nil)
	name := cr.Name + "-workers-default-discovery"
	output := sharedOutput(cr, name)
	output.ConfigMaps[0].Labels["app.kubernetes.io/component"] = "workers"
	output.ConfigMaps[0].Labels["role-group"] = "default"
	if _, errs := r.reconcileShared(t.Context(), cr, output); len(errs) != 0 {
		t.Fatal(errs)
	}
	initial := sharedRead(t, r.Client, cr, name)
	groups, failures := r.retirementInventory(t.Context(), cr)
	if len(failures) != 0 || len(groups) != 0 {
		t.Fatalf("validated shared source became a workload candidate: %v %v", groups, failures)
	}
	if pending, errs := r.stopWorkloads(t.Context(), cr, nil); pending || len(errs) != 0 {
		t.Fatalf("shared source blocked stopped observation: %t %v", pending, errs)
	}
	status := framework.ReconcileStatus{ObservedGeneration: cr.Generation}
	if pending, errs := r.retireGroups(t.Context(), cr, nil, &status); pending || len(errs) != 0 {
		t.Fatalf("shared source blocked group retirement: %t %v", pending, errs)
	}
	if !meta.IsStatusConditionTrue(status.Conditions, "Retired") {
		t.Fatalf("retirement: %+v", status)
	}
	after := sharedRead(t, r.Client, cr, name)
	if after.UID != initial.UID || after.ResourceVersion != initial.ResourceVersion {
		t.Fatal("workload lifecycle mutated a shared ConfigMap")
	}
	slots, errs := r.sharedInventory(t.Context(), cr)
	if len(errs) != 0 || len(slots) != 1 || slots[0].Name != name {
		t.Fatalf("shared source disappeared from its own inventory: %v %v", slots, errs)
	}
}
