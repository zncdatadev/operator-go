package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/zncdatadev/operator-go/internal/framework/pipeline"
	"github.com/zncdatadev/operator-go/pkg/framework"
	"github.com/zncdatadev/operator-go/pkg/framework/dataops"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	kubernetesjson "sigs.k8s.io/json"
)

const retainedSourceAnnotation = "framework.kubedoop.dev/retained-data"
const retainedBindingAnnotation = "framework.kubedoop.dev/retained-binding"
const statefulSetKind = "StatefulSet"
const nullReceipt = "null"

var errStoragePending = errors.New("retained storage observation pending")
var errStorageUnbound = errors.New("retained claims have not yet acquired observed bindings")

// These are controller receipts, not user merge annotations or GC ownership.
type retainedSource struct {
	Version      int       `json:"version"`
	CRUID        types.UID `json:"crUID"`
	Role         string    `json:"role"`
	Group        string    `json:"group"`
	Slot         string    `json:"slot"`
	StorageClass string    `json:"storageClass"`
	Capacity     string    `json:"capacity"`
}

type retainedBinding struct {
	Version    int       `json:"version"`
	PVCUID     types.UID `json:"pvcUID"`
	PVUID      types.UID `json:"pvUID"`
	VolumeName string    `json:"volumeName"`
}

func strictReceipt(text string, into any) error {
	if text == "" || strings.TrimSpace(text) == nullReceipt {
		return fmt.Errorf("missing storage receipt")
	}
	strict, err := kubernetesjson.UnmarshalStrict([]byte(text), into)
	if err != nil || len(strict) > 0 {
		return fmt.Errorf("invalid storage receipt: %w", errors.Join(append(strict, err)...))
	}
	return nil
}

func sourceFor(owner client.Object, group groupSlot, data *pipeline.RetainedDataSlot) retainedSource {
	return retainedSource{Version: 1, CRUID: owner.GetUID(), Role: group.Role, Group: group.Group,
		Slot: data.Name, StorageClass: data.StorageClassName, Capacity: data.Capacity.String()}
}

func retainedClaimSpec(data *pipeline.RetainedDataSlot) corev1.PersistentVolumeClaimSpec {
	mode, class := corev1.PersistentVolumeFilesystem, data.StorageClassName
	return corev1.PersistentVolumeClaimSpec{AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
		StorageClassName: &class, VolumeMode: &mode,
		Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{
			corev1.ResourceStorage: data.Capacity.DeepCopy(),
		}}}
}

func validRetainedTemplate(sts *appsv1.StatefulSet, data *pipeline.RetainedDataSlot) error {
	platformNames, err := platformClaimNames(sts)
	if err != nil {
		return err
	}
	if data == nil {
		if storageRetirementUnsupported(sts) {
			return fmt.Errorf("spec.volumeClaimTemplates/Pod PVC retirement requires a separate data policy; " +
				"no explicit retained slot was declared")
		}
		return nil
	}
	if data.Name == "" || data.StorageClassName == "" || data.Capacity.Sign() <= 0 ||
		len(sts.Spec.VolumeClaimTemplates) != 1 {
		return fmt.Errorf("retained storage requires exactly one named, explicitly sized and classified claim template")
	}
	policy := sts.Spec.PersistentVolumeClaimRetentionPolicy
	if policy == nil || policy.WhenScaled != appsv1.RetainPersistentVolumeClaimRetentionPolicyType ||
		policy.WhenDeleted != appsv1.RetainPersistentVolumeClaimRetentionPolicyType {
		return fmt.Errorf("retained storage requires explicit Retain/Retain; existing Delete policy is not migrated")
	}
	claim := &sts.Spec.VolumeClaimTemplates[0]
	if claim.Name != data.Name || len(claim.OwnerReferences) != 0 ||
		!apiequality.Semantic.DeepEqual(claim.Spec, retainedClaimSpec(data)) {
		return fmt.Errorf("retained declaration differs from the final claim template")
	}
	for _, volume := range sts.Spec.Template.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil || (volume.Ephemeral != nil && !platformNames[volume.Name]) {
			return fmt.Errorf("additional Pod PVC or ephemeral claims are unsupported")
		}
	}
	return nil
}

