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
	"maps"

	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/config"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

// DeploymentBuilder constructs Deployment resources for the roles whose pods are interchangeable —
// a stateless UI, an API gateway — where a StatefulSet's per-pod identity would only slow rollouts
// down. The pod template is assembled by the embedded workloadBuilder, the same implementation
// StatefulSetBuilder uses, so a Deployment role renders exactly the pod a StatefulSet role would
// for identical inputs: same containers, same volumes, same security contexts, same
// MainContainerCustomizer timing, same podOverrides precedence.
//
// What a Deployment does NOT have, this builder deliberately does not offer:
//
//   - no volumeClaimTemplates — a Deployment's pods share nothing and own no PVCs, which is why
//     BaseRoleGroupHandler.StorageMountPath is ignored for Deployment roles;
//   - no serviceName — there is no per-pod DNS identity for a headless Service to provide;
//   - no podManagementPolicy, and no update strategy field either: the Kubernetes default
//     (RollingUpdate, 25% maxUnavailable / 25% maxSurge) is the right rollout for interchangeable
//     pods, so Build() leaves .spec.strategy unset rather than restating it.
//
// The .spec.selector is immutable after creation, exactly as a StatefulSet's is; the apply path
// preserves the live value and warns when the desired one differs (see copyDeploymentState).
type DeploymentBuilder struct {
	workloadBuilder
}

// NewDeploymentBuilder creates a new DeploymentBuilder.
func NewDeploymentBuilder(name, namespace string) *DeploymentBuilder {
	return &DeploymentBuilder{
		workloadBuilder: newWorkloadBuilder(name, namespace),
	}
}

// WithLabels sets the labels.
func (b *DeploymentBuilder) WithLabels(labels map[string]string) *DeploymentBuilder {
	b.addLabels(labels)
	return b
}

// WithSelectorLabels sets the labels used for the Deployment's immutable .spec.selector.
// The labels are cloned (to avoid external mutation) and also merged into the pod template
// labels, enforcing the invariant that the selector is a subset of the template labels —
// otherwise the API server would reject the Deployment.
func (b *DeploymentBuilder) WithSelectorLabels(labels map[string]string) *DeploymentBuilder {
	b.mergeSelectorLabels(labels)
	return b
}

// WithMainContainerName names the primary container (default: the Deployment name). Set it
// before Build(): the podOverrides strategic merge keys containers by name, so the primary
// container must already carry its final, user-facing name when overrides are applied.
func (b *DeploymentBuilder) WithMainContainerName(name string) *DeploymentBuilder {
	b.MainContainerName = name
	return b
}

// WithAnnotations sets the annotations.
func (b *DeploymentBuilder) WithAnnotations(annotations map[string]string) *DeploymentBuilder {
	b.addAnnotations(annotations)
	return b
}

// WithReplicas sets the replica count.
func (b *DeploymentBuilder) WithReplicas(replicas int32) *DeploymentBuilder {
	b.Replicas = replicas
	return b
}

// WithImage sets the container image.
func (b *DeploymentBuilder) WithImage(image string, pullPolicy corev1.PullPolicy) *DeploymentBuilder {
	b.setImage(image, pullPolicy)
	return b
}

// WithConfig sets the merged configuration.
func (b *DeploymentBuilder) WithConfig(cfg *config.MergedConfig) *DeploymentBuilder {
	b.Config = cfg
	return b
}

// WithResources sets the resource requirements.
func (b *DeploymentBuilder) WithResources(resources *v1alpha1.ResourcesSpec) *DeploymentBuilder {
	b.setResources(resources)
	return b
}

// WithPorts sets the container ports. The slice is copied, so a later AddPort cannot append into
// the caller's backing array (callers commonly pass a slice they keep, e.g. a per-role port list).
func (b *DeploymentBuilder) WithPorts(ports []corev1.ContainerPort) *DeploymentBuilder {
	b.Ports = cloneSlice(ports)
	return b
}

