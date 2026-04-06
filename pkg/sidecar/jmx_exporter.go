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
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// JMXExporterSidecarName is the name of the JMX Exporter sidecar container.
	JMXExporterSidecarName = "jmx-exporter"
	// JMXExporterDefaultImage is the default JMX Exporter image.
	JMXExporterDefaultImage = "bitnami/jmx-exporter:0.20.0"
	// JMXExporterPort is the default JMX Exporter metrics port.
	JMXExporterPort = 5556
	// JMXExporterConfigVolumeName is the name of the config volume.
	JMXExporterConfigVolumeName = "jmx-exporter-config"
	// JMXExporterConfigMountPath is the mount path for config.
	JMXExporterConfigMountPath = "/opt/jmx_exporter"
	// JMXExporterDefaultConfigMapName is the default ConfigMap name for JMX Exporter config.
	JMXExporterDefaultConfigMapName = "jmx-exporter-config"
)

// JMXExporterSidecarProvider injects the Prometheus JMX Exporter sidecar.
type JMXExporterSidecarProvider struct {
	name          string
	port          int32
	configMapName string
}

// NewJMXExporterSidecarProvider creates a new JMXExporterSidecarProvider.
func NewJMXExporterSidecarProvider() *JMXExporterSidecarProvider {
	return &JMXExporterSidecarProvider{
		name:          JMXExporterSidecarName,
		port:          JMXExporterPort,
		configMapName: JMXExporterDefaultConfigMapName,
	}
}

// NOTE: JMX Exporter uses mutating builder methods (WithPort, WithConfigMapName).
// Vector provider uses functional options (WithImage, WithConfigMapName).
// A future refactor should unify both to functional options for API consistency.

// WithPort sets a custom metrics port.
func (p *JMXExporterSidecarProvider) WithPort(port int32) *JMXExporterSidecarProvider {
	p.port = port
	return p
}

// WithConfigMapName sets a custom ConfigMap name for the JMX Exporter configuration.
func (p *JMXExporterSidecarProvider) WithConfigMapName(name string) *JMXExporterSidecarProvider {
	p.configMapName = name
	return p
}

// Name returns the sidecar name.
func (p *JMXExporterSidecarProvider) Name() string {
	return p.name
}

// Validate validates that the JMX Exporter ConfigMap exists.
func (p *JMXExporterSidecarProvider) Validate(ctx context.Context, c client.Client, namespace string) error {
	if err := ValidateConfigMapExists(ctx, c, namespace, p.configMapName); err != nil {
		return fmt.Errorf("jmx-exporter config map %q not found: %w", p.configMapName, err)
	}
	return nil
}

// Inject injects the JMX Exporter sidecar into the pod spec.
// This method is idempotent — calling it multiple times will not duplicate the container.
func (p *JMXExporterSidecarProvider) Inject(podSpec *corev1.PodSpec, config *SidecarConfig) error {
	if config == nil {
		config = &SidecarConfig{Enabled: true}
	}

	// Get image
	image := config.Image
	if image == "" {
		image = JMXExporterDefaultImage
	}

	// Get pull policy
	pullPolicy := corev1.PullIfNotPresent
	if config.ImagePullPolicy != "" {
		pullPolicy = config.ImagePullPolicy
	}

	// Get port
	port := p.port
	if len(config.Ports) > 0 {
		port = config.Ports[0].ContainerPort
	}

	// Create JMX Exporter container
	container := &corev1.Container{
		Name:            p.name,
		Image:           image,
		ImagePullPolicy: pullPolicy,
		Ports: []corev1.ContainerPort{
			{
				Name:          "metrics",
				ContainerPort: port,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Command: []string{
			"java",
			"-jar",
			"/opt/jmx_exporter/jmx_prometheus_httpserver.jar",
			fmt.Sprintf("%d", port),
			JMXExporterConfigMountPath + "/config.yaml",
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      JMXExporterConfigVolumeName,
				MountPath: JMXExporterConfigMountPath,
				ReadOnly:  true,
			},
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/metrics",
					Port: intstr.FromInt(int(port)),
				},
			},
			InitialDelaySeconds: 10,
			TimeoutSeconds:      5,
			PeriodSeconds:       10,
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
			Name: JMXExporterConfigVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: p.configMapName,
					},
				},
			},
		},
	}

	AddVolumes(podSpec, volumes)

	return nil
}
