package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"

	"github.com/zncdatadev/operator-go/internal/framework/pipeline"
	"github.com/zncdatadev/operator-go/pkg/framework"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	kubernetesjson "sigs.k8s.io/json"
)

// ManagedMetadataAnnotation records only keys previously declared by this
// controller, so withdrawing one does not remove another controller's metadata.
const ManagedMetadataAnnotation = "framework.kubedoop.dev/managed-metadata"

type metadataKeys struct {
	Labels      []string `json:"labels,omitempty"`
	Annotations []string `json:"annotations,omitempty"`
}

type managedMetadata struct {
	Object   metadataKeys `json:"object"`
	Template metadataKeys `json:"template"`
}

// ApplyObject supports ConfigMap, Service, StatefulSet and PodDisruptionBudget. The client MUST
// read directly from the API server: every conflict retry starts with a fresh
// Get; an informer cache cannot provide that contract.
//
// Existing objects are canonicalized with a dry-run Update before comparison.
// Public client-go schemes do not contain the server's complete defaulting
// functions. The controller pays one additional request per existing
// object per pass rather than maintain a partial copy of Kubernetes defaults.
// A no-op performs no persisted Update and leaves resourceVersion unchanged.
func ApplyObject(
	ctx context.Context, c client.Client, owner client.Object, desired client.Object, scheme *runtime.Scheme,
) (bool, error) {
	return applyObject(ctx, c, owner, desired, scheme, nil, nil)
}

func applyObject(ctx context.Context, c client.Client, owner client.Object, desired client.Object,
	scheme *runtime.Scheme, slot *groupSlot, retained *pipeline.RetainedDataSlot,
) (bool, error) {
	return applyScopedObject(ctx, c, owner, desired, scheme, slot, retained, nil, nil, nil)
}

func applyScopedObject(ctx context.Context, c client.Client, owner client.Object, desired client.Object,
	scheme *runtime.Scheme, slot *groupSlot, retained *pipeline.RetainedDataSlot, role *rolePDBSlot,
	shared *sharedConfigMapSlot, platformRuntime *framework.RuntimeDescription,
	policies ...*framework.WorkloadCoordination,
) (bool, error) {
	if err := validateApplyRequest(c, owner, desired, scheme); err != nil {
		return false, err
	}
	desired, err := stampPlatformClaims(owner, desired, slot, platformRuntime)
	if err != nil {
		return false, err
	}
	desired = stampCoordination(desired, policies)
	desired, err = prepareScopedObject(owner, desired, slot, retained, role, shared)
	if err != nil {
		return false, err
	}
	ownerKind, err := apiutil.GVKForObject(owner, scheme)
	if err != nil {
		return false, err
	}
	changed := false
	err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		live := desired.DeepCopyObject().(client.Object)
		if err := c.Get(ctx, client.ObjectKeyFromObject(desired), live); err != nil {
			if !apierrors.IsNotFound(err) {
				return err
			}
			if err := checkApplyStorage(ctx, c, owner, desired, nil, slot, retained); err != nil {
				return err
			}
			created, err := createOwnedObject(ctx, c, owner, desired, scheme)
			changed = created
			return err
		}
		ownerCopy := live.DeepCopyObject().(client.Object)
		ownerCopy.SetDeletionTimestamp(nil)
		if err := checkOwnership(owner, ownerCopy, ownerKind); err != nil {
			return err
		}
		if err := checkApplyShared(owner, live, ownerKind, shared); err != nil {
			return err
		}
		if err := checkApplyRolePDB(owner, live, ownerKind, role); err != nil {
			return err
		}
		if err := checkOwnership(owner, live, ownerKind); err != nil {
			return err
		}
		if err := checkApplyGroupSlot(owner, live, ownerKind, slot); err != nil {
			return err
		}
		if err := checkApplyStorage(ctx, c, owner, desired, live, slot, retained); err != nil {
			return err
		}
		next, err := desiredObject(desired, live)
		if err != nil {
			return err
		}
		if err := coordinateDesired(ctx, c, next, live); err != nil {
			return err
		}
		if err := c.Update(ctx, next, client.DryRunAll); err != nil {
			if apierrors.IsInvalid(err) {
				if immutable := checkImmutableStatefulSet(next, live); immutable != nil {
					return fmt.Errorf("%v: %w", immutable, err)
				}
			}
			return fmt.Errorf("canonicalize %T %s: %w", desired, desired.GetName(), err)
		}
		if err := checkImmutableStatefulSet(next, live); err != nil {
			return err
		}
		if equalAppliedState(next, live) {
			return nil
		}
		// Dry-run response metadata/status are not another write authority.
		next.SetResourceVersion(live.GetResourceVersion())
		next.SetGeneration(live.GetGeneration())
		next.SetUID(live.GetUID())
		next.SetOwnerReferences(live.GetOwnerReferences())
		next.SetManagedFields(live.GetManagedFields())
		preserveStatus(next, live)
		if err := c.Update(ctx, next); err != nil {
			return err
		}
		changed = true
		return nil
	})
	return changed, err
}

