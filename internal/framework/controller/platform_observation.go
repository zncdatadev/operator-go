package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/zncdatadev/operator-go/internal/framework/pipeline"
	"github.com/zncdatadev/operator-go/pkg/framework"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

const platformObserving = "Observing"

func (r *Reconciler[CR, C, S, F]) platformReader() *trackedFactsReader {
	return &trackedFactsReader{cache: &factReadCache{client: r.Client, scheme: r.Scheme,
		reads: map[factReadKey]factRead{}},
		observed: map[factReadKey]framework.FactObject{}}
}
func platformObject(group, kind string) *unstructured.Unstructured {
	value := &unstructured.Unstructured{}
	value.SetGroupVersionKind(schema.GroupVersionKind{Group: group, Version: "v1alpha1", Kind: kind})
	return value
}
func platformDirectory(d framework.Directory) bool { return d.Secret != nil || d.Listener != nil }
func platformReference(namespace string, d framework.Directory) (framework.FactResource, client.ObjectKey) {
	if s := d.Secret; s != nil {
		if s.SecretName != "" {
			return &corev1.Secret{}, client.ObjectKey{Namespace: namespace, Name: s.SecretName}
		}
		return platformObject("secrets.kubedoop.dev", "SecretClass"), client.ObjectKey{Name: s.SecretClass}
	}
	if d.Listener.Name != "" {
		return platformObject("listeners.kubedoop.dev", "Listener"),
			client.ObjectKey{Namespace: namespace, Name: d.Listener.Name}
	}
	return platformObject("listeners.kubedoop.dev", "ListenerClass"), client.ObjectKey{Name: d.Listener.Class}
}
func platformPending(out *framework.PlatformObservation, message string) {
	out.Diagnostic.State = framework.FactsPending
	out.Diagnostic.Reason = "PlatformNotReady"
	out.Diagnostic.Message = message
}
func finishPlatformObservation(out *framework.PlatformObservation, reader *trackedFactsReader, err error) {
	out.Diagnostic.Observed = reader.observations()
	if err != nil {
		out.Diagnostic.State = framework.FactsReadError
		out.Diagnostic.Reason = "PlatformObservationFailed"
		out.Diagnostic.Message = safeFactError(err)
	}
	if out.Diagnostic.State != framework.FactsResolved {
		out.Listeners = nil
	}
}

// Source references exist before apply; ephemeral claims and Listener results do
// not. Only Secret data revisions and source identity/spec revisions roll Pods.
func (r *Reconciler[CR, C, S, F]) preparePlatformVolumes(
	ctx context.Context, namespace string, runtime *framework.RuntimeDescription, pod *corev1.PodSpec,
) (out *framework.PlatformObservation, stamp string, err error) {
	references := platformReferences(namespace, runtime, pod)
	if len(references) == 0 {
		return nil, "", nil
	}
	reader := r.platformReader()
	out = &framework.PlatformObservation{Phase: "Preparing",
		Diagnostic: framework.FactDiagnostic{State: framework.FactsResolved}}
	defer func() { finishPlatformObservation(out, reader, err) }()
	var revisions []string
	for _, reference := range references {
		object, key := reference.object, reference.key
		if err = reader.Get(ctx, key, object); err != nil {
			if apierrors.IsNotFound(err) {
				if reference.optional {
					revisions = append(revisions, reference.slot+":absent")
					err = nil
					continue
				}
				platformPending(out, "Waiting for platform reference "+key.String())
				return out, "", nil
			}
			return out, "", err
		}
		if !object.GetDeletionTimestamp().IsZero() {
			platformPending(out, "Waiting for terminating platform reference "+key.String())
			return out, "", nil
		}
		version := strconv.FormatInt(object.GetGeneration(), 10)
		if _, native := object.(*corev1.Secret); native {
			version = object.GetResourceVersion()
		}
		revisions = append(revisions, reference.slot+":"+string(object.GetUID())+":"+version)
	}
	data, _ := json.Marshal(revisions)
	sum := sha256.Sum256(data)
	return out, hex.EncodeToString(sum[:]), nil
}

