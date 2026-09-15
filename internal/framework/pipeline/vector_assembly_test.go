package pipeline

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/zncdatadev/operator-go/pkg/framework"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

func TestVectorCentralDestinationAndRefresh(t *testing.T) {
	options := assemblyBuildOptions()
	options.VectorDestination = &framework.VectorDestination{Address: "receiver-a.logs.svc:6000"}
	first, files, _, err := buildGroup(assemblyGroupIdentity(), assemblyCommon(true), ResolvedImage{},
		assemblyRuntimeFixture(), GroupSource{}, options, nil)
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Sinks map[string]struct {
			Type, Address string
			Inputs        []string
		} `json:"sinks"`
	}
	file := findFile(files, "config", vectorConfigFile)
	if file == nil {
		t.Fatal("no generated Vector configuration")
	}
	if err := yaml.Unmarshal([]byte(file.Content.(Text)), &config); err != nil {
		t.Fatal(err)
	}
	sink := config.Sinks["collected"]
	if sink.Type != "vector" || sink.Address != options.VectorDestination.Address || len(sink.Inputs) == 0 {
		t.Fatalf("discovery destination was not consumed: %+v", sink)
	}
	options.VectorDestination.Address = "receiver-b.logs.svc:6000"
	second, _, _, err := buildGroup(assemblyGroupIdentity(), assemblyCommon(true), ResolvedImage{},
		assemblyRuntimeFixture(), GroupSource{}, options, nil)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(first.StatefulSet.Spec.Template, second.StatefulSet.Spec.Template) ||
		reflect.DeepEqual(first.ConfigMap.Data, second.ConfigMap.Data) {
		t.Fatal("discovery refresh did not change both materialized configuration and workload template")
	}
	if findContainer(first.StatefulSet.Spec.Template, vectorContainerName).Env[0].Value != "receiver-a.logs.svc:6000" {
		t.Fatal("a later resolved destination mutated a previously built plan")
	}
}

func TestVectorFrameworkGateAndActualOutputs(t *testing.T) {
	for _, tc := range []struct {
		name               string
		enabled, hasOutput bool
		image              string
		wantCollector      bool
		wantError          bool
	}{
		{"disabled-with-output", false, true, "", false, false},
		{"disabled-without-output", false, false, "", false, false},
		{"enabled-without-output", true, false, "", false, false},
		{"enabled-missing-image", true, true, "", false, true},
		{"enabled-with-output", true, true, "example.invalid/vector:test", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runtime := assemblyRuntimeFixture()
			if !tc.hasOutput {
				runtime.LogOutputs = nil // The native file sink is OFF; there is no actual file declaration.
				runtime.ConfigDirectory, runtime.Files, runtime.Directories, runtime.Main.Access = "", nil, nil, nil
			}
			original := CloneRuntime(runtime)
			options := assemblyBuildOptions()
			options.VectorImage = tc.image
			if !tc.hasOutput {
				options.MaterializerImage = ""
			}
			var collectionKnown bool
			resources, files, checks, err := buildGroup(assemblyGroupIdentity(), assemblyCommon(tc.enabled), ResolvedImage{},
				runtime, GroupSource{}, options, func(view FinalView) []Check {
					collectionKnown = view.LogCollectionKnown
					return nil
				})
			if (err != nil) != tc.wantError {
				t.Fatalf("unexpected build result: %v", err)
			}
			if tc.wantError {
				if resources != nil || !strings.Contains(err.Error(), "vector image") {
					t.Fatalf("missing collector image should fail the group: %+v %v", resources, err)
				}
				return
			}
			collector := findContainer(resources.StatefulSet.Spec.Template, vectorContainerName)
			config := findFile(files, runtime.ConfigDirectory, vectorConfigFile)
			if (collector != nil) != tc.wantCollector || (config != nil) != tc.wantCollector ||
				collectionKnown != tc.wantCollector || !reflect.DeepEqual(runtime, original) {
				t.Fatalf("framework selection or declaration isolation failed: collector=%v config=%v known=%v",
					collector != nil, config != nil, collectionKnown)
			}
			if !tc.hasOutput && len(resources.StatefulSet.Spec.Template.Spec.InitContainers) != 0 {
				t.Fatal("no files or outputs should not introduce a preparation process")
			}
			for _, check := range checks {
				if !tc.wantCollector && strings.HasPrefix(check.Subject, "vector.") {
					t.Fatalf("unselected collector acquired a proof: %+v", check)
				}
			}
		})
	}
}

