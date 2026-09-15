package consumer_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"example.com/framework-consumer/generated/presence"
	"example.com/framework-consumer/generated/trino"
	framework "github.com/zncdatadev/operator-go/pkg/framework"
	"github.com/zncdatadev/operator-go/pkg/framework/input"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

const consumerNamespace = "external-consumer"

func TestStrictDecodeAndVersionedBinding(t *testing.T) {
	binding := trino.Binding()
	if binding.Version != input.ContractVersion || trino.InputContractVersion != input.ContractVersion ||
		presence.InputContractVersion != input.ContractVersion || input.CheckVersion(binding.Version) != nil ||
		input.CheckVersion(binding.Version+1) == nil ||
		!reflect.DeepEqual(binding.Roles, []string{"coordinators", "workers"}) {
		t.Fatalf("binding lost its generated version or complete roles: %+v", binding)
	}
	binding.Roles[0] = "caller-mutation"
	if trino.Binding().Roles[0] != "coordinators" {
		t.Fatal("generated role inventory aliases a previous binding")
	}
	if binding.NewObject() == nil || binding.AddToScheme(runtime.NewScheme()) != nil {
		t.Fatal("generated construction or scheme binding is unusable")
	}
	for _, data := range []string{
		`{}`, `{"spec":null}`, `{"spec":{"workers":{"config":null}}}`,
		`{"spec":{"workers":{"config":{"httpPort":null}}}}`,
		`{"spec":{"workers":{"config":{"httpPort":1,"httpPort":2}}}}`,
		`{"spec":{"workers":{"config":{"httpport":1}}}}`,
		`{"spec":{"workers":{"config":{"unknown":1}}}}`,
		`{"spec":{"workers":{"config":{"resources":{"memory":{"limit":42}}}}}}`,
		`{"spec":{"clusterConfig":{"nodeEnvironment":null}}}`,
		`{"spec":{"clusterConfig":{"stopped":false,"stopped":true}}}`,
	} {
		if _, err := trino.Decode([]byte(data)); err == nil {
			t.Fatalf("ambiguous local input accepted: %s", data)
		}
	}
	for _, data := range []string{
		`{"spec":{"workers":{"config":{"args":[null]}}}}`,
		`{"spec":{"workers":{"config":{"backends":{"db":null}}}}}`,
	} {
		if _, err := presence.Decode([]byte(data)); err == nil {
			t.Fatalf("null collection child accepted: %s", data)
		}
	}
	cr, err := presence.Decode([]byte(`{"metadata":{"creationTimestamp":null},"spec":{
	  "clusterConfig":{"stopped":true,"reconciliationPaused":true},"workers":{"config":{"args":[]}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	var invalidArgs []string
	cr.Spec.Workers.Config.Args = &invalidArgs
	if _, err := presence.Binding().Project(cr); err == nil {
		t.Fatal("manually constructed null collection unexpectedly projected")
	}
	operation := presence.Binding().Operation(cr)
	if !operation.Stopped || !operation.ReconciliationPaused {
		t.Fatal("early operation reading depended on the complete invalid projection")
	}
}

func TestProjectionAndStatusOwnTheirData(t *testing.T) {
	cr, err := presence.Decode(objectJSON(t, "PresenceCluster", "copy", presenceSpec()))
	if err != nil {
		t.Fatal(err)
	}
	binding := presence.Binding()
	status := binding.Status(cr)
	status.ObservedGeneration = 3
	status.Groups = []framework.GroupReconcileStatus{{Role: "workers", Name: "default",
		Facts: &framework.FactDiagnostic{State: framework.FactsResolved,
			Observed: []framework.FactObject{{Name: "source", UID: "source-uid"}}}}}
	copy := cr.DeepCopy()
	copy.Status.Groups[0].Facts.Observed[0].UID = "mutated"
	if cr.Status.Groups[0].Facts.Observed[0].UID != "source-uid" {
		t.Fatal("generated status DeepCopy shares nested observations")
	}
	projection, err := binding.Project(cr)
	if err != nil {
		t.Fatal(err)
	}
	assertPresenceProjection(t, projection)
	projection.Roles[0].Config[0] = '!'
	*projection.Roles[0].Replicas = 9
	projection.Roles[0].Groups[0].Config[0] = '!'
	if *cr.Spec.Workers.Replicas != 2 {
		t.Fatal("Projection shares replica pointers with the CR")
	}
	fresh, err := binding.Project(cr)
	if err != nil {
		t.Fatal(err)
	}
	assertPresenceProjection(t, fresh)
}

func TestPersistedInputProjection(t *testing.T) {
	assets := consumerAssets(t)
	useExisting := false
	environment := &envtest.Environment{BinaryAssetsDirectory: assets, UseExistingCluster: &useExisting,
		CRDDirectoryPaths: []string{filepath.Join("generated", "trino", "crd.yaml"),
			filepath.Join("generated", "presence", "crd.yaml")}, ErrorIfCRDPathMissing: true}
	configuration, err := environment.Start()
	if err != nil {
		_ = environment.Stop()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := environment.Stop(); err != nil {
			t.Error(err)
		}
	})
	scheme := runtime.NewScheme()
	for _, register := range []func(*runtime.Scheme) error{
		corev1.AddToScheme, trino.Binding().AddToScheme, presence.Binding().AddToScheme,
	} {
		if err := register(scheme); err != nil {
			t.Fatal(err)
		}
	}
	c, err := client.New(configuration, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: consumerNamespace}}
	if err := c.Create(t.Context(), namespace); err != nil {
		t.Fatal(err)
	}
	d, err := dynamic.NewForConfig(configuration)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("trino-empty-role-and-native-fields", func(t *testing.T) {
		spec := trinoSpec()
		cr, err := trino.Decode(objectJSON(t, "TrinoCluster", "trino", spec))
		if err != nil {
			t.Fatal(err)
		}
		if err := c.Create(t.Context(), cr); err != nil {
			t.Fatal(err)
		}
		stored := trino.Binding().NewObject()
		if err := c.Get(t.Context(), client.ObjectKeyFromObject(cr), stored); err != nil {
			t.Fatal(err)
		}
		assertSpec(t, stored, spec)
		projection, err := trino.Project(stored)
		if err != nil || len(projection.Roles) != 2 || projection.Roles[0].Name != "coordinators" ||
			projection.Roles[0].Replicas != nil || len(projection.Roles[0].Groups) != 0 {
			t.Fatalf("empty role inventory or presence changed: %+v %v", projection, err)
		}
		assertJSON(t, projection.Image, spec["image"])
		assertJSON(t, projection.ClusterConfig, map[string]any{"nodeEnvironment": "consumer"})
		assertJSON(t, projection.Roles[1].RoleConfig, spec["workers"].(map[string]any)["roleConfig"])
		if projection.Roles[1].Groups[0].Replicas == nil || *projection.Roles[1].Groups[0].Replicas != 0 {
			t.Fatal("explicit group zero disappeared")
		}
	})
	t.Run("presence-typed-update-and-status", func(t *testing.T) {
		spec := presenceSpec()
		cr, err := presence.Decode(objectJSON(t, "PresenceCluster", "presence", spec))
		if err != nil {
			t.Fatal(err)
		}
		if err := c.Create(t.Context(), cr); err != nil {
			t.Fatal(err)
		}
		stored := presence.Binding().NewObject()
		if err := c.Get(t.Context(), client.ObjectKeyFromObject(cr), stored); err != nil {
			t.Fatal(err)
		}
		stored.Annotations = map[string]string{"test.example.com/update": "typed"}
		if err := c.Update(t.Context(), stored); err != nil {
			t.Fatal(err)
		}
		if err := c.Get(t.Context(), client.ObjectKeyFromObject(cr), stored); err != nil {
			t.Fatal(err)
		}
		assertSpec(t, stored, spec)
		projection, err := presence.Project(stored)
		if err != nil {
			t.Fatal(err)
		}
		assertPresenceProjection(t, projection)
		status := presence.Binding().Status(stored)
		status.ObservedGeneration = stored.Generation
		status.Roles = []framework.RoleReconcileStatus{{Name: "workers", Applied: false}}
		status.Conditions = []metav1.Condition{{Type: "Built", Status: metav1.ConditionFalse,
			Reason: "Fixture", Message: "observation only", LastTransitionTime: metav1.Now(),
			ObservedGeneration: stored.Generation}}
		if err := c.Status().Update(t.Context(), stored); err != nil {
			t.Fatal(err)
		}
		if err := c.Get(t.Context(), client.ObjectKeyFromObject(cr), stored); err != nil {
			t.Fatal(err)
		}
		assertSpec(t, stored, spec)
		if stored.Status.ObservedGeneration != stored.Generation || len(stored.Status.Roles) != 1 {
			t.Fatal("status binding did not survive API persistence")
		}
	})
	t.Run("admission-and-merge-patch", func(t *testing.T) {
		resource := d.Resource(schema.GroupVersionResource{Group: "consumer.example.com", Version: "v1alpha1",
			Resource: "trinoclusters"}).Namespace(consumerNamespace)
		admissionChecks(t, c, resource)
	})
	t.Run("generated-registration-and-refresh", func(t *testing.T) { testGeneratedRegistration(t, configuration) })
}

func consumerAssets(t *testing.T) string {
	t.Helper()
	assets, explicit := os.LookupEnv("KUBEBUILDER_ASSETS")
	if assets == "" {
		assets = os.Getenv("FRAMEWORK_ENVTEST_ASSETS")
	}
	for _, name := range []string{"kube-apiserver", "etcd"} {
		if _, err := os.Stat(filepath.Join(assets, name)); err != nil {
			if explicit {
				t.Fatalf("explicit envtest assets are unavailable: %v", err)
			}
			t.Skipf("envtest assets unavailable; generation/compilation still tested; set KUBEBUILDER_ASSETS: %v", err)
		}
	}
	return assets
}

func presenceSpec() map[string]any {
	return map[string]any{
		"clusterConfig": map[string]any{"enabled": false, "count": 0, "labels": map[string]any{}, "args": []any{}},
		"workers": map[string]any{"replicas": 2,
			"config": map[string]any{"label": "role", "enabled": true, "args": []any{"role"},
				"backends": map[string]any{"db.internal": map[string]any{"host": "role", "enabled": true}}},
			"envOverrides": map[string]any{"EMPTY": ""}, "cliOverrides": []any{},
			"podOverrides": map[string]any{"spec": map[string]any{"containers": []any{
				map[string]any{"name": "trino", "env": nil, "$patch": "merge"}}}},
			"roleGroups": map[string]any{"default": map[string]any{"replicas": 0,
				"config": map[string]any{"label": "", "enabled": false, "args": []any{}, "backends": map[string]any{}}},
				"inherited": map[string]any{}},
		},
	}
}

func trinoSpec() map[string]any {
	return map[string]any{
		"image": map[string]any{"custom": "", "repo": "registry.example", "productVersion": "476",
			"kubedoopVersion": "", "pullPolicy": "Never", "pullSecretName": ""},
		"clusterConfig": map[string]any{"nodeEnvironment": "consumer", "stopped": false, "reconciliationPaused": false},
		"coordinators":  map[string]any{},
		"workers": map[string]any{"replicas": 2, "config": map[string]any{
			"httpPort": 0, "catalogConfigMapName": "", "gracefulShutdownTimeout": "0s",
			"affinity": map[string]any{"nodeAffinity": map[string]any{}},
			"resources": map[string]any{"cpu": map[string]any{"min": "100m", "max": "1"},
				"memory": map[string]any{"limit": "1Gi"}},
			"logging": map[string]any{"enableVectorAgent": false, "containers": map[string]any{"trino": map[string]any{
				"console": map[string]any{"level": "OFF"}, "file": map[string]any{"level": "TRACE"},
				"loggers": map[string]any{"ROOT": map[string]any{"level": "INFO"}}}}}},
			"roleConfig": map[string]any{"podDisruptionBudget": map[string]any{"enabled": false, "maxUnavailable": 0}},
			"roleGroups": map[string]any{"default": map[string]any{"replicas": 0}}},
	}
}

func assertPresenceProjection(t *testing.T, projection input.Projection) {
	t.Helper()
	if len(projection.Roles) != 1 || projection.Roles[0].Name != "workers" {
		t.Fatalf("unexpected roles: %+v", projection.Roles)
	}
	role := projection.Roles[0]
	if role.Replicas == nil || *role.Replicas != 2 || len(role.Groups) != 2 ||
		role.Groups[0].Name != "default" || role.Groups[0].Replicas == nil || *role.Groups[0].Replicas != 0 ||
		role.Groups[1].Name != "inherited" || role.Groups[1].Replicas != nil || len(role.Groups[1].Config) != 0 {
		t.Fatalf("projection folded or erased declared presence: %+v", role)
	}
	spec := presenceSpec()
	worker := spec["workers"].(map[string]any)
	assertJSON(t, projection.ClusterConfig, spec["clusterConfig"])
	assertJSON(t, role.Config, worker["config"])
	assertJSON(t, role.Groups[0].Config,
		worker["roleGroups"].(map[string]any)["default"].(map[string]any)["config"])
	if role.Overrides == nil || role.Overrides.CLIOverrides == nil || len(*role.Overrides.CLIOverrides) != 0 ||
		len(role.Overrides.EnvOverrides) != 1 || role.Overrides.EnvOverrides["EMPTY"] != "" {
		t.Fatal("override presence was lost")
	}
	if _, present := role.Overrides.EnvOverrides["EMPTY"]; !present {
		t.Fatal("explicit empty environment value disappeared")
	}
	assertJSON(t, role.Overrides.PodOverrides, worker["podOverrides"])
}

func admissionChecks(t *testing.T, c client.Client, resource dynamic.ResourceInterface) {
	t.Helper()
	for index, config := range []any{nil, map[string]any{"httpPort": nil}, map[string]any{"httpPort": "wrong"}} {
		spec := map[string]any{"workers": map[string]any{"config": config}}
		object := rawObject(t, "TrinoCluster", []string{"null-config", "null-field", "wrong-type"}[index], spec)
		if _, err := resource.Create(t.Context(), object, metav1.CreateOptions{}); !apierrors.IsInvalid(err) {
			t.Fatalf("API accepted invalid typed config %v: %v", config, err)
		}
	}
	spec := map[string]any{"workers": map[string]any{"config": map[string]any{"httpPort": 8080, "unknown": "prune"}}}
	object := rawObject(t, "TrinoCluster", "unknown-strict", spec)
	if _, err := resource.Create(t.Context(), object,
		metav1.CreateOptions{FieldValidation: metav1.FieldValidationStrict}); err == nil {
		t.Fatal("strict API request accepted an unknown field")
	}
	object.SetName("unknown-ignore")
	createOptions := metav1.CreateOptions{FieldValidation: metav1.FieldValidationIgnore}
	created, err := resource.Create(t.Context(), object, createOptions)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"workers": map[string]any{"config": map[string]any{"httpPort": 8080}}}
	assertSpec(t, created, want)
	stored := trino.Binding().NewObject()
	if err := c.Get(t.Context(), client.ObjectKeyFromObject(created), stored); err != nil {
		t.Fatal(err)
	}
	assertSpec(t, stored, want)
	patch := []byte(`{"spec":{"workers":{"config":{"httpPort":null}}}}`)
	patched, err := resource.Patch(t.Context(), created.GetName(), types.MergePatchType, patch, metav1.PatchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertSpec(t, patched, map[string]any{"workers": map[string]any{"config": map[string]any{}}})
	t.Log("API Ignore prunes unknown fields; strict requests reject them; merge-patch null removes input")
}

func rawObject(t *testing.T, kind, name string, spec any) *unstructured.Unstructured {
	t.Helper()
	object := &unstructured.Unstructured{}
	if err := json.Unmarshal(objectJSON(t, kind, name, spec), &object.Object); err != nil {
		t.Fatal(err)
	}
	return object
}

func objectJSON(t *testing.T, kind, name string, spec any) []byte {
	t.Helper()
	return marshal(t, map[string]any{"apiVersion": "consumer.example.com/v1alpha1", "kind": kind,
		"metadata": map[string]any{"name": name, "namespace": consumerNamespace}, "spec": spec})
}

func marshal(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func assertSpec(t *testing.T, value, want any) {
	t.Helper()
	var object map[string]json.RawMessage
	if err := json.Unmarshal(marshal(t, value), &object); err != nil {
		t.Fatal(err)
	}
	assertJSON(t, object["spec"], want)
}

func assertJSON(t *testing.T, actual []byte, want any) {
	t.Helper()
	var a, b any
	if err := json.Unmarshal(actual, &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(marshal(t, want), &b); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("raw presence changed: actual=%s expected=%s", actual, marshal(t, want))
	}
}
