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

	"github.com/zncdatadev/operator-go/pkg/constant"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// VectorSidecarName is the name of the Vector sidecar container.
	VectorSidecarName = "vector"
	// VectorConfigVolumeName is the name of the config volume.
	VectorConfigVolumeName = "vector-config"
	// VectorDataVolumeName is the name of the data volume.
	VectorDataVolumeName = "vector-data"
	// VectorLogVolumeName is the default name of the shared log volume that carries the
	// product's application logs from the main container into the Vector sidecar.
	//
	// Convention: the product writes its logs (e.g. "<container>.stdout.log") to a volume
	// mounted at constant.KubedoopLogDir on the main container, and the framework mounts the
	// SAME volume read-only into the Vector container at the same path, so Vector's glob
	// "<KubedoopLogDir>/*.stdout.log" sees the files. By default the framework creates this
	// volume as an emptyDir; use WithLogVolume to instead reuse a product-supplied volume.
	VectorLogVolumeName = "log"
	// VectorConfigMountPath is the mount path for Vector config.
	VectorConfigMountPath = "/etc/vector"
	// VectorDataMountPath is the mount path for Vector data.
	VectorDataMountPath = "/var/lib/vector"
	// VectorLogMountPath is the mount path of the shared product log volume in both the main
	// container and the Vector sidecar. It is the canonical Kubedoop log directory, matching
	// where products write logs and where the Vector config globs for log files.
	VectorLogMountPath = constant.KubedoopLogDir
)

// VectorSidecarProvider injects the Vector log collection sidecar.
type VectorSidecarProvider struct {
	name          string
	configMapName string
	logVolumeName string
}

// NewVectorSidecarProvider creates a new VectorSidecarProvider.
func NewVectorSidecarProvider() *VectorSidecarProvider {
	return &VectorSidecarProvider{
		name:          VectorSidecarName,
		configMapName: "vector-config",
		logVolumeName: VectorLogVolumeName,
	}
}

// WithConfigMapName sets a custom ConfigMap name for the Vector configuration.
func (p *VectorSidecarProvider) WithConfigMapName(name string) *VectorSidecarProvider {
	p.configMapName = name
	return p
}

// WithLogVolume points the provider at a product-supplied log volume instead of the default
// framework-managed emptyDir.
//
// When set, the provider mounts the named volume read-only into the Vector container at
// constant.KubedoopLogDir and does NOT create its own emptyDir for it; the product owns the
// volume's declaration (and its main-container mount at constant.KubedoopLogDir). This lets a
// product that already writes logs to its own volume — e.g. a size-limited emptyDir — have
// Vector read them automatically, without hand-wiring a VolumeMount onto the Vector container.
func (p *VectorSidecarProvider) WithLogVolume(name string) *VectorSidecarProvider {
	if name != "" {
		p.logVolumeName = name
	}
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
	if config.Image == "" {
		return fmt.Errorf("sidecar %s: image is required but not set", p.name)
	}

	// Get pull policy
	pullPolicy := corev1.PullIfNotPresent
	if config.ImagePullPolicy != "" {
		pullPolicy = config.ImagePullPolicy
	}

	// Create Vector container
	container := &corev1.Container{
		Name:            p.name,
		Image:           config.Image,
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
			// Mount the shared product log volume read-only so Vector can read the files the
			// main container writes to constant.KubedoopLogDir. This removes the need for each
			// product to hand-wire the log mount onto the Vector container.
			{
				Name:      p.logVolumeName,
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

	// Vector is a long-running sidecar: inject it as a native sidecar (init container with
	// restartPolicy: Always) so the kubelet keeps it running until the main container exits,
	// guaranteeing logs are shipped through shutdown.
	container.RestartPolicy = SidecarRestartPolicy()
	addOrReplaceInitContainer(podSpec, container)

	// Add required volumes if not present.
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
	}

	// The log volume is only framework-managed when using the default name. When the product
	// supplies its own log volume via WithLogVolume, it owns the volume declaration, so we do
	// not create an emptyDir for it (AddVolumes dedups by name, so this is also safe if the
	// product happens to use the default name and pre-declares its own volume).
	if p.logVolumeName == VectorLogVolumeName {
		volumes = append(volumes, corev1.Volume{
			Name: VectorLogVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}

	AddVolumes(podSpec, volumes)

	// Mount the log volume on the main container for shared log access. When the product owns
	// the log volume (WithLogVolume) it normally mounts it itself, but AddVolumeMounts dedups
	// by name so adding it here is idempotent and keeps the default path consistent.
	mainContainer := FindMainContainer(podSpec, config.MainContainerName)
	if config.MainContainerName != "" && mainContainer == nil {
		return fmt.Errorf("main container %q not found for vector log volume mount", config.MainContainerName)
	}
	if mainContainer != nil {
		AddVolumeMounts(mainContainer, []corev1.VolumeMount{
			{
				Name:      p.logVolumeName,
				MountPath: VectorLogMountPath,
			},
		})
	}

	return nil
}
