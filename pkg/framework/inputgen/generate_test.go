package inputgen

import (
	"bytes"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

type ProductConfig struct {
	Port    int32             `json:"port"`
	Enabled bool              `json:"enabled"`
	Args    []string          `json:"args"`
	Labels  map[string]string `json:"labels"`
}

type ProductClusterConfig struct {
	Environment string `json:"environment"`
}

func artifactNames() Names {
	return Names{Package: "generated", Group: "example.test", Version: "v1alpha1",
		Kind: "ExampleCluster", Plural: "exampleclusters"}
}

func generatedArtifacts(t *testing.T) Artifacts {
	t.Helper()
	artifacts, err := Generate[ProductConfig, ProductClusterConfig](artifactNames(), []string{"workers", "coordinators"})
	if err != nil {
		t.Fatal(err)
	}
	return artifacts
}

func TestGenerateDeterministicContract(t *testing.T) {
	a := generatedArtifacts(t)
	b, err := Generate[ProductConfig, ProductClusterConfig](artifactNames(), []string{"coordinators", "workers"})
	if err != nil || !bytes.Equal(a.GoSource, b.GoSource) || !bytes.Equal(a.CRD, b.CRD) {
		t.Fatalf("role declaration order changed artifacts: %v", err)
	}
	fields := string(bytes.Join(bytes.Fields(a.GoSource), []byte(" ")))
	for _, expected := range []string{
		"const InputContractVersion = 1", "Port *int32", "Enabled *bool", "Args *[]string",
		"Labels *map[string]string", "func Project(in *ExampleCluster) (input.Projection, error)",
		"func Binding() input.Binding[*ExampleCluster]", "func Operation(in *ExampleCluster) framework.ClusterOperation",
	} {
		if !strings.Contains(fields, expected) {
			t.Errorf("generated source lacks %q", expected)
		}
	}
	for _, forbidden := range []string{
		"docs/discussions", "internal/framework", "SourceSnapshot", "Snapshot[", "Register[",
	} {
		if bytes.Contains(a.GoSource, []byte(forbidden)) {
			t.Errorf("generated API depends on %q", forbidden)
		}
	}
	if bytes.Contains(a.CRD, []byte("default:")) {
		t.Fatal("admission defaults erase inheritance presence")
	}
}

func TestGenerateRejectsAmbiguousNames(t *testing.T) {
	for _, roles := range [][]string{
		{"foo-bar", "foo--bar"}, {"workers", "workers"}, {"9role"}, nil,
		{"image"}, {"clusterConfig"}, {"cluster-config"},
	} {
		if _, err := Generate[ProductConfig, struct{}](artifactNames(), roles); err == nil {
			t.Fatalf("invalid roles accepted: %v", roles)
		}
	}
	for _, kind := range []string{"ConfigInput", "SpecInput", "Project", "Binding", contractVersionName} {
		names := artifactNames()
		names.Kind = kind
		if _, err := Generate[ProductConfig, struct{}](names, []string{"workers"}); err == nil {
			t.Fatalf("conflicting kind %q accepted", kind)
		}
	}
}

type privateRoot struct{ Value bool }
type GenericRoot[T any] struct{ Value T }
type NativeAffinityProduct struct{ Affinity corev1.Affinity }
type PointerProduct struct{ Value *bool }

func TestGenerateRejectsUnpublishableRoots(t *testing.T) {
	for _, generate := range []func(Names, []string) (Artifacts, error){
		Generate[privateRoot, struct{}], Generate[GenericRoot[bool], struct{}],
		Generate[struct{ Value bool }, struct{}], Generate[struct{}, privateRoot],
		Generate[PointerProduct, struct{}], Generate[NativeAffinityProduct, struct{}],
	} {
		if _, err := generate(artifactNames(), []string{"workers"}); err == nil {
			t.Fatal("unpublishable or unsupported product root accepted")
		}
	}
	if _, err := Generate[struct{}, struct{}](artifactNames(), []string{"workers"}); err != nil {
		t.Fatalf("empty product domains rejected: %v", err)
	}
}

type CollidingClusterConfig struct {
	Stopped string `json:"productStopped"`
}

func TestGenerateClusterOperationCollision(t *testing.T) {
	_, err := Generate[ProductConfig, CollidingClusterConfig](artifactNames(), []string{"workers"})
	if err == nil || !strings.Contains(err.Error(), "collides") {
		t.Fatalf("operation Go name collision accepted: %v", err)
	}
}

func TestCheckRejectsLegacyAndChangedArtifacts(t *testing.T) {
	expected := generatedArtifacts(t)
	if err := Check(expected, expected.GoSource, expected.CRD); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct {
		name, want string
		goSource   []byte
		crd        []byte
	}{
		{"legacy", "missing InputContractVersion", []byte("package generated\n"), expected.CRD},
		{"old contract", "incompatible", bytes.Replace(expected.GoSource,
			[]byte("InputContractVersion = 1"), []byte("InputContractVersion = 0"), 1), expected.CRD},
		{"expression", "integer literal", bytes.Replace(expected.GoSource,
			[]byte("InputContractVersion = 1"), []byte("InputContractVersion = 1 + 0"), 1), expected.CRD},
		{"source drift", "Go source differs", append(bytes.Clone(expected.GoSource), '\n'), expected.CRD},
		{"CRD drift", "CRD differs", expected.GoSource, append(bytes.Clone(expected.CRD), '\n')},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			err := Check(expected, scenario.goSource, scenario.crd)
			if err == nil || !strings.Contains(err.Error(), scenario.want) {
				t.Fatalf("want %q, got %v", scenario.want, err)
			}
		})
	}
}

func TestSchemaNativeDomainsRetainTheirBoundaries(t *testing.T) {
	schema, err := configSchema[ProductConfig]()
	if err != nil {
		t.Fatal(err)
	}
	affinity := schema.Properties["affinity"]
	terms := affinity.Properties["nodeAffinity"].Properties["requiredDuringSchedulingIgnoredDuringExecution"].
		Properties["nodeSelectorTerms"]
	if terms.MaxItems == nil || *terms.MaxItems != AffinityCollectionLimit ||
		schema.Properties["labels"].MaxProperties == nil || *schema.Properties["labels"].MaxProperties != CollectionLimit {
		t.Fatal("native and ordinary collection capacities were combined")
	}
	duration := schema.Properties["gracefulShutdownTimeout"]
	if duration.MaxLength == nil || *duration.MaxLength != 128 || len(duration.XValidations) != 1 ||
		duration.XValidations[0].Rule != "duration(self) == duration(self)" {
		t.Fatal("duration schema must check syntax without imposing effective shutdown policy")
	}
	declaration, err := emitConfigTypes[ProductConfig]()
	if err != nil || !strings.Contains(declaration, "*corev1.Affinity") ||
		!strings.Contains(declaration, "*metav1.Duration") {
		t.Fatalf("native presence types lost: %v", err)
	}
}
