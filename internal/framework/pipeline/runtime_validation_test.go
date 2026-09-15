package pipeline

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

// A sentinel codec: declaration validation must never invoke serialization.
type unusedCodec struct{}

func (*unusedCodec) Encode(map[string]string) (string, error) {
	panic("ValidateRuntime must not execute a codec")
}

func runtimeFixture() RuntimeDescription {
	return RuntimeDescription{
		Main: Process{Name: "trino", Image: "example.invalid/trino:fixture", Access: []DirectoryAccess{
			{Directory: "config", MountPath: "/etc/trino", ReadOnly: true},
			{Directory: "logs", MountPath: "/var/log/trino"},
		}},
		Directories: []Directory{{Name: "config"}, {Name: "logs"}},
		Files: []File{{Directory: "config", Path: "node.properties", Content: KeyValues{
			Codec: &unusedCodec{}, Values: map[string]PropertyValue{
				"node.environment": Literal("test"), "node.id": PodNameBinding{},
			},
		}}},
		Endpoints:  []Endpoint{{Name: "http", Port: 8080}},
		LogOutputs: []LogOutput{{Container: "trino", Directory: "logs", RelativePath: "server.json"}},
	}
}

func TestRuntimeDeclarationSeparatesBindingsAndCollection(t *testing.T) {
	r := runtimeFixture()
	r.LogOutputs = nil
	if err := ValidateRuntime(r); err != nil {
		t.Fatal(err)
	}
	values := r.Files[0].Content.(KeyValues).Values
	if _, ok := values["node.id"].(PodNameBinding); !ok {
		t.Fatal("binding must remain expressible without a collector")
	}
	values["node.id"] = Literal("explicit-id")
	if err := ValidateRuntime(r); err != nil {
		t.Fatal(err)
	}
	if _, stillBound := values["node.id"].(PodNameBinding); stillBound {
		t.Fatal("one property cannot remain bound after it is replaced with a literal")
	}
}

func TestRuntimeRejectsBrokenReferencesAndContent(t *testing.T) {
	cases := []struct {
		name, want string
		change     func(*RuntimeDescription)
	}{
		{"unknown-directory", "unknown directory", func(r *RuntimeDescription) { r.Files[0].Directory = "absent" }},
		{"escape", "invalid file path", func(r *RuntimeDescription) { r.Files[0].Path = "../secret" }},
		{"prefix-collision", "collide", func(r *RuntimeDescription) {
			r.Files = append(r.Files, File{Directory: "config", Path: "node.properties/nested", Content: Text("")})
		}},
		{"duplicate-mount", "duplicate mount", func(r *RuntimeDescription) {
			r.Main.Access = append(r.Main.Access, r.Main.Access[0])
		}},
		{"read-only-log", "invalid log output", func(r *RuntimeDescription) { r.Main.Access[1].ReadOnly = true }},
		{"bad-port", "endpoint", func(r *RuntimeDescription) { r.Endpoints[0].Port = 65536 }},
		{"nil-content", "nil file content", func(r *RuntimeDescription) { r.Files[0].Content = nil }},
		{"nil-codec", "require a codec", func(r *RuntimeDescription) {
			r.Files[0].Content = KeyValues{Codec: (*unusedCodec)(nil)}
		}},
		{"nil-property", "nil value", func(r *RuntimeDescription) {
			r.Files[0].Content.(KeyValues).Values["node.id"] = nil
		}},
		{"multiline", "CR or LF", func(r *RuntimeDescription) { r.Files[0].Content = Lines{"one\ntwo"} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := runtimeFixture()
			tc.change(&r)
			if err := ValidateRuntime(r); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("wanted %q, got %v", tc.want, err)
			}
		})
	}
}

func TestRuntimeIdentityPresenceAndEmptyFiles(t *testing.T) {
	r := runtimeFixture()
	r.Files = []File{
		{Directory: "config", Path: "empty.properties", Content: KeyValues{Codec: &unusedCodec{}}},
		{Directory: "config", Path: "empty.lines", Content: Lines{}},
		{Directory: "config", Path: "empty.text", Content: Text("")},
	}
	zero, negative, group, nonRoot := int64(0), int64(-1), int64(1001), true
	r.SharedGroup = &group
	r.Main.Identity = &corev1.SecurityContext{RunAsUser: &zero}
	if err := ValidateRuntime(r); err != nil {
		t.Fatalf("explicit zero and empty files are values: %v", err)
	}
	r.Main.Identity.RunAsNonRoot = &nonRoot
	if err := ValidateRuntime(r); err == nil {
		t.Fatal("explicit root contradicts runAsNonRoot")
	}
	r.Main.Identity = &corev1.SecurityContext{RunAsGroup: &negative}
	if err := ValidateRuntime(r); err == nil {
		t.Fatal("negative GID must fail")
	}
	r.Main.Identity = nil
	r.SharedGroup = &negative
	if err := ValidateRuntime(r); err == nil {
		t.Fatal("negative shared group must fail")
	}
}