func stampRetainedSource(sts *appsv1.StatefulSet, owner client.Object, group groupSlot,
	data *pipeline.RetainedDataSlot,
) (*appsv1.StatefulSet, error) {
	if err := validRetainedTemplate(sts, data); err != nil {
		return nil, err
	}
	next := sts.DeepCopy()
	if data != nil {
		encoded, err := json.Marshal(sourceFor(owner, group, data))
		if err != nil {
			return nil, err
		}
		claim := &next.Spec.VolumeClaimTemplates[0]
		if claim.Annotations == nil {
			claim.Annotations = map[string]string{}
		}
		claim.Annotations[retainedSourceAnnotation] = string(encoded)
	}
	return next, nil
}

func declaredRetainedSource(sts *appsv1.StatefulSet, owner client.Object,
	group groupSlot,
) (*pipeline.RetainedDataSlot, error) {
	if err := validatePlatformClaimOwner(sts, owner, group); err != nil {
		return nil, err
	}
	if !storageRetirementUnsupported(sts) {
		return nil, nil
	}
	if len(sts.Spec.VolumeClaimTemplates) != 1 {
		return nil, fmt.Errorf("PVC retirement requires a separate data policy")
	}
	var source retainedSource
	if err := strictReceipt(sts.Spec.VolumeClaimTemplates[0].Annotations[retainedSourceAnnotation], &source); err != nil {
		return nil, fmt.Errorf("PVC retirement requires a separate data policy: %w", err)
	}
	capacity, err := resource.ParseQuantity(source.Capacity)
	if err != nil {
		return nil, fmt.Errorf("invalid retained capacity receipt: %w", err)
	}
	data := &pipeline.RetainedDataSlot{Name: source.Slot,
		RetainedData: pipeline.RetainedData{StorageClassName: source.StorageClass, Capacity: capacity}}
	if source != sourceFor(owner, group, data) {
		return nil, fmt.Errorf("retained source belongs to another CR or group")
	}
	if err := validRetainedTemplate(sts, data); err != nil {
		return nil, err
	}
	return data, nil
}

// All reads are direct. Pending never means that an existing data binding is
// proven; a never-bound WFFC claim can coexist with creating its first Pod.
func checkRetainedStorage(ctx context.Context, c client.Client, owner client.Object, group groupSlot,
	desired *appsv1.StatefulSet, data *pipeline.RetainedDataSlot, live *appsv1.StatefulSet,
) error {
	if err := validRetainedTemplate(desired, data); err != nil {
		return err
	}
	if live != nil {
		actual, err := declaredRetainedSource(live, owner, group)
		if err != nil {
			return err
		}
		if !apiequality.Semantic.DeepEqual(actual, data) {
			return fmt.Errorf("live retained storage declaration changed; use an explicit DataOperation for migration")
		}
	}
	var claims corev1.PersistentVolumeClaimList
	if err := c.List(ctx, &claims, client.InNamespace(owner.GetNamespace())); err != nil {
		return err
	}
	if data == nil {
		for _, claim := range claims.Items {
			var prior retainedSource
			if strictReceipt(claim.Annotations[retainedSourceAnnotation], &prior) == nil && prior.CRUID == owner.GetUID() &&
				prior.Role == group.Role && prior.Group == group.Group {
				return fmt.Errorf("retained claims remain but the group no longer declares their data slot")
			}
		}
		return nil
	}
	class := &storagev1.StorageClass{}
	if err := c.Get(ctx, client.ObjectKey{Name: data.StorageClassName}, class); err != nil {
		return err
	}
	if !class.DeletionTimestamp.IsZero() || class.ReclaimPolicy == nil ||
		*class.ReclaimPolicy != corev1.PersistentVolumeReclaimRetain {
		return fmt.Errorf("StorageClass must explicitly retain volumes")
	}
	want := sourceFor(owner, group, data)
	prefix := data.Name + "-" + desired.Name + "-"
	selected, err := retainedClaims(claims.Items, prefix, want, data)
	if err != nil {
		return err
	}
	historySource := dataops.Source{Version: want.Version, CRUID: want.CRUID, Role: want.Role, Group: want.Group,
		Slot: want.Slot, StorageClass: want.StorageClass, Capacity: want.Capacity}
	if err := dataops.CheckHistory(ctx, c, owner.GetNamespace(), historySource, selected); err != nil {
		return err
	}
	if len(selected) > 0 {
		if err := checkClaimConsumers(ctx, c, desired, live, selected); err != nil {
			return err
		}
	}
	unbound := false
	for i := range selected {
		if err := observeRetainedBinding(ctx, c, owner, group, live, &selected[i], data); err != nil {
			if errors.Is(err, errStorageUnbound) {
				unbound = true
			} else {
				return err
			}
		}
	}
	// Missing desired ordinals may be first creation; this is a poll hint, not
	// evidence of data absence; the independent ledger was checked above.
	if desired.Spec.Replicas != nil {
		names := map[string]bool{}
		for _, claim := range selected {
			names[claim.Name] = true
		}
		for ordinal := int32(0); ordinal < *desired.Spec.Replicas; ordinal++ {
			if !names[prefix+strconv.FormatInt(int64(ordinal), 10)] {
				unbound = true
				break
			}
		}
	}
	if unbound {
		return errStorageUnbound
	}
	return nil
}

