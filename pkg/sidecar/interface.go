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

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SidecarConfig contains configuration for sidecar injection.
type SidecarConfig struct {
	// Image is the container image to use.
	Image string

	// ImagePullPolicy is the image pull policy.
	ImagePullPolicy corev1.PullPolicy

	// Resources defines resource requirements for the sidecar.
	Resources *corev1.ResourceRequirements

	// EnvVars are additional environment variables.
	EnvVars map[string]string

	// Volumes are additional volumes needed by the sidecar.
	Volumes []corev1.Volume

	// VolumeMounts are additional volume mounts for the sidecar.
	VolumeMounts []corev1.VolumeMount

	// Ports are ports exposed by the sidecar.
	Ports []corev1.ContainerPort

	// Enabled determines if this sidecar should be injected.
	Enabled bool

	// SecurityContext defines the security context for the sidecar container.
	SecurityContext *corev1.SecurityContext

	// MainContainerName specifies which container to target for shared volume mounts.
	// If empty, defaults to the first container in the pod spec.
	MainContainerName string
}

// SidecarProvider defines the interface for sidecar injection.
type SidecarProvider interface {
	// Name returns the sidecar name.
	Name() string

	// Inject injects the sidecar into the pod spec.
	// Returns the modified pod spec or an error.
	Inject(podSpec *corev1.PodSpec, config *SidecarConfig) error

	// Validate validates the provider's dependencies (e.g., required ConfigMaps).
	Validate(ctx context.Context, c client.Client, namespace string) error
}
