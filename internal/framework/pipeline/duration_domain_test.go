package pipeline

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/zncdatadev/operator-go/pkg/framework"
	frameworkinput "github.com/zncdatadev/operator-go/pkg/framework/input"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestShutdownDurationPresenceAndExactSeconds(t *testing.T) {
	defaults := testDefaults()
	defaults.Common.GracefulShutdownTimeout = metav1.Duration{Duration: 30 * time.Second}
	for _, item := range []struct {
		role, group string
		want        time.Duration
	}{
		{"", "", 30 * time.Second},
		{`{"gracefulShutdownTimeout":"90s"}`, `{"port":9000}`, 90 * time.Second},
		{`{"gracefulShutdownTimeout":"90s"}`, `{"gracefulShutdownTimeout":"0s"}`, 0},
		{"", `{"gracefulShutdownTimeout":"1.5m"}`, 90 * time.Second},
		{"", `{"gracefulShutdownTimeout":"1000ms"}`, time.Second},
		{`{"gracefulShutdownTimeout":"0.5s"}`, `{"gracefulShutdownTimeout":"1s"}`, time.Second},
		{`{"gracefulShutdownTimeout":"-2s"}`, `{"gracefulShutdownTimeout":"3s"}`, 3 * time.Second},
	} {
		resolved, err := ResolveConfig(defaults, json.RawMessage(item.role), json.RawMessage(item.group))
		if err != nil || resolved.Common.GracefulShutdownTimeout.Duration != item.want {
			t.Fatalf("duration inheritance %q/%q: got=%v want=%v err=%v",
				item.role, item.group, resolved.Common.GracefulShutdownTimeout, item.want, err)
		}
	}
	input := struct {
		GracefulShutdownTimeout *metav1.Duration `json:"gracefulShutdownTimeout,omitempty"`
	}{GracefulShutdownTimeout: &metav1.Duration{}}
	layer, err := frameworkinput.ConfigJSON(&input)
	if err != nil || string(layer) != `{"gracefulShutdownTimeout":"0s"}` {
		t.Fatalf("explicit zero duration disappeared: %s %v", layer, err)
	}
	copy := CloneInput(input)
	copy.GracefulShutdownTimeout.Duration = time.Minute
	if input.GracefulShutdownTimeout.Duration != 0 {
		t.Fatal("duration presence clone shared its pointer")
	}
}

func TestShutdownDurationSeparatesLayerSyntaxFromEffectivePolicy(t *testing.T) {
	for _, value := range []string{`null`, `0`, `""`,
		`"9223372036854775807s"`, `"30"`, `"one minute"`} {
		role := json.RawMessage(`{"gracefulShutdownTimeout":` + value + `}`)
		if _, err := ResolveConfig(testDefaults(), role,
			json.RawMessage(`{"gracefulShutdownTimeout":"30s"}`)); err == nil {
			t.Fatalf("invalid duration was hidden by a later value: %s", value)
		}
	}
	for _, value := range []string{`"-1s"`, `"1.5s"`, `"1ms"`} {
		group := json.RawMessage(`{"gracefulShutdownTimeout":` + value + `}`)
		if _, err := ResolveConfig(testDefaults(), nil, group); err == nil {
			t.Fatalf("invalid effective shutdown policy was accepted: %s", value)
		}
	}
	for _, value := range []time.Duration{-time.Second, time.Millisecond} {
		defaults := testDefaults()
		defaults.Common.GracefulShutdownTimeout.Duration = value
		if _, err := ResolveConfig(defaults, nil, nil); err == nil {
			t.Fatalf("invalid effective default was accepted: %v", value)
		}
	}
	if err := checkProfile(reflect.TypeFor[struct {
		Period metav1.Duration `json:"period"`
	}](), make(map[reflect.Type]bool)); err != nil {
		t.Fatalf("known duration scalar is not supported: %v", err)
	}
	if err := checkProfile(reflect.TypeFor[struct {
		Period *metav1.Duration `json:"period"`
	}](), make(map[reflect.Type]bool)); err == nil {
		t.Fatal("known scalar support opened arbitrary product pointer fields")
	}
}

func TestProductDurationDoesNotInheritShutdownPolicy(t *testing.T) {
	type product struct {
		Offset metav1.Duration `json:"offset"`
	}
	defaults := framework.Config[product]{Common: testDefaults().Common}
	resolved, err := ResolveConfig(defaults, nil, json.RawMessage(`{"offset":"-0.5s"}`))
	if err != nil || resolved.Product.Offset.Duration != -500*time.Millisecond {
		t.Fatalf("known duration scalar received unrelated shutdown policy: %+v %v", resolved.Product, err)
	}
}