func checkClaimShape(claim *corev1.PersistentVolumeClaim, data *pipeline.RetainedDataSlot) error {
	if !claim.DeletionTimestamp.IsZero() {
		return fmt.Errorf("%w: PVC %s is deleting", errStoragePending, claim.Name)
	}
	if claim.UID == "" {
		return fmt.Errorf("PVC %s has no observed UID", claim.Name)
	}
	if len(claim.OwnerReferences) != 0 {
		return fmt.Errorf("PVC %s has potentially reclaiming owner references", claim.Name)
	}
	spec := claim.Spec.DeepCopy()
	spec.VolumeName = ""
	if spec.VolumeMode == nil {
		mode := corev1.PersistentVolumeFilesystem
		spec.VolumeMode = &mode
	}
	if !apiequality.Semantic.DeepEqual(*spec, retainedClaimSpec(data)) {
		return fmt.Errorf("PVC %s specification differs from its retained slot", claim.Name)
	}
	return nil
}

func checkClaimConsumers(ctx context.Context, c client.Client, desired, live *appsv1.StatefulSet,
	claims []corev1.PersistentVolumeClaim,
) error {
	names := map[string]bool{}
	for _, claim := range claims {
		names[claim.Name] = true
	}
	var pods corev1.PodList
	if err := c.List(ctx, &pods, client.InNamespace(desired.Namespace)); err != nil {
		return err
	}
	for _, pod := range pods.Items {
		for _, volume := range pod.Spec.Volumes {
			if volume.PersistentVolumeClaim == nil || !names[volume.PersistentVolumeClaim.ClaimName] {
				continue
			}
			controller := metav1.GetControllerOf(&pod)
			suffix, match := strings.CutPrefix(pod.Name, desired.Name+"-")
			ordinal, err := strconv.ParseUint(suffix, 10, 32)
			if live != nil && controller != nil && controller.APIVersion == appsv1.SchemeGroupVersion.String() &&
				controller.Kind == statefulSetKind &&
				controller.Name == live.Name && controller.UID == live.UID && match && err == nil &&
				strconv.FormatUint(ordinal, 10) == suffix &&
				strings.HasSuffix(volume.PersistentVolumeClaim.ClaimName, "-"+pod.Name) {
				continue
			}
			if !pod.DeletionTimestamp.IsZero() {
				return fmt.Errorf("%w: old consuming Pod %s is terminating", errStoragePending, pod.Name)
			}
			return fmt.Errorf("PVC %s is consumed by unrelated Pod %s", volume.PersistentVolumeClaim.ClaimName, pod.Name)
		}
	}
	return nil
}