// AddPort adds a container port.
func (b *DeploymentBuilder) AddPort(name string, port int32, protocol corev1.Protocol) *DeploymentBuilder {
	b.Ports = append(b.Ports, corev1.ContainerPort{
		Name:          name,
		ContainerPort: port,
		Protocol:      protocol,
	})
	return b
}

// AddVolume adds a volume.
func (b *DeploymentBuilder) AddVolume(volume corev1.Volume) *DeploymentBuilder {
	b.Volumes = append(b.Volumes, volume)
	return b
}

// AddVolumeMount adds a volume mount.
func (b *DeploymentBuilder) AddVolumeMount(mount corev1.VolumeMount) *DeploymentBuilder {
	b.VolumeMounts = append(b.VolumeMounts, mount)
	return b
}

// AddEnvVar adds an environment variable.
func (b *DeploymentBuilder) AddEnvVar(name, value string) *DeploymentBuilder {
	b.EnvVars = append(b.EnvVars, corev1.EnvVar{
		Name:  name,
		Value: value,
	})
	return b
}

// WithCommand sets the main container entrypoint command.
func (b *DeploymentBuilder) WithCommand(command []string) *DeploymentBuilder {
	b.Command = command
	return b
}

// WithArgs sets the main container args.
func (b *DeploymentBuilder) WithArgs(args []string) *DeploymentBuilder {
	b.Args = args
	return b
}

// AddInitContainer appends an init container that runs before the main container.
func (b *DeploymentBuilder) AddInitContainer(container corev1.Container) *DeploymentBuilder {
	b.InitContainers = append(b.InitContainers, container)
	return b
}

// WithInitContainers replaces the init containers. The slice is copied for the same reason as
// WithPorts.
func (b *DeploymentBuilder) WithInitContainers(containers []corev1.Container) *DeploymentBuilder {
	b.InitContainers = cloneSlice(containers)
	return b
}

// WithServiceAccount sets the service account name.
func (b *DeploymentBuilder) WithServiceAccount(name string) *DeploymentBuilder {
	b.ServiceAccountName = name
	return b
}

// WithAffinity sets the affinity configuration.
func (b *DeploymentBuilder) WithAffinity(affinity *corev1.Affinity) *DeploymentBuilder {
	b.Affinity = affinity
	return b
}

// WithSecurityContext sets the security context.
func (b *DeploymentBuilder) WithSecurityContext(containerCtx *corev1.SecurityContext, podCtx *corev1.PodSecurityContext) *DeploymentBuilder {
	b.SecurityContext = containerCtx
	b.PodSecurityContext = podCtx
	return b
}

// WithEnableServiceLinks sets the pod's .spec.enableServiceLinks. The kubedoop framework defaults
// this to false (kubedoop products use DNS + config, never the injected <SVC>_SERVICE_HOST/PORT
// env vars), but the value is fully overridable via per-role-group PodOverrides. When this option
// is never called, the field stays nil and k8s applies its own default (true), keeping direct
// builder users backward compatible.
func (b *DeploymentBuilder) WithEnableServiceLinks(enable bool) *DeploymentBuilder {
	b.EnableServiceLinks = &enable
	return b
}

// WithMainContainerCustomizer registers a function that gets the assembled primary container just
// before pod overrides are applied. The semantics — and above all the TIMING — are identical to
// StatefulSetBuilder.WithMainContainerCustomizer: the customizer runs after the framework has
// assembled the container and before podOverrides are strategic-merged, so a user's podOverrides
// still outrank whatever the product set here, and an image change is recorded as a violation
// rather than accepted (set RoleGroupBuildContext.Image instead).
func (b *DeploymentBuilder) WithMainContainerCustomizer(fn func(*corev1.Container) error) *DeploymentBuilder {
	b.mainContainerCustomizer = fn
	return b
}

// WithPodOverrides sets the pod template overrides.
func (b *DeploymentBuilder) WithPodOverrides(overrides *corev1.PodTemplateSpec) *DeploymentBuilder {
	b.PodOverrides = overrides
	return b
}

