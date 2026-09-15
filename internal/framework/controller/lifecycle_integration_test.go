package controller

import (
	"testing"
	"time"

	generatedtrino "github.com/zncdatadev/operator-go/internal/framework/pipeline/testinput"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func waitAPI(t *testing.T, check func() (bool, error)) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		ok, err := check()
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("API observation did not converge")
}
func updateOperationAPI(t *testing.T, c client.Client, cr *generatedtrino.TrinoCluster,
	mutate func(*generatedtrino.TrinoCluster),
) {
	t.Helper()
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current := cr.DeepCopy()
		if err := c.Get(t.Context(), client.ObjectKeyFromObject(cr), current); err != nil {
			return err
		}
		mutate(current)
		return c.Update(t.Context(), current)
	}); err != nil {
		t.Fatal(err)
	}
}
func workloadAPI(t *testing.T, c client.Client, cr client.Object, replicas int32) *appsv1.StatefulSet {
	t.Helper()
	sts := &appsv1.StatefulSet{}
	waitAPI(t, func() (bool, error) {
		err := c.Get(t.Context(), client.ObjectKey{Name: cr.GetName() + "-workers-default",
			Namespace: cr.GetNamespace()}, sts)
		return err == nil && sts.Spec.Replicas != nil && *sts.Spec.Replicas == replicas, client.IgnoreNotFound(err)
	})
	return sts
}
func observeZeroAPI(t *testing.T, c client.Client, sts *appsv1.StatefulSet) {
	t.Helper()
	applyTestGet(t, c, sts)
	sts.Status = appsv1.StatefulSetStatus{ObservedGeneration: sts.Generation}
	if err := c.Status().Update(t.Context(), sts); err != nil {
		t.Fatal(err)
	}
}

func integrationOperationAndRetirement(t *testing.T, c client.Client,
	r *Reconciler[*generatedtrino.TrinoCluster, testConfig, testClusterConfig, testFacts], cr *generatedtrino.TrinoCluster,
) {
	request := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(cr)}
	three := int32(3)
	updateOperationAPI(t, c, cr, func(current *generatedtrino.TrinoCluster) {
		current.Spec.Workers.RoleGroups = map[string]generatedtrino.RoleGroupInput{"default": {Replicas: &three}}
	})
	workloadAPI(t, c, cr, 3)
	stop := true
	updateOperationAPI(t, c, cr, func(current *generatedtrino.TrinoCluster) { current.Spec.ClusterConfig.Stopped = &stop })
	sts := workloadAPI(t, c, cr, 0)
	observeZeroAPI(t, c, sts) // explicit fixture observation, not a StatefulSet controller run
	waitAPI(t, func() (bool, error) {
		current := cr.DeepCopy()
		if err := c.Get(t.Context(), request.NamespacedName, current); err != nil {
			return false, err
		}
		return meta.IsStatusConditionTrue(current.Status.Conditions, "Stopped"), nil
	})
	var pdb policyv1.PodDisruptionBudget
	if err := c.Get(t.Context(), client.ObjectKey{Name: cr.Name + "-workers-pdb", Namespace: cr.Namespace},
		&pdb); err != nil {
		t.Fatal(err)
	}
	if pdb.Spec.MinAvailable.IntVal != 2 {
		t.Fatal("stop reduced the declared role budget")
	}
	pause := true
	four := int32(4)
	updateOperationAPI(t, c, cr, func(current *generatedtrino.TrinoCluster) {
		current.Spec.ClusterConfig.ReconciliationPaused = &pause
		current.Spec.Workers.RoleGroups = map[string]generatedtrino.RoleGroupInput{"default": {Replicas: &four}}
	})
	waitAPI(t, func() (bool, error) {
		current := cr.DeepCopy()
		if err := c.Get(t.Context(), request.NamespacedName, current); err != nil {
			return false, err
		}
		return meta.IsStatusConditionTrue(current.Status.Conditions, "Paused"), nil
	})
	paused := cr.DeepCopy()
	applyTestGet(t, c, paused)
	built := meta.FindStatusCondition(paused.Status.Conditions, "Built")
	if built == nil || built.ObservedGeneration >= paused.Generation {
		t.Fatal("pause relabeled prior execution")
	}
	before := workloadAPI(t, c, cr, 0).ResourceVersion
	if _, err := r.Reconcile(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	if current := workloadAPI(t, c, cr, 0); current.ResourceVersion != before {
		t.Fatal("paused pass changed workload")
	}
	pause, stop = false, false
	updateOperationAPI(t, c, cr, func(current *generatedtrino.TrinoCluster) {
		current.Spec.ClusterConfig.ReconciliationPaused = &pause
		current.Spec.ClusterConfig.Stopped = &stop
	})
	sts = workloadAPI(t, c, cr, 4)
	controller := true
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: sts.Name + "-0", Namespace: cr.Namespace,
		Labels: map[string]string{"app.kubernetes.io/instance": cr.Name,
			"app.kubernetes.io/component": "workers", "role-group": "default"},
		OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "StatefulSet", Name: sts.Name,
			UID: sts.UID, Controller: &controller}},
	}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "trino", Image: "example.invalid/fixture:1"}}}}
	if err := c.Create(t.Context(), pod); err != nil {
		t.Fatal(err)
	}
	updateOperationAPI(t, c, cr, func(current *generatedtrino.TrinoCluster) { current.Spec.Workers.RoleGroups = nil })
	sts = workloadAPI(t, c, cr, 0)
	observeZeroAPI(t, c, sts)
	if _, err := r.Reconcile(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	applyTestGet(t, c, sts) // the real Pod still blocks retirement
	if err := c.Delete(t.Context(), pod, client.GracePeriodSeconds(0)); err != nil {
		t.Fatal(err)
	}
	waitAPI(t, func() (bool, error) {
		if _, err := r.Reconcile(t.Context(), request); err != nil {
			return false, err
		}
		current := cr.DeepCopy()
		if err := c.Get(t.Context(), request.NamespacedName, current); err != nil {
			return false, err
		}
		return meta.IsStatusConditionTrue(current.Status.Conditions, "Retired"), nil
	})
	for _, slot := range []string{slotStatefulset, slotService, slotHeadless, slotConfigmap} {
		object := (groupSlot{Role: "workers", Group: "default", Slot: slot}).object(cr)
		if err := c.Get(t.Context(), client.ObjectKeyFromObject(object), object); !apierrors.IsNotFound(err) {
			t.Fatalf("retired slot remained: %s %v", slot, err)
		}
	}
}