func bindingObservation(ctx context.Context, c client.Client, claim *corev1.PersistentVolumeClaim,
	data *pipeline.RetainedDataSlot,
) (*retainedBinding, error) {
	previous, err := recordedBinding(claim)
	if err != nil {
		return nil, err
	}
	if claim.Status.Phase == corev1.ClaimLost {
		return nil, fmt.Errorf("PVC %s is Lost", claim.Name)
	}
	if claim.Spec.VolumeName == "" {
		if previous != nil || claim.Status.Phase == corev1.ClaimBound {
			return nil, fmt.Errorf("PVC %s lost its volume", claim.Name)
		}
		return nil, nil
	}
	pv := &corev1.PersistentVolume{}
	if err := c.Get(ctx, client.ObjectKey{Name: claim.Spec.VolumeName}, pv); err != nil {
		if apierrors.IsNotFound(err) && previous == nil && claim.Status.Phase != corev1.ClaimBound {
			return nil, fmt.Errorf("%w: PV not yet visible", errStoragePending)
		}
		return nil, fmt.Errorf("recorded PVC volume unavailable: %w", err)
	}
	if err := checkRetainedPV(pv, data); err != nil {
		return nil, err
	}
	if previous != nil && pv.UID != previous.PVUID {
		return nil, fmt.Errorf("PV %s identity changed", pv.Name)
	}
	ref := pv.Spec.ClaimRef
	if ref != nil && ((ref.UID != "" && ref.UID != claim.UID) || (ref.Name != "" && ref.Name != claim.Name) ||
		(ref.Namespace != "" && ref.Namespace != claim.Namespace)) {
		return nil, fmt.Errorf("PV %s is bound to another claim identity", pv.Name)
	}
	if ref == nil || ref.UID == "" || ref.Name == "" || ref.Namespace == "" ||
		claim.Status.Phase != corev1.ClaimBound || pv.Status.Phase != corev1.VolumeBound {
		if previous != nil {
			return nil, fmt.Errorf("PVC %s lost its recorded bidirectional binding", claim.Name)
		}
		return nil, fmt.Errorf("%w: PVC/PV binding is not yet complete", errStoragePending)
	}
	return &retainedBinding{Version: 1, PVCUID: claim.UID, PVUID: pv.UID, VolumeName: pv.Name}, nil
}

