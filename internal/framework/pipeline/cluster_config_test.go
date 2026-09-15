package pipeline

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type clusterConfigFixture struct {
	Enabled   bool                         `json:"enabled"`
	Count     int                          `json:"count"`
	Name      string                       `json:"name"`
	Resources string                       `json:"resources"`
	Labels    map[string]map[string]string `json:"labels"`
	Args      []string                     `json:"args"`
	Capacity  resource.Quantity            `json:"capacity"`
	Interval  metav1.Duration              `json:"interval"`
}

func clusterConfigDefaults() clusterConfigFixture {
	return clusterConfigFixture{Enabled: true, Count: 5, Name: "default", Resources: "product-owned",
		Labels: map[string]map[string]string{"shared": {"keep": "yes", "replace": "old"}},
		Args:   []string{"one", "two"}, Capacity: resource.MustParse("1Gi"),
		Interval: metav1.Duration{Duration: time.Second}}
}

func TestClusterConfigFixedInheritanceAndPresence(t *testing.T) {
	defaults := clusterConfigDefaults()
	before := CloneInput(defaults)
	raw := json.RawMessage(`{"enabled":false,"count":0,"name":"","resources":"cluster-owned",
		"labels":{"shared":{"replace":"new"},"added":{"key":"value"}},"args":[],
		"capacity":"2Gi","interval":"-500ms"}`)
	actual, err := ResolveClusterConfig(defaults, raw)
	if err != nil {
		t.Fatal(err)
	}
	want := clusterConfigDefaults()
	want.Enabled, want.Count, want.Name, want.Resources = false, 0, "", "cluster-owned"
	want.Labels = map[string]map[string]string{"shared": {"keep": "yes", "replace": "new"}, "added": {"key": "value"}}
	want.Args, want.Capacity = []string{}, resource.MustParse("2Gi")
	want.Interval.Duration = -500 * time.Millisecond
	if !reflect.DeepEqual(actual, want) || !reflect.DeepEqual(defaults, before) {
		t.Fatalf("cluster fixed-rule fold changed: %+v; defaults %+v", actual, defaults)
	}
	actual.Labels["shared"]["keep"] = "caller mutation"
	if defaults.Labels["shared"]["keep"] != "yes" {
		t.Fatal("effective cluster config aliases defaults")
	}
}

func TestClusterConfigOmittedAndEmptyObjectsInherit(t *testing.T) {
	for _, raw := range []json.RawMessage{nil, json.RawMessage(`{}`), json.RawMessage(`{"labels":{"shared":{}}}`)} {
		defaults := clusterConfigDefaults()
		actual, err := ResolveClusterConfig(defaults, raw)
		if err != nil || !reflect.DeepEqual(actual, defaults) {
			t.Fatalf("empty object cleared defaults: %s %+v %v", raw, actual, err)
		}
		actual.Args[0], actual.Labels["shared"]["keep"] = "changed", "changed"
		if defaults.Args[0] != "one" || defaults.Labels["shared"]["keep"] != "yes" {
			t.Fatal("omitted cluster input returned mutable defaults")
		}
	}
	if _, err := ResolveClusterConfig(struct{}{}, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("product without cluster fields requires fake defaults: %v", err)
	}
}

func TestClusterConfigRejectsMalformedUserLayer(t *testing.T) {
	for _, raw := range []string{
		`null`, `[]`, `{"unknown":1}`, `{"enabled":null}`, `{"labels":{"shared":null}}`,
		`{"labels":{"shared":{"keep":null}}}`, `{"args":null}`, `{"args":[null]}`, `{"args":"scalar"}`,
		`{"enabled":true,"enabled":false}`, `{"labels":{"shared":{"keep":"a","keep":"b"}}}`,
		`{"labels":{"shared":{},"shared":{}}}`, `{"capacity":"invalid"}`, `{"interval":"later"}`,
	} {
		t.Run(raw, func(t *testing.T) {
			_, err := ResolveClusterConfig(clusterConfigDefaults(), json.RawMessage(raw))
			if err == nil || !strings.Contains(err.Error(), "clusterConfig") {
				t.Fatalf("malformed layer lost its path or was accepted: %v", err)
			}
		})
	}
}

func TestClusterConfigRejectsUnsupportedProfilesAndDefaults(t *testing.T) {
	errors := make([]error, 0, 7)
	_, err := ResolveClusterConfig("scalar", nil)
	errors = append(errors, err)
	_, err = ResolveClusterConfig(map[string]string{}, nil)
	errors = append(errors, err)
	_, err = ResolveClusterConfig(resource.MustParse("1Gi"), nil)
	errors = append(errors, err)
	_, err = ResolveClusterConfig(metav1.Duration{}, nil)
	errors = append(errors, err)
	_, err = ResolveClusterConfig(struct{ Value *string }{}, nil)
	errors = append(errors, err)
	_, err = ResolveClusterConfig(struct{ Scheduling corev1.Affinity }{}, nil)
	errors = append(errors, err)
	_, err = ResolveClusterConfig(struct{ Value float64 }{Value: math.NaN()}, nil)
	errors = append(errors, err)
	for i, err := range errors {
		if err == nil || !strings.Contains(err.Error(), "clusterConfig") {
			t.Fatalf("unsupported cluster profile/default %d accepted: %v", i, err)
		}
	}
}
