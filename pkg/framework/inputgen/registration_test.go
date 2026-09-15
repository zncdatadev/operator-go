package inputgen

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/zncdatadev/operator-go/pkg/framework"
	"github.com/zncdatadev/operator-go/pkg/framework/input"
)

func companionNames() Names {
	names := artifactNames()
	names.ImportPath = "example.com/operator/generated"
	return names
}

func TestRegistrationGenerationKeepsAPIPackageIndependent(t *testing.T) {
	names := companionNames()
	artifacts, err := Generate[ProductConfig, ProductClusterConfig](names, []string{"workers", "coordinators"})
	if err != nil {
		t.Fatal(err)
	}
	reordered, err := Generate[ProductConfig, ProductClusterConfig](names, []string{"coordinators", "workers"})
	if err != nil || !bytes.Equal(artifacts.RegistrationSource, reordered.RegistrationSource) {
		t.Fatalf("registration is not deterministic: %v", err)
	}
	names.ImportPath = ""
	apiOnly, err := Generate[ProductConfig, ProductClusterConfig](names, []string{"workers", "coordinators"})
	if err != nil || len(apiOnly.RegistrationSource) != 0 || !bytes.Equal(apiOnly.GoSource, artifacts.GoSource) ||
		!bytes.Equal(apiOnly.CRD, artifacts.CRD) {
		t.Fatalf("registration changed the API/schema: %v", err)
	}
	source := string(bytes.Join(bytes.Fields(artifacts.RegistrationSource), []byte(" ")))
	for _, expected := range []string{
		"package registration", "const InputContractVersion = 1", "func Register[F any]",
		"type Options[F any] = operator.Options[product0.ProductConfig, product0.ProductClusterConfig, F]",
		"definition framework.ProductDefinition[product0.ProductConfig, product0.ProductClusterConfig, F]",
		"input.CheckVersion(InputContractVersion)", "operator.Register(manager, definition, options, generated.Binding())",
	} {
		if !strings.Contains(source, expected) {
			t.Fatalf("missing static registration contract %q", expected)
		}
	}
	for _, forbidden := range []string{"internal/", "docs/discussions", "InputRegistration", "Bind(F)", "SourceSnapshot"} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("public companion leaked %q", forbidden)
		}
	}
	if bytes.Contains(artifacts.GoSource, []byte(frameworkImportPath+"/operator")) {
		t.Fatal("generated API now depends on the controller registration")
	}
	if err := Check(artifacts, artifacts.GoSource, artifacts.CRD, artifacts.RegistrationSource); err != nil {
		t.Fatal(err)
	}
}

func TestRegistrationGenerationStaticReferences(t *testing.T) {
	artifacts, err := Generate[framework.ClusterOperation, struct{}](companionNames(), []string{"workers"})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(artifacts.RegistrationSource, []byte("operator.Options[framework.ClusterOperation, struct{}, F]")) ||
		bytes.Count(artifacts.RegistrationSource, []byte(`"`+frameworkImportPath+`"`)) != 1 {
		t.Fatal("framework root types did not reuse one import, or empty cluster lost its literal")
	}
	for _, typ := range []reflect.Type{reflect.TypeFor[privateRoot](), reflect.TypeFor[GenericRoot[string]](),
		reflect.TypeFor[struct{ Value string }](), reflect.TypeFor[*ProductConfig]()} {
		if _, err := registrationTypeFor(typ); err == nil {
			t.Fatalf("unreferenceable registration type accepted: %v", typ)
		}
	}
	for _, path := range []string{"/absolute", "./relative", "../relative", "example.com//api", "example.com/api/",
		"example.com/../api", `example.com\api`, "example.com/api@v1", "example.com/a b", "example.com/a\nb",
		frameworkImportPath, frameworkImportPath + "/operator", frameworkImportPath + "/input", "sigs.k8s.io/controller-runtime"} {
		names := companionNames()
		names.ImportPath = path
		if _, err := Generate[ProductConfig, ProductClusterConfig](names, []string{"workers"}); err == nil {
			t.Fatalf("invalid/conflicting registration import path accepted: %q", path)
		}
	}
}

func TestRegistrationCheckRequiresCompatibleCompanion(t *testing.T) {
	a, err := Generate[ProductConfig, ProductClusterConfig](companionNames(), []string{"workers"})
	if err != nil {
		t.Fatal(err)
	}
	for _, actual := range [][][]byte{nil, {nil}, {a.RegistrationSource, a.RegistrationSource},
		{append(bytes.Clone(a.RegistrationSource), '\n')}, {[]byte("package registration\n")}} {
		if err := Check(a, a.GoSource, a.CRD, actual...); err == nil {
			t.Fatal("missing, duplicate or stale companion accepted")
		}
	}
	future := bytes.Replace(a.RegistrationSource, []byte("InputContractVersion = 1"),
		[]byte("InputContractVersion = 999"), 1)
	if input.CheckVersion(999) == nil {
		t.Fatal("test requires an unsupported version")
	}
	a.RegistrationSource = future
	if err := Check(a, a.GoSource, a.CRD, future); err == nil || !strings.Contains(err.Error(), "incompatible") {
		t.Fatalf("mutually matching unsupported companions accepted: %v", err)
	}
	a.RegistrationSource = nil
	if err := Check(a, a.GoSource, a.CRD, future); err == nil {
		t.Fatal("unexpected companion silently ignored")
	}
}
