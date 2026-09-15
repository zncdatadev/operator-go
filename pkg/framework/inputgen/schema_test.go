package inputgen

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

type schemaSampleConfig struct {
	HTTPPort int32               `json:"httpPort"`
	Enabled  bool                `json:"enabled"`
	Args     []string            `json:"args"`
	Labels   map[string]string   `json:"labels"`
	Children []schemaSampleChild `json:"children"`
	Dotted   string              `json:"with.dot"`
	Reserved string              `json:"namespace"`
}

type schemaSampleChild struct {
	Count int16 `json:"count"`
}

func TestGeneratedConfigSchema(t *testing.T) {
	schema, err := configSchema[schemaSampleConfig]()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"resources", "logging", "httpPort", "enabled", "args", "labels", "children", "with.dot", "namespace",
	} {
		if _, exists := schema.Properties[name]; !exists {
			t.Fatalf("generated config lost field %s", name)
		}
	}
	if _, exists := schema.Properties["product"]; exists {
		t.Fatal("common/product Go ownership must not change the flat CR layout")
	}
	port := schema.Properties["httpPort"]
	if port.Type != "integer" || port.Format != "int32" || port.Minimum == nil || *port.Minimum >= 0 {
		t.Fatalf("schema must preserve the Go domain without imposing the product port range: %#v", port)
	}
	quantity := schema.Properties["resources"].Properties["cpu"].Properties["min"]
	if quantity.Type != "string" || quantity.Minimum != nil || len(quantity.Properties) != 0 {
		t.Fatalf("quantity must remain a scalar and permit later layers to repair zero: %#v", quantity)
	}
	if !strings.Contains(schema.XValidations[0].Rule, "with__dot__dot") {
		t.Fatalf("JSON field name was not escaped for CEL: %s", schema.XValidations[0].Rule)
	}
	assertSchemaPresence(t, schema)
	second, err := configSchema[schemaSampleConfig]()
	if err != nil || !reflect.DeepEqual(schema, second) {
		t.Fatalf("schema generation must be deterministic: %v", err)
	}
	delete(second.Properties, "httpPort")
	if _, exists := schema.Properties["httpPort"]; !exists {
		t.Fatal("generated schemas must not share a mutable cache")
	}
}

func TestGeneratedClusterConfigSchema(t *testing.T) {
	schema, err := clusterConfigSchema[schemaSampleConfig]()
	if err != nil {
		t.Fatal(err)
	}
	if schema.Properties["resources"].Type != "" || schema.Properties["logging"].Type != "" ||
		schema.Properties["httpPort"].Type != schemaIntegerType {
		t.Fatal("cluster schema was mixed with CommonConfig")
	}
	assertSchemaPresence(t, schema)
	empty, err := clusterConfigSchema[struct{}]()
	if err != nil || empty.Type != schemaObjectType || len(empty.Properties) != 4 ||
		empty.Properties["stopped"].Type != schemaBooleanType ||
		empty.Properties["reconciliationPaused"].Type != schemaBooleanType {
		t.Fatalf("empty product cluster config must expose only framework controls: %+v %v", empty, err)
	}
	for _, build := range []func() (apiextensionsv1.JSONSchemaProps, error){
		clusterConfigSchema[string], clusterConfigSchema[struct{ Value *string }],
		clusterConfigSchema[struct{ Tree schemaRecursive }],
	} {
		if _, err := build(); err == nil {
			t.Fatal("unsupported cluster config profile accepted")
		}
	}
}

func assertSchemaPresence(t *testing.T, node apiextensionsv1.JSONSchemaProps) {
	t.Helper()
	if !node.Nullable || node.Default != nil || len(node.Required) != 0 {
		t.Fatalf("config node loses input presence: %#v", node)
	}
	if (node.Type == "object" || node.Type == "array") && len(node.XValidations) == 0 {
		t.Fatalf("container node does not reject null children: %#v", node)
	}
	for _, child := range node.Properties {
		assertSchemaPresence(t, child)
	}
	if node.AdditionalProperties != nil && node.AdditionalProperties.Schema != nil {
		assertSchemaPresence(t, *node.AdditionalProperties.Schema)
	}
	if node.Items != nil && node.Items.Schema != nil {
		assertSchemaPresence(t, *node.Items.Schema)
	}
}

func TestGeneratedInputSchemaEnvelope(t *testing.T) {
	schema, err := crdSchema[schemaSampleConfig, struct{}]([]string{"coordinator", "worker"})
	if err != nil {
		t.Fatal(err)
	}
	if schema.Nullable || !reflect.DeepEqual(schema.Required, []string{"spec"}) {
		t.Fatalf("root must require a non-null spec: %#v", schema)
	}
	role := schema.Properties["spec"].Properties["worker"]
	if role.Properties["replicas"].Default != nil || *role.Properties["replicas"].Minimum != 0 {
		t.Fatal("replicas must preserve absence and accept explicit zero")
	}
	data, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	hasDefaults := strings.Contains(string(data), `"default"`)
	if hasDefaults {
		t.Fatal("generated CRD must not default inherited input")
	}
	assertPodOnlyUnknownPreservation(t, schema, "")
}

func assertPodOnlyUnknownPreservation(t *testing.T, node apiextensionsv1.JSONSchemaProps, name string) {
	t.Helper()
	if node.XPreserveUnknownFields != nil && *node.XPreserveUnknownFields && name != "podOverrides" {
		t.Fatalf("unknown fields preserved outside the explicit Pod patch domain: %s", name)
	}
	for key, child := range node.Properties {
		assertPodOnlyUnknownPreservation(t, child, key)
	}
	if node.AdditionalProperties != nil && node.AdditionalProperties.Schema != nil {
		assertPodOnlyUnknownPreservation(t, *node.AdditionalProperties.Schema, name+"[*]")
	}
	if node.Items != nil && node.Items.Schema != nil {
		assertPodOnlyUnknownPreservation(t, *node.Items.Schema, name+"[]")
	}
}

type schemaRecursive map[string]schemaRecursive

func TestGeneratedSchemaRejectsUnsupportedDefinitions(t *testing.T) {
	for _, test := range []struct {
		name string
		run  func() error
	}{
		{"collision", func() error {
			_, err := configSchema[struct {
				Resources string `json:"resources"`
			}]()
			return err
		}},
		{"recursive", func() error {
			_, err := configSchema[struct {
				Tree schemaRecursive `json:"tree"`
			}]()
			return err
		}},
		{"unaddressable-json-name", func() error {
			_, err := configSchema[struct {
				Value string `json:"space name"`
			}]()
			return err
		}},
		{"uint64", func() error {
			_, err := configSchema[struct {
				Value uint64 `json:"value"`
			}]()
			return err
		}},
		{"no-roles", func() error { _, err := crdSchema[schemaSampleConfig, struct{}](nil); return err }},
		{"duplicate-roles", func() error {
			_, err := crdSchema[schemaSampleConfig, struct{}]([]string{"worker", "worker"})
			return err
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(); err == nil {
				t.Fatal("unsupported definition accepted")
			}
		})
	}
}
