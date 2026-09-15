package pipeline

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/zncdatadev/operator-go/pkg/framework"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func roleConfigDefinition() framework.ProductDefinition[struct{}, struct{}, struct{}] {
	return framework.ProductDefinition[struct{}, struct{}, struct{}]{Name: "role-fixture",
		Roles: map[string]framework.RoleDefinition[struct{}]{
			"coordinators": {RoleConfig: RoleConfig{PodDisruptionBudget: PodDisruptionBudgetConfig{
				Enabled: true, MaxUnavailable: 0}}},
			"workers": {RoleConfig: RoleConfig{PodDisruptionBudget: PodDisruptionBudgetConfig{
				Enabled: true, MaxUnavailable: 1}}},
		},
	}
}

func roleConfigSource() SourceSnapshot[struct{}] {
	return SourceSnapshot[struct{}]{
		Cluster: ClusterIdentity{Name: "roles", Namespace: "fixture", Labels: map[string]string{
			"team": "data", "app.kubernetes.io/instance": "forged", "app.kubernetes.io/component": "forged",
			"role-group": "not-a-selector",
		}},
		Roles: []RoleSource{{Name: "workers"}, {Name: "coordinators"}},
		Groups: []GroupSource{
			{Role: "workers", Name: "waiting", Replicas: 2},
			{Role: "workers", Name: "invalid", Replicas: 3, Config: json.RawMessage(`{"port":"invalid"}`)},
			{Role: "coordinators", Name: "default", Replicas: 1},
		},
	}
}

func roleConfigBuilt(t *testing.T, roles []BuiltRole, name string) BuiltRole {
	t.Helper()
	for _, role := range roles {
		if role.Role.Name == name {
			return role
		}
	}
	t.Fatalf("missing role %q", name)
	return BuiltRole{}
}

func TestRoleConfigBuildUsesAllDeclaredReplicasWithoutProductCallbacks(t *testing.T) {
	definition, source := roleConfigDefinition(), roleConfigSource()
	definition.ValidateInput = func(framework.EffectiveInput[struct{}, struct{}, struct{}]) error {
		t.Fatal("management resources called product input validation")
		return nil
	}
	definition.GenerateGroup = func(framework.EffectiveInput[struct{}, struct{}, struct{}]) (RuntimeDescription, error) {
		t.Fatal("management resources called workload generation")
		return RuntimeDescription{}, nil
	}
	definition.GenerateCluster = func(framework.ClusterOutputInput[struct{}, struct{}]) (ClusterOutput, error) {
		t.Fatal("management resources called cluster generation")
		return ClusterOutput{}, nil
	}
	definition.ValidateFinal = func(FinalView) []Check {
		t.Fatal("management resources called workload final validation")
		return nil
	}
	roles, err := BuildRoleResources(definition, source)
	if err != nil || len(roles) != 2 || roles[0].Role.Name != "coordinators" || roles[1].Role.Name != "workers" {
		t.Fatalf("role inventory was lost or unordered: %+v, %v", roles, err)
	}
	for name, wantMinimum := range map[string]int32{"coordinators": 1, "workers": 4} {
		role := roleConfigBuilt(t, roles, name)
		if role.Error != "" || role.Config == nil || role.PodDisruptionBudget == nil {
			t.Fatalf("group input failure blocked role management: %+v", role)
		}
		pdb := role.PodDisruptionBudget
		if pdb.Spec.MinAvailable == nil || pdb.Spec.MinAvailable.Type != intstr.Int ||
			pdb.Spec.MinAvailable.IntVal != wantMinimum || pdb.Spec.MaxUnavailable != nil {
			t.Fatalf("PDB inferred controller scale instead of all desired replicas: %+v", pdb.Spec)
		}
		wantSelector := map[string]string{"app.kubernetes.io/instance": "roles", "app.kubernetes.io/component": name}
		if pdb.Spec.Selector == nil || !reflect.DeepEqual(pdb.Spec.Selector.MatchLabels, wantSelector) ||
			len(pdb.Spec.Selector.MatchExpressions) != 0 {
			t.Fatalf("role PDB must select all groups of exactly this role: %+v", pdb.Spec.Selector)
		}
		if pdb.Name != "roles-"+name+"-pdb" || pdb.Namespace != source.Cluster.Namespace ||
			pdb.Labels["team"] != "data" || pdb.Labels["app.kubernetes.io/instance"] != "roles" ||
			pdb.Labels["app.kubernetes.io/component"] != name {
			t.Fatalf("wrong PDB identity or propagated labels: %+v", pdb.ObjectMeta)
		}
	}
}

