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

package reconciler

import (
	"fmt"
	"maps"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// copyDesiredState copies the handler-built desired state onto the live object inside a
// controllerutil.CreateOrUpdate mutate func. controller-runtime's CreateOrUpdate overwrites
// the passed object with LIVE cluster state on Get before running the mutate func, so without
// this copy the desired spec/data never reaches an existing object and the apply path is
// create-only (issue #526).
//
// Copy rules, in order:
//
//   - Labels are framework-owned and replaced WHOLESALE (consistent with
//     EnsureDiscoveryConfigMap): labels removed from the desired object disappear from the
//     live object, and foreign labels added out-of-band do not survive a reconcile.
//   - Annotations are MERGED (desired wins per key): foreign annotations such as
//     kubectl.kubernetes.io/last-applied-configuration survive reconciles.
//   - Known kinds get a typed copy that respects Kubernetes immutable/system-managed fields
//     (see copyStatefulSetState / copyServiceState and the per-kind cases below).
//   - Any other kind (RoleGroupResources.ExtraResources with arbitrary GVKs, e.g. a
//     listeners.kubedoop.dev Listener) falls back to a generic top-level field copy via
//     unstructured conversion (see copyGenericState).
//
// The desired object must be a deep copy taken BEFORE CreateOrUpdate (the object passed to
// CreateOrUpdate is clobbered with live state on Get) and must have the same concrete type as
// live. On the create path CreateOrUpdate runs the mutate func against the not-yet-created
// object, i.e. live == desired state already; every rule below is a no-op in that case, which
// is exactly why desired-only-at-create fields (e.g. Service.Spec.ClusterIP) need no special
// handling here.
func copyDesiredState(desired, live client.Object) ([]string, error) {
	// Labels: framework-owned, replaced wholesale.
	live.SetLabels(desired.GetLabels())

	// Annotations: merge desired into live so foreign annotations survive.
	if desiredAnnotations := desired.GetAnnotations(); len(desiredAnnotations) > 0 {
		annotations := live.GetAnnotations()
		if annotations == nil {
			annotations = make(map[string]string, len(desiredAnnotations))
		}
		for k, v := range desiredAnnotations {
			annotations[k] = v
		}
		live.SetAnnotations(annotations)
	}

	switch liveObj := live.(type) {
	case *appsv1.StatefulSet:
		desiredObj, err := desiredAs[*appsv1.StatefulSet](desired, live)
		if err != nil {
			return nil, err
		}
		return copyStatefulSetState(desiredObj, liveObj), nil
	case *corev1.ConfigMap:
		desiredObj, err := desiredAs[*corev1.ConfigMap](desired, live)
		if err != nil {
			return nil, err
		}
		// Data and BinaryData are replaced wholesale: keys removed by the product disappear
		// from the live ConfigMap.
		liveObj.Data = desiredObj.Data
		liveObj.BinaryData = desiredObj.BinaryData
		return nil, nil
	case *corev1.Service:
		desiredObj, err := desiredAs[*corev1.Service](desired, live)
		if err != nil {
			return nil, err
		}
		return copyServiceState(desiredObj, liveObj), nil
	case *corev1.ServiceAccount:
		// A ServiceAccount has no spec the framework owns; labels/annotations (above) and the
		// controller owner reference (set by the caller) are the whole desired state. Never
		// touch Secrets/ImagePullSecrets — the token controller manages them.
		return nil, nil
	case *batchv1.Job:
		desiredObj, err := desiredAs[*batchv1.Job](desired, live)
		if err != nil {
			return nil, err
		}
		return copyJobState(desiredObj, liveObj), nil
	case *policyv1.PodDisruptionBudget:
		desiredObj, err := desiredAs[*policyv1.PodDisruptionBudget](desired, live)
		if err != nil {
			return nil, err
		}
		liveObj.Spec = desiredObj.Spec
		return nil, nil
	default:
		return nil, copyGenericState(desired, live)
	}
}

// desiredAs asserts that desired has the same concrete type as live. It always does in
// practice — applyResource deep-copies the live object's original — so this only guards
// against future misuse with a clear error instead of a panic.
func desiredAs[T client.Object](desired, live client.Object) (T, error) {
	typed, ok := desired.(T)
	if !ok {
		return typed, fmt.Errorf("desired object type %T does not match live object type %T", desired, live)
	}
	return typed, nil
}

// copyStatefulSetState copies the desired StatefulSet spec onto the live one, preserving the
// fields the Kubernetes API declares immutable after creation:
//
//   - Spec.Selector
//   - Spec.ServiceName
//   - Spec.VolumeClaimTemplates
//   - Spec.PodManagementPolicy
//
// These keep their LIVE values, so a handler that starts producing different values for them
// (e.g. a new label-selector layout after an operator upgrade) does not make every subsequent
// Update fail against the API server. Changing them for an existing cluster requires a manual
// migration (delete/recreate of the StatefulSet), documented as part of the upgrade path in
// issue #526. Everything else — Replicas, Template, UpdateStrategy, MinReadySeconds,
// PersistentVolumeClaimRetentionPolicy, ... — comes from desired.
//
// It RETURNS the field paths whose desired value differed from the live one, so the caller can
// tell the user their change was dropped. Preserving these fields silently is what made a
// storage resize look successful: the CR reported ReconcileComplete while the PVC never moved,
// and nothing in the API said why.
//
// # The pod template follows the claim templates that were actually preserved
//
// volumeClaimTemplates is the one preserved field the pod template DEPENDS on, and preserving it
// alone leaves an incoherent StatefulSet in both directions:
//
//   - Adding storage to a live role group produces a template mounting a claim the framework just
//     declined to create, which nothing can run: `volumeMounts[0].name: Not found: "data"`, a field
//     the user never wrote. Kubernetes 1.34+ rejects the StatefulSet Update itself, so the role
//     group stays Degraded on every subsequent pass and only a manual delete recovers it; older
//     servers accept the Update and reject every POD the controller then tries to create, so the
//     workload silently never progresses and the error is in the StatefulSet controller's events
//     rather than the cluster's status.
//   - Removing it is worse, because it is ACCEPTED: the claim template is kept, the mount is not,
//     and the pods roll into a product writing to the container's writable layer while its bound
//     PVCs sit there mounted nowhere. No event, no condition, no log line.
//
// So the mounts are reconciled against the claim templates that survived: a mount for a claim that
// was not created is dropped, and a mount for a claim that was preserved is restored from the live
// template. The change is still reported through `ignored` — this makes the transition converge and
// stay coherent, it does not make it happen.
func copyStatefulSetState(desired, live *appsv1.StatefulSet) []string {
	selector := live.Spec.Selector
	serviceName := live.Spec.ServiceName
	volumeClaimTemplates := live.Spec.VolumeClaimTemplates
	podManagementPolicy := live.Spec.PodManagementPolicy

	claimsDiffer := claimTemplatesDiffer(desired.Spec.VolumeClaimTemplates, volumeClaimTemplates)

	// Captured before the spec is overwritten, and only when it is needed: it is the only record of
	// where a preserved claim was mounted.
	var liveTemplate *corev1.PodTemplateSpec
	if claimsDiffer {
		liveTemplate = live.Spec.Template.DeepCopy()
	}

	// Compared BEFORE the spec is overwritten. Only a desired value the handler actually set
	// counts: an unset field is the handler declining to have an opinion, not a change request.
	var ignored []string
	if desired.Spec.Selector != nil && !apiequality.Semantic.DeepEqual(desired.Spec.Selector, selector) {
		ignored = append(ignored, "spec.selector")
	}
	if desired.Spec.ServiceName != "" && desired.Spec.ServiceName != serviceName {
		ignored = append(ignored, "spec.serviceName")
	}
	if claimsDiffer {
		ignored = append(ignored, "spec.volumeClaimTemplates")
	}
	if desired.Spec.PodManagementPolicy != "" && desired.Spec.PodManagementPolicy != podManagementPolicy {
		ignored = append(ignored, "spec.podManagementPolicy")
	}

	// The pod template's annotations are merged, not replaced — the same rule copyDesiredState
	// applies to the object's own annotations, and for the same reason: another controller writes
	// there and the framework must not undo it.
	//
	// This is not hypothetical tidiness. commons-operator's restarter delivers a configOverrides
	// change to running pods by writing "configmap.restarter.kubedoop.dev/<name>: <uid>/<rv>" into
	// exactly this map, which rolls the pods — and AGENTS.md documents that as the ONLY mechanism
	// that does so, since a ConfigMap rewrite leaves the pod template byte-identical. The handler
	// never builds that key, so a wholesale spec assignment removes it; the resulting Update wakes
	// the restarter (its predicate matches the label on every Update, not only on Create), which
	// re-stamps, which wakes this reconciler through its own Owns(&appsv1.StatefulSet{}) watch.
	// Neither side is failing, so the workqueue Forgets each pass and nothing backs off: the pods
	// roll for as long as the label is set.
	//
	// Labels are deliberately NOT given the same treatment. The pod template's labels have to match
	// the StatefulSet's immutable .spec.selector, so they are framework-owned and come from
	// desired, exactly like the object's labels.
	templateAnnotations := live.Spec.Template.Annotations

	live.Spec = desired.Spec

	live.Spec.Selector = selector
	live.Spec.ServiceName = serviceName
	live.Spec.VolumeClaimTemplates = volumeClaimTemplates
	live.Spec.PodManagementPolicy = podManagementPolicy

	if claimsDiffer {
		// live.Spec was assigned wholesale, so its Containers slice still shares a backing array
		// with desired's. Deep-copy before mutating, or the fix writes through into the object the
		// caller built.
		live.Spec.Template = *desired.Spec.Template.DeepCopy()
		reconcileClaimVolumeMounts(&live.Spec.Template, desired.Spec.VolumeClaimTemplates, volumeClaimTemplates, liveTemplate)
	}

	live.Spec.Template.Annotations = mergeAnnotations(templateAnnotations, desired.Spec.Template.Annotations)

	return ignored
}

// claimTemplatesDiffer reports whether the handler is ASKING for volumeClaimTemplates other than
// the live ones — which is a different question from whether the two slices are byte-equal.
//
// The live side comes back from the API server carrying values it filled in itself, which a
// handler-built template has no way to state, so a whole-slice DeepEqual can never be true for a
// role group that HAS storage. Every reconcile of every such role group therefore reported
// spec.volumeClaimTemplates as an ignored immutable change (#627): one unchanged StatefulSet
// accumulated the warning indefinitely while its metadata.generation stayed at 1 and no pod ever
// rolled.
//
// That is not merely noise — it destroys the signal the event exists for. A user who genuinely
// resizes storage gets exactly the warning they were already getting on every pass, so the one
// event that says "your resize was dropped" becomes indistinguishable from the background.
//
// # Which fields the server actually fills in
//
// Measured, rather than assumed, by round-tripping exactly what StatefulSetBuilder.WithStorage
// produces through the API server (envtest, Kubernetes 1.35):
//
//	field                      sent    stored               server-populated
//	metadata.name              data    data                 no
//	spec.accessModes           [RWO]   [RWO]                no
//	spec.resources.requests    1Gi     1Gi                  no
//	spec.storageClassName      unset   unset                no  <- NOT defaulted on a template
//	spec.volumeMode            unset   Filesystem           YES
//	status                     {}      {phase: Pending}     YES
//	apiVersion / kind          unset   unset                no  <- typed decode clears TypeMeta
//
// Only those two are normalised away, and only when the handler stated nothing — the rule
// copyStatefulSetState already documents and applies to selector, serviceName and
// podManagementPolicy. Everything else is still compared wholesale, which is what keeps a genuine
// resize, a rename, or a storageClassName the user changed reportable. The asymmetry is deliberate:
// a field this misses is a dropped change nobody hears about, while a field it over-reports is
// noise that the steady-state spec in apply_storage_test.go fails on.
//
// The one place an EMPTY desired slice still counts is unchanged. Everywhere else an unset value is
// the handler declining to have an opinion; an empty volumeClaimTemplates is the handler stating
// that this role group has no storage, which is exactly the direction that was being applied
// silently.
func claimTemplatesDiffer(desired, live []corev1.PersistentVolumeClaim) bool {
	if len(desired) != len(live) {
		return true
	}
	for i := range desired {
		if !apiequality.Semantic.DeepEqual(desired[i], claimTemplateAsRequested(desired[i], live[i])) {
			return true
		}
	}
	return false
}

// claimTemplateAsRequested returns the live claim template with the server-populated fields the
// desired one does not state cleared, so the two compare for what the handler actually asked for.
func claimTemplateAsRequested(desired, live corev1.PersistentVolumeClaim) corev1.PersistentVolumeClaim {
	requested := *live.DeepCopy()

	if desired.Spec.VolumeMode == nil {
		requested.Spec.VolumeMode = nil
	}
	if apiequality.Semantic.DeepEqual(desired.Status, corev1.PersistentVolumeClaimStatus{}) {
		requested.Status = corev1.PersistentVolumeClaimStatus{}
	}

	return requested
}

// reconcileClaimVolumeMounts makes the pod template consistent with the volumeClaimTemplates that
// actually exist on the live StatefulSet, after copyStatefulSetState has preserved them.
//
// Two directions, and neither is a policy choice — each is the only shape the API server will accept
// or the only one that keeps the pods attached to their data:
//
//   - A mount naming a claim in `desiredClaims` but not in `liveClaims` refers to a volume that will
//     not exist, and the API server rejects the whole StatefulSet for it. It is dropped, unless the
//     template also declares an ordinary volume of that name — then the mount is backed and the
//     claim template was never what satisfied it.
//   - A claim in `liveClaims` but not in `desiredClaims` has been preserved and must stay mounted
//     where it already is, so its mounts are copied back from the live template. Restoring only
//     what the live template had is what keeps this from inventing a mount path: the framework does
//     not know where the product wants the volume, it knows where the volume already is.
//
// A rename (live "data", desired "data2") hits both branches and lands on the preserved claim.
func reconcileClaimVolumeMounts(tpl *corev1.PodTemplateSpec, desiredClaims, liveClaims []corev1.PersistentVolumeClaim, liveTemplate *corev1.PodTemplateSpec) {
	liveNames := claimTemplateNames(liveClaims)
	desiredNames := claimTemplateNames(desiredClaims)

	for name := range desiredNames {
		if _, created := liveNames[name]; created {
			continue
		}
		if hasVolume(tpl.Spec.Volumes, name) {
			continue
		}
		dropVolumeMount(tpl, name)
	}

	if liveTemplate == nil {
		return
	}
	for name := range liveNames {
		if _, stillDesired := desiredNames[name]; stillDesired {
			continue
		}
		restoreVolumeMount(tpl, liveTemplate, name)
	}
}

// claimTemplateNames indexes a volumeClaimTemplates list by claim name.
func claimTemplateNames(claims []corev1.PersistentVolumeClaim) map[string]struct{} {
	names := make(map[string]struct{}, len(claims))
	for i := range claims {
		names[claims[i].Name] = struct{}{}
	}
	return names
}

// hasVolume reports whether the pod spec declares an ordinary volume of this name.
func hasVolume(volumes []corev1.Volume, name string) bool {
	for i := range volumes {
		if volumes[i].Name == name {
			return true
		}
	}
	return false
}

// dropVolumeMount removes every mount of the named volume from every container in the template.
func dropVolumeMount(tpl *corev1.PodTemplateSpec, name string) {
	forEachContainer(tpl, func(c *corev1.Container) {
		kept := make([]corev1.VolumeMount, 0, len(c.VolumeMounts))
		for _, m := range c.VolumeMounts {
			if m.Name != name {
				kept = append(kept, m)
			}
		}
		if len(kept) == 0 {
			c.VolumeMounts = nil
			return
		}
		c.VolumeMounts = kept
	})
}

// restoreVolumeMount copies EVERY live mount of the named volume back onto the same-named
// containers. Kubernetes allows one volume to be mounted several times — different paths, different
// subPaths — and a product that does so has as much claim to each of those paths as to the first, so
// the restore is keyed on the mount PATH rather than the volume name. Keying on the name would stop
// after the first restored mount and drop the rest, and it is also the wrong question in the one
// situation this branch runs in: the claim is gone from the desired state, so a desired mount of
// that volume names something only the preserved claim backs — it is a reason to keep the live
// mounts, not to skip them.
//
// A path the desired template already uses is left alone: the desired template's opinion about a
// path wins, and two mounts at one path is a spec the API server rejects.
func restoreVolumeMount(tpl *corev1.PodTemplateSpec, liveTemplate *corev1.PodTemplateSpec, name string) {
	forEachContainer(liveTemplate, func(liveContainer *corev1.Container) {
		target := findTemplateContainer(tpl, liveContainer.Name)
		if target == nil {
			return // the container is gone from the desired template; nothing to mount onto
		}
		for _, m := range liveContainer.VolumeMounts {
			if m.Name != name {
				continue
			}
			// Re-checked per mount, against the list as it grows, so two live mounts at the same
			// path (or one colliding with a desired mount) cannot produce a duplicate.
			if mountsPath(target.VolumeMounts, m.MountPath) {
				continue
			}
			target.VolumeMounts = append(target.VolumeMounts, *m.DeepCopy())
		}
	})
}

// forEachContainer applies fn to every container in the template, init containers included.
func forEachContainer(tpl *corev1.PodTemplateSpec, fn func(*corev1.Container)) {
	for i := range tpl.Spec.InitContainers {
		fn(&tpl.Spec.InitContainers[i])
	}
	for i := range tpl.Spec.Containers {
		fn(&tpl.Spec.Containers[i])
	}
}

// findTemplateContainer returns the named container in the template, init containers included.
func findTemplateContainer(tpl *corev1.PodTemplateSpec, name string) *corev1.Container {
	for i := range tpl.Spec.InitContainers {
		if tpl.Spec.InitContainers[i].Name == name {
			return &tpl.Spec.InitContainers[i]
		}
	}
	for i := range tpl.Spec.Containers {
		if tpl.Spec.Containers[i].Name == name {
			return &tpl.Spec.Containers[i]
		}
	}
	return nil
}

func mountsPath(mounts []corev1.VolumeMount, path string) bool {
	for i := range mounts {
		if mounts[i].MountPath == path {
			return true
		}
	}
	return false
}

// mergeAnnotations returns live's annotations with desired's merged over them, desired winning per
// key. A nil result is preserved when both are empty, so an object that never had annotations does
// not gain an empty map and churn its resourceVersion.
func mergeAnnotations(live, desired map[string]string) map[string]string {
	if len(live) == 0 {
		return desired
	}
	merged := make(map[string]string, len(live)+len(desired))
	maps.Copy(merged, live)
	maps.Copy(merged, desired)
	return merged
}

// copyServiceState copies the desired Service spec onto the live one, preserving the fields the
// API server owns or declares immutable after creation:
//
//   - Spec.ClusterIP / Spec.ClusterIPs — allocated by the API server. The desired value ("None"
//     for headless services) only matters at CREATE time, where the mutate func runs against
//     the desired object itself and the value is already in place.
//   - Spec.IPFamilies / Spec.IPFamilyPolicy — allocated, and rejected on update.
//   - Spec.HealthCheckNodePort — allocated for a LoadBalancer with local external traffic policy.
//   - Spec.LoadBalancerClass — immutable once set.
//   - Per-port NodePorts: for every desired port with NodePort == 0 the live port's allocated
//     NodePort is carried over (matched by port name, falling back to port number), so
//     NodePort/LoadBalancer services keep their stable node ports across reconciles (same
//     precedent as zookeeper-operator's cluster_extension.go).
//
// Everything else — Type, Selector, Ports, SessionAffinity, traffic policies,
// AllocateLoadBalancerNodePorts, TrafficDistribution, and any field a future Kubernetes version
// adds — comes from desired. Assigning the spec wholesale (as copyStatefulSetState does) is what
// makes new mutable fields converge by default instead of silently sticking at their live value.
// Not all of the preserved set is load-bearing, and the difference is invisible from the code —
// it took an experiment against a real API server (envtest 1.35) to establish. Recorded here
// because the six carry-over lines below read as interchangeable and two of them are not:
//
//	field                    naive Update (no carry-over)          verdict
//	clusterIP / clusterIPs   restored by the API server            defensive only
//	ipFamilies / policy      restored by the API server            defensive only
//	healthCheckNodePort      restored by the API server            defensive only
//	nodePort, port UNCHANGED restored by the API server            defensive only
//	nodePort, port RENAMED   REALLOCATED (31965 -> 32604)          LOAD-BEARING
//	loadBalancerClass        update REJECTED with a 422            LOAD-BEARING
//
// The API server's own patchAllocatedValues matches ports by name, so renaming a port — a
// cosmetic change — loses its allocated node port and breaks every client that had it pinned.
// findServicePort's port-number fallback is what makes the framework strictly more preserving
// than Kubernetes here.
//
// loadBalancerClass is worse than "not restored": it is immutable in the sense that the API
// server rejects the whole update ("may not change once set"), so dropping the carry-over wedges
// the role group's reconcile permanently the moment a user removes the class from their CR.
//
// Both are pinned by specs in generic_reconciler_test.go; deleting either line fails them.
func copyServiceState(desired, live *corev1.Service) []string {
	livePorts := live.Spec.Ports
	clusterIP := live.Spec.ClusterIP
	clusterIPs := live.Spec.ClusterIPs
	ipFamilies := live.Spec.IPFamilies
	ipFamilyPolicy := live.Spec.IPFamilyPolicy
	healthCheckNodePort := live.Spec.HealthCheckNodePort
	loadBalancerClass := live.Spec.LoadBalancerClass

	// Only clusterIP is reported. The rest of the preserved set is allocated BY the API server
	// (IP families, the LoadBalancer health-check node port) or is empty in a handler-built
	// object, so a difference there is the framework declining to fight the allocator, not a
	// user's change being dropped — reporting it would be pure noise on every reconcile.
	//
	// clusterIP is different: it is the one the handler states deliberately, and it encodes
	// headless ("None") versus virtual-IP. Flipping a Service between the two is exactly the
	// change Kubernetes refuses and the user needs told about.
	var ignored []string
	if desired.Spec.ClusterIP != "" && desired.Spec.ClusterIP != clusterIP {
		ignored = append(ignored, "spec.clusterIP")
	}

	live.Spec = desired.Spec

	live.Spec.ClusterIP = clusterIP
	live.Spec.ClusterIPs = clusterIPs
	live.Spec.IPFamilies = ipFamilies
	live.Spec.IPFamilyPolicy = ipFamilyPolicy
	live.Spec.HealthCheckNodePort = healthCheckNodePort
	live.Spec.LoadBalancerClass = loadBalancerClass

	ports := make([]corev1.ServicePort, len(desired.Spec.Ports))
	copy(ports, desired.Spec.Ports)
	for i := range ports {
		if ports[i].NodePort != 0 {
			continue // the handler pinned an explicit NodePort; take it as-is
		}
		if allocated := findServicePort(livePorts, ports[i]); allocated != nil {
			ports[i].NodePort = allocated.NodePort
		}
	}
	live.Spec.Ports = ports

	return ignored
}

// findServicePort finds the live port corresponding to a desired port: by name when the
// desired port is named, falling back to the port number. Returns nil when no live port
// matches (a genuinely new port — the API server will allocate its NodePort if needed).
func findServicePort(livePorts []corev1.ServicePort, desired corev1.ServicePort) *corev1.ServicePort {
	if desired.Name != "" {
		for i := range livePorts {
			if livePorts[i].Name == desired.Name {
				return &livePorts[i]
			}
		}
	}
	for i := range livePorts {
		if livePorts[i].Port == desired.Port {
			return &livePorts[i]
		}
	}
	return nil
}

// copyJobState copies the desired Job spec onto the live one, preserving the fields the batch API
// declares immutable after creation:
//
//   - Spec.Selector and Spec.Template — the API server GENERATES the selector at creation and
//     injects four UID-derived labels into the template, so a handler-built desired object never
//     carries them
//   - Spec.Completions, Spec.CompletionMode, Spec.ManualSelector
//
// Without this rule a Job reached copyGenericState, which assigns `spec` wholesale — so the SECOND
// reconcile of an UNCHANGED Job was rejected with `spec.selector: Required value` plus
// `spec.template: field is immutable`, and the role group went permanently Degraded quoting a field
// the user never wrote. `RoleGroupResources.ExtraResources` is documented as the channel for
// arbitrary GVKs, and a Job — the object two downstream operators actually put there, for database
// migration and one-shot registration — could not survive a no-op pass. Verified against envtest
// before this rule existed.
//
// The effect is create-once, which is also the only semantics a Job HAS: its work is a side effect
// that already happened. A product that needs to re-run it changes the Job's NAME, which the
// framework leaves entirely to the product — the old object is then reclaimed as an orphaned extra
// (§13) or left for owner-reference GC.
//
// Everything else — Parallelism, Suspend, BackoffLimit, ActiveDeadlineSeconds,
// TTLSecondsAfterFinished — comes from desired, so the knobs Kubernetes does allow changing still
// converge. Like copyStatefulSetState it RETURNS the preserved paths whose desired value differs, so
// a product editing a live Job's template is told the edit was dropped instead of finding out from a
// pod that never ran.
func copyJobState(desired, live *batchv1.Job) []string {
	selector := live.Spec.Selector
	template := live.Spec.Template
	completions := live.Spec.Completions
	completionMode := live.Spec.CompletionMode
	manualSelector := live.Spec.ManualSelector

	var ignored []string
	// The template is compared BEFORE the copy and only against what the handler actually built.
	// The live template carries the API server's injected labels, so an equality test would report a
	// difference on every reconcile; comparing the pod SPEC alone asks the question the product
	// meant — "is this still the same work?".
	if !apiequality.Semantic.DeepEqual(desired.Spec.Template.Spec, template.Spec) {
		ignored = append(ignored, "spec.template")
	}
	if desired.Spec.Completions != nil && !apiequality.Semantic.DeepEqual(desired.Spec.Completions, completions) {
		ignored = append(ignored, "spec.completions")
	}

	live.Spec = desired.Spec

	live.Spec.Selector = selector
	live.Spec.Template = template
	live.Spec.Completions = completions
	live.Spec.CompletionMode = completionMode
	live.Spec.ManualSelector = manualSelector

	return ignored
}

// copyGenericState is the fallback for kinds without a typed rule (arbitrary-GVK
// ExtraResources such as a listeners.kubedoop.dev Listener). Both objects are converted to
// unstructured maps, every top-level field of desired EXCEPT apiVersion, kind, metadata and
// status is copied onto live (that covers spec, data, stringData, ... — whatever payload the
// kind defines), and the result is converted back into the live object. Metadata is handled
// by copyDesiredState (labels/annotations) and the caller (owner reference); status belongs
// to the resource's own controller. Top-level fields present only on live are kept — they are
// typically server-managed (deliberately conservative; a product that needs to REMOVE a
// top-level payload field of an extra resource should set it to its empty value instead).
func copyGenericState(desired, live client.Object) error {
	desiredMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(desired)
	if err != nil {
		return fmt.Errorf("failed to convert desired object %T to unstructured: %w", desired, err)
	}

	// When the live object already is unstructured (extras built as
	// *unstructured.Unstructured), mutate its content map directly — the reflection-based
	// FromUnstructured below only fills typed structs.
	if liveUnstructured, ok := live.(*unstructured.Unstructured); ok {
		for field, value := range desiredMap {
			if isReservedTopLevelField(field) {
				continue
			}
			liveUnstructured.Object[field] = value
		}
		return nil
	}

	liveMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(live)
	if err != nil {
		return fmt.Errorf("failed to convert live object %T to unstructured: %w", live, err)
	}

	for field, value := range desiredMap {
		if isReservedTopLevelField(field) {
			continue
		}
		liveMap[field] = value
	}

	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(liveMap, live); err != nil {
		return fmt.Errorf("failed to convert merged state back into live object %T: %w", live, err)
	}
	return nil
}

// isReservedTopLevelField reports whether a top-level unstructured field is identity,
// metadata or controller-owned state that copyGenericState must never overwrite.
func isReservedTopLevelField(field string) bool {
	switch field {
	case "apiVersion", "kind", "metadata", "status":
		return true
	}
	return false
}
