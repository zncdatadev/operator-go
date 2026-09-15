package pipeline

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestProcessOverridesReplaceSourcesAndKeepInputIsolated(t *testing.T) {
	process := Process{Args: []string{"original"}, Env: []corev1.EnvVar{
		{Name: "SOURCE", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"}}},
		{Name: "UNTOUCHED", Value: "keep"},
	}}
	roleArgs, empty := []string{"role"}, []string{}
	role := &Overrides{EnvOverrides: map[string]string{"SOURCE": "role", "ADDED": "role"}, CLIOverrides: &roleArgs}
	group := &Overrides{EnvOverrides: map[string]string{"SOURCE": ""}, CLIOverrides: &empty}
	if err := applyProcessOverrides(&process, nil, role, group); err != nil {
		t.Fatal(err)
	}
	want := []corev1.EnvVar{{Name: "UNTOUCHED", Value: "keep"},
		{Name: "ADDED", Value: "role"}, {Name: "SOURCE", Value: ""}}
	if !reflect.DeepEqual(process.Env, want) || process.Args == nil || len(process.Args) != 0 {
		t.Fatalf("overrides lost empty values or retained ValueFrom: %+v", process)
	}
	if err := applyProcessOverrides(&process, &Overrides{CLIOverrides: &roleArgs}); err != nil {
		t.Fatal(err)
	}
	process.Args[0] = "changed"
	if roleArgs[0] != "role" || role.EnvOverrides["SOURCE"] != "role" || group.EnvOverrides["SOURCE"] != "" {
		t.Fatal("process overrides mutated source layers")
	}
	var nullArgs []string
	if err := applyProcessOverrides(&process, &Overrides{CLIOverrides: &nullArgs}); err == nil {
		t.Fatal("explicit null CLI was accepted")
	}
}

func TestCloneRuntimeIsolatesMutableDeclarationData(t *testing.T) {
	source := retainedRuntime()
	uid := int64(1001)
	source.SharedGroup = &uid
	source.Main.Identity = &corev1.SecurityContext{RunAsUser: &uid}
	source.Main.Command, source.Main.Args = []string{"launcher"}, []string{"run"}
	source.Main.Env = []corev1.EnvVar{{Name: "POD_NAME", ValueFrom: &corev1.EnvVarSource{
		FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"},
	}}}
	source.Files = append(source.Files, File{Directory: "config", Path: "jvm.config", Content: Lines{"original"}})
	before := CloneRuntime(source)
	copy := CloneRuntime(source)
	*copy.SharedGroup = 0
	*copy.Main.Identity.RunAsUser = 0
	copy.Main.Env[0].ValueFrom.FieldRef.FieldPath = "metadata.uid"
	copy.Main.Command[0], copy.Main.Args[0] = "changed", "changed"
	copy.Main.Access[0].MountPath = "/changed"
	copy.Directories[2].Data = false
	copy.Files[0].Content.(KeyValues).Values["node.id"] = Literal("changed")
	copy.Files[1].Content.(Lines)[0] = "changed"
	copy.Endpoints[0].Port = 9000
	copy.LogOutputs[0].RelativePath = "changed"
	if !reflect.DeepEqual(source, before) {
		t.Fatal("cloned runtime shares mutable declaration state with the input")
	}
	if copy.Files[0].Content.(KeyValues).Codec != source.Files[0].Content.(KeyValues).Codec {
		t.Fatal("cloning replaced a stateless codec capability")
	}
}
