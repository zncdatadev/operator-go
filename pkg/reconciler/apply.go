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
func copyStatefulSetState(desired, live *appsv1.StatefulSet) []string {
	selector := live.Spec.Selector
	serviceName := live.Spec.ServiceName
	volumeClaimTemplates := live.Spec.VolumeClaimTemplates
	podManagementPolicy := live.Spec.PodManagementPolicy

	// Compared BEFORE the spec is overwritten. Only a desired value the handler actually set
	// counts: an unset field is the handler declining to have an opinion, not a change request.
	var ignored []string
	if desired.Spec.Selector != nil && !apiequality.Semantic.DeepEqual(desired.Spec.Selector, selector) {
		ignored = append(ignored, "spec.selector")
	}
	if desired.Spec.ServiceName != "" && desired.Spec.ServiceName != serviceName {
		ignored = append(ignored, "spec.serviceName")
	}
	if len(desired.Spec.VolumeClaimTemplates) > 0 &&
		!apiequality.Semantic.DeepEqual(desired.Spec.VolumeClaimTemplates, volumeClaimTemplates) {
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
	live.Spec.Template.Annotations = mergeAnnotations(templateAnnotations, desired.Spec.Template.Annotations)

	return ignored
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