func prepareScopedObject(owner, desired client.Object, slot *groupSlot,
	retained *pipeline.RetainedDataSlot, role *rolePDBSlot, shared *sharedConfigMapSlot,
) (client.Object, error) {
	if shared != nil {
		var err error
		desired, err = stampSharedSlot(owner, desired, *shared)
		if err != nil {
			return nil, err
		}
	}
	if slot != nil {
		desired = desired.DeepCopyObject().(client.Object)
		data, err := json.Marshal(slot)
		if err != nil {
			return nil, err
		}
		annotations := maps.Clone(desired.GetAnnotations())
		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations[GroupSlotAnnotation] = string(data)
		desired.SetAnnotations(annotations)
	}
	if role != nil {
		var err error
		desired, err = stampRolePDB(desired, owner, *role)
		if err != nil {
			return nil, err
		}
	}
	if sts, ok := desired.(*appsv1.StatefulSet); ok {
		if retained != nil && slot == nil {
			return nil, fmt.Errorf("retained storage requires a declared group slot")
		}
		group := groupSlot{}
		if slot != nil {
			group = *slot
		}
		next, err := stampRetainedSource(sts, owner, group, retained)
		if err != nil {
			return nil, err
		}
		desired = next
	}
	return desired, nil
}

func nilObject(object client.Object) bool {
	return object == nil || (reflect.ValueOf(object).Kind() == reflect.Pointer && reflect.ValueOf(object).IsNil())
}

func validateApplyRequest(c client.Client, owner, desired client.Object, scheme *runtime.Scheme) error {
	if c == nil || scheme == nil || nilObject(owner) || nilObject(desired) {
		return fmt.Errorf("apply requires a client, scheme, owner and desired object")
	}
	if owner.GetUID() == "" || owner.GetName() == "" || !owner.GetDeletionTimestamp().IsZero() {
		return fmt.Errorf("apply owner requires a live name and UID")
	}
	if desired.GetName() == "" || desired.GetNamespace() == "" ||
		(owner.GetNamespace() != "" && owner.GetNamespace() != desired.GetNamespace()) {
		return fmt.Errorf("apply requires a named object in its owner's namespace")
	}
	switch desired.(type) {
	case *corev1.ConfigMap, *corev1.Service, *appsv1.StatefulSet, *policyv1.PodDisruptionBudget:
	default:
		return fmt.Errorf("unsupported apply object %T", desired)
	}
	metadata := []metav1.Object{desired}
	if statefulSet, ok := desired.(*appsv1.StatefulSet); ok {
		metadata = append(metadata, &statefulSet.Spec.Template)
		for i := range statefulSet.Spec.VolumeClaimTemplates {
			metadata = append(metadata, &statefulSet.Spec.VolumeClaimTemplates[i])
		}
	}
	for _, item := range metadata {
		for _, key := range []string{ManagedMetadataAnnotation, GroupSlotAnnotation, RolePDBAnnotation,
			SharedConfigMapAnnotation,
			retainedSourceAnnotation, retainedBindingAnnotation, platformClaimsAnnotation,
			coordinationAnnotation, coordinationProgressAnnotation} {
			_, annotation := item.GetAnnotations()[key]
			_, label := item.GetLabels()[key]
			if annotation || label {
				return fmt.Errorf("desired metadata contains reserved key %q", key)
			}
		}
	}
	return nil
}

