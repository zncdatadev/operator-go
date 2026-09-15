package framework

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

type vectorReader struct {
	address string
	err     error
	key     types.NamespacedName
}

func (r *vectorReader) Get(_ context.Context, key types.NamespacedName, object FactResource) error {
	r.key = key
	object.(*corev1.ConfigMap).Data = map[string]string{"ADDRESS": r.address}
	return r.err
}

func TestResolveVectorDestination(t *testing.T) {
	readError := errors.New("api unavailable")
	for _, tc := range []struct {
		name, address string
		err           error
		state         FactState
	}{
		{name: "resolved", address: "aggregator.test.svc:6000", state: FactsResolved},
		{name: "missing", err: apierrors.NewNotFound(schema.GroupResource{Resource: "configmaps"}, "destination"), state: FactsPending},
		{name: "empty", state: FactsInvalid},
		{name: "invalid-port", address: "aggregator.test.svc:0", state: FactsInvalid},
		{name: "url", address: "https://aggregator.test.svc:6000", state: FactsInvalid},
		{name: "substitution", address: "${DESTINATION}:6000", state: FactsInvalid},
		{name: "api-error", err: readError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := &vectorReader{address: tc.address, err: tc.err}
			result, err := ResolveVectorDestination(context.Background(), reader, "test", "destination")
			if tc.name == "api-error" {
				if !errors.Is(err, readError) {
					t.Fatal("failed API read was hidden")
				}
				return
			}
			if err != nil || result.Diagnostic.State != tc.state || (result.Value != nil) != (tc.state == FactsResolved) {
				t.Fatalf("resolution: %+v %v", result, err)
			}
			if reader.key != (types.NamespacedName{Namespace: "test", Name: "destination"}) {
				t.Fatal("reference escaped its namespace")
			}
		})
	}
}