func TestRoleConfigPresenceDefaultsAndExplicitZero(t *testing.T) {
	for _, item := range []struct {
		name, layer string
		enabled     bool
		unavailable int32
	}{
		{"omitted", "", true, 1},
		{"empty object", `{}`, true, 1},
		{"empty pdb", `{"podDisruptionBudget":{}}`, true, 1},
		{"disable only", `{"podDisruptionBudget":{"enabled":false}}`, false, 1},
		{"explicit zero", `{"podDisruptionBudget":{"maxUnavailable":0}}`, true, 0},
		{"both fields", `{"podDisruptionBudget":{"enabled":false,"maxUnavailable":0}}`, false, 0},
		{"above desired", `{"podDisruptionBudget":{"maxUnavailable":9}}`, true, 9},
	} {
		t.Run(item.name, func(t *testing.T) {
			source := roleConfigSource()
			source.Roles[0].Config = json.RawMessage(item.layer)
			roles, err := BuildRoleResources(roleConfigDefinition(), source)
			if err != nil {
				t.Fatal(err)
			}
			role := roleConfigBuilt(t, roles, "workers")
			if role.Error != "" || role.Config == nil ||
				role.Config.PodDisruptionBudget != (PodDisruptionBudgetConfig{
					Enabled: item.enabled, MaxUnavailable: item.unavailable}) {
				t.Fatalf("field presence did not override only the supplied field: %+v", role)
			}
			if (role.PodDisruptionBudget != nil) != item.enabled {
				t.Fatalf("explicit enabled did not control resource emission: %+v", role)
			}
			if item.enabled && role.PodDisruptionBudget.Spec.MinAvailable.IntVal != max(0, 5-item.unavailable) {
				t.Fatal("minimum was not clamped at zero")
			}
		})
	}
	falseValue, zero := false, int32(0)
	input := RoleConfigInput{PodDisruptionBudget: &PodDisruptionBudgetInput{
		Enabled: &falseValue, MaxUnavailable: &zero}}
	data, err := json.Marshal(input)
	if err != nil || string(data) != `{"podDisruptionBudget":{"enabled":false,"maxUnavailable":0}}` {
		t.Fatalf("typed presence input dropped explicit false/zero: %s, %v", data, err)
	}
	empty, err := json.Marshal(RoleConfigInput{})
	if err != nil || string(empty) != `{}` {
		t.Fatalf("omitted management input gained explicit values: %s, %v", empty, err)
	}
}

func TestRoleConfigZeroGroupsAreDifferentFromAbsentRole(t *testing.T) {
	source := roleConfigSource()
	source.Groups = nil
	roles, err := BuildRoleResources(roleConfigDefinition(), source)
	if err != nil || len(roles) != 2 {
		t.Fatalf("zero-group roles disappeared: %+v, %v", roles, err)
	}
	for _, role := range roles {
		if role.Error != "" || role.PodDisruptionBudget == nil || role.PodDisruptionBudget.Spec.MinAvailable.IntVal != 0 {
			t.Fatalf("enabled empty role did not produce minAvailable=0: %+v", role)
		}
	}
	source.Roles = nil
	roles, err = BuildRoleResources(roleConfigDefinition(), source)
	if err != nil || len(roles) != 0 {
		t.Fatalf("product declarations invented absent CR roles: %+v, %v", roles, err)
	}
	source.Roles = []RoleSource{{Name: "workers"}}
	definition := roleConfigDefinition()
	definition.Roles["workers"] = framework.RoleDefinition[struct{}]{}
	roles, err = BuildRoleResources(definition, source)
	if err != nil || len(roles) != 1 || roles[0].Error != "" || roles[0].Config == nil ||
		roles[0].PodDisruptionBudget != nil || roles[0].Config.PodDisruptionBudget.Enabled {
		t.Fatalf("zero-value product policy must disable the PDB: %+v, %v", roles, err)
	}
}

