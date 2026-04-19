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
	if podSpec.Containers[1].Image != "my-product:v2.0" {
		t.Errorf("Image = %q, want %q", podSpec.Containers[1].Image, "my-product:v2.0")
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

	if len(podSpec.Containers) != 2 {
		t.Fatalf("expected 2 containers, got %d", len(podSpec.Containers))
	}
	if podSpec.Containers[1].Name != VectorSidecarName {
		t.Errorf("container name = %q, want %q", podSpec.Containers[1].Name, VectorSidecarName)
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

	if podSpec.Containers[1].Image != "test-product:latest" {
		t.Errorf("Image = %q, want %q", podSpec.Containers[1].Image, "test-product:latest")
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

	if podSpec.Containers[1].Image != "custom/vector:latest" {
		t.Errorf("Image = %q, want %q", podSpec.Containers[1].Image, "custom/vector:latest")
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

	if podSpec.Containers[1].ImagePullPolicy != corev1.PullIfNotPresent {
		t.Errorf("PullPolicy = %q, want %q", podSpec.Containers[1].ImagePullPolicy, corev1.PullIfNotPresent)
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

	if podSpec.Containers[1].ImagePullPolicy != corev1.PullAlways {
		t.Errorf("PullPolicy = %q, want %q", podSpec.Containers[1].ImagePullPolicy, corev1.PullAlways)
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

	cmd := podSpec.Containers[1].Command
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

	volumeMounts := podSpec.Containers[1].VolumeMounts
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

	// Config mount should be read-only
	for _, m := range volumeMounts {
		if m.Name == VectorConfigVolumeName {
			if !m.ReadOnly {
				t.Error("config volume mount should be read-only")
			}
		}
		if m.Name == VectorLogVolumeName {
			if !m.ReadOnly {
				t.Error("log volume mount should be read-only")
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

func TestProvider_Inject_LogVolumeOnMainContainer(t *testing.T) {
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

	mainContainer := podSpec.Containers[0]
	var foundLogMount bool
	for _, m := range mainContainer.VolumeMounts {
		if m.Name == VectorLogVolumeName {
			foundLogMount = true
			if m.MountPath != VectorLogMountPath {
				t.Errorf("log mount path = %q, want %q", m.MountPath, VectorLogMountPath)
			}
			break
		}
	}
	if !foundLogMount {
		t.Error("main container should have log volume mount")
	}
}

func TestProvider_Inject_LogVolumeOnNamedMainContainer(t *testing.T) {
	p := NewVectorSidecarProvider("test-product:latest")
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "other", Image: "other-image"},
			{Name: "app", Image: "app-image"},
		},
	}
	config := &sidecar.SidecarConfig{
		Enabled:           true,
		MainContainerName: "app",
	}

	if err := p.Inject(podSpec, config); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	// The "app" container (index 1) should have the log mount
	appContainer := podSpec.Containers[1]
	var foundLogMount bool
	for _, m := range appContainer.VolumeMounts {
		if m.Name == VectorLogVolumeName {
			foundLogMount = true
			break
		}
	}
	if !foundLogMount {
		t.Error("app container should have log volume mount")
	}

	// The "other" container (index 0) should NOT have the log mount
	otherContainer := podSpec.Containers[0]
	for _, m := range otherContainer.VolumeMounts {
		if m.Name == VectorLogVolumeName {
			t.Error("other container should not have log volume mount")
		}
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

	probe := podSpec.Containers[1].ReadinessProbe
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
	if len(podSpec.Containers) != 2 {
		t.Fatalf("expected 2 containers after first inject, got %d", len(podSpec.Containers))
	}

	// Inject again
	if err := p.Inject(podSpec, config); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	// Should still have 2 containers (main + vector), not 3
	if len(podSpec.Containers) != 2 {
		t.Errorf("expected 2 containers after second inject, got %d", len(podSpec.Containers))
	}

	// Count vector containers
	vectorCount := 0
	for _, c := range podSpec.Containers {
		if c.Name == VectorSidecarName {
			vectorCount++
		}
	}
	if vectorCount != 1 {
		t.Errorf("expected 1 vector container, got %d", vectorCount)
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
	if len(podSpec.Containers) != 2 {
		t.Errorf("expected 2 containers, got %d", len(podSpec.Containers))
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

	if _, ok := podSpec.Containers[1].Resources.Limits[corev1.ResourceCPU]; !ok {
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

	if len(podSpec.Containers[1].Env) == 0 {
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

	if podSpec.Containers[1].SecurityContext == nil {
		t.Fatal("expected security context to be set")
	}
	if !*podSpec.Containers[1].SecurityContext.RunAsNonRoot {
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
	for _, m := range podSpec.Containers[1].VolumeMounts {
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

	var vectorContainer *corev1.Container
	for i := range podSpec.Containers {
		if podSpec.Containers[i].Name == VectorSidecarName {
			vectorContainer = &podSpec.Containers[i]
			break
		}
	}
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