// A fresh exact reader after apply cannot retain a pre-creation NotFound. A
// result is published only for the current Pod -> PVC -> PV -> Listener chain.
func (r *Reconciler[CR, C, S, F]) observePlatform(
	ctx context.Context, cr CR, group framework.GroupIdentity, resources *pipeline.GroupResources,
	runtime *framework.RuntimeDescription,
) (out *framework.PlatformObservation, err error) {
	if len(platformReferences(group.Namespace, runtime, &resources.StatefulSet.Spec.Template.Spec)) == 0 {
		return nil, nil
	}
	out = &framework.PlatformObservation{Phase: platformObserving,
		Diagnostic: framework.FactDiagnostic{State: framework.FactsResolved, Reason: "PlatformReady"}}
	reader := r.platformReader()
	defer func() { finishPlatformObservation(out, reader, err) }()
	live := &appsv1.StatefulSet{}
	if err = reader.Get(ctx, client.ObjectKeyFromObject(&resources.StatefulSet), live); err != nil {
		return out, err
	}
	ownerKind, kindErr := apiutil.GVKForObject(cr, r.Scheme)
	if kindErr != nil {
		return out, kindErr
	}
	if err = checkOwnership(cr, live, ownerKind); err != nil {
		return out, err
	}
	want := group.Replicas
	if resources.StatefulSet.Spec.Replicas != nil {
		want = *resources.StatefulSet.Spec.Replicas
	}
	if want == 0 {
		platformPending(out, "No active producer Pod; preserving prior platform output")
		return out, nil
	}
	if live.Status.ObservedGeneration < live.Generation {
		platformPending(out, "Waiting for current producer revision")
	}
	for ordinal := int32(0); ordinal < want; ordinal++ {
		pod := &corev1.Pod{}
		key := client.ObjectKey{Namespace: group.Namespace, Name: live.Name + "-" + strconv.Itoa(int(ordinal))}
		found, readErr := readPlatformObject(ctx, reader, key, pod, out)
		if readErr != nil {
			return out, readErr
		}
		if !found {
			continue
		}
		if !controlledBy(pod, live.UID) {
			return out, fmt.Errorf("platform producer Pod has a different owner")
		}
		if !platformPodReady(pod, live) {
			platformPending(out, "Waiting for mounted current producer Pod "+pod.Name)
			continue
		}
		for _, d := range runtime.Directories {
			if !platformDirectory(d) || (d.Secret != nil && d.Secret.SecretName != "") {
				continue
			}
			if err = observeCSIDirectory(ctx, reader, pod, d, out); err != nil {
				return out, err
			}
		}
	}
	return out, nil
}
func controlledBy(object metav1.Object, uid types.UID) bool {
	if uid == "" {
		return false
	}
	for _, owner := range object.GetOwnerReferences() {
		if owner.UID == uid && owner.Controller != nil && *owner.Controller {
			return true
		}
	}
	return false
}
func platformPodReady(pod *corev1.Pod, sts *appsv1.StatefulSet) bool {
	if !pod.DeletionTimestamp.IsZero() || sts.Status.UpdateRevision == "" ||
		pod.Labels[appsv1.ControllerRevisionHashLabelKey] != sts.Status.UpdateRevision {
		return false
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}
func readPlatformObject(ctx context.Context, reader *trackedFactsReader, key client.ObjectKey,
	object framework.FactResource, out *framework.PlatformObservation,
) (bool, error) {
	if err := reader.Get(ctx, key, object); err != nil {
		if apierrors.IsNotFound(err) {
			platformPending(out, "Waiting for platform object "+key.String())
			return false, nil
		}
		return false, err
	}
	if !object.GetDeletionTimestamp().IsZero() {
		platformPending(out, "Waiting for terminating platform object "+key.String())
		return false, nil
	}
	return true, nil
}
func observeCSIDirectory(ctx context.Context, reader *trackedFactsReader, pod *corev1.Pod,
	d framework.Directory, out *framework.PlatformObservation,
) error {
	claim := &corev1.PersistentVolumeClaim{}
	key := client.ObjectKey{Namespace: pod.Namespace, Name: pod.Name + "-" + d.Name}
	found, err := readPlatformObject(ctx, reader, key, claim, out)
	if err != nil || !found {
		return err
	}
	if !controlledBy(claim, pod.UID) {
		return fmt.Errorf("CSI claim %s does not belong to producer Pod", claim.Name)
	}
	if claim.Status.Phase != corev1.ClaimBound || claim.Spec.VolumeName == "" {
		platformPending(out, "Waiting for CSI binding "+claim.Name)
		return nil
	}
	pv := &corev1.PersistentVolume{}
	found, err = readPlatformObject(ctx, reader, client.ObjectKey{Name: claim.Spec.VolumeName}, pv, out)
	if err != nil || !found {
		return err
	}
	ref := pv.Spec.ClaimRef
	if ref == nil || ref.UID != claim.UID || ref.Name != claim.Name || ref.Namespace != claim.Namespace {
		return fmt.Errorf("CSI volume binding does not identify current claim")
	}
	if d.Listener == nil {
		return nil
	}
	name := claim.Name
	if d.Listener.Name != "" {
		name = d.Listener.Name
	}
	listener := platformObject("listeners.kubedoop.dev", "Listener")
	found, err = readPlatformObject(ctx, reader, client.ObjectKey{Namespace: pod.Namespace, Name: name}, listener, out)
	if err != nil || !found {
		return err
	}
	if d.Listener.Class != "" && !ownedByUID(listener, pv.UID) {
		return fmt.Errorf("listener does not belong to current CSI volume")
	}
	addresses, err := listenerAddresses(listener, pod.Name, d.Name)
	if err != nil {
		return err
	}
	if len(addresses) == 0 {
		platformPending(out, "Waiting for Listener addresses "+name)
	}
	out.Listeners = append(out.Listeners, addresses...)
	return nil
}

// Listener CSI uses a PV owner reference, without promising controller=true.
func ownedByUID(object metav1.Object, uid types.UID) bool {
	if uid == "" {
		return false
	}
	for _, owner := range object.GetOwnerReferences() {
		if owner.UID == uid {
			return true
		}
	}
	return false
}
func listenerAddresses(listener *unstructured.Unstructured, pod, directory string,
) ([]framework.ListenerAddress, error) {
	values, _, err := unstructured.NestedSlice(listener.Object, "status", "ingressAddresses")
	if err != nil {
		return nil, err
	}
	var out []framework.ListenerAddress
	for _, raw := range values {
		entry, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid Listener address")
		}
		address, _, err := unstructured.NestedString(entry, "address")
		if err != nil || address == "" {
			return nil, fmt.Errorf("invalid Listener address")
		}
		ports, _, err := unstructured.NestedMap(entry, "ports")
		if err != nil {
			return nil, err
		}
		item := framework.ListenerAddress{Pod: pod, Directory: directory, Address: address, Ports: map[string]int32{}}
		for key, value := range ports {
			number, ok := value.(int64)
			if !ok || number < 1 || number > 65535 {
				return nil, fmt.Errorf("invalid Listener port")
			}
			item.Ports[key] = int32(number)
		}
		out = append(out, item)
	}
	return out, nil
}
