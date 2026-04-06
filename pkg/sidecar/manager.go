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

package sidecar

import (
	"context"
	"fmt"
	"sort"

	"github.com/zncdatadev/operator-go/pkg/common"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SidecarManager manages sidecar providers and injection.
type SidecarManager struct {
	providers map[string]SidecarProvider
	configs   map[string]*SidecarConfig
	client    client.Client
	namespace string
}

// NewSidecarManager creates a new SidecarManager.
func NewSidecarManager() *SidecarManager {
	return &SidecarManager{
		providers: make(map[string]SidecarProvider),
		configs:   make(map[string]*SidecarConfig),
	}
}

// WithClient sets the Kubernetes client for validation
func (m *SidecarManager) WithClient(c client.Client, namespace string) *SidecarManager {
	m.client = c
	m.namespace = namespace
	return m
}

// ValidateProvider validates a sidecar provider configuration
func (m *SidecarManager) ValidateProvider(ctx context.Context, name string) error {
	if m.client == nil || m.namespace == "" {
		return nil
	}

	provider, exists := m.providers[name]
	if !exists {
		return common.ResourceNotFoundError("sidecar provider", m.namespace, name, fmt.Errorf("sidecar provider %s not found", name))
	}

	config, exists := m.configs[name]
	if !exists {
		config = &SidecarConfig{Enabled: true}
	}

	if !config.Enabled {
		return nil
	}

	return provider.Validate(ctx, m.client, m.namespace)
}

// ValidateAll validates all registered providers
func (m *SidecarManager) ValidateAll(ctx context.Context) error {
	if m.client == nil || m.namespace == "" {
		return nil
	}

	var errors []error
	names := m.ListProviders()
	sort.Strings(names)
	for _, name := range names {
		if err := m.ValidateProvider(ctx, name); err != nil {
			errors = append(errors, common.CreateResourceError("sidecar", m.namespace, name, fmt.Errorf("provider %s: %w", name, err)))
		}
	}

	if len(errors) > 0 {
		return common.ConfigMergeError("sidecar validation", fmt.Errorf("validation errors: %v", errors))
	}
	return nil
}

// ValidateConfigMapExists validates that a ConfigMap exists.
func ValidateConfigMapExists(ctx context.Context, c client.Client, namespace, name string) error {
	cm := &corev1.ConfigMap{}
	err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, cm)
	if err != nil {
		return err
	}
	return nil
}

// Register registers a sidecar provider with its configuration.
func (m *SidecarManager) Register(provider SidecarProvider, config *SidecarConfig) {
	m.providers[provider.Name()] = provider
	m.configs[provider.Name()] = config
}

// Unregister removes a sidecar provider.
func (m *SidecarManager) Unregister(name string) {
	delete(m.providers, name)
	delete(m.configs, name)
}

// GetProvider returns a sidecar provider by name.
func (m *SidecarManager) GetProvider(name string) (SidecarProvider, bool) {
	provider, exists := m.providers[name]
	return provider, exists
}

// GetConfig returns a sidecar configuration by name.
func (m *SidecarManager) GetConfig(name string) (*SidecarConfig, bool) {
	config, exists := m.configs[name]
	return config, exists
}

// ListProviders returns all registered provider names.
func (m *SidecarManager) ListProviders() []string {
	names := make([]string, 0, len(m.providers))
	for name := range m.providers {
		names = append(names, name)
	}
	return names
}

// InjectAll injects all enabled sidecars into the pod spec in deterministic order.
func (m *SidecarManager) InjectAll(podSpec *corev1.PodSpec) error {
	names := m.ListProviders()
	sort.Strings(names)

	for _, name := range names {
		provider := m.providers[name]
		config, exists := m.configs[name]
		if !exists {
			config = &SidecarConfig{Enabled: true}
		}

		if !config.Enabled {
			continue
		}

		if err := provider.Inject(podSpec, config); err != nil {
			return fmt.Errorf("failed to inject sidecar %s: %w", name, err)
		}
	}
	return nil
}

