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

package builder

import (
	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/config"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// StatefulSetBuilder constructs StatefulSet resources.
type StatefulSetBuilder struct {
	Name            string
	Namespace       string
	Labels          map[string]string
	Annotations     map[string]string
	Replicas        int32
	Image           string
	ImagePullPolicy corev1.PullPolicy
	Config          *config.MergedConfig
	Ports           []corev1.ContainerPort
	Volumes         []corev1.Volume
	VolumeMounts    []corev1.VolumeMount
	EnvVars         []corev1.EnvVar
	Command         []string
	Args            []string

	// Resource requirements
	Resources *corev1.ResourceRequirements

	// Security context
	SecurityContext    *corev1.SecurityContext
	PodSecurityContext *corev1.PodSecurityContext

	// Affinity
	Affinity *corev1.Affinity

	// Service account
	ServiceAccountName string

	// Storage configuration
	StorageConfig *StorageConfig

	// Pod overrides from merged config
	PodOverrides *corev1.PodTemplateSpec

	// Graceful shutdown timeout
	TerminationGracePeriodSeconds *int64

	// Lifecycle hooks
	lifecycle *corev1.Lifecycle

	// Probes
	livenessProbe  *corev1.Probe
	readinessProbe *corev1.Probe
	startupProbe   *corev1.Probe
}

// StorageConfig defines storage configuration for StatefulSet.
type StorageConfig struct {
	// VolumeClaimTemplates defines PVC templates
	VolumeClaimTemplates []corev1.PersistentVolumeClaim
	// StorageClass for PVCs
	StorageClass string
}

// NewStatefulSetBuilder creates a new StatefulSetBuilder.
func NewStatefulSetBuilder(name, namespace string) *StatefulSetBuilder {
	return &StatefulSetBuilder{
		Name:            name,
		Namespace:       namespace,
		Labels:          make(map[string]string),
		Annotations:     make(map[string]string),
		Replicas:        1,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Volumes:         make([]corev1.Volume, 0),
		VolumeMounts:    make([]corev1.VolumeMount, 0),
		EnvVars:         make([]corev1.EnvVar, 0),
		Ports:           make([]corev1.ContainerPort, 0),
	}
}

// WithLabels sets the labels.
func (b *StatefulSetBuilder) WithLabels(labels map[string]string) *StatefulSetBuilder {
	for k, v := range labels {
		b.Labels[k] = v
	}
	return b
}

// WithAnnotations sets the annotations.
func (b *StatefulSetBuilder) WithAnnotations(annotations map[string]string) *StatefulSetBuilder {
	for k, v := range annotations {
		b.Annotations[k] = v
	}
	return b
}

// WithReplicas sets the replica count.
func (b *StatefulSetBuilder) WithReplicas(replicas int32) *StatefulSetBuilder {
	b.Replicas = replicas
	return b
}

// WithImage sets the container image.
func (b *StatefulSetBuilder) WithImage(image string, pullPolicy corev1.PullPolicy) *StatefulSetBuilder {
	b.Image = image
	if pullPolicy != "" {
		b.ImagePullPolicy = pullPolicy
	}
	return b
}

// WithConfig sets the merged configuration.
func (b *StatefulSetBuilder) WithConfig(cfg *config.MergedConfig) *StatefulSetBuilder {
	b.Config = cfg
	return b
}

// WithResources sets the resource requirements.
func (b *StatefulSetBuilder) WithResources(resources *v1alpha1.ResourcesSpec) *StatefulSetBuilder {
	if resources == nil {
		return b
	}

	req := &corev1.ResourceRequirements{
		Requests: make(corev1.ResourceList),
		Limits:   make(corev1.ResourceList),
	}

	if resources.CPU != nil {
		if !resources.CPU.Min.IsZero() {
			req.Requests[corev1.ResourceCPU] = resources.CPU.Min
		}
		if !resources.CPU.Max.IsZero() {
			req.Limits[corev1.ResourceCPU] = resources.CPU.Max
		}
	}

	if resources.Memory != nil {
		if !resources.Memory.Limit.IsZero() {
			req.Limits[corev1.ResourceMemory] = resources.Memory.Limit
			req.Requests[corev1.ResourceMemory] = resources.Memory.Limit
		}
	}

	b.Resources = req
	return b
}

// WithPorts sets the container ports.
func (b *StatefulSetBuilder) WithPorts(ports []corev1.ContainerPort) *StatefulSetBuilder {
	b.Ports = ports
	return b
}

// AddPort adds a container port.
func (b *StatefulSetBuilder) AddPort(name string, port int32, protocol corev1.Protocol) *StatefulSetBuilder {
	b.Ports = append(b.Ports, corev1.ContainerPort{
		Name:          name,
		ContainerPort: port,
		Protocol:      protocol,
	})
	return b
}

// AddVolume adds a volume.
func (b *StatefulSetBuilder) AddVolume(volume corev1.Volume) *StatefulSetBuilder {
	b.Volumes = append(b.Volumes, volume)
	return b
}

// AddVolumeMount adds a volume mount.
func (b *StatefulSetBuilder) AddVolumeMount(mount corev1.VolumeMount) *StatefulSetBuilder {
	b.VolumeMounts = append(b.VolumeMounts, mount)
	return b
}

// AddEnvVar adds an environment variable.
func (b *StatefulSetBuilder) AddEnvVar(name, value string) *StatefulSetBuilder {
	b.EnvVars = append(b.EnvVars, corev1.EnvVar{
		Name:  name,
		Value: value,
	})
	return b
}

// WithServiceAccount sets the service account name.
func (b *StatefulSetBuilder) WithServiceAccount(name string) *StatefulSetBuilder {
	b.ServiceAccountName = name
	return b
}

// WithAffinity sets the affinity configuration.
func (b *StatefulSetBuilder) WithAffinity(affinity *corev1.Affinity) *StatefulSetBuilder {
	b.Affinity = affinity
	return b
}

// WithSecurityContext sets the security context.
func (b *StatefulSetBuilder) WithSecurityContext(containerCtx *corev1.SecurityContext, podCtx *corev1.PodSecurityContext) *StatefulSetBuilder {
	b.SecurityContext = containerCtx
	b.PodSecurityContext = podCtx
	return b
}

// WithPodOverrides sets the pod template overrides.
func (b *StatefulSetBuilder) WithPodOverrides(overrides *corev1.PodTemplateSpec) *StatefulSetBuilder {
	b.PodOverrides = overrides
	return b
}

// WithStorage sets the storage configuration.
func (b *StatefulSetBuilder) WithStorage(storage *v1alpha1.StorageResource, mountPath string) *StatefulSetBuilder {
	if storage == nil {
		return b
	}

	b.StorageConfig = &StorageConfig{
		StorageClass: storage.StorageClass,
		VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "data",
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: storage.Capacity,
						},
					},
				},
			},
		},
	}

	if storage.StorageClass != "" {
		b.StorageConfig.VolumeClaimTemplates[0].Spec.StorageClassName = &storage.StorageClass
	}

	// Add volume mount for data
	b.VolumeMounts = append(b.VolumeMounts, corev1.VolumeMount{
		Name:      "data",
		MountPath: mountPath,
	})

	return b
}