func observeRetainedBinding(ctx context.Context, c client.Client, owner client.Object, group groupSlot,
	live *appsv1.StatefulSet, observed *corev1.PersistentVolumeClaim, data *pipeline.RetainedDataSlot,
) error {
	binding, err := bindingObservation(ctx, c, observed, data)
	if err != nil {
		return err
	}
	if binding == nil {
		return errStorageUnbound
	}
	if _, present := observed.Annotations[retainedBindingAnnotation]; present {
		return nil
	}
	if live == nil || !live.DeletionTimestamp.IsZero() {
		return fmt.Errorf("bound PVC %s lacks a binding receipt and a live same-source StatefulSet", observed.Name)
	}
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		current := &corev1.PersistentVolumeClaim{}
		if err := c.Get(ctx, client.ObjectKeyFromObject(observed), current); err != nil {
			return err
		}
		if current.UID != observed.UID {
			return fmt.Errorf("PVC identity changed before recording binding")
		}
		if err := checkClaimShape(current, data); err != nil {
			return err
		}
		var source retainedSource
		if err := strictReceipt(current.Annotations[retainedSourceAnnotation], &source); err != nil {
			return err
		}
		if source != sourceFor(owner, group, data) {
			return fmt.Errorf("PVC provenance changed before recording binding")
		}
		fresh := &appsv1.StatefulSet{}
		if err := c.Get(ctx, client.ObjectKeyFromObject(live), fresh); err != nil {
			return err
		}
		kind, err := apiutil.GVKForObject(owner, c.Scheme())
		if err != nil {
			return err
		}
		if fresh.UID != live.UID {
			return fmt.Errorf("StatefulSet identity changed before recording binding")
		}
		if !fresh.DeletionTimestamp.IsZero() {
			return fmt.Errorf("%w: StatefulSet is deleting before recording binding", errStoragePending)
		}
		if err := checkSlotObject(owner, fresh, kind, group); err != nil {
			return err
		}
		actualData, err := declaredRetainedSource(fresh, owner, group)
		if err != nil {
			return err
		}
		if !apiequality.Semantic.DeepEqual(actualData, data) {
			return fmt.Errorf("StatefulSet storage changed before recording binding")
		}
		if err := checkClaimConsumers(ctx, c, fresh, fresh, []corev1.PersistentVolumeClaim{*current}); err != nil {
			return err
		}
		actual, err := bindingObservation(ctx, c, current, data)
		if err != nil {
			return err
		}
		if actual == nil || *actual != *binding {
			return fmt.Errorf("binding changed before recording receipt")
		}
		if _, present := current.Annotations[retainedBindingAnnotation]; present {
			return nil
		}
		currentOwner := owner.DeepCopyObject().(client.Object)
		if err := c.Get(ctx, client.ObjectKeyFromObject(owner), currentOwner); err != nil {
			return err
		}
		if currentOwner.GetUID() != owner.GetUID() || currentOwner.GetGeneration() != owner.GetGeneration() ||
			!currentOwner.GetDeletionTimestamp().IsZero() {
			return errSuperseded
		}
		encoded, err := json.Marshal(actual)
		if err != nil {
			return err
		}
		current.Annotations[retainedBindingAnnotation] = string(encoded)
		return c.Update(ctx, current)
	})
}

// The explicit group declaration is private to the typed controller path.
func checkApplyStorage(ctx context.Context, c client.Client, owner, desired, current client.Object,
	group *groupSlot, data *pipeline.RetainedDataSlot,
) error {
	sts, ok := desired.(*appsv1.StatefulSet)
	if !ok {
		return nil
	}
	var live *appsv1.StatefulSet
	if current != nil {
		live = current.(*appsv1.StatefulSet)
	}
	if group == nil {
		if live != nil {
			return validRetainedTemplate(live, nil)
		}
		return validRetainedTemplate(sts, nil)
	}
	err := checkRetainedStorage(ctx, c, owner, *group, sts, data, live)
	if errors.Is(err, errStorageUnbound) {
		return nil
	}
	return err
}

func (r *Reconciler[CR, C, S, F]) preflightStorage(ctx context.Context, cr CR, group groupSlot,
	desired *appsv1.StatefulSet, data *pipeline.RetainedDataSlot,
	runtime ...*framework.RuntimeDescription,
) error {
	if len(runtime) > 0 {
		prepared, err := stampPlatformClaims(cr, desired, &group, runtime[0])
		if err != nil {
			return err
		}
		desired = prepared.(*appsv1.StatefulSet)
	}
	object, err := r.readSlot(ctx, cr, group)
	if err != nil {
		return err
	}
	var live *appsv1.StatefulSet
	if object != nil {
		live = object.(*appsv1.StatefulSet)
	}
	err = checkRetainedStorage(ctx, r.Client, cr, group, desired, data, live)
	if data == nil || (err != nil && !errors.Is(err, errStorageUnbound)) {
		return err
	}
	if ledgerErr := recordDataAssets(ctx, r.Client, cr, group, desired, data); ledgerErr != nil {
		return ledgerErr
	}
	return err
}

func checkRetiringStorage(ctx context.Context, c client.Client, cr client.Object, group groupSlot,
	sts *appsv1.StatefulSet,
) error {
	data, err := declaredRetainedSource(sts, cr, group)
	if err != nil {
		return err
	}
	err = checkRetainedStorage(ctx, c, cr, group, sts, data, sts)
	if errors.Is(err, errStorageUnbound) {
		return nil
	}
	return err
}

