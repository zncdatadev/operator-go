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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
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
func copyDesiredState(desired, live client.Object) error {
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
			return err
		}
		copyStatefulSetState(desiredObj, liveObj)
		return nil
	case *corev1.ConfigMap:
		desiredObj, err := desiredAs[*corev1.ConfigMap](desired, live)
		if err != nil {
			return err
		}
		// Data and BinaryData are replaced wholesale: keys removed by the product disappear
		// from the live ConfigMap.
		liveObj.Data = desiredObj.Data
		liveObj.BinaryData = desiredObj.BinaryData
		return nil
	case *corev1.Service:
		desiredObj, err := desiredAs[*corev1.Service](desired, live)
		if err != nil {
			return err
		}
		copyServiceState(desiredObj, liveObj)
		return nil
	case *corev1.ServiceAccount:
		// A ServiceAccount has no spec the framework owns; labels/annotations (above) and the
		// controller owner reference (set by the caller) are the whole desired state. Never
		// touch Secrets/ImagePullSecrets — the token controller manages them.
		return nil
	case *policyv1.PodDisruptionBudget:
		desiredObj, err := desiredAs[*policyv1.PodDisruptionBudget](desired, live)
		if err != nil {
			return err
		}
		liveObj.Spec = desiredObj.Spec
		return nil
	default:
		return copyGenericState(desired, live)
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
func copyStatefulSetState(desired, live *appsv1.StatefulSet) {
	selector := live.Spec.Selector
	serviceName := live.Spec.ServiceName
	volumeClaimTemplates := live.Spec.VolumeClaimTemplates
	podManagementPolicy := live.Spec.PodManagementPolicy

	live.Spec = desired.Spec

	live.Spec.Selector = selector
	live.Spec.ServiceName = serviceName
	live.Spec.VolumeClaimTemplates = volumeClaimTemplates
	live.Spec.PodManagementPolicy = podManagementPolicy
}

// copyServiceState copies the desired Service spec onto the live one:
//
//   - Mutable framework-owned fields come from desired: Type, Selector, Ports,
//     PublishNotReadyAddresses, SessionAffinity(+Config), External/InternalTrafficPolicy,
//     ExternalName, ExternalIPs, LoadBalancerSourceRanges.
//   - Ports are replaced with desired's, but for every desired port with NodePort == 0 the
//     live port's allocated NodePort is carried over (matched by port name, falling back to
//     port number), so NodePort/LoadBalancer services keep their stable allocated node ports
//     across reconciles (same precedent as zookeeper-operator's cluster_extension.go).
//   - ClusterIP, ClusterIPs, IPFamilies and IPFamilyPolicy are NEVER touched: they are
//     immutable/allocated by the API server. The desired ClusterIP ("None" for headless
//     services) only matters at CREATE time, where the mutate func runs against the desired
//     object itself and the value is already in place.
func copyServiceState(desired, live *corev1.Service) {
	livePorts := live.Spec.Ports

	live.Spec.Type = desired.Spec.Type
	live.Spec.Selector = desired.Spec.Selector
	live.Spec.PublishNotReadyAddresses = desired.Spec.PublishNotReadyAddresses
	live.Spec.SessionAffinity = desired.Spec.SessionAffinity
	live.Spec.SessionAffinityConfig = desired.Spec.SessionAffinityConfig
	live.Spec.ExternalTrafficPolicy = desired.Spec.ExternalTrafficPolicy
	live.Spec.InternalTrafficPolicy = desired.Spec.InternalTrafficPolicy
	live.Spec.ExternalName = desired.Spec.ExternalName
	live.Spec.ExternalIPs = desired.Spec.ExternalIPs
	live.Spec.LoadBalancerSourceRanges = desired.Spec.LoadBalancerSourceRanges

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
