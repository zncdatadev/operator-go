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
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

// StatefulSetBuilder constructs StatefulSet resources. The pod template — containers, volumes,
// probes, security contexts, the MainContainerCustomizer hook and the podOverrides merge — is
// assembled by the embedded workloadBuilder, which DeploymentBuilder shares, so the two workload
// kinds render identical pods for identical inputs. This builder adds only what a StatefulSet has
// and a Deployment does not: volumeClaimTemplates, the headless serviceName, podManagementPolicy
// and updateStrategy.
type StatefulSetBuilder struct {
	workloadBuilder

	// Storage configuration
	StorageConfig *StorageConfig

	// PodManagementPolicy controls whether the StatefulSet controller starts and stops pods one at
	// a time in ordinal order (OrderedReady) or all at once (Parallel). Empty means the framework
	// default, Parallel — see Build() for why that is the right default here and not merely the
	// inherited one. The field is IMMUTABLE on a live StatefulSet, so it can only be chosen at
	// creation.
	PodManagementPolicy appsv1.PodManagementPolicyType

	// UpdateStrategy controls how the StatefulSet controller rolls a template change. Empty leaves
	// the field unset, which Kubernetes defaults to RollingUpdate with partition 0. Unlike
	// PodManagementPolicy this is mutable, which is what makes a partitioned canary rollout
	// (RollingUpdate with a partition that is lowered step by step) possible on a live cluster.
	UpdateStrategy *appsv1.StatefulSetUpdateStrategy
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
		workloadBuilder: newWorkloadBuilder(name, namespace),
	}
}

// WithLabels sets the labels.
func (b *StatefulSetBuilder) WithLabels(labels map[string]string) *StatefulSetBuilder {
	b.addLabels(labels)
	return b
}

// WithSelectorLabels sets the labels used for the StatefulSet's immutable .spec.selector.
// The labels are cloned (to avoid external mutation) and also merged into the pod template
// labels, enforcing the invariant that the selector is a subset of the template labels —
// otherwise the API server would reject the StatefulSet.
func (b *StatefulSetBuilder) WithSelectorLabels(labels map[string]string) *StatefulSetBuilder {
	b.mergeSelectorLabels(labels)
	return b
}

// podManagementPolicy returns the effective policy: the caller's choice, else the framework
// default. See Build() for why the default is Parallel.
func (b *StatefulSetBuilder) podManagementPolicy() appsv1.PodManagementPolicyType {
	if b.PodManagementPolicy != "" {
		return b.PodManagementPolicy
	}
	return appsv1.ParallelPodManagement
}

// WithMainContainerName names the primary container (default: the StatefulSet name). Set it
// before Build(): the podOverrides strategic merge keys containers by name, so the primary
// container must already carry its final, user-facing name when overrides are applied.
func (b *StatefulSetBuilder) WithMainContainerName(name string) *StatefulSetBuilder {
	b.MainContainerName = name
	return b
}

// WithAnnotations sets the annotations.
func (b *StatefulSetBuilder) WithAnnotations(annotations map[string]string) *StatefulSetBuilder {
	b.addAnnotations(annotations)
	return b
}

// WithReplicas sets the replica count.
func (b *StatefulSetBuilder) WithReplicas(replicas int32) *StatefulSetBuilder {
	b.Replicas = replicas
	return b
}

// WithImage sets the container image.
func (b *StatefulSetBuilder) WithImage(image string, pullPolicy corev1.PullPolicy) *StatefulSetBuilder {
	b.setImage(image, pullPolicy)
	return b
}

// WithConfig sets the merged configuration.
func (b *StatefulSetBuilder) WithConfig(cfg *config.MergedConfig) *StatefulSetBuilder {
	b.Config = cfg
	return b
}

// WithResources sets the resource requirements.
func (b *StatefulSetBuilder) WithResources(resources *v1alpha1.ResourcesSpec) *StatefulSetBuilder {
	b.setResources(resources)
	return b
}

