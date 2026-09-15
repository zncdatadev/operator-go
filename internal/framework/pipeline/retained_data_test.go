package pipeline

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zncdatadev/operator-go/pkg/framework"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func retainedRuntime() RuntimeDescription {
	r := runtimeFixture()
	r.Directories = append(r.Directories, Directory{Name: "data", Data: true})
	r.Main.Access = append(r.Main.Access, DirectoryAccess{Directory: "data", MountPath: "/data"})
	return r
}

func TestRetainedRuntimeRejectsOverlappingResponsibilities(t *testing.T) {
	tests := []struct {
		name, want string
		change     func(*RuntimeDescription)
	}{
		{"two-slots", "one data", func(r *RuntimeDescription) {
			r.Directories = append(r.Directories, Directory{Name: "extra", Data: true})
		}},
		{"config-root", "configuration directory", func(r *RuntimeDescription) { r.ConfigDirectory = "data" }},
		{"generated-data", "generated files", func(r *RuntimeDescription) {
			r.Files = append(r.Files, File{Directory: "data", Path: "marker", Content: Text("overwrite")})
		}},
		{"logs", "separate ephemeral", func(r *RuntimeDescription) { r.LogOutputs[0].Directory = "data" }},
		{"read-only", "writable access", func(r *RuntimeDescription) { r.Main.Access[2].ReadOnly = true }},
	}
	if err := ValidateRuntime(retainedRuntime()); err != nil {
		t.Fatal(err)
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := retainedRuntime()
			test.change(&r)
			if err := ValidateRuntime(r); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("want %q, got %v", test.want, err)
			}
		})
	}
}

func retainedAssembly(t *testing.T) (RuntimeDescription, GroupResources) {
	t.Helper()
	r := retainedRuntime()
	r.Files = nil
	r.LogOutputs = nil
	identity := GroupIdentity{ClusterIdentity: ClusterIdentity{Name: "store", Namespace: "test"},
		Role: "workers", Name: "default", Replicas: 1}
	common := CommonConfig{Resources: Resources{Storage: framework.Storage{Type: framework.StoragePersistent,
		StorageClassName: "retained-test", Capacity: resource.MustParse("1Gi")}}}
	assembled, err := assembleGroup(identity, common, ResolvedImage{PullPolicy: corev1.PullIfNotPresent},
		r, r.Main, nil, nil, AssemblyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return r, assembled
}

func TestRetainedAssemblyCarriesExplicitClaimAndIsolatesMutableData(t *testing.T) {
	r, assembled := retainedAssembly(t)
	sts := &assembled.StatefulSet
	if len(sts.Spec.VolumeClaimTemplates) != 1 || assembled.RetainedData == nil {
		t.Fatalf("explicit declaration/claim missing: %+v", assembled)
	}
	claim := sts.Spec.VolumeClaimTemplates[0]
	if claim.Name != "data" || *claim.Spec.StorageClassName != "retained-test" ||
		claim.Spec.Resources.Requests.Storage().Cmp(resource.MustParse("1Gi")) != 0 ||
		*claim.Spec.VolumeMode != corev1.PersistentVolumeFilesystem || len(claim.Spec.AccessModes) != 1 ||
		claim.Spec.AccessModes[0] != corev1.ReadWriteOnce || len(claim.OwnerReferences) != 0 {
		t.Fatalf("incorrect storage contract: %+v", claim)
	}
	policy := sts.Spec.PersistentVolumeClaimRetentionPolicy
	if policy == nil || policy.WhenDeleted != appsv1.RetainPersistentVolumeClaimRetentionPolicyType ||
		policy.WhenScaled != appsv1.RetainPersistentVolumeClaimRetentionPolicyType {
		t.Fatal("both retain directions must be explicit")
	}
	if findPodVolume(sts.Spec.Template.Spec, "data") != nil {
		t.Fatal("claim must not be shadowed by an emptyDir or direct PVC volume")
	}
	for _, check := range CheckAssembly(assembled, assembled, r, nil) {
		if check.State == Conflict {
			t.Fatalf("valid retained volume incorrectly rejected: %+v", check)
		}
	}
	clone := CloneRuntime(r)
	clone.Directories[2].Data = false
	copy := cloneGroupResources(assembled)
	copy.RetainedData.Capacity.Add(resource.MustParse("2Gi"))
	if !r.Directories[2].Data || assembled.RetainedData.Capacity.String() != "1Gi" {
		t.Fatal("clones leaked mutable retained storage declarations")
	}
}

func TestPodOverridesCannotDisplaceRetainedMounts(t *testing.T) {
	patches := []string{
		`{"spec":{"volumes":[{"name":"data","emptyDir":{}}]}}`,
		`{"spec":{"containers":[{"name":"trino","volumeMounts":[{"mountPath":"/data","$patch":"delete"}]}]}}`,
		`{"spec":{"containers":[{"name":"trino","volumeMounts":[{"mountPath":"/data","name":"logs"}]}]}}`,
		`{"spec":{"containers":[{"name":"trino","volumeMounts":[{"mountPath":"/data","readOnly":true}]}]}}`,
		`{"spec":{"containers":[{"name":"trino","volumeMounts":[{"mountPath":"/data","subPath":"other"}]}]}}`,
		`{"spec":{"containers":[{"name":"trino","volumeMounts":[{"mountPath":"/data/marker","name":"logs"}]}]}}`,
	}
	for _, patch := range patches {
		r, expected := retainedAssembly(t)
		actual := cloneGroupResources(expected)
		if err := patchPod(&actual.StatefulSet.Spec.Template, json.RawMessage(patch)); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, check := range CheckAssembly(expected, actual, r, nil) {
			found = found || check.State == Conflict
		}
		if !found {
			t.Fatalf("broken durable mount accepted: %s", patch)
		}
	}
	r, expected := retainedAssembly(t)
	actual := cloneGroupResources(expected)
	if err := patchPod(&actual.StatefulSet.Spec.Template, json.RawMessage(
		`{"spec":{"containers":[{"name":"trino","env":[{"name":"USER_ENV","value":"final"}]}]}}`)); err != nil {
		t.Fatal(err)
	}
	for _, check := range CheckAssembly(expected, actual, r, nil) {
		if check.State == Conflict {
			t.Fatalf("unrelated final override must remain usable: %+v", check)
		}
	}
}
