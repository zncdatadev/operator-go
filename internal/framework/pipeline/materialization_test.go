package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type staticOnlyCodec struct{}

func (staticOnlyCodec) Encode(values map[string]string) (string, error) {
	return "custom:" + values["value"], nil
}

func materializationFixture(t *testing.T) (MaterializationPlan, map[string]string) {
	t.Helper()
	values := map[string]string{
		"node.id": "example-workers-large-0", " edge:key=\\": " leading trailing ",
		"multiline": "first\nsecond\r\ttab\\", "unicode": "日志🦆", "literal": "$(touch NEVER); `exit 42` ${POD_NAME}",
	}
	properties := map[string]PropertyValue{}
	for key, value := range values {
		properties[key] = Literal(value)
	}
	properties["node.id"] = PodNameBinding{}
	plan, err := PrepareMaterialization([]File{
		{Directory: "config", Path: "node.properties", Content: KeyValues{Codec: PropertiesCodec{}, Values: properties}},
		{Directory: "config", Path: "catalog/custom", Content: KeyValues{
			Codec: staticOnlyCodec{}, Values: map[string]PropertyValue{"value": Literal("${POD_NAME}")},
		}},
		{Directory: "config", Path: "jvm.config", Content: Lines{"-Xmx1152m", "-Dfile.encoding=UTF-8"}},
		{Directory: "config", Path: "empty", Content: Text("")},
	})
	if err != nil {
		t.Fatal(err)
	}
	return plan, values
}

func assertMaterializedFixture(t *testing.T, root string, want map[string]string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "config", "node.properties"))
	if err != nil {
		t.Fatal(err)
	}
	expected := `\ edge\:key\=\\=\ leading trailing\ ` + "\n" +
		`literal=$(touch NEVER); ` + "`exit 42`" + ` ${POD_NAME}` + "\n" +
		`multiline=first\nsecond\r\ttab\\` + "\nnode.id=" + want["node.id"] + "\nunicode=日志🦆\n"
	if string(data) != expected {
		t.Fatalf("properties were not encoded as values: got=%q want=%q", data, expected)
	}
	for name, expected := range map[string]string{
		"config/catalog/custom": "custom:${POD_NAME}",
		"config/jvm.config":     "-Xmx1152m\n-Dfile.encoding=UTF-8\n",
		"config/empty":          "",
	} {
		content, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || string(content) != expected {
			t.Fatalf("file %s: got=%q want=%q err=%v", name, content, expected, err)
		}
	}
}

func TestMaterializationResolvesBindingsAsEncodedData(t *testing.T) {
	plan, want := materializationFixture(t)
	encoded, err := EncodeMaterializationPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeMaterializationPlan(encoded)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := Materialize(root, decoded, want["node.id"]); err != nil {
		t.Fatal(err)
	}
	assertMaterializedFixture(t, root, want)
	// A failed init can retry against the same EmptyDir without appending data.
	if err := Materialize(root, decoded, want["node.id"]); err != nil {
		t.Fatal(err)
	}
	assertMaterializedFixture(t, root, want)
	for _, file := range plan.Files {
		if file.Properties != nil && file.Properties.Values["node.id"].Literal != nil {
			t.Fatal("binding execution must not mutate the shared serialized plan")
		}
	}
}