// WithTerminationGracePeriod sets the termination grace period.
func (b *StatefulSetBuilder) WithTerminationGracePeriod(seconds int64) *StatefulSetBuilder {
	b.TerminationGracePeriodSeconds = &seconds
	return b
}

// WithPreStopHook sets a preStop exec hook.
func (b *StatefulSetBuilder) WithPreStopHook(command []string) *StatefulSetBuilder {
	if b.lifecycle == nil {
		b.lifecycle = &corev1.Lifecycle{}
	}
	b.lifecycle.PreStop = &corev1.LifecycleHandler{
		Exec: &corev1.ExecAction{
			Command: command,
		},
	}
	return b
}

// WithPreStopHTTPGet sets a preStop HTTP GET hook.
func (b *StatefulSetBuilder) WithPreStopHTTPGet(path string, port int) *StatefulSetBuilder {
	if b.lifecycle == nil {
		b.lifecycle = &corev1.Lifecycle{}
	}
	b.lifecycle.PreStop = &corev1.LifecycleHandler{
		HTTPGet: &corev1.HTTPGetAction{
			Path: path,
			Port: intstr.FromInt(port),
		},
	}
	return b
}

// WithPostStartHook sets a postStart exec hook.
func (b *StatefulSetBuilder) WithPostStartHook(command []string) *StatefulSetBuilder {
	if b.lifecycle == nil {
		b.lifecycle = &corev1.Lifecycle{}
	}
	b.lifecycle.PostStart = &corev1.LifecycleHandler{
		Exec: &corev1.ExecAction{
			Command: command,
		},
	}
	return b
}

