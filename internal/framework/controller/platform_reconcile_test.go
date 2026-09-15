package controller

import (
	"context"
	"fmt"
	"testing"

	generatedtrino "github.com/zncdatadev/operator-go/internal/framework/pipeline/testinput"
	"github.com/zncdatadev/operator-go/pkg/framework"
	"github.com/zncdatadev/operator-go/pkg/framework/input"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// This exercises the real reconcile/build/apply/shared-output call chain. The
// fixture supplies CSI observations; only the kind acceptance proves CSI ran.
func TestPlatformReconcileCreatesProducerThenRefreshesSharedOutput(t *testing.T) {
	cr, scheme := controllerInput(), controllerScheme(t)
	class := platformObject("listeners.kubedoop.dev", "ListenerClass")
	class.SetName("external")
	class.SetUID("external-uid")
	class.SetGeneration(1)
	c := retirementClient(scheme, cr, []client.Object{class}, interceptor.Funcs{
		Create: func(ctx context.Context, c client.WithWatch, object client.Object, opts ...client.CreateOption) error {
			if object.GetUID() == "" {
				object.SetUID(types.UID(object.GetName() + "-uid"))
			}
			if object.GetGeneration() == 0 {
				object.SetGeneration(1)
			}
			return c.Create(ctx, object, opts...)
		},
	})
	r := newTestReconciler(c, scheme, testFacts{})
	r.Binding.Project = func(current *generatedtrino.TrinoCluster) (input.Projection, error) {
		return input.Projection{Cluster: framework.ClusterIdentity{Name: current.Name, Namespace: current.Namespace},
			Roles: []input.Role{{Name: "workers", Groups: []input.Group{{Name: "default", Replicas: ptr.To(int32(1))}}}}}, nil
	}
	r.Definition.GenerateGroup = func(framework.EffectiveInput[testConfig, testClusterConfig, testFacts]) (
		framework.RuntimeDescription, error,
	) {
		return framework.RuntimeDescription{Main: framework.Process{Name: "main", Command: []string{"run"},
			Access: []framework.DirectoryAccess{{Directory: "listener", MountPath: "/listener", ReadOnly: true}}},
			Endpoints:   []framework.Endpoint{{Name: "http", Port: 8080}},
			Directories: []framework.Directory{{Name: "listener", Listener: &framework.ListenerVolume{Class: "external"}}}}, nil
	}
	r.Definition.GenerateCluster = func(in framework.ClusterOutputInput[testClusterConfig, testFacts]) (
		framework.ClusterOutput, error,
	) {
		for _, group := range in.Groups {
			p := group.Platform
			if p != nil && p.Phase == platformObserving && p.Diagnostic.State == framework.FactsResolved &&
				len(p.Listeners) > 0 {
				out := sharedOutput(cr, "discovery")
				out.ConfigMaps[0].Data["value"] = p.Listeners[0].Address
				return out, nil
			}
		}
		return framework.ClusterOutput{State: framework.ClusterOutputPending, Reason: "waiting for actual Listener"}, nil
	}
	if _, errs := r.reconcileShared(t.Context(), cr, sharedOutput(cr, "discovery")); len(errs) != 0 {
		t.Fatal(errs)
	}
	old := sharedRead(t, c, cr, "discovery")
	reconcile := func() {
		t.Helper()
		if _, err := r.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(cr)}); err != nil {
			t.Fatal(err)
		}
	}
	reconcile()
	sts := &appsv1.StatefulSet{}
	key := client.ObjectKey{Namespace: cr.Namespace, Name: cr.Name + "-workers-default"}
	if err := c.Get(t.Context(), key, sts); err != nil {
		t.Fatalf("producer deadlocked behind its own result: %v", err)
	}
	if got := sharedRead(t, c, cr, "discovery"); got.ResourceVersion != old.ResourceVersion {
		t.Fatal("pending result overwrote shared output")
	}
	if sts.Annotations[platformClaimsAnnotation] == "" {
		t.Fatal("platform declaration was not persisted")
	}
	sts.Status = appsv1.StatefulSetStatus{ObservedGeneration: sts.Generation, Replicas: 1, ReadyReplicas: 1,
		UpdatedReplicas: 1, CurrentRevision: "revision-1", UpdateRevision: "revision-1"}
	if err := c.Status().Update(t.Context(), sts); err != nil {
		t.Fatal(err)
	}
	listener := supplyPlatformObservation(t, c, sts)
	reconcile()
	if got := sharedRead(t, c, cr, "discovery"); got.Data["value"] != "first.example" {
		t.Fatalf("late result not published: %+v", got.Data)
	}
	listener.Object["status"] = platformListenerStatus("second.example")
	if err := c.Update(t.Context(), listener); err != nil {
		t.Fatal(err)
	}
	reconcile()
	if got := sharedRead(t, c, cr, "discovery"); got.Data["value"] != "second.example" {
		t.Fatalf("result refresh used stale cache: %+v", got.Data)
	}
	current := &generatedtrino.TrinoCluster{}
	if err := c.Get(t.Context(), client.ObjectKeyFromObject(cr), current); err != nil {
		t.Fatal(err)
	}
	if current.Generation != cr.Generation {
		t.Fatal("platform observation required changing CR")
	}
}

func supplyPlatformObservation(t *testing.T, c client.Client, sts *appsv1.StatefulSet) *unstructured.Unstructured {
	t.Helper()
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: sts.Name + "-0", Namespace: sts.Namespace, UID: "pod-uid",
		Labels: map[string]string{appsv1.ControllerRevisionHashLabelKey: "revision-1"},
		OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "StatefulSet",
			Name: sts.Name, UID: sts.UID, Controller: ptr.To(true)}}},
		Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}}}
	claim := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: pod.Name + "-listener",
		Namespace: sts.Namespace, UID: "claim-uid",
		OwnerReferences: []metav1.OwnerReference{{APIVersion: "v1", Kind: "Pod",
			Name: pod.Name, UID: pod.UID, Controller: ptr.To(true)}}},
		Spec:   corev1.PersistentVolumeClaimSpec{VolumeName: "listener-pv"},
		Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound}}
	pv := &corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "listener-pv", UID: "pv-uid"},
		Spec: corev1.PersistentVolumeSpec{ClaimRef: &corev1.ObjectReference{Name: claim.Name,
			Namespace: claim.Namespace, UID: claim.UID}}}
	listener := platformObject("listeners.kubedoop.dev", "Listener")
	listener.SetName(claim.Name)
	listener.SetNamespace(sts.Namespace)
	listener.SetOwnerReferences([]metav1.OwnerReference{{APIVersion: "v1", Kind: "PersistentVolume",
		Name: pv.Name, UID: pv.UID}})
	listener.Object["status"] = platformListenerStatus("first.example")
	for _, object := range []client.Object{pod, claim, pv, listener} {
		if err := c.Create(t.Context(), object); err != nil {
			t.Fatal(fmt.Errorf("create supplied observation: %w", err))
		}
	}
	return listener
}

func platformListenerStatus(address string) map[string]any {
	return map[string]any{"ingressAddresses": []any{map[string]any{"address": address,
		"ports": map[string]any{"http": int64(8080)}}}}
}
