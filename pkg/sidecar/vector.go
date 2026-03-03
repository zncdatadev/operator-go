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
	corev1 "k8s.io/api/core/v1"
)

const (
	// VectorSidecarName is the name of the Vector sidecar container.
	VectorSidecarName = "vector"
	// VectorDefaultImage is the default Vector image.
	VectorDefaultImage = "timberio/vector:0.30.0-debian"
	// VectorConfigVolumeName is the name of the Vector config volume.
	VectorConfigVolumeName = "vector-config"
	// VectorDataVolumeName is the name of the Vector data volume.
	VectorDataVolumeName = "vector-data"
	// VectorLogVolumeName is the name of the shared log volume.
	VectorLogVolumeName = "log-volume"
	// VectorConfigMountPath is the mount path for Vector config.
	VectorConfigMountPath = "/etc/vector"
	// VectorDataMountPath is the mount path for Vector data.
	VectorDataMountPath = "/var/lib/vector"
	// VectorLogMountPath is the mount path for logs.
	VectorLogMountPath = "/var/log/app"
)

// VectorSidecarProvider injects the Vector log collection sidecar.
type VectorSidecarProvider struct {
	name string
}

// NewVectorSidecarProvider creates a new VectorSidecarProvider.
func NewVectorSidecarProvider() *VectorSidecarProvider {
	return &VectorSidecarProvider{
		name: VectorSidecarName,
	}
}

// Name returns the sidecar name.
func (p *VectorSidecarProvider) Name() string {
	return p.name
}

// Inject injects the Vector sidecar into the pod spec.
func (p *VectorSidecarProvider) Inject(podSpec *corev1.PodSpec, config *SidecarConfig) error {
	if config == nil {
		config = &SidecarConfig{Enabled: true}
	}

	// Get image
	image := config.Image
	if image == "" {
		image = VectorDefaultImage
	}

	// Create Vector container
	container := &corev1.Container{
		Name:            p.name,
		Image:           image,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command: []string{
			"vector",
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

	// Apply custom configuration
	if len(config.EnvVars) > 0 {
		AddEnvVars(container, config.EnvVars)
	}

	if len(config.VolumeMounts) > 0 {
		AddVolumeMounts(container, config.VolumeMounts)
	}

	// Add container to pod
	podSpec.Containers = append(podSpec.Containers, *container)

	// Add required volumes if not present
	volumes := []corev1.Volume{
		{
			Name: VectorConfigVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "vector-config",
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

	// Also mount log volume to main container for shared logging
	if len(podSpec.Containers) > 0 {
		AddVolumeMounts(&podSpec.Containers[0], []corev1.VolumeMount{
			{
				Name:      VectorLogVolumeName,
				MountPath: VectorLogMountPath,
			},
		})
	}

	return nil
}
