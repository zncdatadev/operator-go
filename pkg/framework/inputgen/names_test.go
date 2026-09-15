package inputgen

import (
	"bytes"
	"strings"
	"testing"
	"text/template"
)

func TestValidateGeneratedNamesWithRequestedKind(t *testing.T) {
	declarations, err := emitConfigTypes[struct{ Port int32 }]()
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := emitClusterConfigTypes[struct{ Environment struct{ Name string } }]()
	if err != nil {
		t.Fatal(err)
	}
	declarations += "\n" + cluster
	parsed, err := template.New("input").Parse(inputGoTemplate)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		kind string
		bad  bool
	}{
		{"TrinoInputCluster", false},
		{"CloneInput", false},
		{"ConfigInput", true},
		{"ConfigInputResources", true},
		{"ClusterConfigInput", true},
		{"ClusterConfigInputEnvironment", true},
		{"SpecInput", true},
		{"GroupVersion", true},
		{"Decode", true},
		{"Operation", true},
		{"Project", true}, {"Binding", true}, {contractVersionName, true},
	} {
		t.Run(item.kind, func(t *testing.T) {
			var source bytes.Buffer
			data := inputTemplateData{
				Names: Names{Package: "resource", Group: "example.com", Version: "v1", Kind: item.kind},
				Roles: []inputRoleName{{Wire: "workers", Go: "Workers"}}, Types: declarations,
			}
			if err := parsed.Execute(&source, data); err != nil {
				t.Fatal(err)
			}
			err := validateGeneratedNames(source.Bytes())
			if item.bad && (err == nil || !strings.Contains(err.Error(), "conflicts")) {
				t.Fatalf("requested kind collision was not rejected: %v", err)
			}
			if !item.bad && err != nil {
				t.Fatalf("valid generated kind rejected: %v", err)
			}
		})
	}
}

func TestValidateGeneratedNamesScope(t *testing.T) {
	cases := []struct {
		name, source string
		bad          bool
	}{
		{"import and type", `package generated; import "fmt"; type fmt struct{}`, true},
		{"alias and function", `package generated; import resource "fmt"; func resource() {}`, true},
		{"constant and variable", `package generated; const A = 1; var B, A int`, true},
		{"type and function", `package generated; type A struct{}; func A() {}`, true},
		{"receiver method is separate", `package generated; type A struct{}; func (A) A() {}`, false},
		{"different receiver methods", `package generated; type A struct{}; type B struct{}
func (A) DeepCopy() {}; func (B) DeepCopy() {}`, false},
		{"blank names and init", `package generated; var _ int; var _ bool; func init() {}; func init() {}`, false},
		{"syntax", `package generated; type`, true},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			err := validateGeneratedNames([]byte(item.source))
			if (err != nil) != item.bad {
				t.Fatalf("bad=%t, got %v", item.bad, err)
			}
		})
	}
}
