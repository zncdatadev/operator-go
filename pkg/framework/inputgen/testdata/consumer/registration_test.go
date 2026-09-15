package consumer_test

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"example.com/framework-consumer/generated/trino"
	registration "example.com/framework-consumer/generated/trino/registration"
	"example.com/framework-consumer/product"
	"github.com/zncdatadev/operator-go/pkg/framework"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

const registrationNamespace = "registered-consumer"

func testGeneratedRegistration(t *testing.T, configuration *rest.Config) {
	manager, err := ctrl.NewManager(configuration, ctrl.Options{Scheme: runtime.NewScheme(),
		Metrics: metricsserver.Options{BindAddress: "0"}, HealthProbeBindAddress: "0",
		Cache: cache.Options{DefaultNamespaces: map[string]cache.Config{registrationNamespace: {}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	options := registration.Options[product.TrinoFacts]{ResolveFacts: product.ResolveCatalogs,
		FactRefreshInterval: 30 * time.Millisecond,
		Assembly:            framework.AssemblyOptions{MaterializerImage: "example.invalid/materializer:fixture"},
	}
	if err := registration.Register(manager, product.Definition(), options); err != nil {
		t.Fatal(err)
	}
	c, err := client.New(configuration, client.Options{Scheme: manager.GetScheme()})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Create(t.Context(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: registrationNamespace}}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	finished := make(chan error, 1)
	go func() { finished <- manager.Start(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-finished:
			if err != nil {
				t.Errorf("manager shutdown: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("registered manager did not terminate")
		}
	})
	if !manager.GetCache().WaitForCacheSync(ctx) {
		t.Fatal("registered manager cache never synchronized")
	}
	spec := map[string]any{
		"coordinators": map[string]any{"roleGroups": map[string]any{"default": map[string]any{}}},
		"workers": map[string]any{
			"config": map[string]any{"catalogConfigMapName": "external-catalog"},
			"configOverrides": map[string]any{"catalog/tpch.properties": map[string]any{
				"properties": map[string]any{"set": map[string]any{"fixture": "overridden"}}}},
			"roleGroups": map[string]any{"default": map[string]any{}},
		},
	}
	cr, err := trino.Decode(objectJSON(t, "TrinoCluster", "registered", spec))
	if err != nil {
		t.Fatal(err)
	}
	cr.Namespace = registrationNamespace
	if err := c.Create(t.Context(), cr); err != nil {
		t.Fatal(err)
	}
	key, generation := client.ObjectKeyFromObject(cr), cr.Generation
	awaitFactState(t, c, key, framework.FactsPending, nil, generation)
	awaitRegistration(t, func() error {
		return c.Get(t.Context(), types.NamespacedName{Namespace: key.Namespace, Name: "registered-coordinators-default"}, &appsv1.StatefulSet{})
	})
	if err := c.Get(t.Context(), types.NamespacedName{Namespace: key.Namespace, Name: "registered-workers-default"},
		&appsv1.StatefulSet{}); !apierrors.IsNotFound(err) {
		t.Fatalf("pending facts allowed a worker workload: %v", err)
	}
	dependency := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "external-catalog", Namespace: key.Namespace},
		Data: map[string]string{"catalogs.json": `{"tpch":{"connector.name":"tpch"}}`}}
	if err := c.Create(t.Context(), dependency); err != nil {
		t.Fatal(err)
	}
	awaitFactState(t, c, key, framework.FactsResolved, dependency, generation)
	first := awaitCatalogOutput(t, c, key.Namespace, "connector.name=tpch\nfixture=overridden\n")
	// Invalid contents withhold this group, retaining its previous owned resources.
	dependency.Data["catalogs.json"] = "invalid JSON"
	if err := c.Update(t.Context(), dependency); err != nil {
		t.Fatal(err)
	}
	awaitFactState(t, c, key, framework.FactsInvalid, dependency, generation)
	assertCatalogRetained(t, c, first)
	dependency.Data["catalogs.json"] = `{"tpch":{"connector.name":"blackhole"}}`
	if err := c.Update(t.Context(), dependency); err != nil {
		t.Fatal(err)
	}
	awaitFactState(t, c, key, framework.FactsResolved, dependency, generation)
	updated := awaitCatalogOutput(t, c, key.Namespace, "connector.name=blackhole\nfixture=overridden\n")
	if updated.UID != first.UID {
		t.Fatal("dependency update recreated the generated ConfigMap")
	}
	oldUID := dependency.UID
	if err := c.Delete(t.Context(), dependency); err != nil {
		t.Fatal(err)
	}
	awaitFactState(t, c, key, framework.FactsPending, nil, generation)
	assertCatalogRetained(t, c, updated)
	dependency = &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "external-catalog", Namespace: key.Namespace},
		Data: map[string]string{"catalogs.json": `{"tpch":{"connector.name":"tpch"}}`}}
	if err := c.Create(t.Context(), dependency); err != nil {
		t.Fatal(err)
	}
	if dependency.UID == oldUID {
		t.Fatal("dependency recreation did not exercise a new UID")
	}
	awaitFactState(t, c, key, framework.FactsResolved, dependency, generation)
	awaitCatalogOutput(t, c, key.Namespace, "connector.name=tpch\nfixture=overridden\n")
	t.Log("external generated Register started a real manager; dependency create/invalid/update/delete/new UID refreshed without CR edits")
	t.Log("catalog bytes and owned resources verified; envtest has no kubelet, Trino process or restarter")
}

func awaitRegistration(t *testing.T, inspect func() error) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		if last = inspect(); last == nil {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("registration did not converge: %v", last)
}

func awaitFactState(t *testing.T, c client.Client, key types.NamespacedName, state framework.FactState,
	dependency *corev1.ConfigMap, generation int64,
) {
	t.Helper()
	awaitRegistration(t, func() error {
		stored := &trino.TrinoCluster{}
		if err := c.Get(t.Context(), key, stored); err != nil {
			return err
		}
		if stored.Generation != generation {
			return fmt.Errorf("CR generation changed: %d", stored.Generation)
		}
		for _, group := range stored.Status.Groups {
			if group.Role != "workers" || group.Name != "default" || group.Facts == nil {
				continue
			}
			if group.Facts.State != state {
				return fmt.Errorf("facts state %s, want %s: %s", group.Facts.State, state, group.Message)
			}
			if dependency == nil {
				return nil
			}
			for _, object := range group.Facts.Observed {
				if object.Name == dependency.Name && object.Namespace == dependency.Namespace &&
					object.UID == string(dependency.UID) && object.ResourceVersion == dependency.ResourceVersion {
					return nil
				}
			}
			return fmt.Errorf("dependency provenance has not refreshed: %+v", group.Facts.Observed)
		}
		return fmt.Errorf("worker facts have not been reported: %+v", stored.Status)
	})
}

func awaitCatalogOutput(t *testing.T, c client.Client, namespace, want string) *corev1.ConfigMap {
	t.Helper()
	var found *corev1.ConfigMap
	awaitRegistration(t, func() error {
		cm := &corev1.ConfigMap{}
		if err := c.Get(t.Context(), types.NamespacedName{Namespace: namespace, Name: "registered-workers-default"}, cm); err != nil {
			return err
		}
		// Inspect the actual helper payload without importing the SDK's private plan types.
		var plan struct {
			Files []struct {
				Directory, Path string
				Encoded         *string
			}
		}
		if err := json.Unmarshal([]byte(cm.Data["materialization.json"]), &plan); err != nil {
			return err
		}
		for _, file := range plan.Files {
			if file.Directory == "config" && file.Path == "catalog/tpch.properties" && file.Encoded != nil && *file.Encoded == want {
				found = cm
				return nil
			}
		}
		return fmt.Errorf("generated catalog has not refreshed: %s", cm.Data["materialization.json"])
	})
	return found
}

func assertCatalogRetained(t *testing.T, c client.Client, before *corev1.ConfigMap) {
	t.Helper()
	after := &corev1.ConfigMap{}
	if err := c.Get(t.Context(), client.ObjectKeyFromObject(before), after); err != nil {
		t.Fatal(err)
	}
	if after.UID != before.UID || !reflect.DeepEqual(after.Data, before.Data) {
		t.Fatal("unresolved facts replaced the last ConfigMap")
	}
}
