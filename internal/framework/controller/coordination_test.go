package controller

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/zncdatadev/operator-go/pkg/framework"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestCoordinationScaleDownWaitsForOriginalPodAndSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	cr, scheme := controllerInput(), controllerScheme(t)
	objects := retirementObjects(t, cr, "default", 3)
	set := objects[0].(*appsv1.StatefulSet)
	policy, _ := json.Marshal(framework.WorkloadCoordination{ProgressDeadline: metav1.Duration{Duration: time.Minute}})
	set.Annotations[coordinationAnnotation] = string(policy)
	controller := true
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: set.Name + "-2", Namespace: set.Namespace,
		UID: "old-pod", OwnerReferences: []metav1.OwnerReference{{
			APIVersion: appsv1.SchemeGroupVersion.String(), Kind: statefulSetKind, Name: set.Name,
			UID: set.UID, Controller: &controller}}}}
	objects = append(objects, pod)
	c := retirementClient(scheme, cr, objects, interceptor.Funcs{})
	r := newTestReconciler(c, scheme, testFacts{})
	slot := groupSlot{Role: "workers", Group: "default", Slot: slotStatefulset}
	if waiting, err := r.stopWorkload(ctx, cr, slot); err != nil || !waiting {
		t.Fatalf("first stop: %v %v", waiting, err)
	}
	if err := c.Get(ctx, client.ObjectKeyFromObject(set), set); err != nil {
		t.Fatal(err)
	}
	if replicas(set) != 2 {
		t.Fatalf("jumped ordinals: %d", replicas(set))
	}
	// A new controller cannot assume the previous process finished from the count.
	r = newTestReconciler(c, scheme, testFacts{})
	if _, err := r.stopWorkload(ctx, cr, slot); err != nil {
		t.Fatal(err)
	}
	if err := c.Get(ctx, client.ObjectKeyFromObject(set), set); err != nil {
		t.Fatal(err)
	}
	if replicas(set) != 2 {
		t.Fatal("remaining original Pod was bypassed after restart")
	}
	if err := c.Delete(ctx, pod); err != nil {
		t.Fatal(err)
	}
	set.Status.Replicas = 2
	set.Status.ReadyReplicas = 2
	set.Status.UpdatedReplicas = 2
	if err := c.Status().Update(ctx, set); err != nil {
		t.Fatal(err)
	}
	if _, err := r.stopWorkload(ctx, cr, slot); err != nil {
		t.Fatal(err)
	}
	if err := c.Get(ctx, client.ObjectKeyFromObject(set), set); err != nil {
		t.Fatal(err)
	}
	if replicas(set) != 1 {
		t.Fatalf("did not continue after observed disappearance: %d", replicas(set))
	}
}

func TestCoordinationInitializationDeadlinePersistsAndDoesNotForceDelete(t *testing.T) {
	ctx := context.Background()
	cr, scheme := controllerInput(), controllerScheme(t)
	objects := retirementObjects(t, cr, "default", 1)
	set := objects[0].(*appsv1.StatefulSet)
	policy, _ := json.Marshal(framework.WorkloadCoordination{ProgressDeadline: metav1.Duration{Duration: time.Second}})
	set.Annotations[coordinationAnnotation] = string(policy)
	set.Status.ReadyReplicas = 0
	set.Status.UpdatedReplicas = 0
	c := retirementClient(scheme, cr, objects, interceptor.Funcs{})
	r := newTestReconciler(c, scheme, testFacts{})
	if err := r.observeCoordination(ctx, cr, set, 1, false); err != nil {
		t.Fatal(err)
	}
	if err := c.Get(ctx, client.ObjectKeyFromObject(set), set); err != nil {
		t.Fatal(err)
	}
	var receipt workloadProgress
	if err := json.Unmarshal([]byte(set.Annotations[coordinationProgressAnnotation]), &receipt); err != nil {
		t.Fatal(err)
	}
	receipt.Since = metav1.NewTime(time.Now().Add(-time.Minute))
	raw, _ := json.Marshal(receipt)
	set.Annotations[coordinationProgressAnnotation] = string(raw)
	if err := c.Update(ctx, set); err != nil {
		t.Fatal(err)
	}
	r = newTestReconciler(c, scheme, testFacts{})
	err := r.observeCoordination(ctx, cr, set, 1, false)
	if err == nil || !strings.Contains(err.Error(), "deadline exceeded") {
		t.Fatalf("restart reset deadline: %v", err)
	}
	if err := c.Get(ctx, client.ObjectKeyFromObject(set), set); err != nil {
		t.Fatal("timed-out workload was removed", err)
	}
	if replicas(set) != 1 {
		t.Fatal("timeout forced a replica change")
	}
	if err := r.observeCoordination(ctx, cr, set, 1, true); err != nil {
		t.Fatal(err)
	}
	if err := c.Get(ctx, client.ObjectKeyFromObject(set), set); err != nil {
		t.Fatal(err)
	}
	if set.Annotations[coordinationProgressAnnotation] != "" {
		t.Fatal("observed recovery retained stale deadline")
	}
}
