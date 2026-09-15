package controller

import (
	"testing"

	generatedtrino "github.com/zncdatadev/operator-go/internal/framework/pipeline/testinput"
	"github.com/zncdatadev/operator-go/pkg/framework"
	"github.com/zncdatadev/operator-go/pkg/framework/input"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// The wire fixture is generated formally by U02. Product behavior below is a
// minimal test model; no controller production file imports any product adapter.
type testConfig struct {
	HTTPPort             int32  `json:"httpPort"`
	CatalogConfigMapName string `json:"catalogConfigMapName"`
}
type testClusterConfig struct {
	NodeEnvironment string `json:"nodeEnvironment"`
}
type testFacts struct {
	Catalogs map[string]map[string]string `json:"catalogs"`
}

func testDefinition() framework.ProductDefinition[testConfig, testClusterConfig, testFacts] {
	defaults := framework.Config[testConfig]{Common: framework.CommonConfig{Resources: framework.Resources{
		CPU:    framework.CPU{Min: resource.MustParse("500m"), Max: resource.MustParse("2")},
		Memory: framework.Memory{Limit: resource.MustParse("1Gi")},
	}}, Product: testConfig{HTTPPort: 8080}}
	return framework.ProductDefinition[testConfig, testClusterConfig, testFacts]{
		Name: "fixture", ImageDefaults: framework.ImageConfig{Custom: "example.invalid/product:1",
			PullPolicy: corev1.PullIfNotPresent},
		Roles: map[string]framework.RoleDefinition[testConfig]{
			"coordinators": {Config: defaults,
				RoleConfig: framework.RoleConfig{PodDisruptionBudget: framework.PodDisruptionBudgetConfig{Enabled: true}}},
			"workers": {Config: defaults,
				RoleConfig: framework.RoleConfig{PodDisruptionBudget: framework.PodDisruptionBudgetConfig{Enabled: true,
					MaxUnavailable: 1}}},
		},
		GenerateGroup: func(framework.EffectiveInput[testConfig, testClusterConfig,
			testFacts]) (framework.RuntimeDescription, error) {
			return framework.RuntimeDescription{Main: framework.Process{Name: "trino", Command: []string{"run"}},
				Endpoints: []framework.Endpoint{{Name: "http", Port: 8080}}}, nil
		},
	}
}
func newTestReconciler(c client.Client, scheme *runtime.Scheme, facts testFacts,
) *Reconciler[*generatedtrino.TrinoCluster, testConfig, testClusterConfig, testFacts] {
	return &Reconciler[*generatedtrino.TrinoCluster, testConfig, testClusterConfig, testFacts]{
		Client: c, Scheme: scheme, Binding: generatedtrino.Binding(), Definition: testDefinition(), Facts: facts,
	}
}

func factsTestReconciler(t *testing.T, objects []client.Object) (
	*Reconciler[*generatedtrino.TrinoCluster, testConfig, testClusterConfig, testFacts], *generatedtrino.TrinoCluster,
) {
	t.Helper()
	cr, scheme := controllerInput(), controllerScheme(t)
	c := retirementClient(scheme, cr, objects, interceptor.Funcs{})
	r := newTestReconciler(c, scheme, testFacts{Catalogs: map[string]map[string]string{"test": {"key": "base"}}})
	r.Binding.Project = func(current *generatedtrino.TrinoCluster) (input.Projection, error) {
		return input.Projection{Cluster: framework.ClusterIdentity{Name: current.Name, Namespace: current.Namespace},
			Roles: []input.Role{{Name: "workers", Groups: []input.Group{{Name: "blocked"}, {Name: "healthy"}}}}}, nil
	}
	r.Definition.ValidateInput = nil
	r.Definition.GenerateCluster = nil
	r.Definition.GenerateGroup = func(in framework.EffectiveInput[testConfig, testClusterConfig, testFacts]) (
		framework.RuntimeDescription, error,
	) {
		return framework.RuntimeDescription{Main: framework.Process{Name: "trino",
			Env: []corev1.EnvVar{{Name: "FACT_VALUE", Value: in.Facts.Catalogs["test"]["key"]}}}}, nil
	}
	return r, cr
}
