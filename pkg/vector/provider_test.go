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

func TestProvider_Validate_Success(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name:      "vector-config",
		},
	}
	c := newTestFakeClient(cm)
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

func TestProvider_Validate_CustomConfigMap(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name:      "custom-config",
		},
	}
	c := newTestFakeClient(cm)
	p := NewVectorSidecarProvider("test-product:latest", WithConfigMapName("custom-config"))
	if err := p.Validate(context.Background(), c, "test-ns"); err != nil {
		t.Fatalf("Validate() error = %v", err)
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

	// Config and shared-log mounts must be read-only on the Vector (consumer) container.
	for _, m := range volumeMounts {
		if m.Name == VectorConfigVolumeName && !m.ReadOnly {
			t.Error("config volume mount should be read-only")
		}
		if m.Name == VectorLogVolumeName {
			if !m.ReadOnly {
				t.Error("log volume mount should be read-only")
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

	// The provider creates only its own volumes (config + data). The shared log volume is
	// owned by the producer (role-group base handler), so the consumer must NOT create it.
	if len(podSpec.Volumes) != 2 {
		t.Fatalf("expected 2 volumes, got %d", len(podSpec.Volumes))
	}

	volNames := make(map[string]bool)
	for _, v := range podSpec.Volumes {
		volNames[v.Name] = true
	}
	for _, name := range []string{VectorConfigVolumeName, VectorDataVolumeName} {
		if !volNames[name] {
			t.Errorf("missing volume %q", name)
		}
	}
	if volNames[VectorLogVolumeName] {
		t.Errorf("provider must NOT create the shared log volume %q (producer owns it)", VectorLogVolumeName)
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

// TestProvider_Inject_DoesNotMountLogOnMainContainer asserts the consumer/producer split:
// the Vector provider is a pure consumer and must NOT mount the shared log volume on the
// main/product container. That RW mount is owned by the producer (the role-group base
// handler), preventing the previous double-mount hazard.
func TestProvider_Inject_DoesNotMountLogOnMainContainer(t *testing.T) {
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
			t.Error("provider must NOT mount the shared log volume on the main container (producer owns it)")
		}
	}
}

// TestProvider_Inject_LogMountOnVectorContainer asserts the consumer side: the shared log
// volume is RO-mounted on the Vector container at the framework-canonical log dir.
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
			if !m.ReadOnly {
				t.Error("vector log mount should be read-only")
			}
			if m.MountPath != VectorLogMountPath {
				t.Errorf("log mount path = %q, want %q", m.MountPath, VectorLogMountPath)
			}
		}
	}
	if !found {
		t.Error("vector container should RO-mount the shared log volume")
	}
}

func TestProvider_Inject_ReadinessProbe(t *testing.T) {
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

	probe := vectorInitContainer(podSpec).ReadinessProbe
	if probe == nil {
		t.Fatal("readiness probe should not be nil")
		return
	}
	if probe.HTTPGet == nil {
		t.Fatal("readiness probe HTTPGet should not be nil")
	}
	if probe.HTTPGet.Path != VectorHealthEndpoint {
		t.Errorf("probe path = %q, want %q", probe.HTTPGet.Path, VectorHealthEndpoint)
	}
	if probe.InitialDelaySeconds != VectorReadinessInitialDelaySeconds {
		t.Errorf("initial delay = %d, want %d", probe.InitialDelaySeconds, VectorReadinessInitialDelaySeconds)
	}
	if probe.PeriodSeconds != VectorReadinessPeriodSeconds {
		t.Errorf("period = %d, want %d", probe.PeriodSeconds, VectorReadinessPeriodSeconds)
	}
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