func TestRoleConfigRejectsInvalidManagementOnlyForItsRole(t *testing.T) {
	for _, layer := range []string{
		`null`, `[]`, `"text"`, `{`, `{"unknown":true}`, `{"podDisruptionBudget":null}`,
		`{"podDisruptionBudget":{"unknown":1}}`, `{"podDisruptionBudget":{"enabled":null}}`,
		`{"podDisruptionBudget":{"enabled":"true"}}`, `{"podDisruptionBudget":{"maxUnavailable":null}}`,
		`{"podDisruptionBudget":{"maxUnavailable":-1}}`,
		`{"podDisruptionBudget":{"enabled":false,"maxUnavailable":-1}}`,
		`{"podDisruptionBudget":{"maxUnavailable":2147483648}}`,
		`{"podDisruptionBudget":{"maxUnavailable":0.5}}`,
	} {
		t.Run(layer, func(t *testing.T) {
			source := roleConfigSource()
			source.Roles[0].Config = json.RawMessage(layer)
			roles, err := BuildRoleResources(roleConfigDefinition(), source)
			if err != nil {
				t.Fatalf("invalid management config invalidated the whole identity inventory: %v", err)
			}
			bad, good := roleConfigBuilt(t, roles, "workers"), roleConfigBuilt(t, roles, "coordinators")
			if bad.Error == "" || bad.Config != nil || bad.PodDisruptionBudget != nil ||
				good.Error != "" || good.PodDisruptionBudget == nil {
				t.Fatalf("management validation did not isolate the role: %+v", roles)
			}
		})
	}
	definition := roleConfigDefinition()
	definition.Roles["workers"] = framework.RoleDefinition[struct{}]{RoleConfig: RoleConfig{
		PodDisruptionBudget: PodDisruptionBudgetConfig{Enabled: false, MaxUnavailable: -1}}}
	roles, err := BuildRoleResources(definition, roleConfigSource())
	if err != nil || roleConfigBuilt(t, roles, "workers").Error == "" {
		t.Fatalf("disabled invalid defaults escaped validation: %+v, %v", roles, err)
	}
	delete(definition.Roles, "workers")
	roles, err = BuildRoleResources(definition, roleConfigSource())
	if err != nil || roleConfigBuilt(t, roles, "workers").Error == "" ||
		roleConfigBuilt(t, roles, "coordinators").PodDisruptionBudget == nil {
		t.Fatalf("undeclared product role lost its identity or blocked another role: %+v, %v", roles, err)
	}
}

func TestRoleConfigDuplicateFieldsDoNotBecomeDisabledBudgets(t *testing.T) {
	for _, layer := range []string{
		`{"podDisruptionBudget":{"enabled":true,"enabled":false}}`,
		`{"podDisruptionBudget":{"enabled":true},"podDisruptionBudget":{"enabled":false}}`,
	} {
		t.Run(layer, func(t *testing.T) {
			source := roleConfigSource()
			source.Roles[0].Config = json.RawMessage(layer)
			roles, err := BuildRoleResources(roleConfigDefinition(), source)
			if err != nil {
				t.Fatalf("duplicate management field blocked the complete source: %v", err)
			}
			bad, good := roleConfigBuilt(t, roles, "workers"), roleConfigBuilt(t, roles, "coordinators")
			if !strings.Contains(bad.Error, "duplicate field") || bad.Config != nil || bad.PodDisruptionBudget != nil {
				t.Fatalf("duplicate field was treated as valid budget configuration: %+v", bad)
			}
			if good.Error != "" || good.Config == nil || good.PodDisruptionBudget == nil ||
				good.PodDisruptionBudget.Spec.MinAvailable.IntVal != 1 {
				t.Fatalf("duplicate management field blocked the other role: %+v", good)
			}
		})
	}
}

func TestRoleConfigRejectsUnreliableIdentityInventories(t *testing.T) {
	for _, item := range []struct {
		name string
		edit func(*SourceSnapshot[struct{}])
	}{
		{"cluster name", func(s *SourceSnapshot[struct{}]) { s.Cluster.Name = "" }},
		{"namespace", func(s *SourceSnapshot[struct{}]) { s.Cluster.Namespace = "INVALID" }},
		{"empty role", func(s *SourceSnapshot[struct{}]) { s.Roles[0].Name = "" }},
		{"invalid role", func(s *SourceSnapshot[struct{}]) { s.Roles[0].Name = "workers.invalid" }},
		{"duplicate role", func(s *SourceSnapshot[struct{}]) { s.Roles = append(s.Roles, s.Roles[0]) }},
		{"missing roles", func(s *SourceSnapshot[struct{}]) { s.Roles = nil }},
		{"unlisted group role", func(s *SourceSnapshot[struct{}]) { s.Groups[0].Role = "unlisted" }},
		{"duplicate group", func(s *SourceSnapshot[struct{}]) { s.Groups = append(s.Groups, s.Groups[0]) }},
		{"invalid group name", func(s *SourceSnapshot[struct{}]) { s.Groups[0].Name = "INVALID" }},
		{"negative replicas", func(s *SourceSnapshot[struct{}]) { s.Groups[0].Replicas = -1 }},
	} {
		t.Run(item.name, func(t *testing.T) {
			source := roleConfigSource()
			item.edit(&source)
			if identities, err := SourceRoleIdentities(source); err == nil || identities != nil {
				t.Fatalf("unsafe or partial inventory accepted: %+v, %v", identities, err)
			}
			if roles, err := BuildRoleResources(roleConfigDefinition(), source); err == nil || roles != nil {
				t.Fatalf("unsafe inventory reached resource generation: %+v, %v", roles, err)
			}
		})
	}
}