// WithLivenessProbe sets a custom liveness probe, replacing the default TCP probe.
func (b *StatefulSetBuilder) WithLivenessProbe(probe *corev1.Probe) *StatefulSetBuilder {
	b.livenessProbe = probe
	return b
}

// WithReadinessProbe sets a custom readiness probe, replacing the default TCP probe.
func (b *StatefulSetBuilder) WithReadinessProbe(probe *corev1.Probe) *StatefulSetBuilder {
	b.readinessProbe = probe
	return b
}

// WithStartupProbe sets a startup probe.
func (b *StatefulSetBuilder) WithStartupProbe(probe *corev1.Probe) *StatefulSetBuilder {
	b.startupProbe = probe
	return b
}

// NewTCPSocketProbe creates a TCP socket probe for the given port with the provided timing overrides.
// Zero values in the timing fields are replaced with sensible defaults.
func NewTCPSocketProbe(port int32, initialDelay, timeout, period, success, failure int32) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			TCPSocket: &corev1.TCPSocketAction{
				Port: intstr.FromInt(int(port)),
			},
		},
		InitialDelaySeconds: initialDelay,
		TimeoutSeconds:      timeout,
		PeriodSeconds:       period,
		SuccessThreshold:    success,
		FailureThreshold:    failure,
	}
}

// NewHTTPGetProbe creates an HTTP GET probe for the given path and port with the provided timing overrides.
func NewHTTPGetProbe(path string, port int32, initialDelay, timeout, period, success, failure int32) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: path,
				Port: intstr.FromInt(int(port)),
			},
		},
		InitialDelaySeconds: initialDelay,
		TimeoutSeconds:      timeout,
		PeriodSeconds:       period,
		SuccessThreshold:    success,
		FailureThreshold:    failure,
	}
}

// NewExecProbe creates an exec probe that runs the given command with the provided timing overrides.
func NewExecProbe(command []string, initialDelay, timeout, period, success, failure int32) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: command,
			},
		},
		InitialDelaySeconds: initialDelay,
		TimeoutSeconds:      timeout,
		PeriodSeconds:       period,
		SuccessThreshold:    success,
		FailureThreshold:    failure,
	}
}

// Build creates the StatefulSet.
func (b *StatefulSetBuilder) Build() *appsv1.StatefulSet {
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        b.Name,
			Namespace:   b.Namespace,
			Labels:      b.Labels,
			Annotations: b.Annotations,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:            &b.Replicas,
			ServiceName:         b.Name + "-headless",
			PodManagementPolicy: appsv1.ParallelPodManagement,
			Selector: &metav1.LabelSelector{
				MatchLabels: b.Labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      b.Labels,
					Annotations: b.Annotations,
				},
				Spec: b.buildPodSpec(),
			},
		},
	}

	// Add volume claim templates if storage is configured
	if b.StorageConfig != nil {
		sts.Spec.VolumeClaimTemplates = b.StorageConfig.VolumeClaimTemplates
	}

	// Apply pod overrides
	if b.PodOverrides != nil {
		b.applyPodOverrides(sts)
	}

	return sts
}

// buildPodSpec builds the pod spec.
func (b *StatefulSetBuilder) buildPodSpec() corev1.PodSpec {
	spec := corev1.PodSpec{
		ServiceAccountName:            b.ServiceAccountName,
		TerminationGracePeriodSeconds: b.TerminationGracePeriodSeconds,
		SecurityContext:               b.PodSecurityContext,
		Affinity:                      b.Affinity,
		Volumes:                       b.Volumes,
		Containers: []corev1.Container{
			b.buildContainer(),
		},
	}

	return spec
}

// buildContainer builds the main container.
func (b *StatefulSetBuilder) buildContainer() corev1.Container {
	container := corev1.Container{
		Name:            b.Name,
		Image:           b.Image,
		ImagePullPolicy: b.ImagePullPolicy,
		Ports:           b.Ports,
		VolumeMounts:    b.VolumeMounts,
		SecurityContext: b.SecurityContext,
	}

	// Set resources if provided
	if b.Resources != nil {
		container.Resources = *b.Resources
	}

	// Set command and args
	if len(b.Command) > 0 {
		container.Command = b.Command
	}
	if len(b.Args) > 0 {
		container.Args = b.Args
	}

	// Add environment variables from merged config
	if b.Config != nil {
		for k, v := range b.Config.EnvVars {
			container.Env = append(container.Env, corev1.EnvVar{
				Name:  k,
				Value: v,
			})
		}
		// Add CLI args
		if len(b.Config.CliArgs) > 0 {
			container.Args = append(container.Args, b.Config.CliArgs...)
		}
	}

	// Add explicit env vars (these override config env vars)
	container.Env = append(container.Env, b.EnvVars...)

	// Apply lifecycle hooks
	if b.lifecycle != nil {
		container.Lifecycle = b.lifecycle
	}

	// Setup probes
	container.LivenessProbe = b.buildLivenessProbe()
	container.ReadinessProbe = b.buildReadinessProbe()
	container.StartupProbe = b.buildStartupProbe()

	return container
}

