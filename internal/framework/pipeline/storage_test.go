package pipeline

import (
	"encoding/json"
	"testing"

	"github.com/zncdatadev/operator-go/pkg/framework"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestStorageInheritanceAndBranchSwitch(t *testing.T) {
	persistent := `{"resources":{"storage":{"type":"persistent","storageClassName":"retained","capacity":"1Gi"}}}`
	got, err := ResolveConfig(testDefaults(), json.RawMessage(persistent),
		json.RawMessage(`{"resources":{"storage":{"capacity":"2Gi"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	storage := got.Common.Resources.Storage
	if storage.Type != framework.StoragePersistent || storage.StorageClassName != "retained" ||
		storage.Capacity.Cmp(resource.MustParse("2Gi")) != 0 ||
		got.Common.Resources.CPU.Min.String() != "500m" {
		t.Fatalf("storage inheritance lost branch fields or sibling resources: %+v", got.Common.Resources)
	}
	got, err = ResolveConfig(testDefaults(), json.RawMessage(persistent),
		json.RawMessage(`{"resources":{"storage":{"type":"ephemeral"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	storage = got.Common.Resources.Storage
	if storage.Type != framework.StorageEphemeral || storage.StorageClassName != "" || !storage.Capacity.IsZero() {
		t.Fatalf("old branch survived switch: %+v", storage)
	}
	// Effective-value validation permits a higher layer to correct capacity.
	got, err = ResolveConfig(testDefaults(),
		json.RawMessage(`{"resources":{"storage":{"type":"persistent","storageClassName":"retained","capacity":"0"}}}`),
		json.RawMessage(`{"resources":{"storage":{"capacity":"1Gi"}}}`))
	if err != nil ||
		got.Common.Resources.Storage.Capacity.Cmp(resource.MustParse("1Gi")) != 0 {
		t.Fatalf("final capacity was not validated after inheritance: %v", err)
	}
}

func TestStorageLayerAndEffectiveValidation(t *testing.T) {
	for _, raw := range []string{
		`{"type":"ephemeral","capacity":"1Gi"}`, `{"type":"persistent"}`,
		`{"type":""}`, `{"storageClassName":"retained"}`,
		`{"type":"persistent","storageClassName":"retained","capacity":"0"}`,
	} {
		if _, err := ResolveConfig(testDefaults(), nil, json.RawMessage(`{"resources":{"storage":`+raw+`}}`)); err == nil {
			t.Fatalf("invalid storage accepted: %s", raw)
		}
	}
	defaults := testDefaults()
	defaults.Common.Resources.Storage = framework.Storage{Type: framework.StoragePersistent,
		StorageClassName: "retained", Capacity: resource.MustParse("1Gi")}
	got, err := ResolveConfig(defaults, nil, json.RawMessage(`{"resources":{"storage":{}}}`))
	if err != nil ||
		got.Common.Resources.Storage.Type != framework.StoragePersistent {
		t.Fatalf("empty object did not inherit defaults: %v", err)
	}
}

func TestStorageRequiresRuntimeConsumer(t *testing.T) {
	r := runtimeFixture()
	r.Files, r.LogOutputs = nil, nil
	common := testDefaults().Common
	common.Resources.Storage = framework.Storage{Type: framework.StoragePersistent,
		StorageClassName: "retained", Capacity: resource.MustParse("1Gi")}
	identity := GroupIdentity{ClusterIdentity: ClusterIdentity{Name: "store", Namespace: "test"},
		Role: "workers", Name: "default", Replicas: 1}
	if _, err := assembleGroup(identity, common, ResolvedImage{},
		r, r.Main, nil, nil, AssemblyOptions{}); err == nil {
		t.Fatal("persistent configuration without a Data directory was silently ignored")
	}
	r = retainedRuntime()
	r.Files, r.LogOutputs = nil, nil
	common.Resources.Storage = framework.Storage{Type: framework.StorageEphemeral}
	out, err := assembleGroup(identity, common, ResolvedImage{}, r, r.Main, nil, nil, AssemblyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	volume := findPodVolume(out.StatefulSet.Spec.Template.Spec, "data")
	if out.RetainedData != nil || len(out.StatefulSet.Spec.VolumeClaimTemplates) != 0 ||
		volume == nil || volume.EmptyDir == nil {
		t.Fatal("ephemeral Data directory must produce only emptyDir")
	}
}
