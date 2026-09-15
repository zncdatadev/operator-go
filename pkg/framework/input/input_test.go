package input_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/zncdatadev/operator-go/pkg/framework/input"
)

type localSpec struct {
	Enabled  *bool              `json:"enabled,omitempty"`
	Args     *[]string          `json:"args,omitempty"`
	CPU      *resource.Quantity `json:"cpu,omitempty"`
	Timeout  *metav1.Duration   `json:"timeout,omitempty"`
	Affinity *corev1.Affinity   `json:"affinity,omitempty"`
	Pod      json.RawMessage    `json:"pod,omitempty"`
}

type localCR struct {
	Spec localSpec `json:"spec"`
}

func TestDecodePreservesPresenceAndNativePatch(t *testing.T) {
	var object localCR
	data := []byte(`{"spec":{"enabled":false,"args":[],"cpu":"250m","timeout":"0s",` +
		`"affinity":{"nodeAffinity":{}},"pod":{"spec":{"containers":[{"name":"main","env":null,"$patch":"replace"}]}}}}`)
	if err := input.DecodeJSON(data, &object); err != nil {
		t.Fatal(err)
	}
	if object.Spec.Enabled == nil || *object.Spec.Enabled || object.Spec.Args == nil ||
		*object.Spec.Args == nil || len(*object.Spec.Args) != 0 || object.Spec.Affinity.NodeAffinity == nil {
		t.Fatalf("explicit values were lost: %#v", object.Spec)
	}
	if object.Spec.CPU.Cmp(resource.MustParse("250m")) != 0 || object.Spec.Timeout.Duration != 0 ||
		!strings.Contains(string(object.Spec.Pod), `"env":null`) {
		t.Fatalf("native input changed: %#v", object.Spec)
	}
	before := input.Clone(object)
	for _, data := range []string{
		`{"spec":{"enabled":true,"enabled":false}}`,
		`{"spec":{"Enabled":true}}`,
		`{"spec":{"cpu":0.5}}`,
		`{"spec":{"args":null}}`,
		`{"spec":{"affinity":{"nodeAffinity":null}}}`,
		`{"spec":{"timeout":"not-a-duration"}}`,
		`{"spec":{"pod":{"spec":{"hostNetwork":true,"hostNetwork":false}}}}`,
	} {
		if err := input.DecodeJSON([]byte(data), &object); err == nil {
			t.Errorf("ambiguous input accepted: %s", data)
		}
		if !reflect.DeepEqual(before, object) {
			t.Fatalf("failed decode changed the previous object: %s", data)
		}
	}
}

func TestCloneIsolatesCollectionsAndNativeValues(t *testing.T) {
	original := struct {
		Values map[string]*[]string
		CPU    resource.Quantity
		Wait   metav1.Duration
		Rules  corev1.Affinity
	}{
		Values: map[string]*[]string{"empty": ptr.To([]string{}), "list": ptr.To([]string{"original"}), "absent": nil},
		CPU:    resource.MustParse("123456789012345678901m"),
		Wait:   metav1.Duration{Duration: time.Second},
		Rules: corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{{Weight: 1}},
		}},
	}
	copy := input.Clone(original)
	(*copy.Values["list"])[0] = "changed"
	copy.CPU.Add(resource.MustParse("1"))
	copy.Rules.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution[0].Weight = 10
	if (*original.Values["list"])[0] != "original" || original.CPU.Cmp(copy.CPU) == 0 ||
		original.Rules.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution[0].Weight != 1 {
		t.Fatal("clone retained mutable aliases")
	}
	if copy.Values["empty"] == nil || *copy.Values["empty"] == nil || copy.Values["absent"] != nil || copy.Wait != original.Wait {
		t.Fatal("clone lost collection presence or native duration")
	}
}

func TestRawConfigSeparatesOperationsAndPresence(t *testing.T) {
	type cluster struct {
		Stopped *bool   `json:"stopped,omitempty"`
		Paused  *bool   `json:"reconciliationPaused,omitempty"`
		Label   *string `json:"label,omitempty"`
	}
	data, err := input.ClusterConfigJSON(&cluster{Stopped: ptr.To(true), Paused: ptr.To(false), Label: ptr.To("")})
	if err != nil || string(data) != `{"label":""}` {
		t.Fatalf("product projection includes controls or lost value: %s, %v", data, err)
	}
	absent, err := input.ConfigJSON[cluster](nil)
	if err != nil || absent != nil {
		t.Fatalf("absent config changed: %s, %v", absent, err)
	}
	empty, err := input.ConfigJSON(&cluster{})
	if err != nil || string(empty) != `{}` {
		t.Fatalf("empty object changed: %s, %v", empty, err)
	}
}

type recursive map[string]recursive
type encoded string

func (encoded) MarshalText() ([]byte, error) { return []byte("custom"), nil }

func TestProductProfileRejectsUnsupportedRepresentations(t *testing.T) {
	for _, typ := range []reflect.Type{
		nil, reflect.TypeFor[*string](), reflect.TypeFor[any](), reflect.TypeFor[[]byte](),
		reflect.TypeFor[map[int]string](), reflect.TypeFor[recursive](), reflect.TypeFor[encoded](),
		reflect.TypeFor[struct{ Embedded localSpec }](),
	} {
		if err := input.ValidateProductType(typ); err == nil {
			t.Errorf("unsupported product type accepted: %v", typ)
		}
	}
	if err := input.ValidateProductType(reflect.TypeFor[struct {
		Enabled bool               `json:"enabled"`
		Values  map[string][]int32 `json:"values"`
		CPU     resource.Quantity  `json:"cpu"`
		Wait    metav1.Duration    `json:"wait"`
	}]()); err != nil {
		t.Fatal(err)
	}
	if err := input.CheckVersion(input.ContractVersion); err != nil {
		t.Fatal(err)
	}
	if err := input.CheckVersion(input.ContractVersion + 1); err == nil {
		t.Fatal("incompatible generated contract accepted")
	}
}
