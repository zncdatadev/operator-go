package pipeline

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/zncdatadev/operator-go/pkg/framework"
	"k8s.io/apimachinery/pkg/api/resource"
)

type testProductConfig struct {
	Port    int32                        `json:"port"`
	Enabled bool                         `json:"enabled"`
	Name    string                       `json:"name"`
	Args    []string                     `json:"args"`
	Servers map[string]testServer        `json:"servers"`
	Limits  map[string]resource.Quantity `json:"limits"`
}

type testServer struct {
	Host   string            `json:"host"`
	Labels map[string]string `json:"labels"`
}

func testDefaults() framework.Config[testProductConfig] {
	return framework.Config[testProductConfig]{
		Common: CommonConfig{
			Resources: Resources{CPU: CPU{Min: resource.MustParse("500m"), Max: resource.MustParse("2")},
				Memory: Memory{Limit: resource.MustParse("4Gi")}},
			Logging: Logging{EnableVectorAgent: true, Containers: map[string]ContainerLogging{
				"trino": {Loggers: map[string]Logger{"io.trino": {Level: "INFO"}}},
			}},
		},
		Product: testProductConfig{Port: 8080, Enabled: true, Name: "default", Args: []string{"run"},
			Servers: map[string]testServer{"db.example": {Host: "db", Labels: map[string]string{"env": "test"}}}},
	}
}

func TestResolveConfigInheritance(t *testing.T) {
	cases := []struct {
		name, role, group string
		check             func(*testing.T, framework.Config[testProductConfig])
	}{
		{"explicit values", `{"port":9090,"enabled":true,"name":"role","args":["role"]}`,
			`{"port":0,"enabled":false,"name":"","args":[]}`, func(t *testing.T, got framework.Config[testProductConfig]) {
				if got.Product.Port != 0 || got.Product.Enabled || got.Product.Name != "" ||
					got.Product.Args == nil || len(got.Product.Args) != 0 {
					t.Fatalf("explicit empty and zero values were not preserved: %#v", got.Product)
				}
			}},
		{"field and map inheritance", `{"servers":{"db.example":{"host":"role","labels":{"a":"1"}}}}`,
			`{"servers":{"db.example":{"labels":{"a":"2"}}}}`, func(t *testing.T, got framework.Config[testProductConfig]) {
				want := testServer{Host: "role", Labels: map[string]string{"env": "test", "a": "2"}}
				if !reflect.DeepEqual(got.Product.Servers["db.example"], want) {
					t.Fatalf("dotted map key lost its identity or siblings: %#v", got.Product.Servers)
				}
			}},
		{"common domain", `{"resources":{"cpu":{"min":"250m"},"memory":{"limit":"8Gi"}}}`,
			`{"logging":{"enableVectorAgent":false,"containers":{"trino":{"loggers":{"io.trino":{"level":""}}}}}}`,
			func(t *testing.T, got framework.Config[testProductConfig]) {
				if got.Common.Resources.CPU.Min.String() != "250m" || got.Common.Resources.CPU.Max.String() != "2" ||
					got.Common.Resources.Memory.Limit.String() != "8Gi" || got.Common.Logging.EnableVectorAgent ||
					got.Common.Logging.Containers["trino"].Loggers["io.trino"].Level != "" {
					t.Fatalf("common fields did not inherit independently: %#v", got.Common)
				}
			}},
		{"empty object inherits", `{"servers":{"db.example":{"host":"role"}}}`, `{"servers":{}}`,
			func(t *testing.T, got framework.Config[testProductConfig]) {
				if got.Product.Servers["db.example"].Host != "role" {
					t.Fatal("empty object unexpectedly cleared inherited entries")
				}
			}},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			got, err := ResolveConfig(testDefaults(), json.RawMessage(item.role), json.RawMessage(item.group))
			if err != nil {
				t.Fatal(err)
			}
			item.check(t, got)
		})
	}
}