// WithPorts sets the container ports. The slice is copied, so a later AddPort cannot append into
// the caller's backing array (callers commonly pass a slice they keep, e.g. a per-role port list).
func (b *StatefulSetBuilder) WithPorts(ports []corev1.ContainerPort) *StatefulSetBuilder {
	b.Ports = cloneSlice(ports)
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

// WithCommand sets the main container entrypoint command.
func (b *StatefulSetBuilder) WithCommand(command []string) *StatefulSetBuilder {
	b.Command = command
	return b
}

// WithArgs sets the main container args.
func (b *StatefulSetBuilder) WithArgs(args []string) *StatefulSetBuilder {
	b.Args = args
	return b
}

// AddInitContainer appends an init container that runs before the main container.
func (b *StatefulSetBuilder) AddInitContainer(container corev1.Container) *StatefulSetBuilder {
	b.InitContainers = append(b.InitContainers, container)
	return b
}

// WithInitContainers replaces the init containers. The slice is copied for the same reason as
// WithPorts.
func (b *StatefulSetBuilder) WithInitContainers(containers []corev1.Container) *StatefulSetBuilder {
	b.InitContainers = cloneSlice(containers)
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

// WithEnableServiceLinks sets the pod's .spec.enableServiceLinks. The kubedoop framework defaults
// this to false (kubedoop products use DNS + config, never the injected <SVC>_SERVICE_HOST/PORT
// env vars), but the value is fully overridable via per-role-group PodOverrides. When this option
// is never called, the field stays nil and k8s applies its own default (true), keeping direct
// builder users backward compatible.
func (b *StatefulSetBuilder) WithEnableServiceLinks(enable bool) *StatefulSetBuilder {
	b.EnableServiceLinks = &enable
	return b
}

// WithMainContainerCustomizer registers a function that gets the assembled primary container just
// before pod overrides are applied.
//
// It exists because the framework owns the primary container's name, image, ports, security
// context, volume mounts and probes, but nothing else — so every product reached into the
// StatefulSet the framework had just returned and edited it in place, re-deriving the container it
// was handed. zookeeper-operator located it as `Containers[0]`, an assumption the framework has
// never promised: the moment a sidecar provider inserts a container earlier, that silently
// configures the wrong one.
//
// TIMING IS THE POINT, and it is why this cannot be a post-Build patch. The customizer runs AFTER
// the framework has assembled the container and BEFORE podOverrides are strategic-merged, so a
// user's podOverrides still outrank whatever the product set here — a post-Build edit would invert
// that precedence silently.
//
// The image is off limits: it is resolved once and propagated to the sidecars before the
// StatefulSet is built (the Vector agent ships inside the product image), so changing it here would
// leave them on a different one. Build() records that as a violation rather than accepting it; set
// RoleGroupBuildContext.Image instead.
func (b *StatefulSetBuilder) WithMainContainerCustomizer(fn func(*corev1.Container) error) *StatefulSetBuilder {
	b.mainContainerCustomizer = fn
	return b
}

// WithPodManagementPolicy overrides the framework's Parallel default. It must be set before the
// StatefulSet is first created: Kubernetes rejects a change to this field on a live object, and the
// framework's apply path preserves the live value (emitting an ImmutableFieldIgnored warning) for
// exactly that reason.
func (b *StatefulSetBuilder) WithPodManagementPolicy(policy appsv1.PodManagementPolicyType) *StatefulSetBuilder {
	b.PodManagementPolicy = policy
	return b
}

// WithUpdateStrategy sets .spec.updateStrategy — the knob behind a partitioned canary rollout
// (upgrade the highest ordinals first, verify, then lower the partition) and behind OnDelete for a
// fully manual upgrade. It is mutable on a live StatefulSet and is not preserved by the apply path,
// so a change to it converges on the next reconcile.
func (b *StatefulSetBuilder) WithUpdateStrategy(strategy appsv1.StatefulSetUpdateStrategy) *StatefulSetBuilder {
	b.UpdateStrategy = &strategy
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
		StorageClass: ptr.Deref(storage.StorageClass, ""),
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
							// GetCapacity applies DefaultStorageCapacity when the user set none.
							// The default lives here rather than in the CRD schema: a
							// `+kubebuilder:default` is stamped in as soon as the storage block
							// exists, so overriding only storageClass would silently downgrade the
							// capacity the role asked for.
							corev1.ResourceStorage: storage.GetCapacity(),
						},
					},
				},
			},
		},
	}

	if storage.StorageClass != nil {
		// Set whenever the user said something, INCLUDING the empty string: Kubernetes reads
		// `storageClassName: ""` as "no class, bind a pre-provisioned PV", which is a different
		// request from leaving the field out (use the cluster default). Copy rather than point into
		// the caller's spec, which the caller still owns.
		b.StorageConfig.VolumeClaimTemplates[0].Spec.StorageClassName = ptr.To(*storage.StorageClass)
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
	b.setPreStopExec(command)
	return b
}

// WithPreStopHTTPGet sets a preStop HTTP GET hook.
func (b *StatefulSetBuilder) WithPreStopHTTPGet(path string, port int) *StatefulSetBuilder {
	b.setPreStopHTTPGet(path, port)
	return b
}

// WithPostStartHook sets a postStart exec hook.
func (b *StatefulSetBuilder) WithPostStartHook(command []string) *StatefulSetBuilder {
	b.setPostStartExec(command)
	return b
}

// WithLivenessProbe sets the liveness probe on the primary container. There is NO auto-generated
// liveness probe: a liveness probe kills the container, and the framework does not know enough
// about a product to author one (see workloadBuilder.buildLivenessProbe). Products opt in
// explicitly, and DefaultTCPLivenessProbe is available for the plain "is this port accepting
// connections" shape.
func (b *StatefulSetBuilder) WithLivenessProbe(probe *corev1.Probe) *StatefulSetBuilder {
	b.livenessProbe = probe
	b.disableLiveness = false
	return b
}

