package pipeline

import (
	"reflect"
	"strings"
	"testing"
)

func overridePtr[T any](value T) *T { return &value }

func propertySet(values map[string]string) FileOverride {
	return FileOverride{Properties: &PropertyOverride{Set: &values}}
}

func propertyReplace(values map[string]string) FileOverride {
	return FileOverride{Properties: &PropertyOverride{Replace: &values}}
}

func TestFileOverridesExecuteLayersAndCancelBindings(t *testing.T) {
	deleted := FileOverride{Remove: overridePtr(true)}
	emptyPatch := FileOverride{Properties: &PropertyOverride{}}
	removeIdentity := FileOverride{Properties: &PropertyOverride{Remove: overridePtr([]string{"node.id"})}}
	content := func(values map[string]PropertyValue) FileContent {
		return KeyValues{Codec: &unusedCodec{}, Values: values}
	}
	cases := []struct {
		name    string
		actions []FileOverride
		want    FileContent
	}{
		{"empty-patch-keeps-binding", []FileOverride{emptyPatch}, content(map[string]PropertyValue{
			"node.environment": Literal("test"), "node.id": PodNameBinding{},
		})},
		{"literal-cancels-binding", []FileOverride{propertySet(map[string]string{"node.id": ""})},
			content(map[string]PropertyValue{"node.environment": Literal("test"), "node.id": Literal("")})},
		{"remove-key-cancels-binding", []FileOverride{removeIdentity},
			content(map[string]PropertyValue{"node.environment": Literal("test")})},
		{"set-after-remove", []FileOverride{removeIdentity, propertySet(map[string]string{"node.id": "new"})},
			content(map[string]PropertyValue{"node.environment": Literal("test"), "node.id": Literal("new")})},
		{"replace-is-empty-file", []FileOverride{propertyReplace(map[string]string{})}, content(map[string]PropertyValue{})},
		{"text-is-empty-file", []FileOverride{{Text: overridePtr("")}}, Text("")},
		{"lines-is-empty-file", []FileOverride{{Lines: overridePtr([]string{})}}, Lines{}},
		{"remove-is-absent", []FileOverride{deleted}, nil},
		{"empty-patch-after-remove-stays-absent", []FileOverride{deleted, emptyPatch}, nil},
		{"empty-set-after-remove-stays-absent", []FileOverride{deleted, propertySet(map[string]string{})}, nil},
		{"key-remove-after-file-remove-stays-absent", []FileOverride{deleted, removeIdentity}, nil},
		{"recreate-does-not-restore-defaults", []FileOverride{deleted, propertySet(map[string]string{"node.id": "new"})},
			content(map[string]PropertyValue{"node.id": Literal("new")})},
		{"recreate-empty-structured-file", []FileOverride{deleted, propertyReplace(map[string]string{})},
			content(map[string]PropertyValue{})},
		{"replace-after-text-keeps-codec-only", []FileOverride{
			{Text: overridePtr("foreign")},
			propertyReplace(map[string]string{"remove": "literal", "set": "also-literal", "a.b": "a=b"}),
		}, content(map[string]PropertyValue{
			"remove": Literal("literal"), "set": Literal("also-literal"), "a.b": Literal("a=b"),
		})},
		{"replace-after-lines-keeps-codec-only", []FileOverride{{Lines: overridePtr([]string{"foreign"})},
			propertyReplace(map[string]string{})}, content(map[string]PropertyValue{})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			original := runtimeFixture().Files
			layers := make([]map[string]FileOverride, 0, len(tc.actions))
			for _, action := range tc.actions {
				layers = append(layers, map[string]FileOverride{"node.properties": action})
			}
			files, err := ApplyFileOverrides(original, "config", layers...)
			if err != nil {
				t.Fatal(err)
			}
			if tc.want == nil {
				if len(files) != 0 {
					t.Fatalf("deleted file reappeared: %#v", files)
				}
			} else if len(files) != 1 || !reflect.DeepEqual(files[0].Content, tc.want) {
				t.Fatalf("wanted %#v, got %#v", tc.want, files)
			}
			if !reflect.DeepEqual(original, runtimeFixture().Files) {
				t.Fatal("overrides mutated the original declaration")
			}
		})
	}
}

func TestFileOverridesRejectAmbiguousActions(t *testing.T) {
	cases := []struct {
		name, want string
		override   FileOverride
	}{
		{"empty-action", "exactly one", FileOverride{}},
		{"two-modes", "exactly one", FileOverride{Text: overridePtr(""), Remove: overridePtr(true)}},
		{"false-remove", "must be true", FileOverride{Remove: overridePtr(false)}},
		{"nil-lines", "cannot be null", FileOverride{Lines: overridePtr([]string(nil))}},
		{"nil-set", "cannot be null", propertySet(nil)},
		{"nil-replace", "cannot be null", propertyReplace(nil)},
		{"nil-key-remove", "cannot be null", FileOverride{
			Properties: &PropertyOverride{Remove: overridePtr([]string(nil))},
		}},
		{"replace-and-empty-set", "cannot be combined", FileOverride{Properties: &PropertyOverride{
			Set: overridePtr(map[string]string{}), Replace: overridePtr(map[string]string{}),
		}}},
		{"replace-and-empty-remove", "cannot be combined", FileOverride{Properties: &PropertyOverride{
			Remove: overridePtr([]string{}), Replace: overridePtr(map[string]string{}),
		}}},
		{"set-and-remove-same-key", "both set and removed", FileOverride{Properties: &PropertyOverride{
			Set: overridePtr(map[string]string{"a.b": ""}), Remove: overridePtr([]string{"a.b"}),
		}}},
		{"embedded-newline", "CR or LF", FileOverride{Lines: overridePtr([]string{"a\nb"})}},
		{"embedded-carriage-return", "CR or LF", FileOverride{Lines: overridePtr([]string{"a\rb"})}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateFileOverride(tc.override)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("wanted %q, got %v", tc.want, err)
			}
		})
	}
}

