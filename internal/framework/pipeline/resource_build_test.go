package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
)

func assemblyRuntimeFixture() RuntimeDescription {
	return RuntimeDescription{
		ConfigDirectory: "config",
		Main: Process{Name: "trino", Image: "example.invalid/trino:fixture", Command: []string{"launcher"},
			Args: []string{"run"}, Env: []corev1.EnvVar{{Name: "SETTING", Value: "default"}},
			Access: []DirectoryAccess{
				{Directory: "config", MountPath: "/etc/trino", ReadOnly: true},
				{Directory: "logs", MountPath: "/var/log/trino"},
			}},
		Directories: []Directory{{Name: "config"}, {Name: "logs"}},
		Files: []File{{Directory: "config", Path: "node.properties", Content: KeyValues{
			Codec: PropertiesCodec{}, Values: map[string]PropertyValue{
				"node.environment": Literal("test"), "node.id": PodNameBinding{},
			},
		}}},
		Endpoints:  []Endpoint{{Name: "http", Port: 8080}},
		LogOutputs: []LogOutput{{Container: "trino", Directory: "logs", RelativePath: "server.json"}},
	}
}

func assemblyCommon(enabled bool) CommonConfig {
	return CommonConfig{Resources: Resources{
		CPU:    CPU{Min: resource.MustParse("500m"), Max: resource.MustParse("1")},
		Memory: Memory{Limit: resource.MustParse("1Gi")},
	}, Logging: Logging{EnableVectorAgent: enabled}}
}

func assemblyGroupIdentity() GroupIdentity {
	return GroupIdentity{ClusterIdentity: ClusterIdentity{Name: "sample", Namespace: "test"},
		Role: "workers", Name: "default", Replicas: 2}
}

func assemblyBuildOptions() AssemblyOptions {
	return AssemblyOptions{MaterializerImage: "example.invalid/materializer:test",
		VectorImage: "example.invalid/vector:test"}
}

func TestBuildGroupOverrideChannelsAndIsolation(t *testing.T) {
	runtime := assemblyRuntimeFixture()
	before := CloneRuntime(runtime)
	roleArgs, groupArgs := []string{"role-cli"}, []string{}
	source := GroupSource{RoleOverrides: &Overrides{
		EnvOverrides: map[string]string{"SETTING": "role-env"}, CLIOverrides: &roleArgs,
		ConfigOverrides: map[string]FileOverride{"node.properties": {
			Properties: &PropertyOverride{Set: ptr.To(map[string]string{"node.id": "explicit-id"})},
		}},
		PodOverrides: json.RawMessage(`{"spec":{"containers":[{"name":"trino","args":["role-pod"],
		  "env":[{"name":"SETTING","value":"role-pod"}]}]}}`),
	}, Overrides: &Overrides{
		EnvOverrides: map[string]string{"SETTING": "group-env"}, CLIOverrides: &groupArgs,
		ConfigOverrides: map[string]FileOverride{"node.properties": {
			Properties: &PropertyOverride{Set: ptr.To(map[string]string{"node.environment": "group"})},
		}},
	}}
	inputBefore := CloneInput(source)
	image := ResolvedImage{PullPolicy: corev1.PullNever, PullSecretName: "registry"}
	resources, files, checks, err := buildGroup(assemblyGroupIdentity(), assemblyCommon(false), image,
		runtime, source, assemblyBuildOptions(), nil)
	if err != nil {
		t.Fatalf("build failed: %v, checks=%+v", err, checks)
	}
	main := resources.StatefulSet.Spec.Template.Spec.Containers[0]
	if !slices.Equal(main.Command, runtime.Main.Command) || !slices.Equal(main.Args, []string{"role-pod"}) ||
		main.Env[0].Value != "role-pod" || main.ImagePullPolicy != corev1.PullNever ||
		resources.StatefulSet.Spec.Template.Spec.ImagePullSecrets[0].Name != "registry" {
		t.Fatalf("role Pod patch did not win over group env/CLI: %+v", main)
	}
	if !reflect.DeepEqual(runtime, before) || !reflect.DeepEqual(source, inputBefore) {
		t.Fatal("assembly changed product declarations or override input")
	}
	file := findFile(files, "config", "node.properties")
	if id, known := literalProperty(file, "node.id"); !known || id != "explicit-id" {
		t.Fatal("file override failed to cancel the Pod name binding")
	}
	plan, err := DecodeMaterializationPlan([]byte(resources.ConfigMap.Data[materializationPlanFile]))
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	if err := Materialize(directory, plan, "actual-pod-name"); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(directory, "config", "node.properties"))
	if err != nil || !strings.Contains(string(content), "node.id=explicit-id") ||
		!strings.Contains(string(content), "node.environment=group") || strings.Contains(string(content), "actual-pod-name") {
		t.Fatalf("materialized bytes lost layered override semantics: %s, %v", content, err)
	}
	source.Overrides.PodOverrides = json.RawMessage(`{"spec":{"containers":[{"name":"trino","args":["group-pod"]}]}}`)
	resources, _, _, err = buildGroup(assemblyGroupIdentity(), assemblyCommon(false), image,
		runtime, source, assemblyBuildOptions(), nil)
	if err != nil || !slices.Equal(resources.StatefulSet.Spec.Template.Spec.Containers[0].Args, []string{"group-pod"}) {
		t.Fatalf("group Pod patch did not run last: %v", err)
	}
}

func TestBuildGroupFinalViewIsIsolated(t *testing.T) {
	runtime := assemblyRuntimeFixture()
	var observed FinalView
	resources, files, _, err := buildGroup(assemblyGroupIdentity(), assemblyCommon(true), ResolvedImage{}, runtime,
		GroupSource{}, assemblyBuildOptions(), func(view FinalView) []Check {
			observed = view
			view.Generated.Main.Command[0] = "mutated"
			view.Pod.Spec.Containers[0].Name = "mutated"
			view.Files[0].Content.(KeyValues).Values["node.id"] = Literal("mutated")
			view.Services[0].Spec.Ports[0].Port = 9
			return []Check{{Subject: "product-observation", State: Unknown, Reason: "test premise"}}
		})
	if err != nil {
		t.Fatal(err)
	}
	if !observed.FilePreparationKnown || !observed.LogCollectionKnown {
		t.Fatalf("unmodified selected helpers should remain known: %+v", observed)
	}
	if runtime.Main.Command[0] != "launcher" || resources.StatefulSet.Spec.Template.Spec.Containers[0].Name != "trino" ||
		resources.Service.Spec.Ports[0].Port != 8080 {
		t.Fatal("final validator mutated the generated resources or original runtime")
	}
	if _, ok := files[0].Content.(KeyValues).Values["node.id"].(PodNameBinding); !ok {
		t.Fatal("final validator mutated the materialized file description")
	}
}

func TestBuildGroupRejectsIndependentCollectorConflict(t *testing.T) {
	source := GroupSource{Overrides: &Overrides{
		ConfigOverrides: map[string]FileOverride{vectorConfigFile: {Remove: ptr.To(true)}},
		PodOverrides:    json.RawMessage(`{"spec":{"containers":[{"name":"trino","command":["custom"]}]}}`),
	}}
	resources, _, checks, err := buildGroup(assemblyGroupIdentity(), assemblyCommon(true), ResolvedImage{},
		assemblyRuntimeFixture(), source, assemblyBuildOptions(), nil)
	if err == nil || resources != nil {
		t.Fatal("a changed main premise hid the retained Vector's missing config")
	}
	requireAssemblyCheck(t, checks, "assembly.main.execution", Unknown)
	requireAssemblyCheck(t, checks, "vector.config", Conflict)
}