func TestMaterializationRejectsUnknownRuntimeCodecsAndAmbiguousPlans(t *testing.T) {
	_, err := PrepareMaterialization([]File{{Directory: "config", Path: "node.properties", Content: KeyValues{
		Codec: staticOnlyCodec{}, Values: map[string]PropertyValue{"node.id": PodNameBinding{}},
	}}})
	if err == nil || !strings.Contains(err.Error(), "built-in PropertiesCodec") {
		t.Fatalf("arbitrary Go codec must not cross the runtime boundary: %v", err)
	}
	_, err = PrepareMaterialization([]File{{Directory: "config", Path: "binary", Content: Text("\xff")}})
	if err == nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("JSON transport must not silently corrupt non-text bytes: %v", err)
	}
	empty := ""
	base := PlannedFile{Directory: "config", Path: "node.properties", Encoded: &empty}
	for name, mutate := range map[string]func(*MaterializationPlan){
		"version":   func(p *MaterializationPlan) { p.Version = "v2" },
		"directory": func(p *MaterializationPlan) { p.Files[0].Directory = "../escape" },
		"absolute":  func(p *MaterializationPlan) { p.Files[0].Path = "/escape" },
		"traversal": func(p *MaterializationPlan) { p.Files[0].Path = "nested/../../escape" },
		"backslash": func(p *MaterializationPlan) { p.Files[0].Path = "nested\\escape" },
		"duplicate": func(p *MaterializationPlan) { p.Files = append(p.Files, base) },
		"prefix": func(p *MaterializationPlan) {
			p.Files = []PlannedFile{
				{Directory: "config", Path: "a", Encoded: &empty},
				{Directory: "config", Path: "a-b", Encoded: &empty},
				{Directory: "config", Path: "a/child", Encoded: &empty},
			}
		},
		"missing content": func(p *MaterializationPlan) { p.Files[0].Encoded = nil },
		"two forms": func(p *MaterializationPlan) {
			p.Files[0].Properties = &PlannedProperties{Codec: propertiesCodecID}
		},
		"codec": func(p *MaterializationPlan) {
			p.Files[0].Encoded = nil
			p.Files[0].Properties = &PlannedProperties{Codec: "shell"}
		},
		"two sources": func(p *MaterializationPlan) {
			p.Files[0].Encoded = nil
			p.Files[0].Properties = &PlannedProperties{Codec: propertiesCodecID, Values: map[string]PlannedProperty{
				"node.id": {Literal: &empty, PodName: true},
			}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			plan := MaterializationPlan{Version: materializationVersion, Files: []PlannedFile{base}}
			mutate(&plan)
			root := filepath.Join(t.TempDir(), "uncreated")
			if err := Materialize(root, plan, "pod-0"); err == nil {
				t.Fatal("invalid plan must fail")
			}
			if _, err := os.Stat(root); !os.IsNotExist(err) {
				t.Fatal("plan validation must precede all filesystem writes")
			}
		})
	}
	for _, raw := range []string{
		`{"version":"v1","files":[],"unknown":true}`, `{"version":"v1","files":[]} {}`,
		`{"version":"v1","files":[{"directory":"config","path":"x","encoded":null}]}`,
	} {
		if _, err := DecodeMaterializationPlan([]byte(raw)); err == nil {
			t.Fatalf("ambiguous JSON accepted: %s", raw)
		}
	}
}

func TestMaterializationConfinesFilesystemWrites(t *testing.T) {
	plan, want := materializationFixture(t)
	for _, podName := range []string{"", "$(touch escaped)", "pod\nsecond"} {
		root := filepath.Join(t.TempDir(), "uncreated")
		if err := Materialize(root, plan, podName); err == nil {
			t.Fatalf("invalid Pod name accepted: %q", podName)
		}
		if _, err := os.Stat(root); !os.IsNotExist(err) {
			t.Fatal("all bindings must resolve before the first write")
		}
	}
	root, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "config")); err != nil {
		t.Fatal(err)
	}
	if err := Materialize(root, plan, want["node.id"]); err == nil {
		t.Fatal("a directory symlink cannot escape the output root")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("escaped directory changed: %v %v", entries, err)
	}
	// A pre-existing file link is replaced as a directory entry; its target is untouched.
	root = t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "config"), 0755); err != nil {
		t.Fatal(err)
	}
	protected := filepath.Join(outside, "protected")
	if err := os.WriteFile(protected, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(protected, filepath.Join(root, "config", "node.properties")); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(protected, filepath.Join(root, "config", "empty")); err != nil {
		t.Fatal(err)
	}
	if err := Materialize(root, plan, want["node.id"]); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(protected)
	if err != nil || string(content) != "keep" {
		t.Fatalf("existing link target changed: %q %v", content, err)
	}
	assertMaterializedFixture(t, root, want)
}
