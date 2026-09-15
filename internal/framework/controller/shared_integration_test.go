package controller

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	generatedtrino "github.com/zncdatadev/operator-go/internal/framework/pipeline/testinput"
	"github.com/zncdatadev/operator-go/pkg/framework"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

// Real API persistence, server defaults, and manager watches are exercised here.
// Envtest has no workload controller or kubelet; this does not claim Pod readiness.
func TestControllerAPIConvergence(t *testing.T) {
	assets := os.Getenv("KUBEBUILDER_ASSETS")
	if assets == "" {
		assets = filepath.Join("..", "..", "..", "bin", "k8s", "1.35.0-"+runtime.GOOS+"-"+runtime.GOARCH)
	}
	if _, err := os.Stat(filepath.Join(assets, "kube-apiserver")); err != nil {
		t.Fatalf("envtest assets unavailable: %v", err)
	}
	existing := false
	environment := &envtest.Environment{UseExistingCluster: &existing, BinaryAssetsDirectory: assets,
		CRDDirectoryPaths: []string{filepath.Join("..", "pipeline", "testinput", "crd.yaml"),
			filepath.Join("..", "..", "..", "config", "framework-data", "bases")}, ErrorIfCRDPathMissing: true}
	t.Cleanup(func() {
		if err := environment.Stop(); err != nil {
			t.Error(err)
		}
	})
	config, err := environment.Start()
	if err != nil {
		t.Fatal(err)
	}
	scheme := controllerScheme(t)
	c, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "shared-api-"}}
	if err := c.Create(t.Context(), ns); err != nil {
		t.Fatal(err)
	}
	t.Run("retained-source-and-readdition", func(t *testing.T) { integrationRetainedAPI(t, c, scheme) })
	mgr, err := ctrl.NewManager(config, ctrl.Options{Scheme: scheme, Metrics: metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress: "0"})
	if err != nil {
		t.Fatal(err)
	}
	r := newTestReconciler(c, scheme, testFacts{})
	r.Definition.GenerateCluster = func(in framework.ClusterOutputInput[testClusterConfig, testFacts]) (
		framework.ClusterOutput, error,
	) {
		switch in.ClusterConfig.NodeEnvironment {
		case "pending":
			return framework.ClusterOutput{State: framework.ClusterOutputPending, Reason: "dependency is pending"}, nil
		case "error":
			return framework.ClusterOutput{}, errors.New("shared generation rejected")
		case "empty":
			return framework.ClusterOutput{State: framework.ClusterOutputReady}, nil
		default:
			owner := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: in.Cluster.Name, Namespace: in.Cluster.Namespace}}
			return sharedOutput(owner, in.ClusterConfig.NodeEnvironment), nil
		}
	}
	if err := r.SetupWithManager(mgr); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- mgr.Start(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("manager did not stop")
		}
	})
	zero := int32(0)
	one := "one"
	cr := &generatedtrino.TrinoCluster{ObjectMeta: metav1.ObjectMeta{Name: "shared", Namespace: ns.Name},
		Spec: generatedtrino.SpecInput{ClusterConfig: &generatedtrino.ClusterConfigInput{NodeEnvironment: &one},
			Workers: &generatedtrino.RoleInput{
				RoleGroups: map[string]generatedtrino.RoleGroupInput{"default": {Replicas: &zero}}}}}
	if err := c.Create(t.Context(), cr); err != nil {
		t.Fatal(err)
	}
	waitSharedAPI(t, c, cr, "one", true)
	initial := sharedRead(t, c, cr, "one")
	current := cr.DeepCopy()
	applyTestGet(t, c, current)
	if _, err := r.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(cr)}); err != nil {
		t.Fatal(err)
	}
	if got := sharedRead(t, c, cr, "one"); got.ResourceVersion != initial.ResourceVersion {
		t.Fatal("server-normalized no-op rewrote CM")
	}
	var sts appsv1.StatefulSet
	if err := c.Get(t.Context(), client.ObjectKey{Name: cr.Name + "-workers-default", Namespace: cr.Namespace},
		&sts); err != nil {
		t.Fatal(err)
	}
	if sts.Spec.Replicas == nil || *sts.Spec.Replicas != 0 {
		t.Fatal("generated workload missing")
	}
	for _, mode := range []string{"pending", "error", "two", "empty"} {
		updateOperationAPI(t, c, current, func(latest *generatedtrino.TrinoCluster) {
			latest.Spec.ClusterConfig.NodeEnvironment = &mode
		})
		applyTestGet(t, c, current)
		switch mode {
		case "pending", "error":
			waitSharedCondition(t, c, current)
			if got := sharedRead(t, c, cr, "one"); got.ResourceVersion != initial.ResourceVersion {
				t.Fatal("pending/error changed old CM")
			}
		case "two":
			waitSharedAPI(t, c, cr, "two", true)
			waitSharedAPI(t, c, cr, "one", false)
		default:
			waitSharedAPI(t, c, cr, "two", false)
		}
	}
	t.Run("operation-and-retirement", func(t *testing.T) { integrationOperationAndRetirement(t, c, r, cr) })
}

func waitSharedAPI(t *testing.T, c client.Client, cr client.Object, name string, present bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		cm := &corev1.ConfigMap{}
		err := c.Get(t.Context(), client.ObjectKey{Name: name, Namespace: cr.GetNamespace()}, cm)
		if !present && apierrors.IsNotFound(err) {
			return
		}
		if present && err == nil && cm.Annotations[SharedConfigMapAnnotation] != "" {
			return
		}
		if err != nil && !apierrors.IsNotFound(err) {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("shared %s presence did not reach %t", name, present)
}
func waitSharedCondition(t *testing.T, c client.Client, expected *generatedtrino.TrinoCluster) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		current := expected.DeepCopy()
		applyTestGet(t, c, current)
		cond := meta.FindStatusCondition(current.Status.Conditions, "Applied")
		if cond != nil && cond.ObservedGeneration == expected.Generation && cond.Status == metav1.ConditionFalse {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("pending/error was not reported for the current generation")
}
