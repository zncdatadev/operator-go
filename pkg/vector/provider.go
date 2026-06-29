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
	"fmt"

	"github.com/zncdatadev/operator-go/pkg/sidecar"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ProviderOption is a functional option for configuring VectorSidecarProvider.
type ProviderOption func(*VectorSidecarProvider)

// WithConfigMapName sets a custom ConfigMap name for the Vector configuration.
func WithConfigMapName(name string) ProviderOption {
	return func(p *VectorSidecarProvider) {
		p.configMapName = name
	}
}

// WithDataVolumeSize sets a custom data volume size for Vector.
func WithDataVolumeSize(quantity resource.Quantity) ProviderOption {
	return func(p *VectorSidecarProvider) {
		p.dataVolumeSize = &quantity
	}
}

// Compile-time interface assertion.
var _ sidecar.SidecarProvider = (*VectorSidecarProvider)(nil)

// VectorSidecarProvider injects the Vector log collection sidecar.
// It implements the sidecar.SidecarProvider interface.
type VectorSidecarProvider struct {
	name           string
	image          string
	configMapName  string
	dataVolumeSize *resource.Quantity
}

// NewVectorSidecarProvider creates a new VectorSidecarProvider with the given product image and options.
// The image parameter is required — it should be the product container's image (Vector is built into product images).
func NewVectorSidecarProvider(image string, opts ...ProviderOption) *VectorSidecarProvider {
	p := &VectorSidecarProvider{
		name:          VectorSidecarName,
		image:         image,
		configMapName: VectorDefaultConfigMapName,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Name returns the sidecar name.
func (p *VectorSidecarProvider) Name() string {
	return p.name
}

// Validate validates that the Vector ConfigMap exists.
func (p *VectorSidecarProvider) Validate(ctx context.Context, c client.Client, namespace string) error {
	if err := sidecar.ValidateConfigMapExists(ctx, c, namespace, p.configMapName); err != nil {
		return fmt.Errorf("vector config map %q not found: %w", p.configMapName, err)
	}
	return nil
}

// Inject injects the Vector sidecar into the pod spec.
// This method is idempotent -- calling it multiple times will not duplicate the container.
func (p *VectorSidecarProvider) Inject(podSpec *corev1.PodSpec, config *sidecar.SidecarConfig) error {
	if config == nil {
		config = &sidecar.SidecarConfig{Enabled: true}
	}

	// Get image
	image := config.Image
	if image == "" {
		image = p.image
	}
	// Fail loudly at build time: an empty image would produce an invalid PodSpec (empty container
	// image) that the API server rejects opaquely.
	if image == "" {
		return fmt.Errorf("vector: no image configured; set it via SidecarConfig.Image or SetProductImage")
	}

	// Get pull policy
	pullPolicy := corev1.PullIfNotPresent
	if config.ImagePullPolicy != "" {
		pullPolicy = config.ImagePullPolicy
	}

	// Create Vector container
	container := &corev1.Container{
		Name:            p.name,
		Image:           image,
		ImagePullPolicy: pullPolicy,
		Command: []string{
			VectorSidecarName,
			"--config",
			VectorConfigMountPath + "/" + VectorConfigFileName,
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      VectorConfigVolumeName,
				MountPath: VectorConfigMountPath,
				ReadOnly:  true,
			},
			{
				Name:      VectorDataVolumeName,
				MountPath: VectorDataMountPath,
			},
			{
				Name:      VectorLogVolumeName,
				MountPath: VectorLogMountPath,
				ReadOnly:  true,
			},
		},
		ReadinessProbe:  defaultReadinessProbe(),
		SecurityContext: defaultSecurityContext(),
	}

	// Apply resources if provided
	if config.Resources != nil {
		container.Resources = *config.Resources
	}

	// Apply security context if provided
	if config.SecurityContext != nil {
		container.SecurityContext = config.SecurityContext
	}

	// Apply custom configuration
	if len(config.EnvVars) > 0 {
		sidecar.AddEnvVars(container, config.EnvVars)
	}

	if len(config.VolumeMounts) > 0 {
		sidecar.AddVolumeMounts(container, config.VolumeMounts)
	}

	// Vector is a long-running sidecar: inject it as a native sidecar (init container with
	// restartPolicy: Always) so the kubelet starts it before the main container and keeps it
	// running until the main container exits, guaranteeing logs are shipped through shutdown.
	// Idempotent -- replace if already present.
	container.RestartPolicy = sidecar.SidecarRestartPolicy()
	sidecar.AddOrReplaceInitContainer(podSpec, container)

	// Add Vector-owned volumes (config + data). Resolve data volume size: custom if set,
	// otherwise the default.
	//
	// The shared log volume is intentionally NOT created here. Under the producer/consumer
	// ownership split, the role-group base handler owns the shared log emptyDir (sized,
	// medium=node) and RW-mounts it on each product container; the Vector provider is a pure
	// consumer that only RO-mounts that volume on its own container (above). This removes the
	// previous double-mount hazard and keeps a single owner for the volume lifecycle.
	dataVolumeSizeLimit := resource.MustParse(VectorDataVolumeSize)
	if p.dataVolumeSize != nil {
		dataVolumeSizeLimit = *p.dataVolumeSize
	}
	volumes := []corev1.Volume{
		{
			Name: VectorConfigVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: p.configMapName,
					},
				},
			},
		},
		{
			Name: VectorDataVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{
					SizeLimit: &dataVolumeSizeLimit,
				},
			},
		},
	}

	sidecar.AddVolumes(podSpec, volumes)

	return nil
}

// defaultSecurityContext returns a hardened security context for the Vector container.
func defaultSecurityContext() *corev1.SecurityContext {
	return &corev1.SecurityContext{
		RunAsNonRoot:             ptr.To(true),
		ReadOnlyRootFilesystem:   ptr.To(true),
		AllowPrivilegeEscalation: ptr.To(false),
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
	}
}

// ConfigMapName returns the ConfigMap name used by this provider.
func (p *VectorSidecarProvider) ConfigMapName() string {
	return p.configMapName
}
