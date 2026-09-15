package pipeline

import (
	"slices"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func assemblyCheckFixture(t *testing.T) (GroupResources, RuntimeDescription) {
	t.Helper()
	runtime := assemblyRuntimeFixture()
	runtime.ConfigDirectory = "config"
	runtime.Main.Command, runtime.Main.Args = []string{"launcher"}, []string{"run"}
	file := runtime.Files[0].Content.(KeyValues)
	file.Codec = PropertiesCodec{}
	runtime.Files[0].Content = file
	options := AssemblyOptions{
		MaterializerImage: "example.invalid/materializer:test", VectorImage: "example.invalid/vector:test",
	}
	generated, vector, err := composeVector(runtime, true, options)
	if err != nil {
		t.Fatal(err)
	}
	common := CommonConfig{Resources: Resources{
		CPU:    CPU{Min: resource.MustParse("500m"), Max: resource.MustParse("1")},
		Memory: Memory{Limit: resource.MustParse("1Gi")},
	}}
	resources, err := assembleGroup(GroupIdentity{ClusterIdentity: ClusterIdentity{Name: "sample", Namespace: "test"},
		Role: "worker", Name: "default", Replicas: 1}, common, ResolvedImage{PullPolicy: corev1.PullIfNotPresent},
		generated, generated.Main, vector, generated.Files, options)
	if err != nil {
		t.Fatal(err)
	}
	return resources, generated
}

func requireAssemblyCheck(t *testing.T, checks []Check, subject string, state CheckState) {
	t.Helper()
	for _, check := range checks {
		if check.Subject == subject && check.State == state {
			return
		}
	}
	t.Fatalf("expected %s=%s, got %#v", subject, state, checks)
}

func TestAssemblyChecksKeepIndependentConsumerAndStructureFailures(t *testing.T) {
	expected, generated := assemblyCheckFixture(t)
	for _, check := range CheckAssembly(expected, expected, generated, generated.Files) {
		if check.State != Consistent {
			t.Fatalf("fixture should retain all modeled relations: %#v", check)
		}
	}
	actual := cloneGroupResources(expected)
	pod := &actual.StatefulSet.Spec.Template
	pod.Spec.Containers[0].Command = []string{"custom-launcher"}
	pod.Spec.Containers[0].Ports[0].Name = "renamed"
	pod.Labels = map[string]string{"unrelated": "label"}
	files := slices.DeleteFunc(cloneDeclaredFiles(generated.Files), func(file File) bool {
		return file.Path == vectorConfigFile
	})
	checks := CheckAssembly(expected, actual, generated, files)
	requireAssemblyCheck(t, checks, "assembly.main.execution", Unknown)
	requireAssemblyCheck(t, checks, "vector.config", Conflict)
	requireAssemblyCheck(t, checks, "statefulset.selector", Conflict)
	requireAssemblyCheck(t, checks, "service["+expected.Service.Name+"].selector", Conflict)
	requireAssemblyCheck(t, checks, "service["+expected.Service.Name+"].port[http]", Conflict)
	// A missing main is a declaration conflict, even with other custom containers.
	pod.Spec.Containers = slices.DeleteFunc(pod.Spec.Containers, func(container corev1.Container) bool {
		return container.Name == generated.Main.Name
	})
	requireAssemblyCheck(t, CheckAssembly(expected, actual, generated, files), "assembly.main", Conflict)
}

func TestAssemblyChecksDoNotClaimChangedPreparationIsKnown(t *testing.T) {
	cases := []struct {
		name   string
		change func(*corev1.PodSpec)
	}{
		{"removed", func(pod *corev1.PodSpec) { pod.InitContainers = nil }},
		{"command", func(pod *corev1.PodSpec) { pod.InitContainers[0].Command = []string{"custom"} }},
		{"args", func(pod *corev1.PodSpec) { pod.InitContainers[0].Args = []string{} }},
		{"environment", func(pod *corev1.PodSpec) {
			pod.InitContainers[0].Env = []corev1.EnvVar{{Name: "POD_NAME", Value: "x"}}
		}},
		{"image", func(pod *corev1.PodSpec) { pod.InitContainers[0].Image = "example.invalid/different:test" }},
		{"plan-mount", func(pod *corev1.PodSpec) { pod.InitContainers[0].VolumeMounts[0].MountPath = "/alternate-plan" }},
		{"plan-source", func(pod *corev1.PodSpec) {
			findPodVolume(*pod, materializationPlanVolume).ConfigMap.Name = "other-config-map"
		}},
		{"extra-mount", func(pod *corev1.PodSpec) {
			pod.InitContainers[0].VolumeMounts = append(pod.InitContainers[0].VolumeMounts,
				corev1.VolumeMount{Name: "config", MountPath: "/other"})
		}},
		{"second-init-writer", func(pod *corev1.PodSpec) {
			pod.InitContainers = append(pod.InitContainers, corev1.Container{
				Name: "rewrite-config", Image: "example.invalid/init",
				VolumeMounts: []corev1.VolumeMount{{Name: "config", MountPath: "/write"}}})
		}},
		{"second-running-writer", func(pod *corev1.PodSpec) {
			pod.Containers = append(pod.Containers, corev1.Container{Name: "rewrite-config", Image: "example.invalid/helper",
				VolumeMounts: []corev1.VolumeMount{{Name: "config", MountPath: "/write"}}})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expected, generated := assemblyCheckFixture(t)
			actual := cloneGroupResources(expected)
			tc.change(&actual.StatefulSet.Spec.Template.Spec)
			checks := CheckAssembly(expected, actual, generated, generated.Files)
			requireAssemblyCheck(t, checks, "assembly.materialization", Unknown)
			requireAssemblyCheck(t, checks, "assembly.files[config]", Unknown)
			files := slices.DeleteFunc(cloneDeclaredFiles(generated.Files), func(file File) bool {
				return file.Path == vectorConfigFile
			})
			requireAssemblyCheck(t, CheckAssembly(expected, actual, generated, files), "vector.config", Unknown)
		})
	}
	expected, generated := assemblyCheckFixture(t)
	actual := cloneGroupResources(expected)
	actual.StatefulSet.Spec.Template.Spec.InitContainers = append(actual.StatefulSet.Spec.Template.Spec.InitContainers,
		corev1.Container{Name: "read-config", Image: "example.invalid/init",
			VolumeMounts: []corev1.VolumeMount{{Name: "config", MountPath: "/read", ReadOnly: true}}})
	checks := CheckAssembly(expected, actual, generated, generated.Files)
	requireAssemblyCheck(t, checks, "assembly.materialization", Consistent)
}

func TestAssemblyChecksFollowFinalMountsWithoutRepair(t *testing.T) {
	t.Run("collector-config-file-shadowed", func(t *testing.T) {
		expected, generated := assemblyCheckFixture(t)
		actual := cloneGroupResources(expected)
		collector := findContainer(actual.StatefulSet.Spec.Template, vectorContainerName)
		collector.VolumeMounts = append(collector.VolumeMounts, corev1.VolumeMount{
			Name: "logs", MountPath: "/etc/vector/vector.yaml", SubPath: "custom.yaml", ReadOnly: true,
		})
		files := slices.DeleteFunc(cloneDeclaredFiles(generated.Files), func(file File) bool {
			return file.Path == vectorConfigFile
		})
		requireAssemblyCheck(t, CheckAssembly(expected, actual, generated, files), "vector.config", Unknown)
	})
	t.Run("collector-config-content-unknown", func(t *testing.T) {
		expected, generated := assemblyCheckFixture(t)
		actual := cloneGroupResources(expected)
		actual.StatefulSet.Spec.Template.Spec.Containers[0].Command = []string{"custom"}
		files := cloneDeclaredFiles(generated.Files)
		findFile(files, generated.ConfigDirectory, vectorConfigFile).Content = Text("arbitrary vector config")
		checks := CheckAssembly(expected, actual, generated, files)
		requireAssemblyCheck(t, checks, "vector.config", Unknown)
		for _, check := range checks {
			if check.State == Conflict {
				t.Fatalf("unknown content/execution must not become invalid: %#v", check)
			}
		}
	})
	cases := []struct {
		name, subject string
		state         CheckState
		change        func(*corev1.PodTemplateSpec)
	}{
		{"collector-different-volume", "vector.source[logs/server.json]", Conflict, func(pod *corev1.PodTemplateSpec) {
			findMount(findContainer(*pod, vectorContainerName), "/logs/logs").Name = "spare"
		}},
		{"producer-unknown-different-volume", "vector.source[logs/server.json]", Unknown, func(pod *corev1.PodTemplateSpec) {
			pod.Spec.Containers[0].Command = []string{"custom"}
			findMount(findContainer(*pod, vectorContainerName), "/logs/logs").Name = "spare"
		}},
		{"collector-moved-path-independent-of-main", "vector.source[logs/server.json]", Conflict,
			func(pod *corev1.PodTemplateSpec) {
				pod.Spec.Containers[0].Command = []string{"custom"}
				findMount(findContainer(*pod, vectorContainerName), "/logs/logs").MountPath = "/elsewhere"
			}},
		{"shared-volume-rebound-coherently", "vector.source[logs/server.json]", Consistent,
			func(pod *corev1.PodTemplateSpec) {
				findMount(findContainer(*pod, vectorContainerName), "/logs/logs").Name = "spare"
				findMount(&pod.Spec.Containers[0], "/var/log/trino").Name = "spare"
			}},
		{"producer-read-only", "vector.source[logs/server.json]", Conflict, func(pod *corev1.PodTemplateSpec) {
			findMount(&pod.Spec.Containers[0], "/var/log/trino").ReadOnly = true
		}},
		{"different-subpath", "vector.source[logs/server.json]", Conflict, func(pod *corev1.PodTemplateSpec) {
			findMount(&pod.Spec.Containers[0], "/var/log/trino").SubPath = "different"
		}},
		{"dynamic-subpath", "vector.source[logs/server.json]", Unknown, func(pod *corev1.PodTemplateSpec) {
			findMount(&pod.Spec.Containers[0], "/var/log/trino").SubPathExpr = "$(SOME_PATH)"
		}},
		{"config-no-longer-prepared", "assembly.files[config]", Conflict, func(pod *corev1.PodTemplateSpec) {
			findMount(&pod.Spec.Containers[0], "/etc/trino").Name = "spare"
		}},
		{"config-file-shadowed", "assembly.files[config]", Unknown, func(pod *corev1.PodTemplateSpec) {
			pod.Spec.Containers[0].VolumeMounts = append(pod.Spec.Containers[0].VolumeMounts,
				corev1.VolumeMount{Name: "spare", MountPath: "/etc/trino/node.properties", SubPath: "node.properties"})
		}},
		{"collector-file-shadowed", "vector.source[logs/server.json]", Unknown, func(pod *corev1.PodTemplateSpec) {
			collector := findContainer(*pod, vectorContainerName)
			collector.VolumeMounts = append(collector.VolumeMounts, corev1.VolumeMount{
				Name: "spare", MountPath: "/logs/logs/server.json", SubPath: "other.log", ReadOnly: true,
			})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expected, generated := assemblyCheckFixture(t)
			actual := cloneGroupResources(expected)
			actual.StatefulSet.Spec.Template.Spec.Volumes = append(actual.StatefulSet.Spec.Template.Spec.Volumes,
				corev1.Volume{Name: "spare", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}})
			tc.change(&actual.StatefulSet.Spec.Template)
			checks := CheckAssembly(expected, actual, generated, generated.Files)
			requireAssemblyCheck(t, checks, tc.subject, tc.state)
			if findPodVolume(actual.StatefulSet.Spec.Template.Spec, "spare") == nil {
				t.Fatal("checks must not repair the final Pod")
			}
		})
	}
	// Mounting one volume twice at different paths remains legal; only a
	// repeated path, missing backing volume or duplicate names are conflicts.
	pod := corev1.PodSpec{Volumes: []corev1.Volume{{Name: "data"}}, Containers: []corev1.Container{{Name: "app",
		VolumeMounts: []corev1.VolumeMount{{Name: "data", MountPath: "/one"}, {Name: "data", MountPath: "/two"}},
	}}}
	if checks := checkPodStructure(pod); len(checks) != 0 {
		t.Fatalf("one volume may be mounted at multiple paths: %#v", checks)
	}
	pod.Containers[0].VolumeMounts[1].MountPath = "/one"
	pod.Containers[0].VolumeMounts[1].Name = "absent"
	pod.InitContainers = []corev1.Container{{Name: "app"}}
	pod.Volumes = append(pod.Volumes, pod.Volumes[0])
	checks := checkPodStructure(pod)
	for _, fragment := range []string{"duplicate volume", "duplicate container", "duplicate mount", "is missing"} {
		if !slices.ContainsFunc(checks, func(check Check) bool { return strings.Contains(check.Reason, fragment) }) {
			t.Fatalf("structure failure %q not found: %#v", fragment, checks)
		}
	}
}