// WithTerminationGracePeriod sets the termination grace period.
func (b *DeploymentBuilder) WithTerminationGracePeriod(seconds int64) *DeploymentBuilder {
	b.TerminationGracePeriodSeconds = &seconds
	return b
}

// WithPreStopHook sets a preStop exec hook.
func (b *DeploymentBuilder) WithPreStopHook(command []string) *DeploymentBuilder {
	b.setPreStopExec(command)
	return b
}

// WithPreStopHTTPGet sets a preStop HTTP GET hook.
func (b *DeploymentBuilder) WithPreStopHTTPGet(path string, port int) *DeploymentBuilder {
	b.setPreStopHTTPGet(path, port)
	return b
}

// WithPostStartHook sets a postStart exec hook.
func (b *DeploymentBuilder) WithPostStartHook(command []string) *DeploymentBuilder {
	b.setPostStartExec(command)
	return b
}

// WithLivenessProbe sets the liveness probe on the primary container. There is NO auto-generated
// liveness probe — see workloadBuilder.buildLivenessProbe for why the framework refuses to guess
// one. DefaultTCPLivenessProbe is available for the plain "is this port accepting connections"
// shape.
func (b *DeploymentBuilder) WithLivenessProbe(probe *corev1.Probe) *DeploymentBuilder {
	b.livenessProbe = probe
	b.disableLiveness = false
	return b
}

// DisableLivenessProbe clears a previously set liveness probe. It is only meaningful after
// WithLivenessProbe, since nothing is generated by default.
func (b *DeploymentBuilder) DisableLivenessProbe() *DeploymentBuilder {
	b.disableLiveness = true
	b.livenessProbe = nil
	return b
}

// WithReadinessProbe sets the readiness probe. Pass a custom *corev1.Probe to override
// the default TCP probe. When not called, a default TCP probe is auto-generated if
// ports are configured.
func (b *DeploymentBuilder) WithReadinessProbe(probe *corev1.Probe) *DeploymentBuilder {
	b.readinessProbe = probe
	b.disableReadiness = false
	return b
}

// DisableReadinessProbe disables the auto-generated readiness probe.
func (b *DeploymentBuilder) DisableReadinessProbe() *DeploymentBuilder {
	b.disableReadiness = true
	b.readinessProbe = nil
	return b
}

// WithStartupProbe sets the startup probe. No startup probe is auto-generated by default.
func (b *DeploymentBuilder) WithStartupProbe(probe *corev1.Probe) *DeploymentBuilder {
	b.startupProbe = probe
	b.disableStartup = false
	return b
}

// DisableStartupProbe explicitly opts out of any startup probe.
func (b *DeploymentBuilder) DisableStartupProbe() *DeploymentBuilder {
	b.disableStartup = true
	b.startupProbe = nil
	return b
}

// Build creates the Deployment.
//
// The returned object shares no mutable state with the builder, for the same reason
// StatefulSetBuilder.Build documents: callers mutate the result (the reconciler injects sidecar
// containers and volumes into the pod template), and shared state would let that mutation reach
// ObjectMeta, the immutable .spec.selector, or a second Build() from the same builder.
func (b *DeploymentBuilder) Build() *appsv1.Deployment {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        b.Name,
			Namespace:   b.Namespace,
			Labels:      maps.Clone(b.Labels),
			Annotations: maps.Clone(b.Annotations),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(b.Replicas),
			Selector: &metav1.LabelSelector{
				MatchLabels: maps.Clone(b.selectorMatchLabels()),
			},
			Template: b.buildPodTemplate(),
			// .spec.strategy is deliberately left unset: Kubernetes defaults it to RollingUpdate
			// with 25% maxUnavailable / 25% maxSurge, which is exactly the rollout interchangeable
			// pods want, and restating a server default here would only pin values the platform is
			// free to evolve.
		},
	}

	// A Deployment has no volumeClaimTemplates, so the overrides are checked against the pod
	// volumes alone.
	b.applyPodOverridesTo(&deployment.Spec.Template, nil)

	return deployment
}
