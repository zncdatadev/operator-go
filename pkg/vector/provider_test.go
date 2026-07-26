/*
Copyright 2024 ZNCDataDev.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package vector

import (
	"context"
	"strings"
	"testing"

	"github.com/zncdatadev/operator-go/pkg/productlogging"
	"github.com/zncdatadev/operator-go/pkg/sidecar"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newTestFakeClient(objects ...client.Object) client.Client {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
}

func TestNewVectorSidecarProvider_Defaults(t *testing.T) {
	p := NewVectorSidecarProvider("test-product:latest")
	if p.Name() != VectorSidecarName {
		t.Errorf("Name() = %q, want %q", p.Name(), VectorSidecarName)
	}
	if p.ConfigMapName() != VectorDefaultConfigMapName {
		t.Errorf("ConfigMapName() = %q, want %q", p.ConfigMapName(), VectorDefaultConfigMapName)
	}
}

// vectorInitContainer returns the injected Vector native sidecar (an init container with
// restartPolicy Always), or nil if it was not injected.
func vectorInitContainer(podSpec *corev1.PodSpec) *corev1.Container {
	idx := sidecar.FindInitContainerIndex(podSpec, VectorSidecarName)
	if idx < 0 {
		return nil
	}
	return &podSpec.InitContainers[idx]
}

func TestNewVectorSidecarProvider_ConstructorImage(t *testing.T) {
	p := NewVectorSidecarProvider("my-product:v2.0")
	if p.image != "my-product:v2.0" {
		t.Errorf("image = %q, want %q", p.image, "my-product:v2.0")
	}
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "main", Image: "main-image"},
		},
	}
	config := &sidecar.SidecarConfig{Enabled: true}
	if err := p.Inject(podSpec, config); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	c := vectorInitContainer(podSpec)
	if c == nil {
		t.Fatal("vector init container not found")
	}
	if c.Image != "my-product:v2.0" {
		t.Errorf("Image = %q, want %q", c.Image, "my-product:v2.0")
	}
}

func TestNewVectorSidecarProvider_WithConfigMapName(t *testing.T) {
	p := NewVectorSidecarProvider("test-product:latest", WithConfigMapName("my-vector-config"))
	if p.ConfigMapName() != "my-vector-config" {
		t.Errorf("ConfigMapName() = %q, want %q", p.ConfigMapName(), "my-vector-config")
	}
}

func TestNewVectorSidecarProvider_WithDataVolumeSize(t *testing.T) {
	qty := resource.MustParse("100Mi")
	p := NewVectorSidecarProvider("test-product:latest", WithDataVolumeSize(qty))
	if p.dataVolumeSize == nil {
		t.Fatal("dataVolumeSize should not be nil")
	}
	if p.dataVolumeSize.String() != "100Mi" {
		t.Errorf("dataVolumeSize = %q, want %q", p.dataVolumeSize.String(), "100Mi")
	}
}

func vectorConfigMap(namespace, name string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Data: map[string]string{VectorConfigFileName: "sources: {}\n"},
	}
}

func TestProvider_Validate_Success(t *testing.T) {
	c := newTestFakeClient(vectorConfigMap("test-ns", "vector-config"))
	p := NewVectorSidecarProvider("test-product:latest")
	if err := p.Validate(context.Background(), c, "test-ns"); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestProvider_Validate_MissingConfigMap(t *testing.T) {
	c := newTestFakeClient()
	p := NewVectorSidecarProvider("test-product:latest")
	if err := p.Validate(context.Background(), c, "test-ns"); err == nil {
		t.Fatal("Validate() expected error for missing ConfigMap, got nil")
	}
}

// A ConfigMap without vector.yaml would start the agent with no configuration, so it must fail
// validation just like a missing ConfigMap.
func TestProvider_Validate_MissingConfigKey(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns", Name: "vector-config"},
		Data:       map[string]string{"other.yaml": "irrelevant"},
	}
	c := newTestFakeClient(cm)
	p := NewVectorSidecarProvider("test-product:latest")
	if err := p.Validate(context.Background(), c, "test-ns"); err == nil {
		t.Fatal("Validate() expected error for ConfigMap without vector.yaml, got nil")
	}
}

func TestProvider_Validate_CustomConfigMap(t *testing.T) {
	c := newTestFakeClient(vectorConfigMap("test-ns", "custom-config"))
	p := NewVectorSidecarProvider("test-product:latest", WithConfigMapName("custom-config"))
	if err := p.Validate(context.Background(), c, "test-ns"); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestProvider_Phase(t *testing.T) {
	if got := NewVectorSidecarProvider("test-product:latest").Phase(); got != sidecar.SidecarPhasePipeline {
		t.Errorf("Phase() = %d, want %d (Vector must inject after its producers)", got, sidecar.SidecarPhasePipeline)
	}
}

// A caller-supplied VolumeMount is useless — and makes the whole workload invalid — unless the
// backing Volume reaches the PodSpec too.
func TestProvider_Inject_CallerVolumes(t *testing.T) {
	p := NewVectorSidecarProvider("test-product:latest")
	podSpec := &corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "main-image"}}}
	cfg := &sidecar.SidecarConfig{
		Enabled:      true,
		Volumes:      []corev1.Volume{{Name: "extra", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}},
		VolumeMounts: []corev1.VolumeMount{{Name: "extra", MountPath: "/extra"}},
	}

	if err := p.Inject(podSpec, cfg); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	var found bool
	for _, v := range podSpec.Volumes {
		if v.Name == "extra" {
			found = true
			break
		}
	}
	if !found {
		t.Error("caller-supplied volume missing from the PodSpec; its VolumeMount would dangle")
	}

	c := vectorInitContainer(podSpec)
	if c == nil {
		t.Fatal("vector init container not injected")
	}
	var mounted bool
	for _, m := range c.VolumeMounts {
		if m.Name == "extra" {
			mounted = true
			break
		}
	}
	if !mounted {
		t.Error("caller-supplied volume mount missing from the vector container")
	}
}

func TestProvider_Inject_ContainerInjection(t *testing.T) {
	p := NewVectorSidecarProvider("test-product:latest")
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "main", Image: "main-image"},
		},
	}
	config := &sidecar.SidecarConfig{Enabled: true}

	if err := p.Inject(podSpec, config); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	// Vector is injected as a native sidecar (init container, restartPolicy Always),
	// never as a regular container.
	if len(podSpec.Containers) != 1 {
		t.Fatalf("expected 1 regular container, got %d", len(podSpec.Containers))
	}
	c := vectorInitContainer(podSpec)
	if c == nil {
		t.Fatal("vector init container not found")
	}
	if c.RestartPolicy == nil || *c.RestartPolicy != corev1.ContainerRestartPolicyAlways {
		t.Error("vector init container should have restartPolicy Always (native sidecar)")
	}
	if sidecar.FindContainer(podSpec, VectorSidecarName) != nil {
		t.Error("vector should never be a regular container")
	}
}

func TestProvider_Inject_DefaultImage(t *testing.T) {
	p := NewVectorSidecarProvider("test-product:latest")
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "main", Image: "main-image"},
		},
	}
	config := &sidecar.SidecarConfig{Enabled: true}

	if err := p.Inject(podSpec, config); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	if c := vectorInitContainer(podSpec); c == nil || c.Image != "test-product:latest" {
		t.Errorf("Image = %v, want %q", c, "test-product:latest")
	}
}

func TestProvider_Inject_CustomImage(t *testing.T) {
	p := NewVectorSidecarProvider("test-product:latest")
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "main", Image: "main-image"},
		},
	}
	config := &sidecar.SidecarConfig{
		Enabled: true,
		Image:   "custom/vector:latest",
	}

	if err := p.Inject(podSpec, config); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	if c := vectorInitContainer(podSpec); c == nil || c.Image != "custom/vector:latest" {
		t.Errorf("Image = %v, want %q", c, "custom/vector:latest")
	}
}

func TestProvider_Inject_EmptyImage_ReturnsError(t *testing.T) {
	// Provider built with an empty product image and no SidecarConfig.Image override: the resolved
	// image is empty, which must fail loudly instead of producing an invalid (empty-image) container.
	p := NewVectorSidecarProvider("")
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "main", Image: "main-image"},
		},
	}
	config := &sidecar.SidecarConfig{Enabled: true}

	err := p.Inject(podSpec, config)
	if err == nil {
		t.Fatalf("Inject() with empty resolved image: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no image configured") {
		t.Errorf("Inject() error = %q, want it to mention %q", err.Error(), "no image configured")
	}
	if c := vectorInitContainer(podSpec); c != nil {
		t.Errorf("expected no Vector container to be injected on error, got %v", c)
	}
}

func TestProvider_Inject_EmptyProductImage_OverrideSucceeds(t *testing.T) {
	// Empty product image but a SidecarConfig.Image override resolves to a non-empty image: happy path.
	p := NewVectorSidecarProvider("")
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "main", Image: "main-image"},
		},
	}
	config := &sidecar.SidecarConfig{
		Enabled: true,
		Image:   "custom/vector:latest",
	}

	if err := p.Inject(podSpec, config); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if c := vectorInitContainer(podSpec); c == nil || c.Image != "custom/vector:latest" {
		t.Errorf("Image = %v, want %q", c, "custom/vector:latest")
	}
}

func TestProvider_Inject_DefaultPullPolicy(t *testing.T) {
	p := NewVectorSidecarProvider("test-product:latest")
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "main", Image: "main-image"},
		},
	}
	config := &sidecar.SidecarConfig{Enabled: true}

	if err := p.Inject(podSpec, config); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	if c := vectorInitContainer(podSpec); c == nil || c.ImagePullPolicy != corev1.PullIfNotPresent {
		t.Errorf("PullPolicy = %v, want %q", c, corev1.PullIfNotPresent)
	}
}

func TestProvider_Inject_CustomPullPolicy(t *testing.T) {
	p := NewVectorSidecarProvider("test-product:latest")
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "main", Image: "main-image"},
		},
	}
	config := &sidecar.SidecarConfig{
		Enabled:         true,
		ImagePullPolicy: corev1.PullAlways,
	}

	if err := p.Inject(podSpec, config); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	if c := vectorInitContainer(podSpec); c == nil || c.ImagePullPolicy != corev1.PullAlways {
		t.Errorf("PullPolicy = %v, want %q", c, corev1.PullAlways)
	}
}

func TestProvider_Inject_Command(t *testing.T) {
	p := NewVectorSidecarProvider("test-product:latest")
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "main", Image: "main-image"},
		},
	}
	config := &sidecar.SidecarConfig{Enabled: true}

	if err := p.Inject(podSpec, config); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	// No declared producers: no directories to pre-create, exec vector directly.
	cmd := vectorInitContainer(podSpec).Command
	expectedCmd := []string{"vector", "--config", VectorConfigMountPath + "/" + VectorConfigFileName}
	if len(cmd) != len(expectedCmd) {
		t.Fatalf("Command length = %d, want %d", len(cmd), len(expectedCmd))
	}
	for i, c := range cmd {
		if c != expectedCmd[i] {
			t.Errorf("Command[%d] = %q, want %q", i, c, expectedCmd[i])
		}
	}
}

// TestProvider_Inject_CommandPreCreatesProducerLogDirs asserts the sidecar (which starts
// before the producers, being a native init container) pre-creates each declared producer's
// per-container log directory (lowercased, matching the stable "<LogDir>/<container>/<file>"
// path convention) before exec'ing vector. log4j 1.x and python's FileHandler do not create
// parent directories.
func TestProvider_Inject_CommandPreCreatesProducerLogDirs(t *testing.T) {
	p := NewVectorSidecarProvider("test-product:latest", WithProducers([]string{"Main", "sidekick"}))
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "Main", Image: "main-image"},
			{Name: "sidekick", Image: "sidekick-image"},
		},
	}
	if err := p.Inject(podSpec, &sidecar.SidecarConfig{Enabled: true}); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	cmd := vectorInitContainer(podSpec).Command
	if len(cmd) != 3 || cmd[0] != "/bin/sh" || cmd[1] != "-c" {
		t.Fatalf("Command = %v, want a /bin/sh -c script", cmd)
	}
	script := cmd[2]
	if !strings.Contains(script, "mkdir -p /kubedoop/log/main /kubedoop/log/sidekick") {
		t.Errorf("script must pre-create lowercased per-producer log dirs, got %q", script)
	}
	// The pre-created directories must be the ones the file appenders are configured with, so
	// they are derived from the same function rather than re-implemented here.
	wantDirs := "mkdir -p " + productlogging.ContainerLogDir("Main") + " " + productlogging.ContainerLogDir("sidekick")
	if !strings.Contains(script, wantDirs) {
		t.Errorf("script must pre-create productlogging.ContainerLogDir paths %q, got %q", wantDirs, script)
	}
	if !strings.Contains(script, "exec vector --config "+VectorConfigMountPath+"/"+VectorConfigFileName) {
		t.Errorf("script must exec vector with the mounted config, got %q", script)
	}
}

func TestProvider_Inject_VolumeMounts(t *testing.T) {
	p := NewVectorSidecarProvider("test-product:latest")
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "main", Image: "main-image"},
		},
	}
	config := &sidecar.SidecarConfig{Enabled: true}

	if err := p.Inject(podSpec, config); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	volumeMounts := vectorInitContainer(podSpec).VolumeMounts
	if len(volumeMounts) != 3 {
		t.Fatalf("expected 3 volume mounts, got %d", len(volumeMounts))
	}

	mountNames := make(map[string]bool)
	for _, m := range volumeMounts {
		mountNames[m.Name] = true
	}
	for _, name := range []string{VectorConfigVolumeName, VectorDataVolumeName, VectorLogVolumeName} {
		if !mountNames[name] {
			t.Errorf("missing volume mount %q", name)
		}
	}

	// The config mount is read-only; the shared log mount must be read-write because the
	// sidecar pre-creates the producers' per-container log directories before exec'ing vector.
	for _, m := range volumeMounts {
		if m.Name == VectorConfigVolumeName && !m.ReadOnly {
			t.Error("config volume mount should be read-only")
		}
		if m.Name == VectorLogVolumeName {
			if m.ReadOnly {
				t.Error("log volume mount must be read-write (the sidecar pre-creates log dirs)")
			}
			if m.MountPath != VectorLogMountPath {
				t.Errorf("log mount path = %q, want %q", m.MountPath, VectorLogMountPath)
			}
		}
	}
}

func TestProvider_Inject_Volumes(t *testing.T) {
	p := NewVectorSidecarProvider("test-product:latest")
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "main", Image: "main-image"},
		},
	}
	config := &sidecar.SidecarConfig{Enabled: true}

	if err := p.Inject(podSpec, config); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	// The provider is the single owner of the shared log pipeline: it creates its own config +
	// data volumes AND the shared log volume.
	if len(podSpec.Volumes) != 3 {
		t.Fatalf("expected 3 volumes, got %d", len(podSpec.Volumes))
	}

	volNames := make(map[string]bool)
	for _, v := range podSpec.Volumes {
		volNames[v.Name] = true
	}
	for _, name := range []string{VectorConfigVolumeName, VectorDataVolumeName, VectorLogVolumeName} {
		if !volNames[name] {
			t.Errorf("missing volume %q", name)
		}
	}
	// The shared log volume must be a bounded node-disk emptyDir.
	for _, v := range podSpec.Volumes {
		if v.Name == VectorLogVolumeName {
			if v.EmptyDir == nil {
				t.Fatalf("log volume %q must be an emptyDir", VectorLogVolumeName)
			}
			if v.EmptyDir.SizeLimit == nil {
				t.Errorf("log volume %q must have a SizeLimit", VectorLogVolumeName)
			}
		}
	}
}

// TestProvider_Inject_LogVolumeSizeOverride asserts WithLogVolumeSize sets the shared log
// volume's SizeLimit.
func TestProvider_Inject_LogVolumeSizeOverride(t *testing.T) {
	p := NewVectorSidecarProvider("test-product:latest", WithLogVolumeSize(resource.MustParse("128Mi")))
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main", Image: "main-image"}},
	}
	if err := p.Inject(podSpec, &sidecar.SidecarConfig{Enabled: true}); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	var found bool
	for _, v := range podSpec.Volumes {
		if v.Name == VectorLogVolumeName {
			found = true
			if v.EmptyDir == nil || v.EmptyDir.SizeLimit == nil {
				t.Fatalf("log volume must be a sized emptyDir")
			}
			if got := v.EmptyDir.SizeLimit.String(); got != "128Mi" {
				t.Errorf("log volume SizeLimit = %q, want %q", got, "128Mi")
			}
		}
	}
	if !found {
		t.Error("shared log volume not created")
	}
}

func TestProvider_Inject_ConfigMapVolume(t *testing.T) {
	p := NewVectorSidecarProvider("test-product:latest", WithConfigMapName("custom-vector-config"))
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "main", Image: "main-image"},
		},
	}
	config := &sidecar.SidecarConfig{Enabled: true}

	if err := p.Inject(podSpec, config); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	var configVolume *corev1.Volume
	for i, v := range podSpec.Volumes {
		if v.Name == VectorConfigVolumeName {
			configVolume = &podSpec.Volumes[i]
			break
		}
	}
	if configVolume == nil {
		t.Fatal("config volume not found")
		return
	}
	if configVolume.ConfigMap == nil {
		t.Fatal("config volume should have ConfigMap source")
	}
	if configVolume.ConfigMap.Name != "custom-vector-config" {
		t.Errorf("ConfigMap name = %q, want %q", configVolume.ConfigMap.Name, "custom-vector-config")
	}
}

// TestProvider_Inject_NoProducers_NoProducerMount asserts that with no configured producers the
// provider does not RW-mount the shared log volume on any product container (it still creates the
// volume and mounts it on the Vector container).
func TestProvider_Inject_NoProducers_NoProducerMount(t *testing.T) {
	p := NewVectorSidecarProvider("test-product:latest")
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "main", Image: "main-image"},
		},
	}
	config := &sidecar.SidecarConfig{Enabled: true}

	if err := p.Inject(podSpec, config); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	for _, m := range podSpec.Containers[0].VolumeMounts {
		if m.Name == VectorLogVolumeName {
			t.Error("provider must not mount the shared log volume on a container that is not a configured producer")
		}
	}
}

// TestProvider_Inject_ProducerGetsRWLogMount asserts the provider RW-mounts the shared log volume
// on each configured producer container at the canonical log dir.
func TestProvider_Inject_ProducerGetsRWLogMount(t *testing.T) {
	p := NewVectorSidecarProvider("test-product:latest", WithProducers([]string{"main"}))
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "main", Image: "main-image"},
		},
	}
	config := &sidecar.SidecarConfig{Enabled: true}

	if err := p.Inject(podSpec, config); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	var found bool
	for _, m := range podSpec.Containers[0].VolumeMounts {
		if m.Name == VectorLogVolumeName {
			found = true
			if m.ReadOnly {
				t.Error("producer log mount must be read-write (not read-only)")
			}
			if m.MountPath != VectorLogMountPath {
				t.Errorf("producer log mount path = %q, want %q", m.MountPath, VectorLogMountPath)
			}
		}
	}
	if !found {
		t.Error("producer container must have the shared log volume RW-mounted")
	}
}

// TestProvider_Inject_LogMountOnVectorContainer asserts the consumer side: the shared log
// volume is mounted read-write on the Vector container at the framework-canonical log dir
// (read-write because the sidecar pre-creates the producers' log dirs before exec'ing vector).
func TestProvider_Inject_LogMountOnVectorContainer(t *testing.T) {
	p := NewVectorSidecarProvider("test-product:latest")
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "main", Image: "main-image"},
		},
	}
	config := &sidecar.SidecarConfig{Enabled: true}

	if err := p.Inject(podSpec, config); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	c := vectorInitContainer(podSpec)
	if c == nil {
		t.Fatal("vector init container not found")
	}
	var found bool
	for _, m := range c.VolumeMounts {
		if m.Name == VectorLogVolumeName {
			found = true
			if m.ReadOnly {
				t.Error("vector log mount must be read-write (the sidecar pre-creates log dirs)")
			}
			if m.MountPath != VectorLogMountPath {
				t.Errorf("log mount path = %q, want %q", m.MountPath, VectorLogMountPath)
			}
		}
	}
	if !found {
		t.Error("vector container should mount the shared log volume")
	}
}

// TestProvider_Inject_LivenessNotReadinessProbe guards two availability properties at once, and
// the distinction between them is the whole point.
//
// No readinessProbe: Kubernetes documents that for a sidecar container (an init container with
// restartPolicy Always) "if a readinessProbe is specified for this init container, its result will
// be used to determine the ready state of the Pod", so one here would let a crash-looping or
// slow-starting Vector pull every pod of the role group out of every Service — a product outage
// caused by the log pipeline.
//
// A livenessProbe, though, restarts only this container and never touches Service membership, so
// it delivers the guarantee readiness cannot: a wedged agent is recovered instead of merely being
// visible. Deleting the probe outright, as the previous iteration did, left the agent with neither.
//
// It must target the metrics endpoint, not the API's /health: the kubelet executes httpGet probes
// against the POD IP from outside the pod's network namespace, and the API binds 127.0.0.1 so that
// `vector tap` — which streams application logs — is not reachable from the pod network.
func TestProvider_Inject_LivenessNotReadinessProbe(t *testing.T) {
	p := NewVectorSidecarProvider("test-product:latest")
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "main", Image: "main-image"},
		},
	}
	config := &sidecar.SidecarConfig{Enabled: true}

	if err := p.Inject(podSpec, config); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	container := vectorInitContainer(podSpec)
	if container.ReadinessProbe != nil {
		t.Errorf("readiness probe = %+v, want nil: a log shipper must not gate pod readiness",
			container.ReadinessProbe)
	}
	if container.StartupProbe != nil {
		t.Errorf("startup probe = %+v, want nil: nothing waits on the log agent", container.StartupProbe)
	}

	probe := container.LivenessProbe
	if probe == nil {
		t.Fatal("liveness probe = nil, want one: a wedged agent would never be recovered")
	}
	if probe.HTTPGet == nil {
		t.Fatalf("liveness probe = %+v, want an httpGet handler", probe)
	}
	if got := probe.HTTPGet.Port.IntValue(); got != VectorMetricsPort {
		t.Errorf("liveness probe port = %d, want %d (the metrics endpoint; the API is on loopback and unreachable to the kubelet)",
			got, VectorMetricsPort)
	}
	// The literal, not VectorMetricsPath: Vector hardcodes the prometheus_exporter path, so a
	// constant pointing elsewhere is itself the bug, and an assertion against it would follow.
	if probe.HTTPGet.Path != "/metrics" {
		t.Errorf("liveness probe path = %q, want %q", probe.HTTPGet.Path, "/metrics")
	}
	// Restarting the agent drops its in-memory buffer, so the probe must tolerate a busy agent
	// and fire only on a sustained failure. Anything under a minute of tolerance is too eager.
	if tolerance := probe.PeriodSeconds * probe.FailureThreshold; tolerance < 60 {
		t.Errorf("liveness tolerance = %ds (period %d x threshold %d), want >= 60s: restarting Vector drops buffered logs",
			tolerance, probe.PeriodSeconds, probe.FailureThreshold)
	}

	// The endpoint the probe hits must be declared, otherwise nothing but this test knows it exists.
	var found bool
	for _, port := range container.Ports {
		if port.ContainerPort == VectorMetricsPort {
			found = true
			if port.Name != VectorMetricsPortName {
				t.Errorf("metrics port name = %q, want %q", port.Name, VectorMetricsPortName)
			}
		}
	}
	if !found {
		t.Errorf("container ports = %+v, want one declaring %d", container.Ports, VectorMetricsPort)
	}
}

// TestProvider_Inject_ProbeOverrides proves the framework policy is a default rather than a law:
// before this, SidecarConfig could not express a probe at all, so a product that needed one had no
// option but raw podOverrides.
func TestProvider_Inject_ProbeOverrides(t *testing.T) {
	custom := &corev1.Probe{
		ProbeHandler:  corev1.ProbeHandler{Exec: &corev1.ExecAction{Command: []string{"true"}}},
		PeriodSeconds: 7,
	}

	t.Run("replace", func(t *testing.T) {
		podSpec := &corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "i"}}}
		config := &sidecar.SidecarConfig{Enabled: true}
		config.Probes.Liveness = custom
		if err := NewVectorSidecarProvider("p:latest").Inject(podSpec, config); err != nil {
			t.Fatalf("Inject() error = %v", err)
		}
		probe := vectorInitContainer(podSpec).LivenessProbe
		if probe == nil || probe.Exec == nil {
			t.Fatalf("liveness probe = %+v, want the override's exec handler", probe)
		}
		if probe.HTTPGet != nil {
			t.Errorf("liveness probe = %+v: the override must replace wholesale, not merge — a probe carrying two handlers is rejected by the API server", probe)
		}
		if probe == custom {
			t.Error("liveness probe aliases the caller's SidecarConfig; it must be deep-copied")
		}
	})

	t.Run("disable", func(t *testing.T) {
		podSpec := &corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "i"}}}
		config := &sidecar.SidecarConfig{Enabled: true}
		config.Probes.DisableLiveness = true
		if err := NewVectorSidecarProvider("p:latest").Inject(podSpec, config); err != nil {
			t.Fatalf("Inject() error = %v", err)
		}
		if probe := vectorInitContainer(podSpec).LivenessProbe; probe != nil {
			t.Errorf("liveness probe = %+v, want nil once disabled", probe)
		}
	})

	t.Run("readiness is opt-in", func(t *testing.T) {
		podSpec := &corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "i"}}}
		config := &sidecar.SidecarConfig{Enabled: true}
		config.Probes.Readiness = custom
		if err := NewVectorSidecarProvider("p:latest").Inject(podSpec, config); err != nil {
			t.Fatalf("Inject() error = %v", err)
		}
		if probe := vectorInitContainer(podSpec).ReadinessProbe; probe == nil {
			t.Error("readiness probe = nil: a product must be able to gate its pod on a sidecar when it really is in the request path")
		}
	})
}

func TestProvider_Inject_Idempotency(t *testing.T) {
	p := NewVectorSidecarProvider("test-product:latest")
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "main", Image: "main-image"},
		},
	}
	config := &sidecar.SidecarConfig{Enabled: true}

	if err := p.Inject(podSpec, config); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if len(podSpec.InitContainers) != 1 {
		t.Fatalf("expected 1 init container after first inject, got %d", len(podSpec.InitContainers))
	}

	// Inject again
	if err := p.Inject(podSpec, config); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	// Should still have 1 main container and 1 vector init container, not duplicated.
	if len(podSpec.Containers) != 1 {
		t.Errorf("expected 1 regular container after second inject, got %d", len(podSpec.Containers))
	}

	vectorCount := 0
	for _, c := range podSpec.InitContainers {
		if c.Name == VectorSidecarName {
			vectorCount++
		}
	}
	if vectorCount != 1 {
		t.Errorf("expected 1 vector init container, got %d", vectorCount)
	}
}

func TestProvider_Inject_NilConfig(t *testing.T) {
	p := NewVectorSidecarProvider("test-product:latest")
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "main", Image: "main-image"},
		},
	}

	if err := p.Inject(podSpec, nil); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if len(podSpec.Containers) != 1 {
		t.Errorf("expected 1 regular container, got %d", len(podSpec.Containers))
	}
	if vectorInitContainer(podSpec) == nil {
		t.Error("expected vector init container to be injected with nil config")
	}
}

func TestProvider_Inject_Resources(t *testing.T) {
	p := NewVectorSidecarProvider("test-product:latest")
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "main", Image: "main-image"},
		},
	}
	resources := corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("200m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
	}
	config := &sidecar.SidecarConfig{
		Enabled:   true,
		Resources: &resources,
	}

	if err := p.Inject(podSpec, config); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	if _, ok := vectorInitContainer(podSpec).Resources.Limits[corev1.ResourceCPU]; !ok {
		t.Error("expected CPU resource limit")
	}
}

func TestProvider_Inject_EnvVars(t *testing.T) {
	p := NewVectorSidecarProvider("test-product:latest")
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "main", Image: "main-image"},
		},
	}
	config := &sidecar.SidecarConfig{
		Enabled: true,
		EnvVars: map[string]string{
			"VECTOR_LOG": "debug",
		},
	}

	if err := p.Inject(podSpec, config); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	if len(vectorInitContainer(podSpec).Env) == 0 {
		t.Error("expected env vars to be set")
	}
}

func TestProvider_Inject_SecurityContext(t *testing.T) {
	p := NewVectorSidecarProvider("test-product:latest")
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "main", Image: "main-image"},
		},
	}
	runAsNonRoot := true
	securityContext := &corev1.SecurityContext{
		RunAsNonRoot: &runAsNonRoot,
	}
	config := &sidecar.SidecarConfig{
		Enabled:         true,
		SecurityContext: securityContext,
	}

	if err := p.Inject(podSpec, config); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	c := vectorInitContainer(podSpec)
	if c.SecurityContext == nil {
		t.Fatal("expected security context to be set")
	}
	if !*c.SecurityContext.RunAsNonRoot {
		t.Error("expected RunAsNonRoot to be true")
	}
}

func TestProvider_Inject_CustomVolumeMounts(t *testing.T) {
	p := NewVectorSidecarProvider("test-product:latest")
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "main", Image: "main-image"},
		},
	}
	customMounts := []corev1.VolumeMount{
		{Name: "custom-data", MountPath: "/custom"},
	}
	config := &sidecar.SidecarConfig{
		Enabled:      true,
		VolumeMounts: customMounts,
	}

	if err := p.Inject(podSpec, config); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	var found bool
	for _, m := range vectorInitContainer(podSpec).VolumeMounts {
		if m.Name == "custom-data" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected custom volume mount to be present")
	}
}

func TestProvider_Inject_CustomDataVolumeSize(t *testing.T) {
	qty := resource.MustParse("100Mi")
	p := NewVectorSidecarProvider("test-product:latest", WithDataVolumeSize(qty))
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "main", Image: "main-image"},
		},
	}
	config := &sidecar.SidecarConfig{Enabled: true}

	if err := p.Inject(podSpec, config); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	for _, v := range podSpec.Volumes {
		if v.Name == VectorDataVolumeName {
			if v.EmptyDir == nil || v.EmptyDir.SizeLimit == nil {
				t.Fatal("data volume should have SizeLimit set")
			}
			if v.EmptyDir.SizeLimit.String() != "100Mi" {
				t.Errorf("data volume SizeLimit = %q, want %q", v.EmptyDir.SizeLimit.String(), "100Mi")
			}
			return
		}
	}
	t.Fatal("data volume not found")
}

func TestProvider_Inject_DefaultSecurityContext(t *testing.T) {
	p := NewVectorSidecarProvider("test-product:latest")
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "main", Image: "main-image"},
		},
	}
	config := &sidecar.SidecarConfig{Enabled: true}

	if err := p.Inject(podSpec, config); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	vectorContainer := vectorInitContainer(podSpec)
	if vectorContainer == nil {
		t.Fatal("vector container not found")
		return
	}

	sc := vectorContainer.SecurityContext
	if sc == nil {
		t.Fatal("SecurityContext should not be nil")
		return
	}
	if sc.RunAsNonRoot == nil || !*sc.RunAsNonRoot {
		t.Error("RunAsNonRoot should be true")
	}
	if sc.ReadOnlyRootFilesystem == nil || !*sc.ReadOnlyRootFilesystem {
		t.Error("ReadOnlyRootFilesystem should be true")
	}
	if sc.AllowPrivilegeEscalation == nil || *sc.AllowPrivilegeEscalation {
		t.Error("AllowPrivilegeEscalation should be false")
	}
}
