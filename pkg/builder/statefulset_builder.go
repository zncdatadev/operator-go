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
	"fmt"
	"maps"
	"slices"
	"sort"

	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/config"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

// StatefulSetBuilder constructs StatefulSet resources.
type StatefulSetBuilder struct {
	Name      string
	Namespace string
	// MainContainerName, when set, names the primary container (default: Name). It must be
	// set BEFORE Build() so podOverrides addressing the container by its user-facing name
	// strategic-merge into it instead of appending a phantom container.
	MainContainerName string
	Labels            map[string]string
	// SelectorLabels, when set, is used for the StatefulSet's immutable .spec.selector
	// (and must be a subset of Labels, which are applied to the pod template). When empty,
	// Labels is used for the selector. Decoupling the selector from the full descriptive
	// labels keeps the immutable selector stable and free of user-mutable labels.
	SelectorLabels  map[string]string
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

	// InitContainers are run before the main container starts. Products use these for
	// one-shot preparation steps (e.g. generating node ids, fetching secrets).
	InitContainers []corev1.Container

	// Resource requirements
	Resources *corev1.ResourceRequirements

	// Security context
	SecurityContext    *corev1.SecurityContext
	PodSecurityContext *corev1.PodSecurityContext

	// EnableServiceLinks controls the pod's .spec.enableServiceLinks. When nil, the field is left
	// unset (backward compatible — direct builder users are unaffected and k8s applies its own
	// default of true). Callers set it via WithEnableServiceLinks; per-role-group PodOverrides can
	// still override it (see applyPodOverrides).
	EnableServiceLinks *bool

	// Affinity
	Affinity *corev1.Affinity

	// Service account
	ServiceAccountName string

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

	// Pod overrides from merged config
	PodOverrides *corev1.PodTemplateSpec

	// podOverrideViolations records the framework invariants the applied podOverrides broke.
	// Build() cannot return an error, so they are collected here and read back through
	// PodOverrideViolations() by the caller, which fails the role group.
	podOverrideViolations []error

	// Graceful shutdown timeout
	TerminationGracePeriodSeconds *int64

	// Lifecycle hooks
	lifecycle *corev1.Lifecycle

	// Probe configuration
	livenessProbe    *corev1.Probe
	readinessProbe   *corev1.Probe
	startupProbe     *corev1.Probe
	disableLiveness  bool
	disableReadiness bool
	disableStartup   bool
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

// WithSelectorLabels sets the labels used for the StatefulSet's immutable .spec.selector.
// The labels are cloned (to avoid external mutation) and also merged into the pod template
// labels, enforcing the invariant that the selector is a subset of the template labels —
// otherwise the API server would reject the StatefulSet.
func (b *StatefulSetBuilder) WithSelectorLabels(labels map[string]string) *StatefulSetBuilder {
	b.SelectorLabels = make(map[string]string, len(labels))
	for k, v := range labels {
		b.SelectorLabels[k] = v
		b.Labels[k] = v
	}
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

// selectorMatchLabels returns the labels for .spec.selector, falling back to the full
// labels when no dedicated selector labels are set.
func (b *StatefulSetBuilder) selectorMatchLabels() map[string]string {
	if len(b.SelectorLabels) > 0 {
		return b.SelectorLabels
	}
	return b.Labels
}

// WithMainContainerName names the primary container (default: the StatefulSet name). Set it
// before Build(): the podOverrides strategic merge keys containers by name, so the primary
// container must already carry its final, user-facing name when overrides are applied.
func (b *StatefulSetBuilder) WithMainContainerName(name string) *StatefulSetBuilder {
	b.MainContainerName = name
	return b
}

// primaryContainerName returns the effective primary container name.
func (b *StatefulSetBuilder) primaryContainerName() string {
	if b.MainContainerName != "" {
		return b.MainContainerName
	}
	return b.Name
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

	// Nil means unset, and only an unset field is skipped. An explicit zero is honoured: a user
	// who writes `min: "0"` is asking for no CPU request, which is a legitimate thing to ask for
	// on a burstable workload, and the previous IsZero() check silently ignored it.
	if resources.CPU != nil {
		if resources.CPU.Min != nil {
			req.Requests[corev1.ResourceCPU] = *resources.CPU.Min
		}
		if resources.CPU.Max != nil {
			req.Limits[corev1.ResourceCPU] = *resources.CPU.Max
		}
	}

	if resources.Memory != nil && resources.Memory.Limit != nil {
		req.Limits[corev1.ResourceMemory] = *resources.Memory.Limit
		req.Requests[corev1.ResourceMemory] = *resources.Memory.Limit
	}

	b.Resources = req
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

// PodOverrideViolations returns the framework invariants the applied podOverrides broke — a
// framework-owned volume mount displaced or deleted, or a mount left referencing no declared
// volume. It is populated by Build() — which resets it first, so the result always describes the
// most recent build — so call it afterwards. Empty means the merge preserved everything the
// framework mounted.
//
// The slice is copied out, like every other value Build() hands back: the builder must not share
// mutable state with its callers.
func (b *StatefulSetBuilder) PodOverrideViolations() []error {
	if len(b.podOverrideViolations) == 0 {
		return nil
	}
	return append([]error(nil), b.podOverrideViolations...)
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

// WithLivenessProbe sets the liveness probe on the primary container. There is NO auto-generated
// liveness probe: a liveness probe kills the container, and the framework does not know enough
// about a product to author one (see buildLivenessProbe). Products opt in explicitly, and
// DefaultTCPLivenessProbe is available for the plain "is this port accepting connections" shape.
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
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      maps.Clone(b.Labels),
					Annotations: maps.Clone(b.Annotations),
				},
				Spec: b.buildPodSpec(),
			},
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

	// Apply pod overrides. The violation list describes THIS build, so it is reset first —
	// unconditionally, or a builder reused after dropping its overrides would keep reporting the
	// previous build's, and a second Build() would report each violation twice.
	b.podOverrideViolations = nil
	if b.PodOverrides != nil {
		b.applyPodOverrides(sts)
	}

	return sts
}

// buildPodSpec builds the pod spec.
func (b *StatefulSetBuilder) buildPodSpec() corev1.PodSpec {
	spec := corev1.PodSpec{
		ServiceAccountName:            b.ServiceAccountName,
		TerminationGracePeriodSeconds: clonePtr(b.TerminationGracePeriodSeconds),
		SecurityContext:               b.PodSecurityContext.DeepCopy(),
		Affinity:                      b.Affinity.DeepCopy(),
		Volumes:                       cloneSlice(b.Volumes),
		InitContainers:                cloneSlice(b.InitContainers),
		Containers: []corev1.Container{
			b.buildContainer(),
		},
	}

	// Only set EnableServiceLinks when explicitly configured, so direct builder users that never
	// call WithEnableServiceLinks are unaffected (k8s applies its own default of true).
	if b.EnableServiceLinks != nil {
		spec.EnableServiceLinks = clonePtr(b.EnableServiceLinks)
	}

	return spec
}

// buildContainer builds the main container.
func (b *StatefulSetBuilder) buildContainer() corev1.Container {
	container := corev1.Container{
		Name:            b.primaryContainerName(),
		Image:           b.Image,
		ImagePullPolicy: b.ImagePullPolicy,
		Ports:           cloneSlice(b.Ports),
		VolumeMounts:    cloneSlice(b.VolumeMounts),
		SecurityContext: b.SecurityContext.DeepCopy(),
	}

	// Set resources if provided
	if b.Resources != nil {
		container.Resources = *b.Resources.DeepCopy()
	}

	// Set command and args
	if len(b.Command) > 0 {
		container.Command = slices.Clone(b.Command)
	}
	if len(b.Args) > 0 {
		container.Args = slices.Clone(b.Args)
	}

	// Add environment variables from merged config. Iterate in sorted key order: EnvVars is a
	// map, and Go map iteration order is randomized, so appending directly would produce a
	// different container.Env ordering on every reconcile. That makes the rendered StatefulSet
	// differ each time, so CreateOrUpdate issues an endless stream of no-op updates (the pods are
	// recreated on every reconcile and never stabilize).
	if b.Config != nil {
		envKeys := make([]string, 0, len(b.Config.EnvVars))
		for k := range b.Config.EnvVars {
			envKeys = append(envKeys, k)
		}
		sort.Strings(envKeys)
		for _, k := range envKeys {
			container.Env = append(container.Env, corev1.EnvVar{
				Name:  k,
				Value: b.Config.EnvVars[k],
			})
		}
		// Add CLI args
		if len(b.Config.CliArgs) > 0 {
			container.Args = append(container.Args, b.Config.CliArgs...)
		}
	}

	// Add explicit env vars (these override config env vars)
	container.Env = append(container.Env, cloneSlice(b.EnvVars)...)

	// Apply lifecycle hooks
	if b.lifecycle != nil {
		container.Lifecycle = b.lifecycle.DeepCopy()
	}

	// Setup probes
	container.LivenessProbe = b.buildLivenessProbe()
	container.ReadinessProbe = b.buildReadinessProbe()
	container.StartupProbe = b.buildStartupProbe()

	return container
}

// buildLivenessProbe returns only what the caller set. Nothing is generated.
//
// The builder used to author a TCP liveness probe on b.Ports[0] whenever any port was declared,
// and that is not a decision the framework is in a position to make. It knows neither which port
// means "healthy" — the first declared port is an accident of the product's declaration order, and
// is just as likely to be a metrics port as the service port — nor how long the product takes to
// reach it. The generated budget was ~90-120s to the first kill, which is inside the startup time
// of several products this SDK exists for (a NameNode loading an fsimage, a broker replaying log
// segments), so the probe restarted a container that was doing exactly what it should. The result
// is a CrashLoopBackOff whose cause appears nowhere: the user never wrote the probe.
//
// This is not the same call as the sidecar probes in pkg/sidecar, which the framework DOES author.
// There it owns the container: it chose the image, the port and the endpoint, so it can say what
// healthy means. Here the container belongs to the product, and a guess that kills is worse than
// no probe at all — an unprobed wedged process is at least still visible and still serving whatever
// it can, whereas a wrongly-probed healthy process is killed on a timer forever.
//
// Products opt in with WithLivenessProbe; DefaultTCPLivenessProbe reproduces the old shape on a
// port of their choosing.
func (b *StatefulSetBuilder) buildLivenessProbe() *corev1.Probe {
	if b.disableLiveness || b.livenessProbe == nil {
		return nil
	}
	return b.livenessProbe.DeepCopy()
}

// buildReadinessProbe determines the readiness probe based on configuration.
func (b *StatefulSetBuilder) buildReadinessProbe() *corev1.Probe {
	if b.disableReadiness {
		return nil
	}
	if b.readinessProbe != nil {
		return b.readinessProbe.DeepCopy()
	}
	return b.buildDefaultTCPReadinessProbe()
}

// buildDefaultTCPReadinessProbe creates the default TCP readiness probe on the FIRST declared port.
//
// Readiness is kept — unlike liveness — because its two failure modes are the acceptable ones. A
// readiness probe cannot kill anything: at worst the pod stays out of its Services, which is
// visible in `kubectl get pods` as 0/1 and reverses itself the moment the port opens. And removing
// it would not leave "no opinion", it would assert the opposite one: with no readiness probe a pod
// is Ready the instant its container starts, so a rolling update walks the whole role group without
// ever waiting for a member to actually come up. For a data system that is worse than a probe on an
// imperfect port.
//
// It targets b.Ports[0], which makes the first entry of BaseRoleGroupHandler.SetRoleContainerPorts
// (or WithPorts) a real part of the contract rather than an accident: put the port that means "this
// pod can serve" first. Products needing anything else call WithReadinessProbe.
func (b *StatefulSetBuilder) buildDefaultTCPReadinessProbe() *corev1.Probe {
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

// buildStartupProbe determines the startup probe based on configuration.
// By default, no startup probe is generated — it is application-specific.
func (b *StatefulSetBuilder) buildStartupProbe() *corev1.Probe {
	if b.disableStartup {
		return nil
	}
	if b.startupProbe != nil {
		return b.startupProbe.DeepCopy()
	}
	return nil
}

// applyPodOverrides applies pod template overrides to the StatefulSet with full fidelity: the
// merged podOverrides template is strategic-merge-patched onto the built pod template, exactly
// as kubectl would merge it. Containers merge by name (env/ports/volumeMounts within a container
// merge by their own keys), so container-level overrides — resources, env, extra volume mounts —
// land on the built containers instead of being dropped, and override-only containers are
// appended as additional pod containers. Scalar and struct fields set in the override replace or
// deep-merge into the base per strategic-merge-patch semantics; fields the override omits keep
// the built values (so podOverrides.spec.terminationGracePeriodSeconds still wins over the
// config-declared gracefulShutdownTimeout, and enableServiceLinks still wins over the framework
// default, only when actually set).
//
// Security contexts deep-merge per field rather than being replaced wholesale: an override
// stating only runAsUser keeps the framework-hardened remainder. Overrides that need to unset a
// field must state it explicitly.
//
// Kubernetes' mutually exclusive ("one of") structs are the exception: an override that states a
// different volume source, probe handler, lifecycle handler or env var source than the framework
// replaces it wholesale instead of merging into it (see clearSupersededUnions), because a merged
// object with two members set is rejected by the API server.
//
// After the merge the selector labels are re-asserted on the pod template, so an override can
// never break the invariant that the immutable .spec.selector matches the template labels.
func (b *StatefulSetBuilder) applyPodOverrides(sts *appsv1.StatefulSet) {
	if b.PodOverrides == nil {
		return
	}

	override := b.PodOverrides.DeepCopy()
	// PodSpec.Containers is the one PodSpec field without omitempty: a nil slice marshals as
	// "containers": null, which strategic merge treats as a DELETE directive — an
	// annotations-only override would wipe every container. Normalize nil to empty (an empty
	// merge-strategy list is a no-op).
	if override.Spec.Containers == nil {
		override.Spec.Containers = []corev1.Container{}
	}
	// Back-compat: an unnamed override container has always addressed the main container.
	// Normalize the name before the merge — an empty name would otherwise be treated as a
	// distinct merge key and appended as a broken extra container.
	for i := range override.Spec.Containers {
		if override.Spec.Containers[i].Name == "" {
			override.Spec.Containers[i].Name = b.primaryContainerName()
		}
	}

	// Resolve on a copy the one-of collisions the patch format cannot express, so an override
	// naming a framework-owned volume (or probe handler, or env var source) replaces it instead of
	// producing an object with two members set. The copy keeps the fallback below meaningful: a
	// failed merge must fall back to the fully built template, not to a stripped one.
	base := sts.Spec.Template.DeepCopy()
	clearSupersededUnions(base, override)

	merged, err := strategicMergePodTemplate(base, override)
	if err != nil {
		// Unreachable through the public API, and deliberately still reported. Every step of
		// strategicMergePodTemplate operates on a valid *corev1.PodTemplateSpec: marshalling one
		// cannot fail, the patch metadata comes from a constant type, and the final unmarshal is
		// the inverse of the marshal that just succeeded. It carries NO spec for that reason —
		// inventing a fixture that does not actually reach it would only claim coverage.
		//
		// It records rather than returns because "unreachable" is not "silent". Discarding a user's
		// whole podOverrides without a word is exactly what PodOverrideViolations exists to prevent,
		// and a refactor that made this reachable would otherwise reopen the hole at the one point
		// nobody is watching. The built template is kept — a half-merged one would be worse.
		b.podOverrideViolations = append(b.podOverrideViolations,
			fmt.Errorf("podOverrides could not be applied and were dropped: %w", err))
		return
	}
	sts.Spec.Template = *merged

	// The merge keys volumeMounts by mountPath, so an override can displace a framework mount
	// without ever naming it. Record what it broke; the caller decides what to do about it.
	b.podOverrideViolations = append(b.podOverrideViolations,
		checkPodOverrideMountInvariants(base, merged, sts.Spec.VolumeClaimTemplates)...)

	// Re-assert the selector labels: the override may have replaced or removed labels the
	// immutable .spec.selector matches on.
	if sts.Spec.Template.Labels == nil {
		sts.Spec.Template.Labels = make(map[string]string)
	}
	for k, v := range b.selectorMatchLabels() {
		sts.Spec.Template.Labels[k] = v
	}
}

// NamespacedName returns the NamespacedName for the StatefulSet.
func (b *StatefulSetBuilder) NamespacedName() types.NamespacedName {
	return types.NamespacedName{
		Name:      b.Name,
		Namespace: b.Namespace,
	}
}
