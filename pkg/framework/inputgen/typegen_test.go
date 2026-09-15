package inputgen

import (
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
)

type inputTypePort int32
type inputTypeKey string

type inputTypeFixture struct {
	Port    inputTypePort                           `json:"port"`
	Enabled bool                                    `json:"enabled"`
	Args    []string                                `json:"args"`
	Servers map[inputTypeKey]inputTypeServer        `json:"servers"`
	Peers   []inputTypeServer                       `json:"peers"`
	Limits  map[string]resource.Quantity            `json:"limits"`
	Groups  map[string][]map[string]inputTypeServer `json:"groups"`
}

type inputTypeServer struct {
	Host   string            `json:"host"`
	Labels map[string]string `json:"labels"`
}

func TestEmitConfigTypesPreservesFieldPresence(t *testing.T) {
	declarations, err := emitConfigTypes[inputTypeFixture]()
	if err != nil {
		t.Fatal(err)
	}
	file, err := parser.ParseFile(token.NewFileSet(), "generated.go", "package fixture\n"+declarations, 0)
	if err != nil {
		t.Fatalf("invalid Go declarations: %v\n%s", err, declarations)
	}
	fields := emittedFields(t, file)
	want := map[string]string{
		"ConfigInput.Port":                                    "*int32",
		"ConfigInput.Enabled":                                 "*bool",
		"ConfigInput.Args":                                    "*[]string",
		"ConfigInput.Servers":                                 "*map[string]ConfigInputServersValue",
		"ConfigInput.Peers":                                   "*[]ConfigInputPeersItem",
		"ConfigInput.Limits":                                  "*map[string]resource.Quantity",
		"ConfigInput.Groups":                                  "*map[string][]map[string]ConfigInputGroupsValueItemValue",
		"ConfigInputServersValue.Host":                        "*string",
		"ConfigInputServersValue.Labels":                      "*map[string]string",
		"ConfigInputPeersItem.Host":                           "*string",
		"ConfigInputGroupsValueItemValue.Host":                "*string",
		"ConfigInputResourcesCPU.Min":                         "*resource.Quantity",
		"ConfigInputLogging.EnableVectorAgent":                "*bool",
		"ConfigInputLoggingContainersValueLoggersValue.Level": "*string",
	}
	for field, expected := range want {
		if got := fields[field]; got != expected {
			t.Errorf("%s: want %s, got %s", field, expected, got)
		}
	}
	second, err := emitConfigTypes[inputTypeFixture]()
	if err != nil || declarations != second {
		t.Fatalf("emission is not deterministic: %v", err)
	}
}

func TestEmitClusterConfigTypesPreservesIndependentPresence(t *testing.T) {
	declarations, err := emitClusterConfigTypes[inputTypeFixture]()
	if err != nil {
		t.Fatal(err)
	}
	file, err := parser.ParseFile(token.NewFileSet(), "generated.go", "package fixture\n"+declarations, 0)
	if err != nil {
		t.Fatal(err)
	}
	fields := emittedFields(t, file)
	for field, want := range map[string]string{
		"ClusterConfigInput.Stopped":              "*bool",
		"ClusterConfigInput.ReconciliationPaused": "*bool",
		"ClusterConfigInput.Port":                 "*int32", "ClusterConfigInput.Enabled": "*bool",
		"ClusterConfigInput.Args":    "*[]string",
		"ClusterConfigInput.Servers": "*map[string]ClusterConfigInputServersValue",
		"ClusterConfigInput.Limits":  "*map[string]resource.Quantity",
	} {
		if fields[field] != want {
			t.Errorf("%s: got %s, want %s", field, fields[field], want)
		}
	}
	if fields["ClusterConfigInput.Resources"] != "" || fields["ClusterConfigInput.Logging"] != "" {
		t.Fatal("workload common fields leaked into the cluster input")
	}
	if _, err := emitClusterConfigTypes[struct{ Resources string }](); err != nil {
		t.Fatalf("cluster field was incorrectly checked against CommonConfig: %v", err)
	}
	second, err := emitClusterConfigTypes[inputTypeFixture]()
	if err != nil || declarations != second {
		t.Fatalf("cluster type emission is not deterministic: %v", err)
	}
}

func TestEmitClusterConfigTypesRejectsUnsupportedRoots(t *testing.T) {
	for _, emit := range []func() (string, error){
		emitClusterConfigTypes[string], emitClusterConfigTypes[resource.Quantity],
		emitClusterConfigTypes[struct{ Value *string }], emitClusterConfigTypes[inputTypeRecursive],
	} {
		if value, err := emit(); err == nil || value != "" {
			t.Fatalf("invalid cluster profile produced declarations: %q %v", value, err)
		}
	}
}

func emittedFields(t *testing.T, file *ast.File) map[string]string {
	t.Helper()
	result := make(map[string]string)
	for _, declaration := range file.Decls {
		for _, spec := range declaration.(*ast.GenDecl).Specs {
			typeSpec := spec.(*ast.TypeSpec)
			for _, field := range typeSpec.Type.(*ast.StructType).Fields.List {
				var value strings.Builder
				if err := format.Node(&value, token.NewFileSet(), field.Type); err != nil {
					t.Fatal(err)
				}
				if _, ok := field.Type.(*ast.StarExpr); !ok {
					t.Errorf("%s.%s does not preserve field presence", typeSpec.Name, field.Names[0])
				}
				tag, err := strconv.Unquote(field.Tag.Value)
				if err != nil || !strings.HasSuffix(reflect.StructTag(tag).Get("json"), ",omitempty") {
					t.Errorf("invalid optional input tag %s: %v", field.Tag.Value, err)
				}
				result[typeSpec.Name.Name+"."+field.Names[0].Name] = value.String()
			}
		}
	}
	return result
}

type inputTypeRecursive struct {
	Children map[string]inputTypeRecursive `json:"children"`
}

func TestEmitConfigTypesRejectsAmbiguousOrUnsupportedProfiles(t *testing.T) {
	cases := []struct {
		name string
		emit func() (string, error)
		want string
	}{
		{"Go field collision", emitConfigTypes[struct {
			Resources string `json:"productResources"`
		}], "collides"},
		{"JSON field collision", emitConfigTypes[struct {
			ProductResources string `json:"resources"`
		}], "collides"},
		{"generated path collision", emitConfigTypes[struct {
			ResourcesCPU struct{ Other bool } `json:"resourcesCPU"`
		}], "generated type name"},
		{"recursive map", emitConfigTypes[inputTypeRecursive], "recursive"},
		{"pointer", emitConfigTypes[struct{ Value *string }], "unsupported"},
		{"non-struct", emitConfigTypes[string], "must be a struct"},
		{"quantity root", emitConfigTypes[resource.Quantity], "must be a struct"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			value, err := item.emit()
			if value != "" || err == nil || !strings.Contains(err.Error(), item.want) {
				t.Fatalf("want no output and %q, got output %q and error %v", item.want, value, err)
			}
		})
	}
}