func retainedClaims(claims []corev1.PersistentVolumeClaim, prefix string, want retainedSource,
	data *pipeline.RetainedDataSlot,
) ([]corev1.PersistentVolumeClaim, error) {
	selected := []corev1.PersistentVolumeClaim{}
	for _, claim := range claims {
		var source retainedSource
		sourceErr := strictReceipt(claim.Annotations[retainedSourceAnnotation], &source)
		suffix, prefixMatch := strings.CutPrefix(claim.Name, prefix)
		ordinal, ordinalErr := strconv.ParseUint(suffix, 10, 32)
		canonical := prefixMatch && ordinalErr == nil && strconv.FormatUint(ordinal, 10) == suffix
		sameGroup := sourceErr == nil && source.CRUID == want.CRUID && source.Role == want.Role && source.Group == want.Group
		if !canonical && !sameGroup {
			continue
		}
		if !canonical || sourceErr != nil || source != want {
			return nil, fmt.Errorf("PVC %s has unknown or conflicting retained provenance", claim.Name)
		}
		if err := checkClaimShape(&claim, data); err != nil {
			return nil, err
		}
		selected = append(selected, claim)
	}
	return selected, nil
}

func checkRetainedPV(pv *corev1.PersistentVolume, data *pipeline.RetainedDataSlot) error {
	if pv.UID == "" || !apiequality.Semantic.DeepEqual(pv.Spec.AccessModes,
		[]corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}) || !pv.DeletionTimestamp.IsZero() ||
		len(pv.OwnerReferences) != 0 || pv.Spec.PersistentVolumeReclaimPolicy != corev1.PersistentVolumeReclaimRetain ||
		pv.Spec.StorageClassName != data.StorageClassName || pv.Spec.Capacity.Storage().Cmp(data.Capacity) < 0 ||
		(pv.Spec.VolumeMode != nil && *pv.Spec.VolumeMode != corev1.PersistentVolumeFilesystem) {
		return fmt.Errorf("PV %s no longer satisfies retained storage", pv.Name)
	}
	return nil
}

func recordedBinding(claim *corev1.PersistentVolumeClaim) (*retainedBinding, error) {
	var previous *retainedBinding
	if text, exists := claim.Annotations[retainedBindingAnnotation]; exists {
		previous = &retainedBinding{}
		if err := strictReceipt(text, previous); err != nil {
			return nil, err
		}
		if previous.Version != 1 || previous.PVCUID != claim.UID || previous.PVUID == "" ||
			previous.VolumeName == "" || previous.VolumeName != claim.Spec.VolumeName {
			return nil, fmt.Errorf("PVC %s lost or changed its recorded binding identity", claim.Name)
		}
	}
	return previous, nil
}

// Recording new independent identities belongs to normal apply preparation. Stop
// and retirement invoke the shared read guard without creating ledger resources.
func recordDataAssets(ctx context.Context, c client.Client, owner client.Object, group groupSlot,
	desired *appsv1.StatefulSet, data *pipeline.RetainedDataSlot,
) error {
	var claims corev1.PersistentVolumeClaimList
	if err := c.List(ctx, &claims, client.InNamespace(owner.GetNamespace())); err != nil {
		return err
	}
	selected, err := retainedClaims(claims.Items, data.Name+"-"+desired.Name+"-", sourceFor(owner, group, data), data)
	if err != nil {
		return err
	}
	for i := range selected {
		claim := &selected[i]
		if _, recorded := claim.Annotations[retainedBindingAnnotation]; !recorded {
			continue
		}
		if _, err := bindingObservation(ctx, c, claim, data); err != nil {
			return err
		}
		gvk, err := apiutil.GVKForObject(owner, c.Scheme())
		if err != nil {
			return err
		}
		sourceCluster := dataops.ClusterRef{APIVersion: gvk.GroupVersion().String(), Kind: gvk.Kind,
			Name: owner.GetName(), UID: owner.GetUID()}
		if err := dataops.EnsureAsset(ctx, c, claim, sourceCluster); err != nil {
			return err
		}
	}
	return nil
}
