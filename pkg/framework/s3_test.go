package framework

import (
	"context"
	"encoding/json"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

type s3TestReader struct {
	spec  map[string]any
	err   error
	reads int
	key   types.NamespacedName
}

func (r *s3TestReader) Get(_ context.Context, key types.NamespacedName, object FactResource) error {
	r.reads++
	r.key = key
	object.(*unstructured.Unstructured).Object["spec"] = r.spec
	return r.err
}

func TestS3ConnectionReferencesAndCredentialIdentity(t *testing.T) {
	reader := &s3TestReader{spec: map[string]any{"host": "minio.test.svc", "port": int64(9000), "pathStyle": true,
		"credentials": map[string]any{"secretClass": "s3-credentials", "scope": map[string]any{"pod": true}}}}
	connection := S3Connection{Type: S3Reference, Reference: "warehouse"}
	result, err := ResolveS3Connection(context.Background(), reader, "test", connection)
	if err != nil || result.Diagnostic.State != FactsResolved || result.Value == nil {
		t.Fatalf("reference resolution: %+v %v", result, err)
	}
	if result.Value.Endpoint != "http://minio.test.svc:9000" || !result.Value.PathStyle ||
		result.Value.Region != "us-east-1" || result.Value.Credentials.SecretClass != "s3-credentials" ||
		len(result.Value.Credentials.Scope) != 1 || result.Value.Credentials.Scope[0] != "pod" ||
		reader.key != (types.NamespacedName{Namespace: "test", Name: "warehouse"}) {
		t.Fatalf("resolved resource lost its endpoint or credential identity: %+v", result.Value)
	}
	// JSON facts contain only references. No Secret was read to resolve the
	// endpoint, and no secret bytes become config-generation data.
	if _, err := json.Marshal(result.Value); err != nil || reader.reads != 1 {
		t.Fatal("S3 resolution crossed the endpoint/credential-materialization boundary")
	}
	reader.err = apierrors.NewNotFound(schema.GroupResource{Group: "s3.kubedoop.dev", Resource: "s3connections"}, "warehouse")
	result, err = ResolveS3Connection(context.Background(), reader, "test", connection)
	if err != nil || result.Diagnostic.State != FactsPending || result.Value != nil {
		t.Fatalf("missing S3 reference did not wait: %+v %v", result, err)
	}
	reader.err = nil
	reader.spec["tls"] = map[string]any{"verification": map[string]any{"none": map[string]any{}}}
	result, err = ResolveS3Connection(context.Background(), reader, "test", connection)
	if err != nil || result.Diagnostic.State != FactsInvalid || result.Value != nil {
		t.Fatal("unsupported TLS verification was silently downgraded")
	}
}

func TestS3InlineDoesNotReadExternalConnection(t *testing.T) {
	reader := &s3TestReader{}
	result, err := ResolveS3Connection(context.Background(), reader, "test", S3Connection{Type: S3Inline,
		Inline: S3Endpoint{Host: "s3.example.com", TLS: true, Credentials: S3Credentials{SecretName: "s3"}}})
	if err != nil || result.Diagnostic.State != FactsResolved || result.Value.Endpoint != "https://s3.example.com:443" ||
		reader.reads != 0 {
		t.Fatalf("inline endpoint resolution: %+v %v", result, err)
	}
}