func TestVectorCollectsEveryDeclaredFile(t *testing.T) {
	runtime := assemblyRuntimeFixture()
	runtime.LogOutputs = append(runtime.LogOutputs,
		LogOutput{Container: "trino", Directory: "logs", RelativePath: "audit/events.json"})
	generated, collector, err := composeVector(runtime, true, assemblyBuildOptions())
	if err != nil {
		t.Fatal(err)
	}
	config := findFile(generated.Files, generated.ConfigDirectory, vectorConfigFile)
	var content struct {
		Sources map[string]struct {
			Include []string `json:"include"`
		} `json:"sources"`
	}
	if err := yaml.Unmarshal([]byte(config.Content.(Text)), &content); err != nil {
		t.Fatal(err)
	}
	if len(content.Sources) != 2 ||
		!slices.Equal(content.Sources["source_0"].Include, []string{"/logs/logs/server.json"}) ||
		!slices.Equal(content.Sources["source_1"].Include, []string{"/logs/logs/audit/events.json"}) {
		t.Fatalf("framework did not collect every actual declaration: %+v", content)
	}
	if collector == nil || findMount(collector, "/logs/logs") == nil || len(collector.VolumeMounts) != 3 {
		t.Fatal("outputs from the same directory should share its collector mount")
	}
	second, secondCollector, err := composeVector(runtime, true, assemblyBuildOptions())
	if err != nil || !reflect.DeepEqual(generated, second) || !reflect.DeepEqual(collector, secondCollector) {
		t.Fatal("Vector composition is not deterministic")
	}
}

func TestVectorUnselectedCannotBeClaimedByUserContainer(t *testing.T) {
	source := GroupSource{Overrides: &Overrides{PodOverrides: json.RawMessage(`{"spec":{"containers":[
	  {"name":"vector","image":"example.invalid/custom:test","command":["custom"]}]}}`)}}
	called := false
	resources, _, _, err := buildGroup(assemblyGroupIdentity(), assemblyCommon(false), ResolvedImage{},
		assemblyRuntimeFixture(), source, AssemblyOptions{MaterializerImage: "example.invalid/materializer:test"},
		func(view FinalView) []Check {
			called = true
			if view.LogCollectionKnown {
				t.Fatal("a user-supplied container impersonated the unselected framework collector")
			}
			return nil
		})
	if err != nil || !called || findContainer(resources.StatefulSet.Spec.Template, vectorContainerName) == nil {
		t.Fatalf("custom Pod patch must survive without gaining a platform premise: %v", err)
	}
}

func TestVectorRemovedOrChangedCollectorIsUnknown(t *testing.T) {
	for _, removed := range []bool{false, true} {
		expected, generated := assemblyCheckFixture(t)
		actual := cloneGroupResources(expected)
		if removed {
			actual.StatefulSet.Spec.Template.Spec.Containers = slices.DeleteFunc(
				actual.StatefulSet.Spec.Template.Spec.Containers, func(container corev1.Container) bool {
					return container.Name == vectorContainerName
				})
		} else {
			findContainer(actual.StatefulSet.Spec.Template, vectorContainerName).Command = []string{"custom"}
		}
		if collectorKnown(expected, actual, generated, generated.Files) {
			t.Fatal("changed collector retained its modeled premise")
		}
		requireAssemblyCheck(t, CheckAssembly(expected, actual, generated, generated.Files), "vector.config", Unknown)
	}
}

func TestVectorUnselectedMainNameDoesNotSelectCollector(t *testing.T) {
	runtime := assemblyRuntimeFixture()
	runtime.Main.Name = vectorContainerName
	runtime.LogOutputs[0].Container = vectorContainerName
	called := false
	resources, _, checks, err := buildGroup(assemblyGroupIdentity(), assemblyCommon(false), ResolvedImage{},
		runtime, GroupSource{}, assemblyBuildOptions(), func(view FinalView) []Check {
			called = true
			if view.LogCollectionKnown {
				t.Fatal("the main process name cannot select the framework collector")
			}
			return nil
		})
	if err != nil || resources == nil || !called {
		t.Fatalf("unselected helper name should not prevent a valid main process: %v", err)
	}
	for _, check := range checks {
		if strings.HasPrefix(check.Subject, "vector.") {
			t.Fatalf("an unselected collector gained a relationship check: %+v", check)
		}
	}
}
