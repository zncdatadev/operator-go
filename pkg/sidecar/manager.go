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
	"errors"
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
	phases    map[string]int
	client    client.Client
	namespace string
}

// NewSidecarManager creates a new SidecarManager.
func NewSidecarManager() *SidecarManager {
	return &SidecarManager{
		providers: make(map[string]SidecarProvider),
		configs:   make(map[string]*SidecarConfig),
		phases:    make(map[string]int),
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

	var errs []error
	names := m.ListProviders()
	sort.Strings(names)
	for _, name := range names {
		if err := m.ValidateProvider(ctx, name); err != nil {
			errs = append(errs, common.CreateResourceError("sidecar", m.namespace, name, fmt.Errorf("provider %s: %w", name, err)))
		}
	}

	// Join rather than wrap: every provider's failure stays individually inspectable with
	// errors.As/Is by the caller, which a %v-formatted aggregate would flatten into a string.
	return errors.Join(errs...)
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

// ValidateSecretExists validates that a Secret exists.
func ValidateSecretExists(ctx context.Context, c client.Client, namespace, name string) error {
	secret := &corev1.Secret{}
	err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, secret)
	if err != nil {
		return err
	}
	return nil
}

// Register registers a sidecar provider with its configuration. The provider is injected at
// the phase it reports through PhasedProvider, or at SidecarPhaseDefault when it reports none.
func (m *SidecarManager) Register(provider SidecarProvider, config *SidecarConfig) {
	m.providers[provider.Name()] = provider
	m.configs[provider.Name()] = config
	delete(m.phases, provider.Name())
}

// RegisterWithPhase registers a sidecar provider that must be injected at an explicit phase,
// overriding any phase the provider reports itself. Use it when the dependency lives in the
// product rather than in the provider — e.g. registering a bespoke init container whose log
// files Vector collects at SidecarPhaseProducer, so it exists in the PodSpec by the time the
// Vector provider mounts the shared log volume onto it.
func (m *SidecarManager) RegisterWithPhase(provider SidecarProvider, config *SidecarConfig, phase int) {
	m.providers[provider.Name()] = provider
	m.configs[provider.Name()] = config
	m.phases[provider.Name()] = phase
}

// Unregister removes a sidecar provider.
func (m *SidecarManager) Unregister(name string) {
	delete(m.providers, name)
	delete(m.configs, name)
	delete(m.phases, name)
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

// Phase returns the injection phase of a registered provider: the phase passed to
// RegisterWithPhase if any, otherwise the one the provider reports through PhasedProvider,
// otherwise SidecarPhaseDefault. Unregistered names report SidecarPhaseDefault.
func (m *SidecarManager) Phase(name string) int {
	if phase, ok := m.phases[name]; ok {
		return phase
	}
	if provider, ok := m.providers[name]; ok {
		if phased, ok := provider.(PhasedProvider); ok {
			return phased.Phase()
		}
	}
	return SidecarPhaseDefault
}

// injectionOrder returns the registered provider names ordered by (phase, name).
func (m *SidecarManager) injectionOrder() []string {
	names := m.ListProviders()
	sort.Slice(names, func(i, j int) bool {
		pi, pj := m.Phase(names[i]), m.Phase(names[j])
		if pi != pj {
			return pi < pj
		}
		return names[i] < names[j]
	})
	return names
}

// InjectAll injects all enabled sidecars into the pod spec, ordered by (phase, name).
//
// The ordering is a guarantee providers may rely on: every provider in a lower phase has
// already injected its containers and volumes when a later one runs, so a provider that wires
// itself to another container (mounting a shared volume onto it, say) is certain to find that
// container in the PodSpec. Within a phase the name order keeps injection deterministic, so a
// pod template is byte-identical across reconciles. See SidecarPhaseProducer and friends.
func (m *SidecarManager) InjectAll(podSpec *corev1.PodSpec) error {
	for _, name := range m.injectionOrder() {
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

// CloneForBuild returns a copy safe to mutate for one build.
//
// SetProductImage writes the resolved image into every enabled config, so a manager that outlives a
// single build — one a product registered on its handler with
// BaseRoleGroupHandler.WithSidecarManager, which is process-wide and shared across every CR — would
// otherwise carry one cluster's image into the next cluster's pods, and race outright above
// MaxConcurrentReconciles 1. That is the same defect as the handler's per-CR fields
// (RoleGroupBuildContext.Image), on the framework's own side of the line.
//
// The configs map and its entries are copied because they are what gets written. Providers and
// phases are shared: they are read-only during a build, and a provider is a product-supplied
// implementation that must not be duplicated. Client and namespace are plain values.
func (m *SidecarManager) CloneForBuild() *SidecarManager {
	if m == nil {
		return nil
	}
	clone := &SidecarManager{
		providers: m.providers,
		phases:    m.phases,
		client:    m.client,
		namespace: m.namespace,
		configs:   make(map[string]*SidecarConfig, len(m.configs)),
	}
	for name, config := range m.configs {
		if config == nil {
			clone.configs[name] = nil
			continue
		}
		copied := *config
		clone.configs[name] = &copied
	}
	return clone
}

// SetProductImage sets the product image on all enabled sidecar configs that
// don't already have an image or pull policy set. This is used to propagate
// the product container image to built-in sidecars (e.g. JMX Exporter) that
// run as different commands within the same product image. Providers that ship
// their own upstream image (OwnImageProvider, e.g. oauth2-proxy) are skipped —
// filling their empty config.Image with the product image would silently run
// the product entrypoint in place of the sidecar.
func (m *SidecarManager) SetProductImage(image string, pullPolicy corev1.PullPolicy) error {
	if image == "" {
		return fmt.Errorf("product image must not be empty")
	}
	for name, config := range m.configs {
		if !config.Enabled {
			continue
		}
		if provider, ok := m.providers[name]; ok {
			if own, ok := provider.(OwnImageProvider); ok && own.OwnsImage() {
				continue
			}
		}
		if config.Image == "" {
			config.Image = image
		}
		if config.ImagePullPolicy == "" {
			config.ImagePullPolicy = pullPolicy
		}
	}
	return nil
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

// AddEnvVars adds environment variables to a container. Env vars whose names are already
// present are left untouched ("additional" semantics); use AddOrReplaceEnvVars for override
// semantics. Names are applied in sorted order — map iteration is randomized, and a
// nondeterministic env order would re-render the pod template on every reconcile.
func AddEnvVars(container *corev1.Container, envVars map[string]string) {
	existingEnv := make(map[string]bool)
	for _, e := range container.Env {
		existingEnv[e.Name] = true
	}

	for _, name := range sortedKeys(envVars) {
		if !existingEnv[name] {
			container.Env = append(container.Env, corev1.EnvVar{
				Name:  name,
				Value: envVars[name],
			})
		}
	}
}

// AddOrReplaceEnvVars sets environment variables on a container with override semantics:
// a same-named existing env var is replaced in place, new names are appended in sorted
// order. Providers whose entire configuration surface is env vars (e.g. oauth2-proxy) use
// this so SidecarConfig.EnvVars can actually change their built-in defaults.
func AddOrReplaceEnvVars(container *corev1.Container, envVars map[string]string) {
	index := make(map[string]int, len(container.Env))
	for i, e := range container.Env {
		index[e.Name] = i
	}

	for _, name := range sortedKeys(envVars) {
		if i, ok := index[name]; ok {
			container.Env[i] = corev1.EnvVar{Name: name, Value: envVars[name]}
		} else {
			container.Env = append(container.Env, corev1.EnvVar{Name: name, Value: envVars[name]})
		}
	}
}

// sortedKeys returns the map's keys in sorted order, for deterministic env rendering.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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

// FindContainerIndex returns the index of a container by name, or -1 if not found.
func FindContainerIndex(podSpec *corev1.PodSpec, name string) int {
	for i := range podSpec.Containers {
		if podSpec.Containers[i].Name == name {
			return i
		}
	}
	return -1
}

// AddOrReplaceContainer finds an existing container by name or appends a new one.
// If a container with the same name exists, it is replaced. Otherwise the container is appended.
func AddOrReplaceContainer(podSpec *corev1.PodSpec, container *corev1.Container) {
	if idx := FindContainerIndex(podSpec, container.Name); idx >= 0 {
		podSpec.Containers[idx] = *container
		return
	}
	podSpec.Containers = append(podSpec.Containers, *container)
}

// FindInitContainerIndex returns the index of an init container by name, or -1 if not found.
func FindInitContainerIndex(podSpec *corev1.PodSpec, name string) int {
	for i := range podSpec.InitContainers {
		if podSpec.InitContainers[i].Name == name {
			return i
		}
	}
	return -1
}

// AddOrReplaceInitContainer finds an existing init container by name or appends a new one.
//
// Under the native-sidecar model the SidecarManager is the single owner of pod container
// injection. Providers (Vector, JMX, StaticContainerProvider) use this from their Inject
// methods; products must NOT mutate the pod directly — they register a provider and let
// InjectAll place containers, so the Enabled gating, ordering and deduplication of the
// manager always apply.
//
// All managed containers go into podSpec.InitContainers: a long-running sidecar is simply
// an init container whose RestartPolicy is Always (see SidecarRestartPolicy); a one-shot
// init (e.g. node-id generation) has a nil RestartPolicy. This relies on the kubelet's
// native sidecar semantics (KEP-753: added in Kubernetes v1.28 behind the SidecarContainers
// gate, on by default since v1.29, GA in v1.33) for startup and shutdown ordering.
func AddOrReplaceInitContainer(podSpec *corev1.PodSpec, container *corev1.Container) {
	if idx := FindInitContainerIndex(podSpec, container.Name); idx >= 0 {
		podSpec.InitContainers[idx] = *container
		return
	}
	podSpec.InitContainers = append(podSpec.InitContainers, *container)
}

// SidecarRestartPolicy returns the RestartPolicy that turns an init container into a native
// sidecar (KEP-753; on by default since Kubernetes v1.29, GA in v1.33): the kubelet starts it
// before the main container, keeps it
// running alongside, and terminates it only after the main container exits. Set this on a
// container's RestartPolicy before registering it (e.g. via StaticContainerProvider) to
// make it a long-running sidecar instead of a one-shot init container.
func SidecarRestartPolicy() *corev1.ContainerRestartPolicy {
	policy := corev1.ContainerRestartPolicyAlways
	return &policy
}

// StaticContainerProvider injects a fixed, product-supplied container into InitContainers.
// It is the simplest way to register a bespoke container — whether a one-shot init (e.g.
// node-id generation, RestartPolicy nil) or a sidecar (RestartPolicy set via
// SidecarRestartPolicy) — so it flows through the SidecarManager like any other.
type StaticContainerProvider struct {
	container corev1.Container
}

var _ SidecarProvider = &StaticContainerProvider{}

// NewStaticContainerProvider creates a provider that injects the given container verbatim.
// The provider name is the container name. Set container.RestartPolicy to make it a sidecar.
func NewStaticContainerProvider(container corev1.Container) *StaticContainerProvider {
	return &StaticContainerProvider{container: container}
}

// Name returns the container name.
func (p *StaticContainerProvider) Name() string { return p.container.Name }

// Inject adds a copy of the static container to InitContainers.
func (p *StaticContainerProvider) Inject(podSpec *corev1.PodSpec, _ *SidecarConfig) error {
	AddOrReplaceInitContainer(podSpec, p.container.DeepCopy())
	return nil
}

// Validate has no dependencies for a static container.
func (p *StaticContainerProvider) Validate(_ context.Context, _ client.Client, _ string) error {
	return nil
}
