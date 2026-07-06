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
	"path"
	"strings"

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

// WithProducers sets the names of the log-producer containers. The provider creates the shared
// log volume and RW-mounts it on each of these containers (if present in the PodSpec), in
// addition to mounting it on the Vector container, whose command pre-creates each producer's
// per-container log directory. These are the containers whose log files Vector collects;
// typically the product's main container.
func WithProducers(containers []string) ProviderOption {
	return func(p *VectorSidecarProvider) {
		// Copy so a later caller mutation of the slice can't change the provider's configuration.
		p.producers = append([]string(nil), containers...)
	}
}

// WithLogVolumeSize sets a custom SizeLimit for the shared log emptyDir. Empty falls back to
// DefaultLogVolumeSize.
func WithLogVolumeSize(quantity resource.Quantity) ProviderOption {
	return func(p *VectorSidecarProvider) {
		p.logVolumeSize = &quantity
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
	logVolumeSize  *resource.Quantity
	producers      []string
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
		Command:         vectorCommand(p.producers),
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
				// Read-write (not read-only): the command above pre-creates the producers'
				// per-container log directories in this volume before exec'ing vector.
				Name:      VectorLogVolumeName,
				MountPath: VectorLogMountPath,
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

	// The Vector provider is the single owner of the shared log pipeline: it creates the shared
	// log emptyDir, RW-mounts it on each declared producer container (so the product writes its
	// log files there), and mounts it on the Vector container (above). Creating and mounting
	// the volume in one place removes the previous double-owner split (base handler produced,
	// provider consumed) and makes a double-mount impossible.
	logVolumeSizeLimit := resource.MustParse(DefaultLogVolumeSize)
	if p.logVolumeSize != nil {
		logVolumeSizeLimit = *p.logVolumeSize
	}

	// Add Vector-owned volumes (config + data) plus the shared log volume. Resolve the data
	// volume size: custom if set, otherwise the default.
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
		{
			Name: VectorLogVolumeName,
			VolumeSource: corev1.VolumeSource{
				// Node-disk emptyDir (default medium), bounded by SizeLimit. Explicitly NOT
				// medium=Memory and NOT a PVC.
				EmptyDir: &corev1.EmptyDirVolumeSource{
					SizeLimit: &logVolumeSizeLimit,
				},
			},
		},
	}

	sidecar.AddVolumes(podSpec, volumes)

	// RW-mount the shared log volume on each producer container present in the PodSpec.
	// Producers are expected to exist by now — the main
	// container always does; a SidecarManager-injected init-container producer must be injected
	// before Vector (InjectAll ordering).
	producerMount := corev1.VolumeMount{
		Name:      VectorLogVolumeName,
		MountPath: VectorLogMountPath,
	}
	for _, name := range p.producers {
		if c := sidecar.FindContainer(podSpec, name); c != nil {
			sidecar.AddVolumeMounts(c, []corev1.VolumeMount{producerMount})
		}
		if c := sidecar.FindInitContainer(podSpec, name); c != nil {
			sidecar.AddVolumeMounts(c, []corev1.VolumeMount{producerMount})
		}
	}

	return nil
}

// vectorCommand builds the Vector container command. Vector runs as a native init container
// (restartPolicy Always) so the kubelet starts it BEFORE the producer containers; that makes
// it the right place to pre-create each declared producer's log directory
// ("<LogDir>/<lowercased container>", the stable path convention from
// productlogging.RenderConfigFile). log4j 1.x's RollingFileAppender and Python's FileHandler
// do not create parent directories, so without this step their file appenders would fail to
// open on startup. With no producers declared the command execs vector directly.
func vectorCommand(producers []string) []string {
	if len(producers) == 0 {
		return []string{
			VectorSidecarName,
			"--config",
			VectorConfigMountPath + "/" + VectorConfigFileName,
		}
	}
	dirs := make([]string, 0, len(producers))
	for _, name := range producers {
		dirs = append(dirs, path.Join(VectorLogMountPath, strings.ToLower(name)))
	}
	script := "mkdir -p " + strings.Join(dirs, " ") +
		" && exec " + VectorSidecarName + " --config " + VectorConfigMountPath + "/" + VectorConfigFileName
	return []string{"/bin/sh", "-c", script}
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
