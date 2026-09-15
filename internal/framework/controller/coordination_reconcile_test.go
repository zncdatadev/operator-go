package controller

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	generatedtrino "github.com/zncdatadev/operator-go/internal/framework/pipeline/testinput"
	"github.com/zncdatadev/operator-go/pkg/framework"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestCoordinationSurvivesFinalAssemblyAndOrdersClusterStop(t *testing.T) {
	cr, scheme := controllerInput(), controllerScheme(t)
	if err := json.Unmarshal([]byte(`{"spec":{"coordinators":{"roleGroups":{"default":{"replicas":1}}},
 "workers":{"roleGroups":{"default":{"replicas":1}}}}}`), cr); err != nil {
		t.Fatal(err)
	}
	c := retirementClient(scheme, cr, nil, interceptor.Funcs{Create: func(ctx context.Context,
		c client.WithWatch, object client.Object, opts ...client.CreateOption,
	) error {
		object.SetUID(types.UID(object.GetName() + "-uid"))
		object.SetGeneration(1)
		return c.Create(ctx, object, opts...)
	}})
	r := newTestReconciler(c, scheme, testFacts{})
	r.Definition.GenerateGroup = func(in framework.EffectiveInput[testConfig, testClusterConfig, testFacts]) (
		framework.RuntimeDescription, error,
	) {
		priority := int32(0)
		if in.Group.Role == "coordinators" {
			priority = 100
		}
		return framework.RuntimeDescription{Main: framework.Process{Name: "main", Command: []string{"run"}},
			Initializers: []framework.Process{{Name: "initialize", Command: []string{"initialize"}}},
			Coordination: &framework.WorkloadCoordination{ShutdownPriority: priority,
				ProgressDeadline: metav1.Duration{Duration: time.Minute}}}, nil
	}
	reconcile := func() {
		t.Helper()
		if _, err := r.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(cr)}); err != nil {
			t.Fatal(err)
		}
	}
	reconcile()
	sets, pods := map[string]*appsv1.StatefulSet{}, map[string]*corev1.Pod{}
	for _, role := range []string{"coordinators", "workers"} {
		sts := &appsv1.StatefulSet{}
		key := client.ObjectKey{Namespace: cr.Namespace, Name: cr.Name + "-" + role + "-default"}
		if err := c.Get(t.Context(), key, sts); err != nil {
			t.Fatal(err)
		}
		if policy, err := workloadPolicy(sts); err != nil || policy == nil {
			t.Fatalf("assembled/applied workload lost coordination: %s %+v %v", role, policy, err)
		}
		sts.Status = appsv1.StatefulSetStatus{ObservedGeneration: sts.Generation, Replicas: 1,
			ReadyReplicas: 1, UpdatedReplicas: 1, CurrentRevision: "current", UpdateRevision: "current"}
		if err := c.Status().Update(t.Context(), sts); err != nil {
			t.Fatal(err)
		}
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: sts.Name + "-0", Namespace: sts.Namespace,
			OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: statefulSetKind,
				Name: sts.Name, UID: sts.UID, Controller: ptr.To(true)}}}}
		if err := c.Create(t.Context(), pod); err != nil {
			t.Fatal(err)
		}
		sets[role], pods[role] = sts, pod
	}
	if err := c.Get(t.Context(), client.ObjectKeyFromObject(cr), cr); err != nil {
		t.Fatal(err)
	}
	cr.Spec.ClusterConfig = &generatedtrino.ClusterConfigInput{Stopped: ptr.To(true)}
	cr.Generation++
	if err := c.Update(t.Context(), cr); err != nil {
		t.Fatal(err)
	}
	reconcile()
	assertReplicas := func(role string, want int32) {
		t.Helper()
		if err := c.Get(t.Context(), client.ObjectKeyFromObject(sets[role]), sets[role]); err != nil {
			t.Fatal(err)
		}
		if replicas(sets[role]) != want {
			t.Fatalf("%s replicas=%d want=%d", role, replicas(sets[role]), want)
		}
	}
	assertReplicas("workers", 0)
	assertReplicas("coordinators", 1)
	// A new controller and zero status counts cannot bypass the remaining worker Pod.
	replacement := newTestReconciler(c, scheme, testFacts{})
	replacement.Definition = r.Definition
	r = replacement
	worker := sets["workers"]
	worker.Status = appsv1.StatefulSetStatus{ObservedGeneration: worker.Generation}
	if err := c.Status().Update(t.Context(), worker); err != nil {
		t.Fatal(err)
	}
	reconcile()
	assertReplicas("coordinators", 1)
	if err := c.Delete(t.Context(), pods["workers"]); err != nil {
		t.Fatal(err)
	}
	reconcile()
	assertReplicas("coordinators", 0)
}
