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

	// Probes overrides the probes the provider sets by default. The zero value keeps them; see
	// SidecarProbes for why the probe TYPE is a correctness question on a native sidecar.
	Probes SidecarProbes
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

// Injection phases order the providers SidecarManager.InjectAll runs. A provider that another
// provider depends on — e.g. a container whose logs Vector collects, which Vector can only
// RW-mount the shared log volume onto once that container is in the PodSpec — must run in an
// earlier phase. Within a phase providers are injected in name order, so injection stays
// deterministic and a pod template does not re-render across reconciles.
const (
	// SidecarPhaseProducer runs first: containers that produce data a later provider consumes.
	SidecarPhaseProducer = 10
	// SidecarPhaseDefault is the phase of providers that declare none.
	SidecarPhaseDefault = 50
	// SidecarPhasePipeline runs last: providers that wire up containers injected by earlier
	// phases, such as a log-collection pipeline mounting its shared volume on its producers.
	SidecarPhasePipeline = 90
)

// PhasedProvider is optionally implemented by sidecar providers that must be injected at a
// specific phase regardless of how they are registered. SidecarManager.InjectAll uses the
// phase returned here unless the provider was registered via RegisterWithPhase, whose explicit
// phase wins. Providers that implement neither run at SidecarPhaseDefault.
type PhasedProvider interface {
	// Phase returns the injection phase, e.g. SidecarPhaseProducer.
	Phase() int
}

// OwnImageProvider is optionally implemented by sidecar providers that ship their own
// upstream image (e.g. oauth2-proxy) rather than running a command inside the product
// image (e.g. Vector, JMX Exporter). SidecarManager.SetProductImage skips such providers,
// so their pinned default image stays reachable when a product registers them with an
// empty SidecarConfig.Image.
type OwnImageProvider interface {
	// OwnsImage reports whether the provider supplies its own default image.
	OwnsImage() bool
}
