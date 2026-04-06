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
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ProviderOption is a functional option for configuring VectorSidecarProvider.
type ProviderOption func(*VectorSidecarProvider)

// WithImage sets a custom Vector container image.
func WithImage(image string) ProviderOption {
	return func(p *VectorSidecarProvider) {
		p.image = image
	}
}

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

// NewVectorSidecarProvider creates a new VectorSidecarProvider with the given options.
func NewVectorSidecarProvider(opts ...ProviderOption) *VectorSidecarProvider {
	p := &VectorSidecarProvider{
		name:          VectorSidecarName,
		image:         VectorDefaultImage,
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
			"vector",
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
		ReadinessProbe: defaultReadinessProbe(),
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

	// Add container to pod (idempotent -- replace if exists)
	sidecar.AddOrReplaceContainer(podSpec, container)

	// Add required volumes if not present
	dataVolumeSizeLimit := resource.MustParse(VectorDataVolumeSize)
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
		{
			Name: VectorLogVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}

	sidecar.AddVolumes(podSpec, volumes)

	// Mount log volume to the main container for shared logging
	mainContainer := sidecar.FindMainContainer(podSpec, config.MainContainerName)
	if mainContainer != nil {
		sidecar.AddVolumeMounts(mainContainer, []corev1.VolumeMount{
			{
				Name:      VectorLogVolumeName,
				MountPath: VectorLogMountPath,
			},
		})
	}

	return nil
}

// ConfigMapName returns the ConfigMap name used by this provider.
func (p *VectorSidecarProvider) ConfigMapName() string {
	return p.configMapName
}
