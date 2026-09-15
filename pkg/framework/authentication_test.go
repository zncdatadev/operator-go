package framework

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

type authenticationReader struct {
	object *unstructured.Unstructured
	key    types.NamespacedName
	err    error
}

func (r *authenticationReader) Get(_ context.Context, key types.NamespacedName, into FactResource) error {
	r.key = key
	if r.err != nil {
		return r.err
	}
	into.(*unstructured.Unstructured).Object = r.object.DeepCopy().Object
	return nil
}

func TestAuthenticationClassExactReadAndProviderSelection(t *testing.T) {
	object := &unstructured.Unstructured{Object: map[string]any{"spec": map[string]any{"provider": map[string]any{
		"static": map[string]any{"userCredentialsSecret": map[string]any{"name": "users"}}}}}}
	reader := &authenticationReader{object: object}
	result, err := ResolveAuthenticationClass(t.Context(), reader, "password-users")
	if err != nil || result.Diagnostic.State != FactsResolved || result.Value.Static.UserCredentialsSecret.Name != "users" ||
		reader.key != (types.NamespacedName{Name: "password-users"}) {
		t.Fatalf("wrong provider or scope: %+v %v", result, err)
	}
	encoded, _ := json.Marshal(result.Value)
	if strings.Contains(string(encoded), "password.db") {
		t.Fatal("resolver read provider credentials")
	}
	provider, _, _ := unstructured.NestedMap(object.Object, "spec", "provider")
	provider["tls"] = map[string]any{}
	if err := unstructured.SetNestedMap(object.Object, provider, "spec", "provider"); err != nil {
		t.Fatal(err)
	}
	result, err = ResolveAuthenticationClass(t.Context(), reader, "password-users")
	if err != nil || result.Diagnostic.State != FactsInvalid || result.Value != nil {
		t.Fatal("multiple providers accepted")
	}
	reader.err = apierrors.NewNotFound(schema.GroupResource{Resource: "authenticationclasses"}, "password-users")
	result, err = ResolveAuthenticationClass(t.Context(), reader, "password-users")
	if err != nil || result.Diagnostic.State != FactsPending || result.Value != nil {
		t.Fatal("missing class did not wait")
	}
}