// Inject injects a specific sidecar into the pod spec.
func (m *SidecarManager) Inject(podSpec *corev1.PodSpec, name string) error {
	provider, exists := m.providers[name]
	if !exists {
		return fmt.Errorf("sidecar provider %s not found", name)
	}

	config, exists := m.configs[name]
	if !exists {
		config = &SidecarConfig{Enabled: true}
	}

	if !config.Enabled {
		return nil
	}

	return provider.Inject(podSpec, config)
}

// HasSidecars returns true if any sidecars are registered.
func (m *SidecarManager) HasSidecars() bool {
	return len(m.providers) > 0
}

// Count returns the number of registered sidecars.
func (m *SidecarManager) Count() int {
	return len(m.providers)
}

// AddVolumes adds volumes to the pod spec.
func AddVolumes(podSpec *corev1.PodSpec, volumes []corev1.Volume) {
	existingVolumes := make(map[string]bool)
	for _, v := range podSpec.Volumes {
		existingVolumes[v.Name] = true
	}

	for _, v := range volumes {
		if !existingVolumes[v.Name] {
			podSpec.Volumes = append(podSpec.Volumes, v)
		}
	}
}

// AddVolumeMounts adds volume mounts to a container.
func AddVolumeMounts(container *corev1.Container, mounts []corev1.VolumeMount) {
	existingMounts := make(map[string]bool)
	for _, m := range container.VolumeMounts {
		existingMounts[m.Name] = true
	}

	for _, m := range mounts {
		if !existingMounts[m.Name] {
			container.VolumeMounts = append(container.VolumeMounts, m)
		}
	}
}

// AddEnvVars adds environment variables to a container.
func AddEnvVars(container *corev1.Container, envVars map[string]string) {
	existingEnv := make(map[string]bool)
	for _, e := range container.Env {
		existingEnv[e.Name] = true
	}

	for name, value := range envVars {
		if !existingEnv[name] {
			container.Env = append(container.Env, corev1.EnvVar{
				Name:  name,
				Value: value,
			})
		}
	}
}

// AddPorts adds ports to a container.
func AddPorts(container *corev1.Container, ports []corev1.ContainerPort) {
	existingPorts := make(map[string]bool)
	for _, p := range container.Ports {
		existingPorts[p.Name] = true
	}

	for _, p := range ports {
		if !existingPorts[p.Name] {
			container.Ports = append(container.Ports, p)
		}
	}
}

// FindContainer finds a container by name in the pod spec.
func FindContainer(podSpec *corev1.PodSpec, name string) *corev1.Container {
	for i := range podSpec.Containers {
		if podSpec.Containers[i].Name == name {
			return &podSpec.Containers[i]
		}
	}
	return nil
}

// FindInitContainer finds an init container by name in the pod spec.
func FindInitContainer(podSpec *corev1.PodSpec, name string) *corev1.Container {
	for i := range podSpec.InitContainers {
		if podSpec.InitContainers[i].Name == name {
			return &podSpec.InitContainers[i]
		}
	}
	return nil
}

// findContainerIndex returns the index of a container by name, or -1 if not found.
func findContainerIndex(podSpec *corev1.PodSpec, name string) int {
	for i := range podSpec.Containers {
		if podSpec.Containers[i].Name == name {
			return i
		}
	}
	return -1
}

// findOrAddContainer finds an existing container by name or appends a new one.
func findOrAddContainer(podSpec *corev1.PodSpec, container corev1.Container) {
	if idx := findContainerIndex(podSpec, container.Name); idx >= 0 {
		podSpec.Containers[idx] = container
		return
	}
	podSpec.Containers = append(podSpec.Containers, container)
}

// findMainContainer finds the main container for shared volume mounting.
// Uses MainContainerName from config if set, otherwise defaults to the first container.
func findMainContainer(podSpec *corev1.PodSpec, mainContainerName string) *corev1.Container {
	if mainContainerName != "" {
		return FindContainer(podSpec, mainContainerName)
	}
	if len(podSpec.Containers) > 0 {
		return &podSpec.Containers[0]
	}
	return nil
}
