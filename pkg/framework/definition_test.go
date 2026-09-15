package framework_test

import (
	"reflect"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/zncdatadev/operator-go/pkg/framework"
)

func TestClusterOutputRequiresExplicitCompleteOrPendingResult(t *testing.T) {
	configMap := corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "discovery"}}
	for _, test := range []struct {
		name   string
		output framework.ClusterOutput
		valid  bool
	}{
		{"missing-state", framework.ClusterOutput{}, false},
		{"unknown-state", framework.ClusterOutput{State: "Complete"}, false},
		{"ready-withdraws-all", framework.ClusterOutput{State: framework.ClusterOutputReady}, true},
		{"ready-empty-list", framework.ClusterOutput{
			State: framework.ClusterOutputReady, ConfigMaps: []corev1.ConfigMap{}}, true},
		{"ready-output", framework.ClusterOutput{
			State: framework.ClusterOutputReady, ConfigMaps: []corev1.ConfigMap{configMap}}, true},
		{"pending-preserves", framework.ClusterOutput{
			State: framework.ClusterOutputPending, Reason: "coordinator input unavailable"}, true},
		{"pending-with-partial-output", framework.ClusterOutput{
			State: framework.ClusterOutputPending, Reason: "waiting", ConfigMaps: []corev1.ConfigMap{configMap}}, false},
		{"pending-no-reason", framework.ClusterOutput{State: framework.ClusterOutputPending}, false},
		{"pending-blank-reason", framework.ClusterOutput{State: framework.ClusterOutputPending, Reason: " \n"}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := test.output
			err := framework.ValidateClusterOutput(test.output)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%t, got %v", test.valid, err)
			}
			if !reflect.DeepEqual(before, test.output) {
				t.Fatal("validation modified a product result")
			}
		})
	}
}

func TestProductDefinitionExposesValuesAndExplicitSharedResult(t *testing.T) {
	type groupConfig struct{ Port int32 }
	type clusterConfig struct{ Environment string }
	type facts struct{ Ready bool }
	definition := framework.ProductDefinition[groupConfig, clusterConfig, facts]{
		Name: "example", Roles: map[string]framework.RoleDefinition[groupConfig]{"workers": {
			Config: framework.Config[groupConfig]{Product: groupConfig{Port: 8080}},
		}},
		GenerateGroup: func(in framework.EffectiveInput[groupConfig, clusterConfig, facts],
		) (framework.RuntimeDescription, error) {
			return framework.RuntimeDescription{Main: framework.Process{Name: "example", Image: in.Image.Reference},
				Endpoints:  []framework.Endpoint{{Name: "http", Port: in.Config.Product.Port}},
				LogOutputs: []framework.LogOutput{{Container: "example", Directory: "logs", RelativePath: "server.log"}}}, nil
		},
		GenerateCluster: func(in framework.ClusterOutputInput[clusterConfig, facts]) (framework.ClusterOutput, error) {
			if !in.Shared.Ready {
				return framework.ClusterOutput{State: framework.ClusterOutputPending, Reason: "waiting for facts"}, nil
			}
			return framework.ClusterOutput{State: framework.ClusterOutputReady}, nil
		},
	}
	description, err := definition.GenerateGroup(framework.EffectiveInput[groupConfig, clusterConfig, facts]{
		Config: definition.Roles["workers"].Config, Image: framework.ResolvedImage{Reference: "example:1"}})
	if err != nil || description.Main.Image != "example:1" || description.Endpoints[0].Port != 8080 {
		t.Fatalf("product declaration lost its effective data: %+v, %v", description, err)
	}
	for _, ready := range []bool{false, true} {
		input := framework.ClusterOutputInput[clusterConfig, facts]{Shared: facts{Ready: ready}}
		output, err := definition.GenerateCluster(input)
		valid := framework.ValidateClusterOutput(output)
		if err != nil || valid != nil || (output.State == framework.ClusterOutputReady) != ready {
			t.Fatalf("shared output confused pending with an empty complete inventory: %+v, %v", output, err)
		}
	}
}

func TestIdentityNamesRetainDeclaredComponents(t *testing.T) {
	cluster := framework.ClusterIdentity{Name: "demo", Namespace: "products"}
	group := framework.GroupIdentity{ClusterIdentity: cluster, Role: "workers", Name: "large", Replicas: 3}
	role := framework.RoleIdentity{ClusterIdentity: cluster, Name: "workers"}
	if group.ServiceName() != "demo-workers-large" || group.ServiceDNS() != "demo-workers-large.products.svc" ||
		role.PodDisruptionBudgetName() != "demo-workers-pdb" {
		t.Fatal("identity helpers disagree with the established resource naming rule")
	}
	group.Name = strings.Repeat("x", 100)
	if !strings.HasSuffix(group.ServiceName(), group.Name) {
		t.Fatal("naming helper silently truncated a name instead of leaving validation to the pipeline")
	}
}
