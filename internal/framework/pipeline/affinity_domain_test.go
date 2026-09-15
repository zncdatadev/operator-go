package pipeline

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/zncdatadev/operator-go/pkg/framework"
	frameworkinput "github.com/zncdatadev/operator-go/pkg/framework/input"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func affinityDefaults() framework.Config[testProductConfig] {
	defaults := testDefaults()
	defaults.Common.Affinity = corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{{
			Weight: 20, Preference: corev1.NodeSelectorTerm{MatchExpressions: []corev1.NodeSelectorRequirement{{
				Key: "pool", Operator: corev1.NodeSelectorOpIn, Values: []string{"old"}}}},
		}}},
		PodAffinity: &corev1.PodAffinity{RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{{
			TopologyKey: "kubernetes.io/hostname", LabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "neighbor"}},
		}}},
		PodAntiAffinity: &corev1.PodAntiAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{{
				Weight: 40, PodAffinityTerm: corev1.PodAffinityTerm{TopologyKey: "topology.kubernetes.io/zone"},
			}},
		},
	}
	return defaults
}

func TestAffinityBranchesInheritOrReplaceAsCompletePolicies(t *testing.T) {
	defaults := affinityDefaults()
	role := json.RawMessage(`{"affinity":{"nodeAffinity":{"requiredDuringSchedulingIgnoredDuringExecution":{
	  "nodeSelectorTerms":[{"matchExpressions":[{"key":"pool","operator":"In","values":["new"]}]}]}}}}`)
	group := json.RawMessage(`{"affinity":{"podAntiAffinity":{}}}`)
	resolved, err := ResolveConfig(defaults, role, group)
	if err != nil {
		t.Fatal(err)
	}
	got := resolved.Common.Affinity
	if got.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil ||
		len(got.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution) != 0 {
		t.Fatalf("node branch was deep-merged instead of replaced: %+v", got.NodeAffinity)
	}
	if !reflect.DeepEqual(got.PodAffinity, defaults.Common.Affinity.PodAffinity) ||
		got.PodAntiAffinity == nil || len(got.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution) != 0 {
		t.Fatalf("sibling inheritance or explicit branch clearing failed: %+v", got)
	}
	inherited, err := ResolveConfig(defaults, nil, json.RawMessage(`{"affinity":{}}`))
	if err != nil || !reflect.DeepEqual(inherited.Common.Affinity, defaults.Common.Affinity) {
		t.Fatalf("an empty top-level affinity lost inherited branches: %+v %v", inherited.Common.Affinity, err)
	}
}

func TestNativeAffinityInputRoundTripDoesNotResurrectClearedRules(t *testing.T) {
	input := struct {
		Affinity *corev1.Affinity `json:"affinity,omitempty"`
	}{Affinity: &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{
		PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{},
	}}}
	layer, err := frameworkinput.ConfigJSON(&input)
	if err != nil {
		t.Fatal(err)
	}
	if string(layer) != `{"affinity":{"nodeAffinity":{}}}` {
		t.Fatalf("unexpected native empty-list normalization: %s", layer)
	}
	resolved, err := ResolveConfig(affinityDefaults(), nil, layer)
	if err != nil || resolved.Common.Affinity.NodeAffinity == nil ||
		len(resolved.Common.Affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution) != 0 ||
		resolved.Common.Affinity.PodAffinity == nil {
		t.Fatalf("typed empty list inherited removed rules: %+v %v", resolved.Common.Affinity, err)
	}
	clone := CloneInput(resolved)
	clonedTerms := clone.Common.Affinity.PodAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	clonedTerms[0].LabelSelector.MatchLabels["app"] = "new"
	originalTerms := resolved.Common.Affinity.PodAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	if originalTerms[0].LabelSelector.MatchLabels["app"] != "neighbor" {
		t.Fatal("native affinity clone shared mutable selectors")
	}
}

func TestAffinityStrictInputRejectsUnknownDuplicateAndNull(t *testing.T) {
	for _, layer := range []string{
		`{"affinity":null}`, `{"affinity":[]}`, `{"affinity":{"nodeAffinity":null}}`,
		`{"affinity":{"nodeAffinitY":{}}}`, `{"affinity":{"nodeAffinity":{"unknown":1}}}`,
		`{"affinity":{"podAffinity":{"requiredDuringSchedulingIgnoredDuringExecution":[null]}}}`,
		`{"affinity":{"nodeAffinity":{},"nodeAffinity":{}}}`,
		`{"affinity":{"nodeAffinity":{}},"affinity":{}}`,
	} {
		_, err := ResolveConfig(affinityDefaults(), json.RawMessage(layer), json.RawMessage(`{"affinity":{}}`))
		if err == nil {
			t.Fatalf("invalid lower affinity layer was hidden: %s", layer)
		}
	}
	if err := checkProfile(reflect.TypeFor[struct {
		Scheduling corev1.Affinity `json:"scheduling"`
	}](), make(map[reflect.Type]bool)); err == nil {
		t.Fatal("native common affinity opened arbitrary product pointer graphs")
	}
}
