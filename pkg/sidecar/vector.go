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

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// VectorSidecarName is the name of the Vector sidecar container.
	VectorSidecarName = "vector"
	// VectorDefaultImage is the default Vector image.
	VectorDefaultImage = "timberio/vector:0.40.0-debian"
	// VectorConfigVolumeName is the name of the config volume.
	VectorConfigVolumeName = "vector-config"
	// VectorDataVolumeName is the name of the data volume.
	VectorDataVolumeName = "vector-data"
	// VectorLogVolumeName is the name of the shared log volume.
	VectorLogVolumeName = "vector-logs"
	// VectorConfigMountPath is the mount path for Vector config.
	VectorConfigMountPath = "/etc/vector"
	// VectorDataMountPath is the mount path for Vector data.
	VectorDataMountPath = "/var/lib/vector"
	// VectorLogMountPath is the mount path for shared logs.
	VectorLogMountPath = "/var/log/vector"
)

// VectorSidecarProvider injects the Vector log collection sidecar.
type VectorSidecarProvider struct {
	name          string
	configMapName string
}

// NewVectorSidecarProvider creates a new VectorSidecarProvider.
func NewVectorSidecarProvider() *VectorSidecarProvider {
	return &VectorSidecarProvider{
		name:          VectorSidecarName,
		configMapName: "vector-config",
	}
}

// WithConfigMapName sets a custom ConfigMap name for the Vector configuration.
func (p *VectorSidecarProvider) WithConfigMapName(name string) *VectorSidecarProvider {
	p.configMapName = name
	return p
}

// Name returns the sidecar name.
func (p *VectorSidecarProvider) Name() string {
	return p.name
}

// Validate validates that the Vector ConfigMap exists.
func (p *VectorSidecarProvider) Validate(ctx context.Context, c client.Client, namespace string) error {
	if err := ValidateConfigMapExists(ctx, c, namespace, p.configMapName); err != nil {
		return fmt.Errorf("vector config map %q not found: %w", p.configMapName, err)
	}
	return nil
}

// Inject injects the Vector sidecar into the pod spec.
// This method is idempotent — calling it multiple times will not duplicate the container.
func (p *VectorSidecarProvider) Inject(podSpec *corev1.PodSpec, config *SidecarConfig) error {
	if config == nil {
		config = &SidecarConfig{Enabled: true}
	}

	// Get image
	image := config.Image
	if image == "" {
		image = VectorDefaultImage
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
			"/usr/bin/vector",
			"--config",
			VectorConfigMountPath + "/vector.yaml",
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
		AddEnvVars(container, config.EnvVars)
	}

	if len(config.VolumeMounts) > 0 {
		AddVolumeMounts(container, config.VolumeMounts)
	}

	// Add container to pod (idempotent — replace if exists)
	AddOrReplaceContainer(podSpec, container)

	// Add required volumes if not present
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
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: VectorLogVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}

	AddVolumes(podSpec, volumes)

	// Mount log volume on main container for shared log access
	mainContainer := FindMainContainer(podSpec, config.MainContainerName)
	if mainContainer != nil {
		AddVolumeMounts(mainContainer, []corev1.VolumeMount{
			{
				Name:      VectorLogVolumeName,
				MountPath: VectorLogMountPath,
			},
		})
	}

	return nil
}
