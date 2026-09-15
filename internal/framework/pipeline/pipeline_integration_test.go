package pipeline

import (
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/zncdatadev/operator-go/internal/framework/pipeline/testinput"
	"github.com/zncdatadev/operator-go/pkg/framework/inputgen"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var updateInputFixture = flag.Bool("update-input-fixture", false, "regenerate the pipeline's test-only input fixture")

func pipelineDirectory(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate pipeline test fixture")
	}
	return filepath.Dir(file)
}

func TestInputFixtureCurrent(t *testing.T) {
	artifacts, err := inputgen.Generate[TrinoConfig, TrinoClusterConfig](inputgen.Names{
		Package: "testinput", Group: testinput.GroupVersion.Group, Version: testinput.GroupVersion.Version,
		Kind: "TrinoCluster", Plural: "trinoclusters",
	}, sortedKeys(TrinoDefinition().Roles))
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{"zz_generated.input.go", "crd.yaml"}
	expected := [][]byte{artifacts.GoSource, artifacts.CRD}
	actual := make([][]byte, len(paths))
	for i, name := range paths {
		path := filepath.Join(pipelineDirectory(t), "testinput", name)
		if *updateInputFixture {
			if err := os.WriteFile(path, expected[i], 0o600); err != nil {
				t.Fatal(err)
			}
		}
		actual[i], err = os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := inputgen.Check(artifacts, actual[0], actual[1]); err != nil {
		t.Fatalf("regenerate with -run TestInputFixtureCurrent -update-input-fixture: %v", err)
	}
}

func pipelineAPI(t *testing.T) client.Client {
	t.Helper()
	assets := os.Getenv("KUBEBUILDER_ASSETS")
	if assets == "" {
		assets = filepath.Join(pipelineDirectory(t), "..", "..", "..", "bin", "k8s",
			"1.35.0-"+runtime.GOOS+"-"+runtime.GOARCH)
	}
	if _, err := os.Stat(filepath.Join(assets, "kube-apiserver")); err != nil {
		t.Fatalf("real API acceptance requires envtest assets: %v", err)
	}
	environment := &envtest.Environment{BinaryAssetsDirectory: assets,
		CRDDirectoryPaths: []string{filepath.Join(pipelineDirectory(t), "testinput")}, ErrorIfCRDPathMissing: true}
	config, err := environment.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := environment.Stop(); err != nil {
			t.Errorf("stop envtest: %v", err)
		}
	})
	scheme := kruntime.NewScheme()
	for _, add := range []func(*kruntime.Scheme) error{
		corev1.AddToScheme, appsv1.AddToScheme, policyv1.AddToScheme, testinput.AddToScheme,
	} {
		if err := add(scheme); err != nil {
			t.Fatal(err)
		}
	}
	c, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// The control plane persists the real generated CR and accepts the resulting