func checkOwnership(owner, live client.Object, ownerKind schema.GroupVersionKind) error {
	controller := metav1.GetControllerOf(live)
	if controller == nil || controller.UID != owner.GetUID() || controller.Name != owner.GetName() ||
		controller.APIVersion != ownerKind.GroupVersion().String() || controller.Kind != ownerKind.Kind {
		return fmt.Errorf("refusing to adopt %T %s/%s: matching controller owner UID, name and GVK are required",
			live, live.GetNamespace(), live.GetName())
	}
	if !live.GetDeletionTimestamp().IsZero() {
		return fmt.Errorf("cannot apply terminating %T %s/%s", live, live.GetNamespace(), live.GetName())
	}
	return nil
}

func createOwnedObject(
	ctx context.Context, c client.Client, owner, desired client.Object, scheme *runtime.Scheme,
) (bool, error) {
	next := desired.DeepCopyObject().(client.Object)
	next.SetUID("")
	next.SetResourceVersion("")
	next.SetGeneration(0)
	next.SetCreationTimestamp(metav1.Time{})
	next.SetManagedFields(nil)
	clearStatus(next)
	if err := mergeMetadata(desired, nil, next); err != nil {
		return false, err
	}
	if err := controllerutil.SetControllerReference(owner, next, scheme); err != nil {
		return false, err
	}
	if err := c.Create(ctx, next); err != nil {
		if apierrors.IsAlreadyExists(err) {
			// Race with another creator: retry through Get and ownership validation.
			return false, apierrors.NewConflict(schema.GroupResource{Resource: "objects"}, desired.GetName(), err)
		}
		return false, err
	}
	return true, nil
}

func desiredObject(desired, live client.Object) (client.Object, error) {
	next := live.DeepCopyObject().(client.Object)
	switch target := next.(type) {
	case *corev1.ConfigMap:
		want := desired.(*corev1.ConfigMap).DeepCopy()
		target.Data, target.BinaryData, target.Immutable = want.Data, want.BinaryData, want.Immutable
	case *corev1.Service:
		target.Spec = desired.(*corev1.Service).DeepCopy().Spec
		preserveServiceAllocations(&target.Spec, &live.(*corev1.Service).Spec)
	case *appsv1.StatefulSet:
		target.Spec = desired.(*appsv1.StatefulSet).DeepCopy().Spec
	case *policyv1.PodDisruptionBudget:
		target.Spec = desired.(*policyv1.PodDisruptionBudget).DeepCopy().Spec
	}
	if err := mergeMetadata(desired, live, next); err != nil {
		return nil, err
	}
	return next, nil
}

func metadataDeclaration(object metav1.Object) metadataKeys {
	return metadataKeys{Labels: slices.Sorted(maps.Keys(object.GetLabels())),
		Annotations: slices.Sorted(maps.Keys(object.GetAnnotations()))}
}

func decodeManagedMetadata(live client.Object) (managedMetadata, error) {
	var previous managedMetadata
	if live == nil {
		return previous, nil
	}
	text, present := live.GetAnnotations()[ManagedMetadataAnnotation]
	if !present {
		return previous, fmt.Errorf("existing controlled object is missing its managed metadata record")
	}
	// Strict decoding rejects duplicate keys as well as unknown fields. Null is
	// not an empty record; a damaged ownership record must never silently prune.
	strict, err := kubernetesjson.UnmarshalStrict([]byte(text), &previous)
	if err != nil || len(strict) != 0 {
		return previous, fmt.Errorf("invalid managed metadata record: %w", errors.Join(append(strict, err)...))
	}
	var shape map[string]any
	if err := json.Unmarshal([]byte(text), &shape); err != nil {
		return previous, err
	}
	if _, ok := shape["object"].(map[string]any); !ok {
		return previous, fmt.Errorf("managed metadata record requires an object key set")
	}
	if _, ok := shape["template"].(map[string]any); !ok || containsNull(shape) {
		return previous, fmt.Errorf("managed metadata record requires a template key set and forbids null")
	}
	for _, keys := range []metadataKeys{previous.Object, previous.Template} {
		for _, list := range [][]string{keys.Labels, keys.Annotations} {
			seen := map[string]bool{}
			for _, key := range list {
				if len(validation.IsQualifiedName(key)) != 0 || seen[key] || key == ManagedMetadataAnnotation {
					return previous, fmt.Errorf("invalid managed metadata key %q", key)
				}
				seen[key] = true
			}
		}
	}
	return previous, nil
}