func TestFileOverridesKeepEncodingCapabilitiesAndReportSources(t *testing.T) {
	cases := []struct {
		name, path, want string
		role, group      FileOverride
	}{
		{"unknown-extension", "new.properties", "original structured encoding", FileOverride{Text: overridePtr("a=1")},
			propertyReplace(map[string]string{"a": "2"})},
		{"unknown-empty-patch", "new.properties", "original structured encoding", FileOverride{Remove: overridePtr(true)},
			FileOverride{Properties: &PropertyOverride{}}},
		{"patch-after-text", "node.properties", "cannot edit text or lines", FileOverride{Text: overridePtr("a=1")},
			propertySet(map[string]string{"a": "2"})},
		{"empty-patch-after-lines", "node.properties", "cannot edit text or lines",
			FileOverride{Lines: overridePtr([]string{})}, FileOverride{Properties: &PropertyOverride{}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ApplyFileOverrides(runtimeFixture().Files, "config",
				map[string]FileOverride{tc.path: tc.role}, map[string]FileOverride{tc.path: tc.group})
			if err == nil || !strings.Contains(err.Error(), tc.want) ||
				!strings.Contains(err.Error(), "layer 2 file "+`"`+tc.path+`"`) {
				t.Fatalf("expected layer and file diagnostic containing %q, got %v", tc.want, err)
			}
		})
	}
	original := []File{{Directory: "config", Path: "plain.properties", Content: Text("a=1")}}
	if _, err := ApplyFileOverrides(original, "config", map[string]FileOverride{
		"plain.properties": propertyReplace(map[string]string{}),
	}); err == nil {
		t.Fatal("a file extension must not create a codec for an original text declaration")
	}
}

func TestFileOverridesRespectDirectoryNamespacesAndOwnership(t *testing.T) {
	original := runtimeFixture().Files
	other := cloneFileForOverride(original[0])
	other.Directory = "platform"
	original = append(original, other)
	lines := []string{"first", "second=a=b"}
	files, err := ApplyFileOverrides(original, "config", map[string]FileOverride{
		"node.properties": {Remove: overridePtr(true)},
		"custom.conf":     {Lines: &lines},
		"missing.conf":    {Remove: overridePtr(true)},
		"exact.conf":      {Text: overridePtr("first\nlast")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 || files[0].Directory != "config" || files[0].Path != "custom.conf" ||
		files[1].Content != Text("first\nlast") || files[2].Directory != "platform" {
		t.Fatalf("namespace or deterministic ordering lost: %#v", files)
	}
	files[0].Content.(Lines)[0] = "changed"
	files[2].Content.(KeyValues).Values["node.id"] = Literal("changed")
	if lines[0] != "first" {
		t.Fatal("returned lines alias the override input")
	}
	if _, bound := original[1].Content.(KeyValues).Values["node.id"].(PodNameBinding); !bound {
		t.Fatal("a non-target directory aliases the original declaration")
	}
	// Neither a structured declaration in another directory nor an extension
	// grants the target directory an encoding capability.
	if _, err := ApplyFileOverrides([]File{other}, "config", map[string]FileOverride{
		"node.properties": propertyReplace(map[string]string{}),
	}); err == nil {
		t.Fatal("codec from another directory leaked into the target namespace")
	}
}

func TestFileOverridesRejectPathCollisions(t *testing.T) {
	for _, name := range []string{"", ".", "..", "../a", "/absolute", "a/../b", "a/", "a\x00b"} {
		t.Run(name, func(t *testing.T) {
			_, err := ApplyFileOverrides(nil, "config", map[string]FileOverride{name: {Text: overridePtr("")}})
			if err == nil || !strings.Contains(err.Error(), "layer 1 file") || !strings.Contains(err.Error(), "relative path") {
				t.Fatalf("wanted layer/path diagnostic for %q, got %v", name, err)
			}
		})
	}
	original := []File{
		{Directory: "config", Path: "a", Content: Text("parent")},
		{Directory: "config", Path: "a-file", Content: Text("sorts before a/b")},
	}
	if _, err := ApplyFileOverrides(append(original, original[0]), "config"); err == nil ||
		!strings.Contains(err.Error(), "duplicate path") {
		t.Fatalf("duplicate original path accepted: %v", err)
	}
	child := map[string]FileOverride{"a/b": {Text: overridePtr("")}}
	if _, err := ApplyFileOverrides(original, "config", child); err == nil ||
		!strings.Contains(err.Error(), "layer 1") || !strings.Contains(err.Error(), `"a/b"`) {
		t.Fatalf("prefix collision hidden by another lexically adjacent file: %v", err)
	}
	child["a"] = FileOverride{Remove: overridePtr(true)}
	if files, err := ApplyFileOverrides(original, "config", child); err != nil || len(files) != 2 {
		t.Fatalf("same-layer replacement of parent by child should be order independent: %v, %v", files, err)
	}
	if _, err := ApplyFileOverrides(nil, ""); err == nil {
		t.Fatal("an absent config directory must not create an implicit namespace")
	}
}
