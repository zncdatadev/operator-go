package pipeline

import (
	"encoding/json"
	"strings"
	"testing"
)

func namingSource(groups ...GroupSource) SourceSnapshot[struct{}] {
	return SourceSnapshot[struct{}]{
		Cluster: ClusterIdentity{Name: "c", Namespace: "fixture", Labels: map[string]string{"team": "data"}},
		Roles:   []RoleSource{{Name: "w"}}, Groups: groups,
	}
}

func TestSourceNamesCheckHeadlessBeforeConfiguration(t *testing.T) {
	// c-w- plus 50 characters is the largest base fitting the headless suffix.
	source := namingSource(GroupSource{Role: "w", Name: strings.Repeat("g", 50), Replicas: 1,
		Config: json.RawMessage(`{"invalid":"unresolved configuration"}`)})
	identities, err := SourceGroupIdentities(source)
	if err != nil || len(identities) != 1 || len(identities[0].ServiceName()+"-headless") != 63 {
		t.Fatalf("legal boundary or complete declared group lost: %+v %v", identities, err)
	}
	identities[0].Labels["team"] = "changed"
	if source.Cluster.Labels["team"] != "data" {
		t.Fatal("identity inventory aliases source metadata")
	}
	source.Groups[0].Name += "g"
	identities, err = SourceGroupIdentities(source)
	if err == nil || identities != nil || !strings.Contains(err.Error(), "headless Service name") {
		t.Fatalf("valid ordinary Service hid an invalid headless name: %+v %v", identities, err)
	}
	if _, err := SourceRoleIdentities(source); err == nil {
		t.Fatal("role inventory accepted a known-invalid group resource name")
	}
}

func TestSourceNamesRejectCrossGroupResourceCollisions(t *testing.T) {
	for _, groups := range [][]GroupSource{
		{{Role: "w", Name: "a"}, {Role: "w", Name: "a-headless", Config: json.RawMessage(`null`)}},
		{{Role: "a-b", Name: "c"}, {Role: "a", Name: "b-c"}},
	} {
		identities, err := SourceGroupIdentities(namingSource(groups...))
		if err == nil || identities != nil || !strings.Contains(err.Error(), "same Service name") {
			t.Fatalf("name collision escaped the complete identity stage: %+v %v", identities, err)
		}
	}
}

func TestSourceNamesRespectResourceSpecificRules(t *testing.T) {
	source := namingSource(GroupSource{Role: "w", Name: "a"})
	source.Cluster.Name = "9cluster"
	if _, err := SourceGroupIdentities(source); err == nil {
		t.Fatal("numeric prefix cannot produce a DNS1035 Service name")
	}
	// A role with no groups has a PDB, which follows DNS subdomain rules instead.
	source.Groups = nil
	source.Cluster.Name = strings.Repeat("c", 63)
	source.Roles = []RoleSource{{Name: strings.Repeat("r", 63)}}
	roles, err := SourceRoleIdentities(source)
	if err != nil || len(roles) != 1 || len(roles[0].PodDisruptionBudgetName()) <= 63 {
		t.Fatalf("PDB name incorrectly received the Service length limit: %+v %v", roles, err)
	}
}