// buildLivenessProbe returns the configured liveness probe.
// If a custom probe was set via WithLivenessProbe it is returned as-is.
// Otherwise a default TCP socket probe using the first port is returned (nil when no ports are defined).
func (b *StatefulSetBuilder) buildLivenessProbe() *corev1.Probe {
	if b.livenessProbe != nil {
		return b.livenessProbe
	}

	if len(b.Ports) == 0 {
		return nil
	}

	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			TCPSocket: &corev1.TCPSocketAction{
				Port: intstr.FromInt(int(b.Ports[0].ContainerPort)),
			},
		},
		InitialDelaySeconds: 30,
		TimeoutSeconds:      10,
		PeriodSeconds:       30,
		SuccessThreshold:    1,
		FailureThreshold:    3,
	}
}

// buildReadinessProbe returns the configured readiness probe.
// If a custom probe was set via WithReadinessProbe it is returned as-is.
// Otherwise a default TCP socket probe using the first port is returned (nil when no ports are defined).
func (b *StatefulSetBuilder) buildReadinessProbe() *corev1.Probe {
	if b.readinessProbe != nil {
		return b.readinessProbe
	}

	if len(b.Ports) == 0 {
		return nil
	}

	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			TCPSocket: &corev1.TCPSocketAction{
				Port: intstr.FromInt(int(b.Ports[0].ContainerPort)),
			},
		},
		InitialDelaySeconds: 10,
		TimeoutSeconds:      5,
		PeriodSeconds:       10,
		SuccessThreshold:    1,
		FailureThreshold:    3,
	}
}

// buildStartupProbe returns the startup probe if one was set via WithStartupProbe, otherwise nil.
func (b *StatefulSetBuilder) buildStartupProbe() *corev1.Probe {
	return b.startupProbe
}

// applyPodOverrides applies pod template overrides to the StatefulSet.
func (b *StatefulSetBuilder) applyPodOverrides(sts *appsv1.StatefulSet) {
	if b.PodOverrides == nil {
		return
	}

	// Override annotations
	if len(b.PodOverrides.Annotations) > 0 {
		if sts.Spec.Template.Annotations == nil {
			sts.Spec.Template.Annotations = make(map[string]string)
		}
		for k, v := range b.PodOverrides.Annotations {
			sts.Spec.Template.Annotations[k] = v
		}
	}

	// Override labels
	if len(b.PodOverrides.Labels) > 0 {
		if sts.Spec.Template.Labels == nil {
			sts.Spec.Template.Labels = make(map[string]string)
		}
		for k, v := range b.PodOverrides.Labels {
			sts.Spec.Template.Labels[k] = v
		}
	}

	// Override affinity
	if b.PodOverrides.Spec.Affinity != nil {
		sts.Spec.Template.Spec.Affinity = b.PodOverrides.Spec.Affinity
	}

	// Override tolerations
	if len(b.PodOverrides.Spec.Tolerations) > 0 {
		sts.Spec.Template.Spec.Tolerations = b.PodOverrides.Spec.Tolerations
	}

	// Override node selector
	if len(b.PodOverrides.Spec.NodeSelector) > 0 {
		sts.Spec.Template.Spec.NodeSelector = b.PodOverrides.Spec.NodeSelector
	}

	// Override priority class name
	if b.PodOverrides.Spec.PriorityClassName != "" {
		sts.Spec.Template.Spec.PriorityClassName = b.PodOverrides.Spec.PriorityClassName
	}
}

// NamespacedName returns the NamespacedName for the StatefulSet.
func (b *StatefulSetBuilder) NamespacedName() types.NamespacedName {
	return types.NamespacedName{
		Name:      b.Name,
		Namespace: b.Namespace,
	}
}
