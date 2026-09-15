package product

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/zncdatadev/operator-go/pkg/framework"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

const (
	authUsersFixture    = "users"
	authInternalFixture = "internal"
)

type authenticationReader struct{ secrets map[string]*corev1.Secret }

func (r *authenticationReader) Get(_ context.Context, key types.NamespacedName, into framework.FactResource) error {
	if class, ok := into.(*unstructured.Unstructured); ok {
		class.Object = map[string]any{"spec": map[string]any{"provider": map[string]any{"static": map[string]any{
			"userCredentialsSecret": map[string]any{"name": authUsersFixture}}}}}
		return nil
	}
	secret, ok := r.secrets[key.Name]
	if !ok {
		return apierrors.NewNotFound(schema.GroupResource{Resource: "secrets"}, key.Name)
	}
	*into.(*corev1.Secret) = *secret.DeepCopy()
	return nil
}

func TestPasswordAuthenticationRequiresTLSAndConsumesOnlySecretReferences(t *testing.T) {
	current := effectiveInput()
	current.Platform.Authentication = []framework.Authentication{{AuthenticationClass: authUsersFixture}}
	current.ClusterConfig.TLSSecret = "server-tls"
	current.ClusterConfig.InternalSecret = authInternalFixture
	in := framework.FactInput[TrinoConfig, TrinoClusterConfig, TrinoFacts]{Platform: current.Platform,
		ClusterConfig: current.ClusterConfig, Group: current.Group, Config: current.Config, Shared: BaseFacts()}
	reader := &authenticationReader{secrets: map[string]*corev1.Secret{
		authUsersFixture:    {Data: map[string][]byte{"password.db": []byte("private-hash")}},
		"server-tls":        {Data: map[string][]byte{"tls.key": []byte("private-key"), "tls.crt": []byte("public-cert")}},
		authInternalFixture: {Data: map[string][]byte{trinoInternalSecretKey: []byte("private-internal-secret")}},
	}}
	result, err := ResolveFacts(t.Context(), reader, in)
	if err != nil || result.Diagnostic.State != framework.FactsResolved {
		t.Fatalf("resolution: %+v %v", result, err)
	}
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), "private-") {
		t.Fatal("credential bytes escaped into facts")
	}
	current.Facts = *result.Value
	current.Group.Role = trinoCoordinatorRole
	runtime, err := generateTrino(current)
	if err != nil {
		t.Fatal(err)
	}
	config := findFile(runtime.Files, trinoConfigDirectory, "config.properties")
	value, known := literalProperty(config, "http-server.authentication.type")
	if !known || value != "PASSWORD" {
		t.Fatal("PASSWORD not configured")
	}
	if _, known := literalProperty(config, "http-server.authentication.allow-insecure-over-http"); known {
		t.Fatal("insecure HTTP auth enabled")
	}
	found := false
	for _, env := range runtime.Main.Env {
		if env.Name == "TRINO_INTERNAL_SHARED_SECRET" && env.ValueFrom != nil && env.ValueFrom.SecretKeyRef.Name == authInternalFixture {
			found = true
		}
	}
	if !found || findFile(runtime.Files, trinoConfigDirectory, "password-authenticator.properties") == nil ||
		len(runtime.Initializers) != 2 || runtime.Initializers[1].Name != "initialize-tls" {
		t.Fatal("runtime consumers missing")
	}
	in.ClusterConfig.TLSSecret = ""
	result, err = ResolveFacts(t.Context(), reader, in)
	if err != nil || result.Diagnostic.State != framework.FactsInvalid || result.Value != nil {
		t.Fatal("plaintext authentication accepted")
	}
	in.ClusterConfig = current.ClusterConfig
	delete(reader.secrets, authUsersFixture)
	result, err = ResolveFacts(t.Context(), reader, in)
	if err != nil || result.Diagnostic.State != framework.FactsPending || result.Value != nil {
		t.Fatal("missing password Secret did not wait")
	}
}

func TestAutoTLSIncludesPublishedListenerIdentity(t *testing.T) {
	for _, listenerClass := range []string{"", "external"} {
		t.Run("listener="+listenerClass, func(t *testing.T) {
			in := effectiveInput()
			in.Group.Role = trinoCoordinatorRole
			in.ClusterConfig.TLSSecretClass = "tls"
			in.ClusterConfig.ListenerClass = listenerClass
			runtime, err := generateTrino(in)
			if err != nil {
				t.Fatal(err)
			}
			for _, directory := range runtime.Directories {
				if directory.Name != trinoTLSSourceDirectory {
					continue
				}
				if directory.Secret == nil || directory.Secret.SecretClass != "tls" {
					t.Fatal("AutoTLS source missing")
				}
				scopes := directory.Secret.Scope
				if !slices.Contains(scopes, "pod") || !slices.Contains(scopes, "service="+in.Group.ServiceName()) {
					t.Fatal("internal TLS identities missing")
				}
				if slices.Contains(scopes, "listener-volume="+trinoListenerDirectory) != (listenerClass != "") {
					t.Fatalf("published Listener identity not reflected in TLS scope: %v", scopes)
				}
				return
			}
			t.Fatal("TLS source directory missing")
		})
	}
}