func TestRoleConfigReplicaArithmeticDoesNotOverflow(t *testing.T) {
	source := roleConfigSource()
	source.Groups[0].Replicas, source.Groups[1].Replicas = math.MaxInt32, math.MaxInt32
	definition := roleConfigDefinition()
	roles, err := BuildRoleResources(definition, source)
	if err != nil {
		t.Fatal(err)
	}
	bad, good := roleConfigBuilt(t, roles, "workers"), roleConfigBuilt(t, roles, "coordinators")
	if bad.Error == "" || bad.PodDisruptionBudget != nil || good.PodDisruptionBudget == nil {
		t.Fatalf("overflow produced a permissive PDB or blocked another role: %+v", roles)
	}
	definition.Roles["workers"] = framework.RoleDefinition[struct{}]{RoleConfig: RoleConfig{
		PodDisruptionBudget: PodDisruptionBudgetConfig{Enabled: true, MaxUnavailable: math.MaxInt32}}}
	roles, err = BuildRoleResources(definition, source)
	if err != nil {
		t.Fatal(err)
	}
	worker := roleConfigBuilt(t, roles, "workers")
	if worker.Error != "" || worker.PodDisruptionBudget == nil ||
		worker.PodDisruptionBudget.Spec.MinAvailable.IntVal != math.MaxInt32 {
		t.Fatalf("valid int32 minimum rejected because the intermediate sum exceeds int32: %+v", worker)
	}
}

func TestRoleConfigResourcesOwnDefaultsAndSourceSnapshots(t *testing.T) {
	definition, source := roleConfigDefinition(), roleConfigSource()
	before, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	roles, err := BuildRoleResources(definition, source)
	if err != nil {
		t.Fatal(err)
	}
	worker := roleConfigBuilt(t, roles, "workers")
	worker.Config.PodDisruptionBudget.Enabled = false
	worker.Role.Labels["team"] = "role mutation"
	worker.PodDisruptionBudget.Labels["team"] = "metadata mutation"
	worker.PodDisruptionBudget.Spec.Selector.MatchLabels["app.kubernetes.io/instance"] = "selector mutation"
	after, err := json.Marshal(source)
	if err != nil || string(before) != string(after) ||
		!definition.Roles["workers"].RoleConfig.PodDisruptionBudget.Enabled {
		t.Fatalf("built role resources mutated source/defaults: %s, %v", after, err)
	}
	coordinator := roleConfigBuilt(t, roles, "coordinators")
	if coordinator.Role.Labels["team"] != "data" || coordinator.PodDisruptionBudget.Labels["team"] != "data" ||
		coordinator.PodDisruptionBudget.Spec.Selector.MatchLabels["app.kubernetes.io/instance"] != "roles" {
		t.Fatal("one role's resources alias another role")
	}
	again, err := BuildRoleResources(definition, source)
	if err != nil || !roleConfigBuilt(t, again, "workers").Config.PodDisruptionBudget.Enabled {
		t.Fatalf("a previous result altered later builds: %+v, %v", again, err)
	}
	// PDB names use the subdomain resource-name limit, not a Service label limit.
	source.Cluster.Name = strings.Repeat("c", 63)
	source.Roles = []RoleSource{{Name: strings.Repeat("r", 63)}}
	source.Groups = nil
	definition.Roles[source.Roles[0].Name] = framework.RoleDefinition[struct{}]{RoleConfig: RoleConfig{
		PodDisruptionBudget: PodDisruptionBudgetConfig{Enabled: true}}}
	roles, err = BuildRoleResources(definition, source)
	if err != nil || len(roles) != 1 || roles[0].Error != "" || roles[0].PodDisruptionBudget == nil {
		t.Fatalf("valid PDB subdomain name was rejected: %+v, %v", roles, err)
	}
}