func TestResolveConfigRejectsInvalidLayers(t *testing.T) {
	cases := []struct{ name, role, group, message string }{
		{"unknown product", `{"typo":1}`, `{}`, "role.config.typo"},
		{"unknown nested", `{"servers":{"db.example":{"typo":1}}}`, `{}`, `"typo"`},
		{"unknown common", `{}`, `{"resources":{"unknown":{}}}`, `"unknown"`},
		{"invalid lower layer", `{"port":"wrong"}`, `{"port":8080}`, "role.config.port"},
		{"overflow", `{"port":2147483648}`, `{}`, "role.config.port"},
		{"null layer", jsonNull, `{}`, "must be an object"},
		{"null scalar", `{"name":null}`, `{"name":"fixed"}`, jsonNull},
		{"null map value", `{"servers":{"db.example":null}}`, `{}`, jsonNull},
		{"null list item", `{"args":[null]}`, `{}`, jsonNull},
		{"wrong list", `{"args":"run"}`, `{}`, "expected an array"},
		{"wrong quantity", `{"resources":{"memory":{"limit":42}}}`, `{}`, "quantity must be a string"},
		{"malformed quantity", `{"resources":{"memory":{"limit":"lots"}}}`, `{}`, "invalid quantity"},
		{"wrong map quantity", `{"limits":{"heap":{}}}`, `{}`, "quantity must be a string"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			_, err := ResolveConfig(testDefaults(), json.RawMessage(item.role), json.RawMessage(item.group))
			if err == nil || !strings.Contains(err.Error(), item.message) {
				t.Fatalf("want %q, got %v", item.message, err)
			}
		})
	}
}

func TestResolveConfigRejectsUnsupportedTypes(t *testing.T) {
	_, err := ResolveConfig(framework.Config[struct {
		Resources string `json:"resources"`
	}]{}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "collides") {
		t.Fatalf("common/product collision accepted: %v", err)
	}
	for _, typ := range []reflect.Type{
		reflect.TypeFor[*string](), reflect.TypeFor[any](), reflect.TypeFor[[]byte](),
		reflect.TypeFor[map[int]string](), reflect.TypeFor[json.RawMessage](),
		reflect.TypeFor[struct{ testServer }](),
	} {
		t.Run(typ.String(), func(t *testing.T) {
			if err := checkProfile(typ, make(map[reflect.Type]bool)); err == nil {
				t.Fatalf("unsupported type %s accepted", typ)
			}
		})
	}
}

func TestResolveConfigValidatesFinalCommonValues(t *testing.T) {
	cases := []struct{ name, role, group, message string }{
		{"explicit zero is not inheritance", `{}`, `{"resources":{"memory":{"limit":"0"}}}`, "limit"},
		{"zero corrected by group", `{"resources":{"memory":{"limit":"0"}}}`,
			`{"resources":{"memory":{"limit":"1Gi"}}}`, ""},
		{"range corrected by group", `{"resources":{"cpu":{"min":"3"}}}`,
			`{"resources":{"cpu":{"max":"4"}}}`, ""},
		{"negative minimum", `{"resources":{"cpu":{"min":"-1"}}}`, `{}`, "positive"},
		{"zero maximum", `{"resources":{"cpu":{"max":"0"}}}`, `{}`, "positive"},
		{"minimum exceeds maximum", `{"resources":{"cpu":{"min":"3"}}}`, `{}`, "exceed"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			_, err := ResolveConfig(testDefaults(), json.RawMessage(item.role), json.RawMessage(item.group))
			if item.message == "" {
				if err != nil {
					t.Fatalf("higher layer did not correct the final business value: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), item.message) {
				t.Fatalf("want %q, got %v", item.message, err)
			}
		})
	}
}

func TestResolveConfigDoesNotMutateDefaults(t *testing.T) {
	defaults := testDefaults()
	before, err := json.Marshal(defaults)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := ResolveConfig(defaults, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	resolved.Product.Servers["db.example"].Labels["env"] = "changed"
	resolved.Product.Args[0] = "changed"
	resolved.Common.Logging.Containers["trino"].Loggers["io.trino"] = Logger{Level: "ERROR"}
	resolved.Common.Resources.Memory.Limit.Add(resource.MustParse("1Gi"))
	after, err := json.Marshal(defaults)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("resolved config aliases defaults: before %s, after %s", before, after)
	}
}
