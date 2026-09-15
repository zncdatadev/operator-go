package v1alpha1_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zncdatadev/operator-go/examples/trino-operator/api/v1alpha1"
	"github.com/zncdatadev/operator-go/examples/trino-operator/api/v1alpha1/registration"
	"github.com/zncdatadev/operator-go/examples/trino-operator/internal/product"
	"github.com/zncdatadev/operator-go/pkg/framework"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/yaml"
)

func sample(t *testing.T) *v1alpha1.TrinoCluster {
	t.Helper()
	data, err := os.ReadFile("../../config/samples/trino_v1alpha1_trinocluster.yaml")
	if err != nil {
		t.Fatal(err)
	}
	jsonData, err := yaml.YAMLToJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	object, err := v1alpha1.Decode(jsonData)
	if err != nil {
		t.Fatal(err)
	}
	return object
}

func checkSample(t *testing.T, object *v1alpha1.TrinoCluster) {
	t.Helper()
	projected, err := v1alpha1.Project(object)
	if err != nil {
		t.Fatal(err)
	}
	if len(projected.Roles) != 2 || len(projected.Roles[0].Groups) != 1 || len(projected.Roles[1].Groups) != 1 {
		t.Fatalf("sample topology lost during projection: %+v", projected.Roles)
	}
	if !bytes.Contains(projected.Roles[1].Config, []byte(`"level":"OFF"`)) {
		t.Fatalf("YAML OFF must remain a string through API projection: %s", projected.Roles[1].Config)
	}
	if bytes.Contains(projected.Roles[1].Overrides.PodOverrides, []byte(`"startupProbe"`)) ||
		projected.Roles[1].Replicas == nil || *projected.Roles[1].Replicas != 1 ||
		projected.Roles[1].Groups[0].Replicas != nil {
		t.Fatal("sample reintroduced a duplicate probe patch or lost role/group replica presence")
	}
	if object.Spec.ClusterConfig.Stopped == nil || *object.Spec.ClusterConfig.Stopped ||
		object.Spec.ClusterConfig.ReconciliationPaused == nil || *object.Spec.ClusterConfig.ReconciliationPaused {
		t.Fatal("explicit false operation controls did not survive")
	}
	if bytes.Contains(projected.ClusterConfig, []byte("stopped")) {
		t.Fatal("product cluster configuration must exclude operation controls")
	}
}

func TestPublishedSampleDecodeAndProjection(t *testing.T) {
	checkSample(t, sample(t))
}

func TestPublishedSampleAPIRoundtrip(t *testing.T) {
	assets := os.Getenv("KUBEBUILDER_ASSETS")
	if assets == "" {
		t.Skip("set KUBEBUILDER_ASSETS to run the real API-server sample roundtrip")
	}
	for _, name := range []string{"etcd", "kube-apiserver"} {
		if _, err := os.Stat(filepath.Join(assets, name)); err != nil {
			t.Fatalf("explicit envtest assets are unavailable: %v", err)
		}
	}
	environment := &envtest.Environment{BinaryAssetsDirectory: assets,
		CRDDirectoryPaths: []string{"../../config/crd/bases", "../../../../config/framework-data/bases"}, ErrorIfCRDPathMissing: true}
	configuration, err := environment.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := environment.Stop(); err != nil {
			t.Error(err)
		}
	})
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	apiClient, err := client.New(configuration, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	if err := apiClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "sample-test"}}); err != nil {
		t.Fatal(err)
	}
	object := sample(t)
	object.Namespace = "sample-test"
	if err := apiClient.Create(ctx, object, client.FieldValidation("Strict")); err != nil {
		t.Fatal(err)
	}
	observed := &v1alpha1.TrinoCluster{}
	if err := apiClient.Get(ctx, types.NamespacedName{Name: object.Name, Namespace: object.Namespace}, observed); err != nil {
		t.Fatal(err)
	}
	checkSample(t, observed)
	checkSampleResources(t, ctx, configuration, scheme, apiClient, object)
}

func checkSampleResources(t *testing.T, ctx context.Context, configuration *rest.Config, scheme *runtime.Scheme,
	apiClient client.Client, object *v1alpha1.TrinoCluster,
) {
	t.Helper()
	ctx, cancel := context.WithCancel(ctx)
	manager, err := ctrl.NewManager(configuration, ctrl.Options{Scheme: scheme, Metrics: metricsserver.Options{BindAddress: "0"}, HealthProbeBindAddress: "0"})
	if err != nil {
		t.Fatal(err)
	}
	if err := registration.Register(manager, product.Definition(), registration.Options[product.TrinoFacts]{
		Facts: product.BaseFacts(), ResolveFacts: product.ResolveFacts,
		Assembly: framework.AssemblyOptions{MaterializerImage: "example.invalid/materializer:1", VectorImage: "example.invalid/vector:1"},
	}); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- manager.Start(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(10 * time.Second):
			t.Error("manager did not stop")
		}
	})
	for {
		set := &appsv1.StatefulSet{}
		err := apiClient.Get(ctx, types.NamespacedName{Namespace: object.Namespace, Name: object.Name + "-workers-default"}, set)
		if err == nil {
			var main *corev1.Container
			for i := range set.Spec.Template.Spec.Containers {
				if set.Spec.Template.Spec.Containers[i].Name == "trino" {
					main = &set.Spec.Template.Spec.Containers[i]
				}
			}
			if main == nil || main.StartupProbe == nil || main.StartupProbe.Exec == nil || main.ReadinessProbe == nil || main.ReadinessProbe.Exec == nil {
				t.Fatal("formal sample did not generate native product probes")
			}
			init := set.Spec.Template.Spec.InitContainers
			if len(init) != 2 || init[0].Name != "prepare-files" || init[1].Name != "initialize-trino" || len(init[1].Command) == 0 ||
				set.Spec.PodManagementPolicy != appsv1.OrderedReadyPodManagement {
				t.Fatal("formal sample lost ordered initialization")
			}
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("formal sample did not produce a StatefulSet", err)
		case <-time.After(50 * time.Millisecond):
		}
	}
}
