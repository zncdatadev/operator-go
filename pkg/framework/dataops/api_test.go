package dataops

import (
	"os"
	"path/filepath"
	goruntime "runtime"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestDataProtocolAPIIdentityAndImmutableIntent(t *testing.T) {
	assets := os.Getenv("KUBEBUILDER_ASSETS")
	if assets == "" {
		assets = filepath.Join("..", "..", "..", "bin", "k8s", "1.35.0-"+goruntime.GOOS+"-"+goruntime.GOARCH)
	}
	existing := false
	environment := &envtest.Environment{UseExistingCluster: &existing, BinaryAssetsDirectory: assets, CRDDirectoryPaths: []string{filepath.Join("..", "..", "..", "config", "framework-data", "bases")}, ErrorIfCRDPathMissing: true}
	t.Cleanup(func() {
		if err := environment.Stop(); err != nil {
			t.Error(err)
		}
	})
	config, err := environment.Start()
	if err != nil {
		t.Fatal(err)
	}
	scheme := runtime.NewScheme()
	if err = corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err = AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	c, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "data-protocol"}}
	if err = c.Create(t.Context(), namespace); err != nil {
		t.Fatal(err)
	}
	_, op, asset := testController(t, ActionDestroy)
	asset.Namespace = namespace.Name
	asset.UID = ""
	asset.ResourceVersion = ""
	if err = c.Create(t.Context(), asset); err != nil {
		t.Fatal(err)
	}
	asset.Spec.Source.CRUID = "another"
	if err = c.Update(t.Context(), asset); err == nil {
		t.Fatal("asset initial identity was mutable")
	}
	op.Namespace = namespace.Name
	op.UID = ""
	op.ResourceVersion = ""
	op.Spec.AssetUID = asset.UID
	op.Spec.Approval = Approval(op.Spec)
	if err = c.Create(t.Context(), op); err != nil {
		t.Fatal(err)
	}
	op.Spec.Action = ActionAdopt
	if err = c.Update(t.Context(), op); err == nil {
		t.Fatal("operation spec changed after authorization")
	}
	if err = c.Get(t.Context(), client.ObjectKeyFromObject(op), op); err != nil {
		t.Fatal(err)
	}
	op.Status.Phase = phaseLocked
	op.Status.SpecDigest = Approval(op.Spec)
	if err = c.Status().Update(t.Context(), op); err != nil {
		t.Fatal(err)
	}
	if err = c.Get(t.Context(), client.ObjectKeyFromObject(op), op); err != nil {
		t.Fatal(err)
	}
	if op.Status.Phase != phaseLocked || op.UID == "" {
		t.Fatal("durable identity/phase did not roundtrip")
	}
}