// resources. Local materialization proves bytes, not kubelet or Trino execution.
func TestPersistedInputBuildsResourcesAndMaterializedBytes(t *testing.T) {
	c := pipelineAPI(t)
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "u02-input"}}
	if err := c.Create(t.Context(), ns); err != nil {
		t.Fatal(err)
	}
	cr, err := testinput.Decode([]byte(`{
		"apiVersion":"pipeline.tests.kubedoop.dev/v1alpha1", "kind":"TrinoCluster",
		"metadata":{"name":"demo","namespace":"u02-input"},
		"spec":{
			"clusterConfig":{"nodeEnvironment":"pipeline_env"},
			"coordinators":{"roleGroups":{"default":{}}},
			"workers":{
				"replicas":3,
				"config":{"gracefulShutdownTimeout":"15s","resources":{"cpu":{"min":"750m"}}},
				"envOverrides":{"FROM_ROLE":"role"},
				"cliOverrides":["--etc-dir=/etc/trino","run"],
				"podOverrides":{"spec":{"containers":[{"name":"trino","env":[{"name":"FROM_ROLE","value":"pod-role"}]}]}},
				"roleGroups":{
					"default":{"replicas":2,"config":{"resources":{"memory":{"limit":"2Gi"}}},
						"envOverrides":{"FROM_ROLE":"group","GROUP":"yes"},"cliOverrides":[],
						"configOverrides":{"config.properties":{"properties":{"set":{"task.concurrency":"17"}}},
							"jvm.config":{"lines":["-Xmx512m"]}}},
					"batch":{"replicas":0,"config":{"logging":{"enableVectorAgent":false}}}
				}
			}
		}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Create(t.Context(), cr); err != nil {
		t.Fatal(err)
	}
	persisted := &testinput.TrinoCluster{}
	if err := c.Get(t.Context(), types.NamespacedName{Name: cr.Name, Namespace: cr.Namespace}, persisted); err != nil {
		t.Fatal(err)
	}
	before := persisted.DeepCopy()
	if persisted.Spec.Coordinators.Replicas != nil || persisted.Spec.Workers.RoleGroups["default"].CLIOverrides == nil ||
		persisted.Spec.Workers.RoleGroups["batch"].Config.Logging.EnableVectorAgent == nil {
		t.Fatal("API persistence lost omitted, empty or explicit false input")
	}
	projection, err := testinput.Project(persisted)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := Build(TrinoDefinition(), projection, trinoTestFacts(), assemblyBuildOptions())
	if err != nil {
		t.Fatal(err)
	}
	if plan.ClusterError != "" || plan.ClusterOutput.State != ClusterOutputReady ||
		len(plan.ClusterOutput.ConfigMaps) != 1 {
		t.Fatalf("shared output: %+v / %s", plan.ClusterOutput, plan.ClusterError)
	}
	if len(plan.Groups) != 3 || len(plan.Roles) != 2 {
		t.Fatalf("incomplete resource plan: %d groups, %d roles", len(plan.Groups), len(plan.Roles))
	}
	for _, group := range plan.Groups {
		if group.Outcome.Error != "" || group.Resources == nil {
			t.Fatalf("group %s: %s", group.Outcome.Group.Name, group.Outcome.Error)
		}
		resources := group.Resources
		for _, object := range []client.Object{
			&resources.ConfigMap, &resources.Service, &resources.HeadlessService, &resources.StatefulSet,
		} {
			if err := c.Create(t.Context(), object); err != nil {
				t.Fatalf("create %T %s: %v", object, object.GetName(), err)
			}
		}
		assertPersistedWorker(t, group)
	}
	for _, role := range plan.Roles {
		if role.Error != "" || role.PodDisruptionBudget == nil {
			t.Fatalf("role budget: %+v", role)
		}
		if role.Role.Name == trinoWorkerRole && role.PodDisruptionBudget.Spec.MinAvailable.IntVal != 1 {
			t.Fatalf("budget must use all declared replicas: %+v", role.PodDisruptionBudget.Spec)
		}
		if err := c.Create(t.Context(), role.PodDisruptionBudget); err != nil {
			t.Fatal(err)
		}
	}
	for i := range plan.ClusterOutput.ConfigMaps {
		if err := c.Create(t.Context(), &plan.ClusterOutput.ConfigMaps[i]); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(before, persisted) {
		t.Fatal("pipeline mutated the persisted input")
	}
}

func assertPersistedWorker(t *testing.T, group BuiltGroup[TrinoConfig, TrinoClusterConfig, TrinoFacts]) {
	t.Helper()
	resources := group.Resources
	if group.Outcome.Group.Role != trinoWorkerRole {
		return
	}
	pod := resources.StatefulSet.Spec.Template
	if group.Outcome.Group.Name == "batch" {
		if findContainer(pod, vectorContainerName) != nil || *resources.StatefulSet.Spec.Replicas != 0 {
			t.Fatal("explicit false/zero did not survive API roundtrip and folding")
		}
		return
	}
	main := findContainer(pod, trinoName)
	if *resources.StatefulSet.Spec.Replicas != 2 || len(main.Args) != 0 ||
		findContainer(pod, vectorContainerName) == nil || *pod.Spec.TerminationGracePeriodSeconds != 15 ||
		main.Resources.Requests.Cpu().String() != "750m" || main.Resources.Limits.Memory().String() != "2Gi" {
		t.Fatalf("incorrect folded workload: %+v", main)
	}
	env := map[string]string{}
	for _, variable := range main.Env {
		env[variable.Name] = variable.Value
	}
	if env["FROM_ROLE"] != "pod-role" || env["GROUP"] != "yes" {
		t.Fatalf("role podOverrides must beat group envOverrides: %v", env)
	}
	materialization, err := DecodeMaterializationPlan([]byte(resources.ConfigMap.Data[materializationPlanFile]))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := Materialize(root, materialization, resources.StatefulSet.Name+"-0"); err != nil {
		t.Fatal(err)
	}
	assertMaterializedContains(t, root, map[string][]string{
		"config/config.properties":       {"task.concurrency=17\n"},
		"config/node.properties":         {"node.environment=pipeline_env\n", "node.id=demo-workers-default-0\n"},
		"config/jvm.config":              {"-Xmx512m\n"},
		"config/catalog/tpch.properties": {"connector.name=tpch\n"},
	})
}

func assertMaterializedContains(t *testing.T, root string, expected map[string][]string) {
	t.Helper()
	for name, fragments := range expected {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, fragment := range fragments {
			if !strings.Contains(string(data), fragment) {
				t.Fatalf("materialized %s lacks %q: %s", name, fragment, data)
			}
		}
	}
}