func containsNull(value any) bool {
	switch item := value.(type) {
	case nil:
		return true
	case map[string]any:
		for _, child := range item {
			if containsNull(child) {
				return true
			}
		}
	case []any:
		for _, child := range item {
			if containsNull(child) {
				return true
			}
		}
	}
	return false
}

func mergeDeclaredMap(live, desired map[string]string, previous []string) map[string]string {
	out := maps.Clone(live)
	if out == nil {
		out = map[string]string{}
	}
	for _, key := range previous {
		delete(out, key)
	}
	maps.Copy(out, desired)
	if len(out) == 0 {
		return nil
	}
	return out
}

func mergeMetadata(desired, live, next client.Object) error {
	previous, err := decodeManagedMetadata(live)
	if err != nil {
		return err
	}
	var labels, annotations map[string]string
	if live != nil {
		labels, annotations = live.GetLabels(), live.GetAnnotations()
	}
	next.SetLabels(mergeDeclaredMap(labels, desired.GetLabels(), previous.Object.Labels))
	next.SetAnnotations(mergeDeclaredMap(annotations, desired.GetAnnotations(), previous.Object.Annotations))
	declared := managedMetadata{Object: metadataDeclaration(desired)}
	if target, ok := next.(*appsv1.StatefulSet); ok {
		want := desired.(*appsv1.StatefulSet)
		var liveLabels, liveAnnotations map[string]string
		if live != nil {
			stored := live.(*appsv1.StatefulSet)
			liveLabels, liveAnnotations = stored.Spec.Template.Labels, stored.Spec.Template.Annotations
		}
		target.Spec.Template.Labels = mergeDeclaredMap(liveLabels, want.Spec.Template.Labels, previous.Template.Labels)
		target.Spec.Template.Annotations = mergeDeclaredMap(
			liveAnnotations, want.Spec.Template.Annotations, previous.Template.Annotations)
		declared.Template = metadataDeclaration(&want.Spec.Template)
	}
	data, err := json.Marshal(declared)
	if err != nil {
		return err
	}
	annotations = next.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[ManagedMetadataAnnotation] = string(data)
	next.SetAnnotations(annotations)
	return nil
}

func serviceType(spec *corev1.ServiceSpec) corev1.ServiceType {
	if spec.Type == "" {
		return corev1.ServiceTypeClusterIP
	}
	return spec.Type
}

func preserveServiceAllocations(next, live *corev1.ServiceSpec) {
	if serviceType(next) != corev1.ServiceTypeExternalName {
		if next.ClusterIP == "" {
			next.ClusterIP = live.ClusterIP
		}
		if len(next.ClusterIPs) == 0 {
			next.ClusterIPs = slices.Clone(live.ClusterIPs)
		}
		if len(next.IPFamilies) == 0 {
			next.IPFamilies = slices.Clone(live.IPFamilies)
		}
		if next.IPFamilyPolicy == nil && live.IPFamilyPolicy != nil {
			value := *live.IPFamilyPolicy
			next.IPFamilyPolicy = &value
		}
	}
	if serviceType(next) == corev1.ServiceTypeLoadBalancer &&
		next.ExternalTrafficPolicy == corev1.ServiceExternalTrafficPolicyLocal && next.HealthCheckNodePort == 0 {
		next.HealthCheckNodePort = live.HealthCheckNodePort
	}
	if serviceType(next) == corev1.ServiceTypeNodePort || serviceType(next) == corev1.ServiceTypeLoadBalancer {
		for i := range next.Ports {
			if next.Ports[i].NodePort != 0 {
				continue
			}
			for _, old := range live.Ports {
				if sameServicePort(next.Ports[i], old) {
					next.Ports[i].NodePort = old.NodePort
				}
			}
		}
	}
}

