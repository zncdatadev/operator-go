package product

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zncdatadev/operator-go/pkg/framework"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

type catalogReader struct {
	object *corev1.ConfigMap
	err    error
	calls  int
	key    types.NamespacedName
}

func (r *catalogReader) Get(_ context.Context, key types.NamespacedName, into framework.FactResource) error {
	r.calls++
	r.key = key
	if r.err != nil {
		return r.err
	}
	*into.(*corev1.ConfigMap) = *r.object.DeepCopy()
	return nil
}

func TestCatalogFactsPresenceWaitAndSanitizedFailure(t *testing.T) {
	current := effectiveInput()
	in := framework.FactInput[TrinoConfig, TrinoClusterConfig, TrinoFacts]{
		Group: current.Group, Config: current.Config, ClusterConfig: current.ClusterConfig, Shared: BaseFacts(),
	}
	reader := &catalogReader{}
	result, err := ResolveFacts(t.Context(), reader, in)
	if err != nil || reader.calls != 0 || result.Diagnostic.State != framework.FactsResolved {
		t.Fatal("no catalog reference should use isolated base facts without IO")
	}
	result.Value.Catalogs[tpchCatalog]["connector.name"] = "changed"
	if in.Shared.Catalogs[tpchCatalog]["connector.name"] != tpchCatalog {
		t.Fatal("resolved facts alias the base facts")
	}
	in.Config.Product.CatalogConfigMapName = "catalogs"
	reader.err = apierrors.NewNotFound(schema.GroupResource{Resource: "configmaps"}, "catalogs")
	result, err = ResolveFacts(t.Context(), reader, in)
	if err != nil || result.Diagnostic.State != framework.FactsPending || result.Value != nil ||
		reader.key != (types.NamespacedName{Namespace: current.Group.Namespace, Name: "catalogs"}) {
		t.Fatalf("missing namespace-scoped reference did not wait: %+v %v", result, err)
	}
	reader.err = nil
	reader.object = &corev1.ConfigMap{Data: map[string]string{trinoCatalogSourceKey: `{"tpch":{"connector.name":"tpch"}}`}}
	result, err = ResolveFacts(t.Context(), reader, in)
	if err != nil || result.Diagnostic.State != framework.FactsResolved || result.Value.Catalogs[tpchCatalog]["connector.name"] != tpchCatalog {
		t.Fatalf("valid catalog was not resolved: %+v %v", result, err)
	}
	reader.object.DeletionTimestamp = &metav1.Time{Time: metav1.Now().Time}
	result, err = ResolveFacts(t.Context(), reader, in)
	if err != nil || result.Diagnostic.State != framework.FactsPending {
		t.Fatal("deleting source should wait instead of providing stale catalogs")
	}
	reader.object.DeletionTimestamp = nil
	for _, document := range []string{`null`, `{"tpch":null}`, `{"tpch":{"connector.name":null}}`,
		`{"tpch":{"connector.name":"tpch","connector.name":"private-password"}}`, `{"tpch":{"password":"private-password"}}`} {
		reader.object.Data[trinoCatalogSourceKey] = document
		result, err = ResolveFacts(t.Context(), reader, in)
		if err != nil || result.Diagnostic.State != framework.FactsInvalid || result.Value != nil ||
			strings.Contains(result.Diagnostic.Message, "private-password") {
			t.Fatalf("invalid dependency was leaked or accepted: %+v %v", result, err)
		}
	}
	reader.err = errors.New("read unavailable")
	if _, err := ResolveFacts(t.Context(), reader, in); !errors.Is(err, reader.err) {
		t.Fatal("read error was misclassified as pending or valid facts")
	}
}
