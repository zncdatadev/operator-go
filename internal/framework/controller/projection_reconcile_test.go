package controller

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	generatedtrino "github.com/zncdatadev/operator-go/internal/framework/pipeline/testinput"
	"github.com/zncdatadev/operator-go/pkg/framework"
	"github.com/zncdatadev/operator-go/pkg/framework/input"
	appsv1 "k8s.io/api/apps/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestPauseReadsOperationBeforeProjectionAndPreservesExecutionGeneration(t *testing.T) {
	cr, scheme := controllerInput(), controllerScheme(t)
	paused := true
	cr.Generation = 5
	cr.Spec.ClusterConfig = &generatedtrino.ClusterConfigInput{ReconciliationPaused: &paused}
	cr.Status = framework.ReconcileStatus{ObservedGeneration: 4, Groups: []framework.GroupReconcileStatus{
		{Role: "workers", Name: "previous", DesiredReplicas: 2, Applied: true},
	}}
	cr.Status.Conditions = []metav1.Condition{{Type: "Built", Status: metav1.ConditionTrue, ObservedGeneration: 4,
		Reason: "ResourcesBuilt", LastTransitionTime: metav1.NewTime(time.Unix(1, 0))}}
	reads := 0
	c := retirementClient(scheme, cr, nil, interceptor.Funcs{Get: func(ctx context.Context, c client.WithWatch,
		key client.ObjectKey, object client.Object, opts ...client.GetOption) error {
		if _, ok := object.(*generatedtrino.TrinoCluster); !ok {
			t.Fatal("paused reconciliation read child resources")
		}
		reads++
		return c.Get(ctx, key, object, opts...)
	}})
	r := newTestReconciler(c, scheme, testFacts{})
	r.Binding.Project = func(*generatedtrino.TrinoCluster) (input.Projection, error) {
		t.Fatal("pause tried full projection")
		return input.Projection{}, nil
	}
	request := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(cr)}
	for pass := range 2 {
		before := cr.DeepCopy()
		applyTestGet(t, c, before)
		result, err := r.Reconcile(t.Context(), request)
		if err != nil || !result.IsZero() {
			t.Fatalf("pause scheduled work: %v %v", result, err)
		}
		current := cr.DeepCopy()
		applyTestGet(t, c, current)
		built := meta.FindStatusCondition(current.Status.Conditions, "Built")
		pause := meta.FindStatusCondition(current.Status.Conditions, "Paused")
		if built.ObservedGeneration != 4 || pause.ObservedGeneration != 5 || current.Status.ObservedGeneration != 5 ||
			len(current.Status.Groups) != 1 || !current.Status.Groups[0].Applied {
			t.Fatalf("pause relabeled old execution: %+v", current.Status)
		}
		if pass == 1 && before.ResourceVersion != current.ResourceVersion {
			t.Fatal("stable pause rewrote status")
		}
	}
	if reads == 0 {
		t.Fatal("test did not observe CR")
	}
}

func TestProjectionFailureBlocksRetirementButStopStillUsesLiveSlots(t *testing.T) {
	for _, stopped := range []bool{false, true} {
		t.Run(fmt.Sprint(stopped), func(t *testing.T) {
			cr, scheme := controllerInput(), controllerScheme(t)
			cr.Spec.ClusterConfig = &generatedtrino.ClusterConfigInput{Stopped: &stopped}
			objects := retirementObjects(t, cr, "existing", 2)
			c := retirementClient(scheme, cr, objects, interceptor.Funcs{})
			r := newTestReconciler(c, scheme, testFacts{})
			r.Binding.Project = func(*generatedtrino.TrinoCluster) (input.Projection, error) {
				return input.Projection{}, errors.New("invalid projection")
			}
			_, err := r.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(cr)})
			if err == nil {
				t.Fatal("projection error was lost")
			}
			sts := objects[0].(*appsv1.StatefulSet).DeepCopy()
			applyTestGet(t, c, sts)
			want := int32(2)
			if stopped {
				want = 0
			}
			if *sts.Spec.Replicas != want {
				t.Fatalf("live stop or retention failed: %+v", sts.Spec.Replicas)
			}
			for _, object := range objects {
				applyTestGet(t, c, object.DeepCopyObject().(client.Object))
			}
			current := cr.DeepCopy()
			applyTestGet(t, c, current)
			condition := meta.FindStatusCondition(current.Status.Conditions, "Retired")
			if condition == nil || condition.Status != metav1.ConditionUnknown {
				t.Fatal("partial projection allowed retirement")
			}
		})
	}
}

