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

package sidecar_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/sidecar"
	"github.com/zncdatadev/operator-go/pkg/testutil"
	"github.com/zncdatadev/operator-go/pkg/vector"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("SidecarManager", func() {
	var manager *sidecar.SidecarManager

	BeforeEach(func() {
		manager = sidecar.NewSidecarManager()
	})

	Describe("NewSidecarManager", func() {
		It("should create a new manager", func() {
			Expect(manager).NotTo(BeNil())
		})
	})

	Describe("Register", func() {
		It("should register a sidecar provider", func() {
			provider := &mockSidecarProvider{name: "test-sidecar"}
			config := &sidecar.SidecarConfig{Enabled: true}
			manager.Register(provider, config)

			Expect(manager.HasSidecars()).To(BeTrue())
		})
	})

	Describe("Unregister", func() {
		It("should unregister a sidecar provider", func() {
			provider := &mockSidecarProvider{name: "test-sidecar"}
			config := &sidecar.SidecarConfig{Enabled: true}
			manager.Register(provider, config)
			manager.Unregister("test-sidecar")

			Expect(manager.HasSidecars()).To(BeFalse())
		})
	})

	Describe("GetProvider", func() {
		It("should return a registered provider", func() {
			provider := &mockSidecarProvider{name: "test-sidecar"}
			config := &sidecar.SidecarConfig{Enabled: true}
			manager.Register(provider, config)

			p, exists := manager.GetProvider("test-sidecar")
			Expect(exists).To(BeTrue())
			Expect(p).NotTo(BeNil())
		})

		It("should return false for non-existent provider", func() {
			_, exists := manager.GetProvider("non-existent")
			Expect(exists).To(BeFalse())
		})
	})

	Describe("GetConfig", func() {
		It("should return a sidecar config", func() {
			provider := &mockSidecarProvider{name: "test-sidecar"}
			config := &sidecar.SidecarConfig{Enabled: true}
			manager.Register(provider, config)

			c, exists := manager.GetConfig("test-sidecar")
			Expect(exists).To(BeTrue())
			Expect(c).NotTo(BeNil())
			Expect(c.Enabled).To(BeTrue())
		})
	})

	Describe("ListProviders", func() {
		It("should list all registered providers", func() {
			provider1 := &mockSidecarProvider{name: "sidecar-1"}
			provider2 := &mockSidecarProvider{name: "sidecar-2"}
			manager.Register(provider1, &sidecar.SidecarConfig{Enabled: true})
			manager.Register(provider2, &sidecar.SidecarConfig{Enabled: true})

			names := manager.ListProviders()
			Expect(names).To(HaveLen(2))
			Expect(names).To(ContainElements("sidecar-1", "sidecar-2"))
		})
	})

	Describe("InjectAll", func() {
		It("should inject all enabled sidecars", func() {
			provider := &mockSidecarProvider{name: "test-sidecar"}
			config := &sidecar.SidecarConfig{Enabled: true}
			manager.Register(provider, config)

			podSpec := &corev1.PodSpec{}
			err := manager.InjectAll(podSpec)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should skip disabled sidecars", func() {
			provider := &mockSidecarProvider{name: "test-sidecar"}
			config := &sidecar.SidecarConfig{Enabled: false}
			manager.Register(provider, config)

			podSpec := &corev1.PodSpec{}
			err := manager.InjectAll(podSpec)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error when provider inject fails", func() {
			injectErr := fmt.Errorf("inject error")
			provider := &mockSidecarProvider{name: "test-sidecar", injectErr: injectErr}
			config := &sidecar.SidecarConfig{Enabled: true}
			manager.Register(provider, config)

			podSpec := &corev1.PodSpec{}
			err := manager.InjectAll(podSpec)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to inject sidecar"))
		})

		It("should inject sidecars in sorted order", func() {
			provider1 := &mockSidecarProvider{name: "z-sidecar"}
			provider2 := &mockSidecarProvider{name: "a-sidecar"}
			provider3 := &mockSidecarProvider{name: "m-sidecar"}
			manager.Register(provider1, &sidecar.SidecarConfig{Enabled: true})
			manager.Register(provider2, &sidecar.SidecarConfig{Enabled: true})
			manager.Register(provider3, &sidecar.SidecarConfig{Enabled: true})

			podSpec := &corev1.PodSpec{}
			err := manager.InjectAll(podSpec)
			Expect(err).NotTo(HaveOccurred())

			Expect(podSpec.Containers).To(HaveLen(3))
			Expect(podSpec.Containers[0].Name).To(Equal("a-sidecar"))
			Expect(podSpec.Containers[1].Name).To(Equal("m-sidecar"))
			Expect(podSpec.Containers[2].Name).To(Equal("z-sidecar"))
		})
	})

	Describe("Inject", func() {
		It("should inject a specific sidecar", func() {
			provider := &mockSidecarProvider{name: "test-sidecar"}
			config := &sidecar.SidecarConfig{Enabled: true}
			manager.Register(provider, config)

			podSpec := &corev1.PodSpec{}
			err := manager.Inject(podSpec, "test-sidecar")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error for non-existent provider", func() {
			podSpec := &corev1.PodSpec{}
			err := manager.Inject(podSpec, "non-existent")
			Expect(err).To(HaveOccurred())
		})

		It("should skip injection for disabled sidecar", func() {
			provider := &mockSidecarProvider{name: "test-sidecar"}
			config := &sidecar.SidecarConfig{Enabled: false}
			manager.Register(provider, config)

			podSpec := &corev1.PodSpec{}
			err := manager.Inject(podSpec, "test-sidecar")
			Expect(err).NotTo(HaveOccurred())
			Expect(podSpec.Containers).To(BeEmpty())
		})
	})

	Describe("HasSidecars", func() {
		It("should return false when no sidecars registered", func() {
			Expect(manager.HasSidecars()).To(BeFalse())
		})

		It("should return true when sidecars registered", func() {
			provider := &mockSidecarProvider{name: "test-sidecar"}
			manager.Register(provider, &sidecar.SidecarConfig{Enabled: true})
			Expect(manager.HasSidecars()).To(BeTrue())
		})
	})

	Describe("Count", func() {
		It("should return 0 when no sidecars registered", func() {
			Expect(manager.Count()).To(Equal(0))
		})

		It("should return correct count", func() {
			provider := &mockSidecarProvider{name: "test-sidecar"}
			manager.Register(provider, &sidecar.SidecarConfig{Enabled: true})
			Expect(manager.Count()).To(Equal(1))
		})
	})

	Describe("SetProductImage", func() {
		It("should set image on all enabled configs", func() {
			provider1 := &mockSidecarProvider{name: "sidecar-1"}
			provider2 := &mockSidecarProvider{name: "sidecar-2"}
			config1 := &sidecar.SidecarConfig{Enabled: true}
			config2 := &sidecar.SidecarConfig{Enabled: true}
			manager.Register(provider1, config1)
			manager.Register(provider2, config2)

			err := manager.SetProductImage("product:latest", corev1.PullAlways)
			Expect(err).NotTo(HaveOccurred())

			Expect(config1.Image).To(Equal("product:latest"))
			Expect(config1.ImagePullPolicy).To(Equal(corev1.PullAlways))
			Expect(config2.Image).To(Equal("product:latest"))
			Expect(config2.ImagePullPolicy).To(Equal(corev1.PullAlways))
		})

		It("should not overwrite existing image", func() {
			provider1 := &mockSidecarProvider{name: "sidecar-1"}
			provider2 := &mockSidecarProvider{name: "sidecar-2"}
			config1 := &sidecar.SidecarConfig{Enabled: true, Image: "custom-image:1.0"}
			config2 := &sidecar.SidecarConfig{Enabled: true}
			manager.Register(provider1, config1)
			manager.Register(provider2, config2)

			err := manager.SetProductImage("product:latest", corev1.PullIfNotPresent)
			Expect(err).NotTo(HaveOccurred())

			// config1 should keep its custom image
			Expect(config1.Image).To(Equal("custom-image:1.0"))
			// config2 should get the product image
			Expect(config2.Image).To(Equal("product:latest"))
		})

		It("should not overwrite existing ImagePullPolicy", func() {
			provider := &mockSidecarProvider{name: "sidecar-1"}
			config := &sidecar.SidecarConfig{
				Enabled:         true,
				ImagePullPolicy: corev1.PullNever,
			}
			manager.Register(provider, config)

			err := manager.SetProductImage("product:latest", corev1.PullAlways)
			Expect(err).NotTo(HaveOccurred())

			Expect(config.ImagePullPolicy).To(Equal(corev1.PullNever))
		})

		It("should skip disabled configs", func() {
			provider := &mockSidecarProvider{name: "sidecar-1"}
			config := &sidecar.SidecarConfig{Enabled: false}
			manager.Register(provider, config)

			err := manager.SetProductImage("product:latest", corev1.PullAlways)
			Expect(err).NotTo(HaveOccurred())

			// Disabled config should not be modified
			Expect(config.Image).To(BeEmpty())
			Expect(config.ImagePullPolicy).To(BeEmpty())
		})

		It("should return error when image is empty", func() {
			err := manager.SetProductImage("", corev1.PullIfNotPresent)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("product image must not be empty"))
		})

		It("should handle empty manager", func() {
			err := manager.SetProductImage("product:latest", corev1.PullIfNotPresent)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("WithClient", func() {
		It("should set client and namespace", func() {
			fakeClient := testutil.NewFakeClient()
			result := manager.WithClient(fakeClient, "test-namespace")
			Expect(result).To(Equal(manager))
		})
	})

	Describe("ValidateProvider", func() {
		It("should return nil when client is not set", func() {
			provider := &mockSidecarProvider{name: "test-sidecar"}
			manager.Register(provider, &sidecar.SidecarConfig{Enabled: true})

			err := manager.ValidateProvider(context.Background(), "test-sidecar")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return nil when namespace is empty", func() {
			fakeClient := testutil.NewFakeClient()
			manager.WithClient(fakeClient, "")

			provider := &mockSidecarProvider{name: "test-sidecar"}
			manager.Register(provider, &sidecar.SidecarConfig{Enabled: true})

			err := manager.ValidateProvider(context.Background(), "test-sidecar")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error for non-existent provider with client", func() {
			fakeClient := testutil.NewFakeClient()
			manager.WithClient(fakeClient, "test-namespace")

			err := manager.ValidateProvider(context.Background(), "non-existent")
			Expect(err).To(HaveOccurred())
		})

		It("should return nil for disabled sidecar with client", func() {
			fakeClient := testutil.NewFakeClient()
			manager.WithClient(fakeClient, "test-namespace")

			provider := &mockSidecarProvider{name: "test-sidecar"}
			manager.Register(provider, &sidecar.SidecarConfig{Enabled: false})

			err := manager.ValidateProvider(context.Background(), "test-sidecar")
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("ValidateAll", func() {
		It("should return nil when client is not set", func() {
			provider := &mockSidecarProvider{name: "test-sidecar"}
			manager.Register(provider, &sidecar.SidecarConfig{Enabled: true})

			err := manager.ValidateAll(context.Background())
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return nil when namespace is empty", func() {
			fakeClient := testutil.NewFakeClient()
			manager.WithClient(fakeClient, "")

			provider := &mockSidecarProvider{name: "test-sidecar"}
			manager.Register(provider, &sidecar.SidecarConfig{Enabled: true})

			err := manager.ValidateAll(context.Background())
			Expect(err).NotTo(HaveOccurred())
		})

		It("should validate all registered providers with client", func() {
			fakeClient := testutil.NewFakeClient()
			manager.WithClient(fakeClient, "test-namespace")

			provider1 := &mockSidecarProvider{name: "sidecar-1"}
			provider2 := &mockSidecarProvider{name: "sidecar-2"}
			manager.Register(provider1, &sidecar.SidecarConfig{Enabled: true})
			manager.Register(provider2, &sidecar.SidecarConfig{Enabled: true})

			// For mock providers, validation should succeed
			err := manager.ValidateAll(context.Background())
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("ValidateProvider with real providers", func() {
		It("should return error when Vector ConfigMap does not exist", func() {
			fakeClient := testutil.NewFakeClient()
			manager.WithClient(fakeClient, "test-namespace")

			vectorProvider := vector.NewVectorSidecarProvider("test-product:latest")
			manager.Register(vectorProvider, &sidecar.SidecarConfig{Enabled: true})

			err := manager.ValidateProvider(context.Background(), vector.VectorSidecarName)
			Expect(err).To(HaveOccurred())
		})

		It("should return nil when Vector ConfigMap exists", func() {
			vectorCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vector-config",
					Namespace: "test-namespace",
				},
			}
			fakeClient := testutil.NewFakeClientWithObjects(vectorCM)
			manager.WithClient(fakeClient, "test-namespace")

			vectorProvider := vector.NewVectorSidecarProvider("test-product:latest")
			manager.Register(vectorProvider, &sidecar.SidecarConfig{Enabled: true})

			err := manager.ValidateProvider(context.Background(), vector.VectorSidecarName)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error when JMX Exporter ConfigMap does not exist", func() {
			fakeClient := testutil.NewFakeClient()
			manager.WithClient(fakeClient, "test-namespace")

			jmxProvider := sidecar.NewJMXExporterSidecarProvider()
			manager.Register(jmxProvider, &sidecar.SidecarConfig{Enabled: true})

			err := manager.ValidateProvider(context.Background(), sidecar.JMXExporterSidecarName)
			Expect(err).To(HaveOccurred())
		})

		It("should return nil when JMX Exporter ConfigMap exists", func() {
			jmxCm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "jmx-exporter-config",
					Namespace: "test-namespace",
				},
			}
			fakeClient := testutil.NewFakeClientWithObjects(jmxCm)
			manager.WithClient(fakeClient, "test-namespace")

			jmxProvider := sidecar.NewJMXExporterSidecarProvider()
			manager.Register(jmxProvider, &sidecar.SidecarConfig{Enabled: true})

			err := manager.ValidateProvider(context.Background(), sidecar.JMXExporterSidecarName)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("ValidateAll with real providers", func() {
		It("should return error when any provider validation fails", func() {
			fakeClient := testutil.NewFakeClient()
			manager.WithClient(fakeClient, "test-namespace")

			vectorProvider := vector.NewVectorSidecarProvider("test-product:latest")
			jmxProvider := sidecar.NewJMXExporterSidecarProvider()
			manager.Register(vectorProvider, &sidecar.SidecarConfig{Enabled: true})
			manager.Register(jmxProvider, &sidecar.SidecarConfig{Enabled: true})

			err := manager.ValidateAll(context.Background())
			Expect(err).To(HaveOccurred())
		})

		It("should return nil when all provider validations succeed", func() {
			vectorCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vector-config",
					Namespace: "test-namespace",
				},
			}
			jmxCm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "jmx-exporter-config",
					Namespace: "test-namespace",
				},
			}
			fakeClient := testutil.NewFakeClientWithObjects(vectorCM, jmxCm)
			manager.WithClient(fakeClient, "test-namespace")

			vectorProvider := vector.NewVectorSidecarProvider("test-product:latest")
			jmxProvider := sidecar.NewJMXExporterSidecarProvider()
			manager.Register(vectorProvider, &sidecar.SidecarConfig{Enabled: true})
			manager.Register(jmxProvider, &sidecar.SidecarConfig{Enabled: true})

			err := manager.ValidateAll(context.Background())
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Validate with custom ConfigMap names", func() {
		It("should validate Vector with custom ConfigMap name", func() {
			customCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-custom-vector-config",
					Namespace: "test-namespace",
				},
			}
			fakeClient := testutil.NewFakeClientWithObjects(customCM)
			manager.WithClient(fakeClient, "test-namespace")

			vectorProvider := vector.NewVectorSidecarProvider("test-product:latest", vector.WithConfigMapName("my-custom-vector-config"))
			manager.Register(vectorProvider, &sidecar.SidecarConfig{Enabled: true})

			err := manager.ValidateProvider(context.Background(), vector.VectorSidecarName)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should validate JMX Exporter with custom ConfigMap name", func() {
			customCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-custom-jmx-config",
					Namespace: "test-namespace",
				},
			}
			fakeClient := testutil.NewFakeClientWithObjects(customCM)
			manager.WithClient(fakeClient, "test-namespace")

			jmxProvider := sidecar.NewJMXExporterSidecarProvider().WithConfigMapName("my-custom-jmx-config")
			manager.Register(jmxProvider, &sidecar.SidecarConfig{Enabled: true})

			err := manager.ValidateProvider(context.Background(), sidecar.JMXExporterSidecarName)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

var _ = Describe("Sidecar Helper Functions", func() {
	Describe("AddVolumes", func() {
		It("should add volumes to pod spec", func() {
			podSpec := &corev1.PodSpec{}
			volumes := []corev1.Volume{
				{Name: "volume-1"},
				{Name: "volume-2"},
			}

			sidecar.AddVolumes(podSpec, volumes)
			Expect(podSpec.Volumes).To(HaveLen(2))
		})

		It("should not add duplicate volumes", func() {
			podSpec := &corev1.PodSpec{
				Volumes: []corev1.Volume{{Name: "volume-1"}},
			}
			volumes := []corev1.Volume{
				{Name: "volume-1"},
				{Name: "volume-2"},
			}

			sidecar.AddVolumes(podSpec, volumes)
			Expect(podSpec.Volumes).To(HaveLen(2))
		})
	})

	Describe("AddVolumeMounts", func() {
		It("should add volume mounts to container", func() {
			container := &corev1.Container{}
			mounts := []corev1.VolumeMount{
				{Name: "mount-1", MountPath: "/path/1"},
				{Name: "mount-2", MountPath: "/path/2"},
			}

			sidecar.AddVolumeMounts(container, mounts)
			Expect(container.VolumeMounts).To(HaveLen(2))
		})

		It("should not add duplicate volume mounts", func() {
			container := &corev1.Container{
				VolumeMounts: []corev1.VolumeMount{{Name: "mount-1", MountPath: "/path/1"}},
			}
			mounts := []corev1.VolumeMount{
				{Name: "mount-1", MountPath: "/path/1"},
				{Name: "mount-2", MountPath: "/path/2"},
			}

			sidecar.AddVolumeMounts(container, mounts)
			Expect(container.VolumeMounts).To(HaveLen(2))
		})
	})

	Describe("AddEnvVars", func() {
		It("should add environment variables to container", func() {
			container := &corev1.Container{}
			envVars := map[string]string{
				"VAR1": "value1",
				"VAR2": "value2",
			}

			sidecar.AddEnvVars(container, envVars)
			Expect(container.Env).To(HaveLen(2))
		})

		It("should not add duplicate environment variables", func() {
			container := &corev1.Container{
				Env: []corev1.EnvVar{{Name: "VAR1", Value: "old"}},
			}
			envVars := map[string]string{
				"VAR1": "new",
				"VAR2": "value2",
			}

			sidecar.AddEnvVars(container, envVars)
			Expect(container.Env).To(HaveLen(2))
			// VAR1 should keep old value since it already exists
			Expect(container.Env[0].Value).To(Equal("old"))
		})
	})

	Describe("AddPorts", func() {
		It("should add ports to container", func() {
			container := &corev1.Container{}
			ports := []corev1.ContainerPort{
				{Name: "port-1", ContainerPort: 8080},
				{Name: "port-2", ContainerPort: 9090},
			}

			sidecar.AddPorts(container, ports)
			Expect(container.Ports).To(HaveLen(2))
		})

		It("should not add duplicate ports", func() {
			container := &corev1.Container{
				Ports: []corev1.ContainerPort{{Name: "port-1", ContainerPort: 8080}},
			}
			ports := []corev1.ContainerPort{
				{Name: "port-1", ContainerPort: 8080},
				{Name: "port-2", ContainerPort: 9090},
			}

			sidecar.AddPorts(container, ports)
			Expect(container.Ports).To(HaveLen(2))
		})
	})

	Describe("FindContainer", func() {
		It("should find a container by name", func() {
			podSpec := &corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "main"},
					{Name: "sidecar"},
				},
			}

			container := sidecar.FindContainer(podSpec, "sidecar")
			Expect(container).NotTo(BeNil())
			Expect(container.Name).To(Equal("sidecar"))
		})

		It("should return nil when container not found", func() {
			podSpec := &corev1.PodSpec{
				Containers: []corev1.Container{{Name: "main"}},
			}

			container := sidecar.FindContainer(podSpec, "non-existent")
			Expect(container).To(BeNil())
		})
	})

	Describe("FindInitContainer", func() {
		It("should find an init container by name", func() {
			podSpec := &corev1.PodSpec{
				InitContainers: []corev1.Container{
					{Name: "init-1"},
					{Name: "init-2"},
				},
			}

			container := sidecar.FindInitContainer(podSpec, "init-2")
			Expect(container).NotTo(BeNil())
			Expect(container.Name).To(Equal("init-2"))
		})

		It("should return nil when init container not found", func() {
			podSpec := &corev1.PodSpec{
				InitContainers: []corev1.Container{{Name: "init-1"}},
			}

			container := sidecar.FindInitContainer(podSpec, "non-existent")
			Expect(container).To(BeNil())
		})
	})
})

// mockSidecarProvider is a test implementation of SidecarProvider
type mockSidecarProvider struct {
	name      string
	injectErr error
}

func (p *mockSidecarProvider) Name() string {
	return p.name
}

func (p *mockSidecarProvider) Inject(podSpec *corev1.PodSpec, config *sidecar.SidecarConfig) error {
	if p.injectErr != nil {
		return p.injectErr
	}
	// Add a simple container
	podSpec.Containers = append(podSpec.Containers, corev1.Container{
		Name:  p.name,
		Image: "test-image",
	})
	return nil
}

func (p *mockSidecarProvider) Validate(ctx context.Context, c client.Client, namespace string) error {
	return nil
}
