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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// workloadBuilder is the shared pod-template half of StatefulSetBuilder and DeploymentBuilder.
// Both embed it, so the primary container, its probes, the volumes, the security contexts, the
// MainContainerCustomizer hook and the podOverrides strategic merge are assembled by exactly one
// implementation — the two workload kinds may not drift in how they build a pod, only in the
// workload-level fields wrapped around it (volumeClaimTemplates, serviceName, podManagementPolicy
// and updateStrategy are StatefulSet-only; a Deployment has none of them).
//
// It is deliberately unexported: products construct one of the concrete builders and never this
// type. The fluent With*/Add* methods therefore live on the concrete builders (they must return
// the concrete type to chain), delegating to the unexported helpers below whenever the step is
// more than a field assignment.
type workloadBuilder struct {
	Name      string
	Namespace string
	// MainContainerName, when set, names the primary container (default: Name). It must be
	// set BEFORE Build() so podOverrides addressing the container by its user-facing name
	// strategic-merge into it instead of appending a phantom container.
	MainContainerName string
	Labels            map[string]string
	// SelectorLabels, when set, is used for the workload's immutable .spec.selector
	// (and must be a subset of Labels, which are applied to the pod template). When empty,
	// Labels is used for the selector. Decoupling the selector from the full descriptive
	// labels keeps the immutable selector stable and free of user-mutable labels. The
	// StatefulSet and Deployment selectors are equally immutable, which is why this lives here.
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
	// still override it (see applyPodOverridesTo).
	EnableServiceLinks *bool

	// Affinity
	Affinity *corev1.Affinity

	// Service account
	ServiceAccountName string

	// Pod overrides from merged config
	PodOverrides *corev1.PodTemplateSpec

	// podOverrideViolations records the framework invariants the applied podOverrides broke.
	// Build() cannot return an error, so they are collected here and read back through
	// PodOverrideViolations() by the caller, which fails the role group.
	podOverrideViolations []error

	// mainContainerCustomizer is the product's hook into the assembled primary container; see
	// WithMainContainerCustomizer on the concrete builders. mainContainerViolations records what
	// it got wrong.
	mainContainerCustomizer func(*corev1.Container) error
	mainContainerViolations []error

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

// newWorkloadBuilder returns the shared defaults both concrete constructors start from.
func newWorkloadBuilder(name, namespace string) workloadBuilder {
	return workloadBuilder{
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

// addLabels merges labels into the builder's label map.
func (b *workloadBuilder) addLabels(labels map[string]string) {
	for k, v := range labels {
		b.Labels[k] = v
	}
}

// addAnnotations merges annotations into the builder's annotation map.
func (b *workloadBuilder) addAnnotations(annotations map[string]string) {
	for k, v := range annotations {
		b.Annotations[k] = v
	}
}

// mergeSelectorLabels sets the labels used for the workload's immutable .spec.selector.
// The labels are cloned (to avoid external mutation) and also merged into the pod template
// labels, enforcing the invariant that the selector is a subset of the template labels —
// otherwise the API server would reject the workload.
func (b *workloadBuilder) mergeSelectorLabels(labels map[string]string) {
	b.SelectorLabels = make(map[string]string, len(labels))
	for k, v := range labels {
		b.SelectorLabels[k] = v
		b.Labels[k] = v
	}
}

// selectorMatchLabels returns the labels for .spec.selector, falling back to the full
// labels when no dedicated selector labels are set.
func (b *workloadBuilder) selectorMatchLabels() map[string]string {
	if len(b.SelectorLabels) > 0 {
		return b.SelectorLabels
	}
	return b.Labels
}

// primaryContainerName returns the effective primary container name.
func (b *workloadBuilder) primaryContainerName() string {
	if b.MainContainerName != "" {
		return b.MainContainerName
	}
	return b.Name
}

// setImage sets the container image, keeping the current pull policy when none is given.
func (b *workloadBuilder) setImage(image string, pullPolicy corev1.PullPolicy) {
	b.Image = image
	if pullPolicy != "" {
		b.ImagePullPolicy = pullPolicy
	}
}

// setResources converts a commons ResourcesSpec into the container's resource requirements.
func (b *workloadBuilder) setResources(resources *v1alpha1.ResourcesSpec) {
	if resources == nil {
		return
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
}

// setPreStopExec sets a preStop exec hook.
func (b *workloadBuilder) setPreStopExec(command []string) {
	if b.lifecycle == nil {
		b.lifecycle = &corev1.Lifecycle{}
	}
	b.lifecycle.PreStop = &corev1.LifecycleHandler{
		Exec: &corev1.ExecAction{
			Command: command,
		},
	}
}

// setPreStopHTTPGet sets a preStop HTTP GET hook.
func (b *workloadBuilder) setPreStopHTTPGet(path string, port int) {
	if b.lifecycle == nil {
		b.lifecycle = &corev1.Lifecycle{}
	}
	b.lifecycle.PreStop = &corev1.LifecycleHandler{
		HTTPGet: &corev1.HTTPGetAction{
			Path: path,
			Port: intstr.FromInt(port),
		},
	}
}

// setPostStartExec sets a postStart exec hook.
func (b *workloadBuilder) setPostStartExec(command []string) {
	if b.lifecycle == nil {
		b.lifecycle = &corev1.Lifecycle{}
	}
	b.lifecycle.PostStart = &corev1.LifecycleHandler{
		Exec: &corev1.ExecAction{
			Command: command,
		},
	}
}

// MainContainerViolations returns the errors the main container customizer produced, if any.
//
// Build() cannot return an error, so — exactly as with PodOverrideViolations — the failure is
// recorded and the caller surfaces it. BaseRoleGroupHandler turns these into a *ValidationError
// that fails the role group, because a customizer that failed silently would ship a workload
// missing the command, args or probes the product meant to set.
func (b *workloadBuilder) MainContainerViolations() []error {
	if len(b.mainContainerViolations) == 0 {
		return nil
	}
	return append([]error(nil), b.mainContainerViolations...)
}

// PodOverrideViolations returns the framework invariants the applied podOverrides broke — a
// framework-owned volume mount displaced or deleted, or a mount left referencing no declared
// volume. It is populated by Build() — which resets it first, so the result always describes the
// most recent build — so call it afterwards. Empty means the merge preserved everything the
// framework mounted.
//
// The slice is copied out, like every other value Build() hands back: the builder must not share
// mutable state with its callers.
func (b *workloadBuilder) PodOverrideViolations() []error {
	if len(b.podOverrideViolations) == 0 {
		return nil
	}
	return append([]error(nil), b.podOverrideViolations...)
}

// buildPodTemplate assembles the pod template both workload kinds wrap: labels and annotations
// cloned from the builder, the pod spec built below. It resets mainContainerViolations first —
// customizedContainer runs inside buildPodSpec, and the list describes THIS build, so a reused
// builder must not report the previous build's violations, nor report this one's twice.
func (b *workloadBuilder) buildPodTemplate() corev1.PodTemplateSpec {
	b.mainContainerViolations = nil

	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      maps.Clone(b.Labels),
			Annotations: maps.Clone(b.Annotations),
		},
		Spec: b.buildPodSpec(),
	}
}

// buildPodSpec builds the pod spec.
func (b *workloadBuilder) buildPodSpec() corev1.PodSpec {
	spec := corev1.PodSpec{
		ServiceAccountName:            b.ServiceAccountName,
		TerminationGracePeriodSeconds: clonePtr(b.TerminationGracePeriodSeconds),
		SecurityContext:               b.PodSecurityContext.DeepCopy(),
		Affinity:                      b.Affinity.DeepCopy(),
		Volumes:                       cloneSlice(b.Volumes),
		InitContainers:                cloneSlice(b.InitContainers),
		Containers: []corev1.Container{
			b.customizedContainer(),
		},
	}

	// Only set EnableServiceLinks when explicitly configured, so direct builder users that never
	// call WithEnableServiceLinks are unaffected (k8s applies its own default of true).
	if b.EnableServiceLinks != nil {
		spec.EnableServiceLinks = clonePtr(b.EnableServiceLinks)
	}

	return spec
}

// customizedContainer assembles the primary container and hands it to the registered customizer.
// Any error, and any attempt to change the image, is recorded for the caller to surface.
func (b *workloadBuilder) customizedContainer() corev1.Container {
	container := b.buildContainer()
	if b.mainContainerCustomizer == nil {
		return container
	}

	image := container.Image
	if err := b.mainContainerCustomizer(&container); err != nil {
		b.mainContainerViolations = append(b.mainContainerViolations,
			fmt.Errorf("main container customizer for %q: %w", container.Name, err))
		return container
	}
	if container.Image != image {
		b.mainContainerViolations = append(b.mainContainerViolations, fmt.Errorf(
			"main container customizer changed the image of %q from %q to %q: the image is resolved "+
				"once and propagated to the sidecars before the workload is built, so changing it "+
				"here would leave them on the old one — set RoleGroupBuildContext.Image instead",
			container.Name, image, container.Image))
		container.Image = image
	}
	return container
}

// buildContainer builds the main container.
func (b *workloadBuilder) buildContainer() corev1.Container {
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
	// different container.Env ordering on every reconcile. That makes the rendered workload
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
func (b *workloadBuilder) buildLivenessProbe() *corev1.Probe {
	if b.disableLiveness || b.livenessProbe == nil {
		return nil
	}
	return b.livenessProbe.DeepCopy()
}

// buildReadinessProbe determines the readiness probe based on configuration.
func (b *workloadBuilder) buildReadinessProbe() *corev1.Probe {
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
func (b *workloadBuilder) buildDefaultTCPReadinessProbe() *corev1.Probe {
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
func (b *workloadBuilder) buildStartupProbe() *corev1.Probe {
	if b.disableStartup {
		return nil
	}
	if b.startupProbe != nil {
		return b.startupProbe.DeepCopy()
	}
	return nil
}

// applyPodOverridesTo applies pod template overrides to a built workload's template with full
// fidelity: the merged podOverrides template is strategic-merge-patched onto the built pod
// template, exactly as kubectl would merge it. Containers merge by name (env/ports/volumeMounts
// within a container merge by their own keys), so container-level overrides — resources, env,
// extra volume mounts — land on the built containers instead of being dropped, and override-only
// containers are appended as additional pod containers. Scalar and struct fields set in the
// override replace or deep-merge into the base per strategic-merge-patch semantics; fields the
// override omits keep the built values (so podOverrides.spec.terminationGracePeriodSeconds still
// wins over the config-declared gracefulShutdownTimeout, and enableServiceLinks still wins over
// the framework default, only when actually set).
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
//
// volumeClaimTemplates carries the StatefulSet's claim templates, whose names are mountable
// without ever appearing in .spec.volumes; a Deployment has none and passes nil. The violation
// list describes THIS build, so it is reset first — unconditionally, or a builder reused after
// dropping its overrides would keep reporting the previous build's, and a second Build() would
// report each violation twice.
func (b *workloadBuilder) applyPodOverridesTo(
	template *corev1.PodTemplateSpec, volumeClaimTemplates []corev1.PersistentVolumeClaim,
) {
	b.podOverrideViolations = nil
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
	base := template.DeepCopy()
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
	*template = *merged

	// The merge keys volumeMounts by mountPath, so an override can displace a framework mount
	// without ever naming it. Record what it broke; the caller decides what to do about it.
	b.podOverrideViolations = append(b.podOverrideViolations,
		checkPodOverrideMountInvariants(base, merged, volumeClaimTemplates)...)

	// Re-assert the selector labels: the override may have replaced or removed labels the
	// immutable .spec.selector matches on.
	if template.Labels == nil {
		template.Labels = make(map[string]string)
	}
	for k, v := range b.selectorMatchLabels() {
		template.Labels[k] = v
	}
}

// NamespacedName returns the NamespacedName of the workload being built.
func (b *workloadBuilder) NamespacedName() types.NamespacedName {
	return types.NamespacedName{
		Name:      b.Name,
		Namespace: b.Namespace,
	}
}