// DisableLivenessProbe clears a previously set liveness probe. It is now only meaningful after
// WithLivenessProbe, since nothing is generated by default.
func (b *StatefulSetBuilder) DisableLivenessProbe() *StatefulSetBuilder {
	b.disableLiveness = true
	b.livenessProbe = nil
	return b
}

// DefaultTCPLivenessProbe returns the TCP-accept liveness probe this builder used to generate
// automatically, so a product that wants exactly that behavior gets it in one line — on a port it
// chose rather than on whichever port it happened to declare first:
//
//	stsBuilder.WithLivenessProbe(builder.DefaultTCPLivenessProbe(8020))
//
// Its budget (~90-120s to the first kill) is only safe for a product that reaches this port within
// that window. A product whose startup can exceed it must widen the probe or pair it with a
// startupProbe; that is precisely the judgement the framework cannot make on a product's behalf.
func DefaultTCPLivenessProbe(port int32) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt(int(port))},
		},
		InitialDelaySeconds: 30,
		TimeoutSeconds:      10,
		PeriodSeconds:       30,
		SuccessThreshold:    1,
		FailureThreshold:    3,
	}
}

// WithReadinessProbe sets the readiness probe. Pass a custom *corev1.Probe to override
// the default TCP probe. When not called, a default TCP probe is auto-generated if
// ports are configured.
func (b *StatefulSetBuilder) WithReadinessProbe(probe *corev1.Probe) *StatefulSetBuilder {
	b.readinessProbe = probe
	b.disableReadiness = false
	return b
}

// DisableReadinessProbe disables the auto-generated readiness probe.
func (b *StatefulSetBuilder) DisableReadinessProbe() *StatefulSetBuilder {
	b.disableReadiness = true
	b.readinessProbe = nil
	return b
}

// WithStartupProbe sets the startup probe. No startup probe is auto-generated by default.
func (b *StatefulSetBuilder) WithStartupProbe(probe *corev1.Probe) *StatefulSetBuilder {
	b.startupProbe = probe
	b.disableStartup = false
	return b
}

// DisableStartupProbe explicitly opts out of any startup probe.
// Useful to guard against future default behavior or to override a previously set probe.
func (b *StatefulSetBuilder) DisableStartupProbe() *StatefulSetBuilder {
	b.disableStartup = true
	b.startupProbe = nil
	return b
}

// Build creates the StatefulSet.
//
// The returned object shares no mutable state with the builder: every map and slice is copied,
// and ObjectMeta gets a copy independent of the pod template's. Callers mutate the result (the
// reconciler injects sidecar containers and volumes into the pod template, and Build itself
// rewrites the template when pod overrides are applied), so sharing would let a pod-level change
// contaminate ObjectMeta, the immutable .spec.selector, or a second Build() from the same builder.
func (b *StatefulSetBuilder) Build() *appsv1.StatefulSet {
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        b.Name,
			Namespace:   b.Namespace,
			Labels:      maps.Clone(b.Labels),
			Annotations: maps.Clone(b.Annotations),
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    ptr.To(b.Replicas),
			ServiceName: b.Name + "-headless",
			// Parallel is chosen, not inherited. OrderedReady starts pod N+1 only once pod N is
			// Ready, and the quorum systems this SDK is built for cannot satisfy that: a ZooKeeper
			// ensemble member or an HDFS JournalNode does not become Ready until it can see a
			// quorum, and the quorum does not exist until its peers are started — OrderedReady
			// deadlocks the whole role group at pod-0. (The headless Service's
			// publishNotReadyAddresses, which those same products set, exists for the other half of
			// the same problem: peers must resolve each other before any of them is Ready.)
			//
			// Products with the opposite requirement — a strict start order, e.g. a primary that
			// must be up before its replicas — call WithPodManagementPolicy at creation time.
			PodManagementPolicy: b.podManagementPolicy(),
			Selector: &metav1.LabelSelector{
				MatchLabels: maps.Clone(b.selectorMatchLabels()),
			},
			Template: b.buildPodTemplate(),
		},
	}

	// Deep-copied like every other value Build() hands back: the caller must not be able to reach
	// back into the builder's state through the returned object.
	if b.UpdateStrategy != nil {
		sts.Spec.UpdateStrategy = *b.UpdateStrategy.DeepCopy()
	}

	// Add volume claim templates if storage is configured
	if b.StorageConfig != nil {
		sts.Spec.VolumeClaimTemplates = cloneSlice(b.StorageConfig.VolumeClaimTemplates)
	}

	// Apply pod overrides. The claim templates are passed along because their names are mountable
	// without appearing in .spec.volumes — the framework's own data PVC among them.
	b.applyPodOverridesTo(&sts.Spec.Template, sts.Spec.VolumeClaimTemplates)

	return sts
}