func sameServicePort(a, b corev1.ServicePort) bool {
	protocol := func(port corev1.ServicePort) corev1.Protocol {
		if port.Protocol == "" {
			return corev1.ProtocolTCP
		}
		return port.Protocol
	}
	return a.Name == b.Name && protocol(a) == protocol(b) && (a.Name != "" || a.Port == b.Port)
}

func checkImmutableStatefulSet(next, live client.Object) error {
	want, ok := next.(*appsv1.StatefulSet)
	if !ok {
		return nil
	}
	stored := live.(*appsv1.StatefulSet)
	fields := []string{}
	if !apiequality.Semantic.DeepEqual(want.Spec.Selector, stored.Spec.Selector) {
		fields = append(fields, "spec.selector")
	}
	if want.Spec.ServiceName != stored.Spec.ServiceName {
		fields = append(fields, "spec.serviceName")
	}
	policy := func(value appsv1.PodManagementPolicyType) appsv1.PodManagementPolicyType {
		if value == "" {
			return appsv1.OrderedReadyPodManagement
		}
		return value
	}
	if policy(want.Spec.PodManagementPolicy) != policy(stored.Spec.PodManagementPolicy) {
		fields = append(fields, "spec.podManagementPolicy")
	}
	if !apiequality.Semantic.DeepEqual(want.Spec.VolumeClaimTemplates, stored.Spec.VolumeClaimTemplates) {
		fields = append(fields, "spec.volumeClaimTemplates")
	}
	if len(fields) != 0 {
		return fmt.Errorf("StatefulSet %s has immutable changes %v; delete/recreate is not performed", want.Name, fields)
	}
	return nil
}

func equalAppliedState(a, b client.Object) bool {
	if !apiequality.Semantic.DeepEqual(a.GetLabels(), b.GetLabels()) ||
		!apiequality.Semantic.DeepEqual(a.GetAnnotations(), b.GetAnnotations()) {
		return false
	}
	switch left := a.(type) {
	case *corev1.ConfigMap:
		right := b.(*corev1.ConfigMap)
		return apiequality.Semantic.DeepEqual(left.Data, right.Data) &&
			apiequality.Semantic.DeepEqual(left.BinaryData, right.BinaryData) &&
			apiequality.Semantic.DeepEqual(left.Immutable, right.Immutable)
	case *corev1.Service:
		return apiequality.Semantic.DeepEqual(left.Spec, b.(*corev1.Service).Spec)
	case *appsv1.StatefulSet:
		return apiequality.Semantic.DeepEqual(left.Spec, b.(*appsv1.StatefulSet).Spec)
	case *policyv1.PodDisruptionBudget:
		return apiequality.Semantic.DeepEqual(left.Spec, b.(*policyv1.PodDisruptionBudget).Spec)
	}
	return false
}

func clearStatus(object client.Object) {
	switch item := object.(type) {
	case *corev1.Service:
		item.Status = corev1.ServiceStatus{}
	case *appsv1.StatefulSet:
		item.Status = appsv1.StatefulSetStatus{}
	case *policyv1.PodDisruptionBudget:
		item.Status = policyv1.PodDisruptionBudgetStatus{}
	}
}

func preserveStatus(next, live client.Object) {
	switch item := next.(type) {
	case *corev1.Service:
		item.Status = *live.(*corev1.Service).Status.DeepCopy()
	case *appsv1.StatefulSet:
		item.Status = *live.(*appsv1.StatefulSet).Status.DeepCopy()
	case *policyv1.PodDisruptionBudget:
		item.Status = *live.(*policyv1.PodDisruptionBudget).Status.DeepCopy()
	}
}