func TestStoppedUsesExecutionZeroAndResumeUsesLatestDeclaredReplicas(t *testing.T) {
	r, cr := factsTestReconciler(t, nil)
	current := cr.DeepCopy()
	applyTestGet(t, r.Client, current)
	stopped := true
	current.Spec.ClusterConfig = &generatedtrino.ClusterConfigInput{Stopped: &stopped}
	if err := r.Client.Update(t.Context(), current); err != nil {
		t.Fatal(err)
	}
	for pass, replicas := range []int32{2, 3} {
		r.Binding.Project = func(observed *generatedtrino.TrinoCluster) (input.Projection, error) {
			return input.Projection{Cluster: framework.ClusterIdentity{Name: observed.Name, Namespace: observed.Namespace},
				Roles: []input.Role{{Name: "workers", Replicas: &replicas, Groups: []input.Group{{Name: "blocked"},
					{Name: "healthy"}}}}}, nil
		}
		if pass == 1 {
			applyTestGet(t, r.Client, current)
			stopped = false
			current.Spec.ClusterConfig.Stopped = &stopped
			current.Generation++
			if err := r.Client.Update(t.Context(), current); err != nil {
				t.Fatal(err)
			}
		}
		_, err := r.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(cr)})
		if err != nil {
			t.Fatal(err)
		}
		applyTestGet(t, r.Client, current)
		execution := replicas
		if stopped {
			execution = 0
		}
		for _, group := range current.Status.Groups {
			if group.DesiredReplicas != replicas || group.ExecutionReplicas == nil || *group.ExecutionReplicas != execution {
				t.Fatalf("declared/execution counts conflated: %+v", group)
			}
		}
		var pdb policyv1.PodDisruptionBudget
		if err := r.Client.Get(t.Context(), client.ObjectKey{Name: cr.Name + "-workers-pdb",
			Namespace: cr.Namespace}, &pdb); err != nil {
			t.Fatal(err)
		}
		if pdb.Spec.MinAvailable.IntVal != 2*replicas-1 {
			t.Fatal("stop changed declared role budget")
		}
		if stopped && meta.FindStatusCondition(current.Status.Conditions,
			"WorkloadsReady").Status != metav1.ConditionUnknown {
			t.Fatal("stop claimed application readiness")
		}
	}
}

func TestAbsentSharedCallbackWithdrawsTrustedLiveSet(t *testing.T) {
	r, cr := factsTestReconciler(t, nil)
	if _, errs := r.reconcileShared(t.Context(), cr, sharedOutput(cr, "old")); len(errs) != 0 {
		t.Fatal(errs)
	}
	r.Definition.GenerateCluster = nil
	if _, err := r.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(cr)}); err != nil {
		t.Fatal(err)
	}
	sharedGone(t, r.Client, cr, "old")
}

func TestFailedDesiredGroupDoesNotHideRemovedGroupRetirement(t *testing.T) {
	cr, scheme := controllerInput(), controllerScheme(t)
	kept, removed := retirementObjects(t, cr, "kept", 1), retirementObjects(t, cr, "removed", 0)
	c := retirementClient(scheme, cr, append(kept, removed...), interceptor.Funcs{})
	r := newTestReconciler(c, scheme, testFacts{})
	r.Binding.Project = func(current *generatedtrino.TrinoCluster) (input.Projection, error) {
		return input.Projection{Cluster: framework.ClusterIdentity{Name: current.Name, Namespace: current.Namespace},
			Roles: []input.Role{{Name: "workers", Groups: []input.Group{{Name: "kept"}}}}}, nil
	}
	r.Definition.GenerateGroup = func(framework.EffectiveInput[testConfig, testClusterConfig, testFacts]) (
		framework.RuntimeDescription, error,
	) {
		return framework.RuntimeDescription{}, errors.New("group generation failed")
	}
	if _, err := r.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(cr)}); err == nil {
		t.Fatal("lost generation error")
	}
	sts := kept[0].(*appsv1.StatefulSet).DeepCopy()
	applyTestGet(t, c, sts)
	if *sts.Spec.Replicas != 1 {
		t.Fatal("failed desired group was retired")
	}
	if err := c.Get(t.Context(), client.ObjectKeyFromObject(removed[0]),
		&appsv1.StatefulSet{}); !apierrors.IsNotFound(err) {
		t.Fatal(err)
	}
}
