package pipeline

import (
	"encoding/json"
	"testing"

	"github.com/zncdatadev/operator-go/pkg/framework"
	corev1 "k8s.io/api/core/v1"
)

func TestPlatformDirectoryAssemblyAndConsumerProtection(t *testing.T) {
	runtime := runtimeFixture()
	runtime.Files = nil
	runtime.LogOutputs = nil
	runtime.Directories = append(runtime.Directories,
		Directory{Name: "credentials", Secret: &framework.SecretVolume{SecretClass: "database", Scope: []string{"pod"}}},
		Directory{Name: "external", Listener: &framework.ListenerVolume{Class: "public"}})
	runtime.Main.Access = append(runtime.Main.Access,
		DirectoryAccess{Directory: "credentials", MountPath: "/credentials", ReadOnly: true},
		DirectoryAccess{Directory: "external", MountPath: "/listener", ReadOnly: true})
	if err := ValidateRuntime(runtime); err != nil {
		t.Fatal(err)
	}
	group := GroupIdentity{ClusterIdentity: ClusterIdentity{Name: "demo", Namespace: "test"}, Role: "workers",
		Name: "default", Replicas: 1}
	expected, err := assembleGroup(group, CommonConfig{}, ResolvedImage{PullPolicy: corev1.PullIfNotPresent},
		runtime, runtime.Main, nil, nil, AssemblyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	pod := expected.StatefulSet.Spec.Template.Spec
	secret := findPodVolume(pod, "credentials")
	listener := findPodVolume(pod, "external")
	if secret.Ephemeral == nil || *secret.Ephemeral.VolumeClaimTemplate.Spec.StorageClassName != "secrets.kubedoop.dev" {
		t.Fatal("SecretClass was not declared to its CSI provisioner")
	}
	if secret.Ephemeral.VolumeClaimTemplate.Annotations["secrets.kubedoop.dev/scope"] != "pod" ||
		listener.Ephemeral.VolumeClaimTemplate.Annotations["listeners.kubedoop.dev/class"] != "public" {
		t.Fatal("platform declarations lost their native provisioner contract")
	}
	for _, check := range CheckAssembly(expected, expected, runtime, nil) {
		if check.State == Conflict {
			t.Fatalf("valid platform declaration rejected: %+v", check)
		}
	}
	actual := cloneGroupResources(expected)
	// The final podOverrides layer keeps its priority, but a declared credential
	// consumer cannot be silently redirected to unrelated files.
	patch := json.RawMessage(`{"spec":{"containers":[{"name":"trino",
 "volumeMounts":[{"mountPath":"/credentials/key","name":"logs"}]}]}}`)
	if err := patchPod(&actual.StatefulSet.Spec.Template, patch); err != nil {
		t.Fatal(err)
	}
	conflict := false
	for _, check := range CheckAssembly(expected, actual, runtime, nil) {
		conflict = conflict || check.State == Conflict
	}
	if !conflict {
		t.Fatal("overlapping override hid the declared credential consumer")
	}
}
