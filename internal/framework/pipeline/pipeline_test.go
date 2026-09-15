package pipeline

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/zncdatadev/operator-go/pkg/framework"
	"github.com/zncdatadev/operator-go/pkg/framework/input"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func trinoTestFacts() TrinoFacts {
	return TrinoFacts{Catalogs: map[string]map[string]string{"tpch": {"connector.name": "tpch"}}}
}

func trinoTestProjection() input.Projection {
	return input.Projection{Cluster: ClusterIdentity{Namespace: "default", Name: "demo"},
		Roles: []input.Role{
			{Name: trinoCoordinatorRole, Groups: []input.Group{{Name: "default"}}},
			{Name: trinoWorkerRole, Replicas: ptr.To(int32(3)), Groups: []input.Group{
				{Name: "default", Replicas: ptr.To(int32(2))}, {Name: "batch", Replicas: ptr.To(int32(0))},
			}},
		}}
}

func TestSourceProjectionRetainsPresenceAndOwnsSnapshots(t *testing.T) {
	projection := trinoTestProjection()
	projection.Roles[1].Config = json.RawMessage(`{"httpPort":8081}`)
	projection.Roles[1].Overrides = &Overrides{EnvOverrides: map[string]string{"LEVEL": "role"}}
	projection.Roles = append(projection.Roles, input.Role{Name: "empty"})
	facts := trinoTestFacts()
	source, err := SourceFromProjection(projection, facts, ClusterOperation{Stopped: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(source.Roles) != 3 || len(source.Groups) != 3 || !source.Operation.Stopped ||
		source.Groups[0].Replicas != 1 || source.Groups[1].Replicas != 2 || source.Groups[2].Replicas != 0 {
		t.Fatalf("wrong internal inventory: %+v", source)
	}
	if projection.Roles[0].Groups[0].Replicas != nil || projection.Roles[0].Replicas != nil {
		t.Fatal("projection must remain raw")
	}
	source.Groups[1].RoleConfigLayer[0] = '['
	source.Groups[1].RoleOverrides.EnvOverrides["LEVEL"] = "changed"
	source.Shared.Catalogs["tpch"]["connector.name"] = "changed"
	if source.Groups[2].RoleConfigLayer[0] != '{' ||
		source.Groups[2].RoleOverrides.EnvOverrides["LEVEL"] != "role" ||
		projection.Roles[1].Overrides.EnvOverrides["LEVEL"] != "role" ||
		facts.Catalogs["tpch"]["connector.name"] != "tpch" {
		t.Fatal("a source group aliases its siblings, projection or caller facts")
	}
}

func TestPipelineCallbacksAreIsolatedAndBuildIsDeterministic(t *testing.T) {
	definition := TrinoDefinition()
	projection, facts := trinoTestProjection(), trinoTestFacts()
	beforeProjection, beforeFacts := CloneInput(projection), CloneInput(facts)
	validate, generate, shared := definition.ValidateInput, definition.GenerateGroup, definition.GenerateCluster
	var retained []RuntimeDescription
	var retainedShared []corev1.ConfigMap
	definition.ValidateInput = func(in framework.EffectiveInput[TrinoConfig, TrinoClusterConfig, TrinoFacts]) error {
		if in.Facts.Catalogs["tpch"]["connector.name"] != "tpch" || in.Topology[0].Error != "" {
			t.Fatal("another callback contaminated validation input")
		}
		err := validate(in)
		in.Facts.Catalogs["tpch"]["connector.name"] = "validator mutation"
		in.Topology[0].Error = "validator mutation"
		return err
	}
	definition.GenerateGroup = func(in framework.EffectiveInput[TrinoConfig, TrinoClusterConfig, TrinoFacts]) (
		RuntimeDescription, error,
	) {
		if in.Facts.Catalogs["tpch"]["connector.name"] != "tpch" || in.Topology[0].Error != "" {
			t.Fatal("validation contaminated generation")
		}
		runtime, err := generate(in)
		in.Facts.Catalogs["tpch"]["connector.name"] = "generator mutation"
		retained = append(retained, runtime)
		return runtime, err
	}
	definition.GenerateCluster = func(in framework.ClusterOutputInput[TrinoClusterConfig, TrinoFacts]) (
		ClusterOutput, error,
	) {
		out, err := shared(in)
		in.Shared.Catalogs["tpch"]["connector.name"] = "cluster mutation"
		in.Groups[0].GeneratedEndpoints[0].Port = 1
		retainedShared = out.ConfigMaps
		return out, err
	}
	first, err := Build(definition, projection, facts, assemblyBuildOptions())
	if err != nil {
		t.Fatal(err)
	}
	second, err := Build(definition, projection, facts, assemblyBuildOptions())
	if err != nil {
		t.Fatal(err)
	}
	if first.ClusterError != "" || !reflect.DeepEqual(first, second) ||
		!reflect.DeepEqual(projection, beforeProjection) || !reflect.DeepEqual(facts, beforeFacts) {
		t.Fatal("identical builds differ or callbacks changed caller data")
	}
	for _, runtime := range retained {
		runtime.Main.Command[0] = "mutated-after-build"
		runtime.Files[0].Content.(KeyValues).Values["coordinator"] = Literal("mutated-after-build")
	}
	retainedShared[0].Data["TRINO_URI"] = "mutated-after-build"
	for _, group := range second.Groups {
		if group.Resources == nil || group.Runtime.Main.Command[0] == "mutated-after-build" ||
			group.Runtime.Files[0].Content.(KeyValues).Values["coordinator"] == Literal("mutated-after-build") {
			t.Fatal("returned plan aliases a product-owned runtime")
		}
	}
	if second.ClusterOutput.ConfigMaps[0].Data["TRINO_URI"] == "mutated-after-build" {
		t.Fatal("returned shared output aliases a product-owned slice or map")
	}
}

func TestPipelineFactsAndStoppedDoNotChangeDeclaredBudgets(t *testing.T) {
	source, err := SourceFromProjection(trinoTestProjection(), trinoTestFacts(), ClusterOperation{Stopped: true})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareInputs(TrinoDefinition(), source)
	if err != nil {
		t.Fatal(err)
	}
	results := map[GroupKey]framework.FactResult[TrinoFacts]{
		{Role: trinoCoordinatorRole, Name: "default"}: {
			Diagnostic: FactDiagnostic{State: FactsPending, Reason: "CatalogPending"},
		},
		{Role: trinoWorkerRole, Name: "default"}: {
			Value: ptr.To(trinoTestFacts()), Diagnostic: FactDiagnostic{State: FactsResolved},
		},
		// The zero-replica worker is deliberately missing a resolver result.
	}
	plan, err := BuildPreparedResources(TrinoDefinition(), prepared, results, assemblyBuildOptions())
	if err != nil {
		t.Fatal(err)
	}
	if plan.ClusterError != "" || plan.ClusterOutput.State != ClusterOutputPending ||
		len(plan.ClusterOutput.ConfigMaps) != 0 {
		t.Fatalf("missing coordinator facts must withhold shared output: %+v", plan.ClusterOutput)
	}
	for _, group := range plan.Groups {
		if group.Outcome.Group.Role == trinoWorkerRole && group.Outcome.Group.Name == "default" {
			if group.Resources == nil || *group.Resources.StatefulSet.Spec.Replicas != 0 ||
				group.Input.Group.Replicas != 2 || len(group.Input.Topology) != 3 {
				t.Fatal("stopped must affect execution only, and an unrelated pending group must not block this group")
			}
		} else if group.Resources != nil || group.Outcome.Facts == nil || group.Outcome.Facts.State == FactsResolved {
			t.Fatal("unresolved groups must have diagnostics and no desired resources")
		}
	}
	role := roleConfigBuilt(t, plan.Roles, trinoWorkerRole)
	if role.PodDisruptionBudget.Spec.MinAvailable.IntVal != 1 {
		t.Fatal("role PDB lost declared replicas due to stop or missing facts")
	}
}

func TestPipelineSharedOutputExplicitStateAndFailuresPreserveGroups(t *testing.T) {
	tests := []struct {
		name  string
		value ClusterOutput
		err   error
		valid bool
	}{
		{name: "empty ready", value: ClusterOutput{State: ClusterOutputReady}, valid: true},
		{name: "pending", value: ClusterOutput{State: ClusterOutputPending, Reason: "dependency pending"}, valid: true},
		{name: "missing state"},
		{name: "generator error", err: errors.New("shared generation failed")},
		{name: "partial pending", value: ClusterOutput{State: ClusterOutputPending, Reason: "pending",
			ConfigMaps: []corev1.ConfigMap{{ObjectMeta: metav1.ObjectMeta{Name: "partial", Namespace: "default"}}}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			definition := TrinoDefinition()
			definition.GenerateCluster = func(framework.ClusterOutputInput[TrinoClusterConfig, TrinoFacts]) (
				ClusterOutput, error,
			) {
				return test.value, test.err
			}
			plan, err := Build(definition, trinoTestProjection(), trinoTestFacts(), assemblyBuildOptions())
			if err != nil {
				t.Fatal(err)
			}
			if (plan.ClusterError == "") != test.valid {
				t.Fatalf("shared error = %q, valid = %v", plan.ClusterError, test.valid)
			}
			if !test.valid && (plan.ClusterOutput.State != "" || len(plan.ClusterOutput.ConfigMaps) != 0) {
				t.Fatal("failed shared output must not expose partial resources")
			}
			for _, group := range plan.Groups {
				if group.Resources == nil || group.Outcome.Error != "" {
					t.Fatal("cluster output failure suppressed valid group resources")
				}
			}
		})
	}
}
