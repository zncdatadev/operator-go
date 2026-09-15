package dataops

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func validateIntent(op *DataOperation) error {
	s := op.Spec.Source
	if op.Spec.WorkerIdentity.UID <= 0 || op.Spec.WorkerIdentity.GID <= 0 {
		return fmt.Errorf("operation requires explicit positive non-root worker UID/GID")
	}
	if op.UID == "" || op.Spec.AssetUID == "" || s.Binding.PVCUID == "" || s.Binding.PVUID == "" || s.Binding.VolumeName == "" || s.Source.CRUID == "" || s.ClaimName == "" {
		return fmt.Errorf("operation requires exact asset, source PVC/PV and CR identities")
	}
	if op.Spec.SourceCluster != s.Cluster || op.Spec.SourceCluster.UID != s.Source.CRUID || op.Spec.SourceCluster.Name == "" || op.Spec.SourceCluster.APIVersion == "" || op.Spec.SourceCluster.Kind == "" {
		return fmt.Errorf("source cluster identity is incomplete")
	}
	if op.Spec.Action == ActionDestroy {
		if op.Spec.Target != nil {
			return fmt.Errorf("destroy cannot have a target")
		}
		return nil
	}
	if err := validateTarget(op); err != nil {
		return err
	}
	t := op.Spec.Target
	capacity, err := resource.ParseQuantity(t.Source.Capacity)
	if err != nil || capacity.Sign() <= 0 || t.Source.StorageClass == "" {
		return fmt.Errorf("invalid target storage declaration")
	}
	prefix := t.Source.Slot + "-" + t.Cluster.Name + "-" + t.Source.Role + "-" + t.Source.Group + "-"
	ordinal, parseErr := strconv.ParseUint(strings.TrimPrefix(t.ClaimName, prefix), 10, 32)
	if !strings.HasPrefix(t.ClaimName, prefix) || parseErr != nil || t.ClaimName != prefix+strconv.FormatUint(ordinal, 10) {
		return fmt.Errorf("target claim does not match the framework role-group slot")
	}
	if op.Spec.Action == ActionMigrate && t.ClaimName == s.ClaimName {
		return fmt.Errorf("migration requires a distinct target claim")
	}
	if op.Spec.Action == ActionAdopt && (t.Source.StorageClass != s.Source.StorageClass || t.Source.Capacity != s.Source.Capacity) {
		return fmt.Errorf("same-volume adoption cannot change class or capacity; use migrate")
	}
	return nil
}
func (r *reconciler) checkCluster(ctx context.Context, namespace string, ref ClusterRef, allowMissing bool) error {
	cluster := &unstructured.Unstructured{}
	cluster.SetAPIVersion(ref.APIVersion)
	cluster.SetKind(ref.Kind)
	err := r.Client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: ref.Name}, cluster)
	if apierrors.IsNotFound(err) && allowMissing {
		return nil
	}
	if err != nil {
		return err
	}
	if cluster.GetUID() != ref.UID {
		if allowMissing {
			return nil
		}
		return fmt.Errorf("cluster %s UID changed", ref.Name)
	}
	paused, _, err := unstructured.NestedBool(cluster.Object, "spec", "clusterConfig", "reconciliationPaused")
	if err != nil || !paused {
		return fmt.Errorf("cluster %s must be explicitly reconciliationPaused", ref.Name)
	}
	return nil
}
func (r *reconciler) checkQuiescent(ctx context.Context, op *DataOperation) error {
	if err := r.checkCluster(ctx, op.Namespace, op.Spec.SourceCluster, true); err != nil {
		return err
	}
	if op.Spec.Target != nil {
		if err := r.checkCluster(ctx, op.Namespace, op.Spec.Target.Cluster, false); err != nil {
			return err
		}
	}
	var workloads appsv1.StatefulSetList
	if err := r.Client.List(ctx, &workloads, client.InNamespace(op.Namespace)); err != nil {
		return err
	}
	for _, sts := range workloads.Items {
		owner := metav1.GetControllerOf(&sts)
		if owner != nil && (owner.UID == op.Spec.SourceCluster.UID || (op.Spec.Target != nil && owner.UID == op.Spec.Target.Cluster.UID)) {
			return fmt.Errorf("retire source and target StatefulSets before data operation: %s remains", sts.Name)
		}
	}
	var pods corev1.PodList
	if err := r.Client.List(ctx, &pods, client.InNamespace(op.Namespace)); err != nil {
		return err
	}
	for _, pod := range pods.Items {
		for _, volume := range pod.Spec.Volumes {
			if volume.PersistentVolumeClaim == nil {
				continue
			}
			name := volume.PersistentVolumeClaim.ClaimName
			if name != op.Spec.Source.ClaimName && (op.Spec.Target == nil || name != op.Spec.Target.ClaimName) {
				continue
			}
			owner := metav1.GetControllerOf(&pod)
			if owner != nil && owner.Kind == "Job" && owner.UID == op.Status.JobUID && op.Status.JobUID != "" && owner.Name == jobName(op) {
				continue
			}
			return fmt.Errorf("PVC %s still has consumer Pod %s", name, pod.Name)
		}
	}
	return nil
}
func checkPV(pv *corev1.PersistentVolume, identity DataIdentity) error {
	if pv.UID != identity.Binding.PVUID || pv.Name != identity.Binding.VolumeName || pv.Spec.PersistentVolumeReclaimPolicy != corev1.PersistentVolumeReclaimRetain || len(pv.OwnerReferences) != 0 || pv.Spec.StorageClassName != identity.Source.StorageClass {
		return fmt.Errorf("PV identity, ownership or Retain policy changed")
	}
	return nil
}
func (r *reconciler) sourcePair(ctx context.Context, op *DataOperation, allowAbsent bool) (*corev1.PersistentVolumeClaim, *corev1.PersistentVolume, error) {
	s := op.Spec.Source
	class := &storagev1.StorageClass{}
	if err := r.Client.Get(ctx, client.ObjectKey{Name: s.Source.StorageClass}, class); err != nil {
		return nil, nil, err
	}
	if class.ReclaimPolicy == nil || *class.ReclaimPolicy != corev1.PersistentVolumeReclaimRetain || !class.DeletionTimestamp.IsZero() {
		return nil, nil, fmt.Errorf("source StorageClass no longer retains")
	}
	pv := &corev1.PersistentVolume{}
	if err := r.Client.Get(ctx, client.ObjectKey{Name: s.Binding.VolumeName}, pv); err != nil {
		return nil, nil, err
	}
	if err := checkPV(pv, s); err != nil {
		return nil, nil, err
	}
	claim := &corev1.PersistentVolumeClaim{}
	err := r.Client.Get(ctx, client.ObjectKey{Namespace: op.Namespace, Name: s.ClaimName}, claim)
	if apierrors.IsNotFound(err) && allowAbsent {
		return nil, pv, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var source Source
	var binding Binding
	if err = json.Unmarshal([]byte(claim.Annotations[SourceAnnotation]), &source); err != nil {
		return nil, nil, err
	}
	if err = json.Unmarshal([]byte(claim.Annotations[BindingAnnotation]), &binding); err != nil {
		return nil, nil, err
	}
	if claim.Annotations[AssetAnnotation] != op.Spec.AssetName || claim.UID != s.Binding.PVCUID || source != s.Source || binding != s.Binding || len(claim.OwnerReferences) != 0 || claim.Spec.VolumeName != pv.Name || pv.Spec.ClaimRef == nil || pv.Spec.ClaimRef.UID != claim.UID || pv.Spec.ClaimRef.Name != claim.Name || pv.Spec.ClaimRef.Namespace != claim.Namespace {
		return nil, nil, fmt.Errorf("source PVC/PV binding or provenance changed")
	}
	return claim, pv, nil
}
func (r *reconciler) targetClaim(ctx context.Context, op *DataOperation, volume string) (*corev1.PersistentVolumeClaim, error) {
	t := op.Spec.Target
	claim := &corev1.PersistentVolumeClaim{}
	err := r.Client.Get(ctx, client.ObjectKey{Namespace: op.Namespace, Name: t.ClaimName}, claim)
	if err == nil {
		if claim.Annotations[LockAnnotation] != string(op.UID) {
			return nil, fmt.Errorf("target PVC is not owned by this operation")
		}
		if op.Status.Target != nil && claim.UID != op.Status.Target.Binding.PVCUID {
			return nil, fmt.Errorf("target PVC UID changed")
		}
		if op.Status.Target == nil {
			op.Status.Target = &DataIdentity{Cluster: t.Cluster, ClaimName: claim.Name, Source: t.Source, Binding: Binding{Version: 1, PVCUID: claim.UID}}
		}
		return claim, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, err
	}
	if op.Status.Target != nil {
		return nil, fmt.Errorf("target PVC disappeared")
	}
	class := &storagev1.StorageClass{}
	if err = r.Client.Get(ctx, client.ObjectKey{Name: t.Source.StorageClass}, class); err != nil {
		return nil, err
	}
	if class.ReclaimPolicy == nil || *class.ReclaimPolicy != corev1.PersistentVolumeReclaimRetain {
		return nil, fmt.Errorf("target StorageClass must retain")
	}
	mode := corev1.PersistentVolumeFilesystem
	capacity := resource.MustParse(t.Source.Capacity)
	claim = &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: t.ClaimName, Namespace: op.Namespace, Annotations: map[string]string{LockAnnotation: string(op.UID)}}, Spec: corev1.PersistentVolumeClaimSpec{StorageClassName: &t.Source.StorageClass, VolumeName: volume, VolumeMode: &mode, AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}, Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: capacity}}}}
	if err = r.Client.Create(ctx, claim); err != nil {
		return nil, err
	}
	op.Status.Target = &DataIdentity{Cluster: t.Cluster, ClaimName: claim.Name, Source: t.Source, Binding: Binding{Version: 1, PVCUID: claim.UID}}
	return claim, nil
}
func (r *reconciler) finishTarget(ctx context.Context, op *DataOperation) (*DataIdentity, error) {
	claim := &corev1.PersistentVolumeClaim{}
	if err := r.Client.Get(ctx, client.ObjectKey{Namespace: op.Namespace, Name: op.Spec.Target.ClaimName}, claim); err != nil {
		return nil, err
	}
	if op.Spec.Target.ClaimName == op.Spec.Source.ClaimName {
		if claim.UID != op.Spec.Source.Binding.PVCUID {
			return nil, fmt.Errorf("same-name adoption PVC UID changed")
		}
	} else if op.Status.Target == nil || claim.UID != op.Status.Target.Binding.PVCUID || claim.Annotations[LockAnnotation] != string(op.UID) {
		return nil, fmt.Errorf("target identity changed")
	}
	pv := &corev1.PersistentVolume{}
	if err := r.Client.Get(ctx, client.ObjectKey{Name: claim.Spec.VolumeName}, pv); err != nil {
		return nil, err
	}
	identity := DataIdentity{Cluster: op.Spec.Target.Cluster, ClaimName: claim.Name, Source: op.Spec.Target.Source, Binding: Binding{Version: 1, PVCUID: claim.UID, PVUID: pv.UID, VolumeName: pv.Name}}
	if err := checkPV(pv, identity); err != nil {
		return nil, err
	}
	if claim.Status.Phase != corev1.ClaimBound || pv.Status.Phase != corev1.VolumeBound || pv.Spec.ClaimRef == nil || pv.Spec.ClaimRef.UID != claim.UID || pv.Spec.ClaimRef.Name != claim.Name || pv.Spec.ClaimRef.Namespace != claim.Namespace {
		return nil, fmt.Errorf("waiting for exact target bidirectional binding")
	}
	if op.Spec.Action == ActionAdopt && pv.UID != op.Spec.Source.Binding.PVUID {
		return nil, fmt.Errorf("adoption changed PV identity")
	}
	source, _ := json.Marshal(identity.Source)
	binding, _ := json.Marshal(identity.Binding)
	if claim.Annotations == nil {
		claim.Annotations = map[string]string{}
	}
	claim.Annotations[SourceAnnotation] = string(source)
	claim.Annotations[BindingAnnotation] = string(binding)
	claim.Annotations[AssetAnnotation] = op.Spec.AssetName
	if err := r.Client.Update(ctx, claim); err != nil {
		return nil, err
	}
	return &identity, nil
}
func (r *reconciler) runJob(ctx context.Context, op *DataOperation) (bool, error) {
	job := &batchv1.Job{}
	err := r.Client.Get(ctx, client.ObjectKey{Namespace: op.Namespace, Name: jobName(op)}, job)
	if apierrors.IsNotFound(err) {
		if op.Status.JobUID != "" {
			return false, fmt.Errorf("operation Job disappeared; refusing to invent completion")
		}
		job = jobFor(op, r.WorkerImage)
		if err = r.Client.Create(ctx, job); err != nil {
			return false, err
		}
		op.Status.JobUID = job.UID
		return false, nil
	}
	if err != nil {
		return false, err
	}
	owner := metav1.GetControllerOf(job)
	if owner == nil || owner.UID != op.UID || (op.Status.JobUID != "" && job.UID != op.Status.JobUID) {
		return false, fmt.Errorf("operation Job identity changed")
	}
	if err := checkWorkerSpec(op, job, r.WorkerImage); err != nil {
		return false, err
	}
	op.Status.JobUID = job.UID
	for _, condition := range job.Status.Conditions {
		if condition.Type == batchv1.JobFailed && condition.Status == corev1.ConditionTrue {
			return false, r.retryWorker(ctx, op, job, condition.Reason+": "+condition.Message)
		}
	}
	complete := false
	for _, condition := range job.Status.Conditions {
		if condition.Type == batchv1.JobComplete && condition.Status == corev1.ConditionTrue {
			complete = true
		}
	}
	if !complete {
		return false, nil
	}
	var pods corev1.PodList
	if err = r.Client.List(ctx, &pods, client.InNamespace(op.Namespace)); err != nil {
		return false, err
	}
	if op.Status.WorkerReceipt == "" {
		return false, captureReceipt(op, job, pods.Items)
	}
	found := false
	for i := range pods.Items {
		pod := &pods.Items[i]
		owner := metav1.GetControllerOf(pod)
		if owner != nil && owner.UID == job.UID {
			found = true
			if err = r.deleteExact(ctx, pod); err != nil {
				return false, err
			}
		}
	}
	return !found, nil
}

func validateTarget(op *DataOperation) error {
	t := op.Spec.Target
	if (op.Spec.Action != ActionMigrate && op.Spec.Action != ActionAdopt) || t == nil {
		return fmt.Errorf("operation requires adopt/migrate target")
	}
	if t.Cluster.UID == "" || t.Cluster.UID != t.Source.CRUID || t.Cluster.Name == "" || t.Cluster.APIVersion == "" || t.Cluster.Kind == "" {
		return fmt.Errorf("complete target cluster identity is required")
	}
	if t.Source.Role == "" || t.Source.Group == "" || t.Source.Slot == "" || t.Source.Version != 1 {
		return fmt.Errorf("complete target source is required")
	}
	return nil
}

func (r *reconciler) retryWorker(ctx context.Context, op *DataOperation, job *batchv1.Job, message string) error {
	requested, err := strconv.ParseInt(op.Annotations[RetryAnnotation], 10, 32)
	if err != nil || requested != int64(op.Status.Attempt)+1 {
		return fmt.Errorf("worker failed: %s; inspect retained Job/logs, remove failed Pods and request next data-retry attempt", message)
	}
	var pods corev1.PodList
	if err := r.Client.List(ctx, &pods, client.InNamespace(op.Namespace)); err != nil {
		return err
	}
	for _, pod := range pods.Items {
		owner := metav1.GetControllerOf(&pod)
		if owner != nil && owner.UID == job.UID {
			return fmt.Errorf("inspect and remove failed worker Pod %s before retry", pod.Name)
		}
	}
	op.Status.Attempt = int32(requested)
	op.Status.JobUID = ""
	op.Status.WorkerReceipt = ""
	return nil
}

func captureReceipt(op *DataOperation, job *batchv1.Job, items []corev1.Pod) error {

	for _, pod := range items {
		owner := metav1.GetControllerOf(&pod)
		if owner == nil || owner.UID != job.UID {
			continue
		}
		for _, status := range pod.Status.ContainerStatuses {
			if status.Name == workerContainer && status.State.Terminated != nil && status.State.Terminated.ExitCode == 0 {
				var receipt struct {
					Operation string `json:"operation"`
					Verified  string `json:"verified"`
					Digest    string `json:"digest"`
				}
				if err := json.Unmarshal([]byte(status.State.Terminated.Message), &receipt); err != nil {
					return err
				}
				expected := "empty-filesystem"
				if op.Spec.Action == ActionMigrate {
					expected = "sha256-tree"
				}
				if receipt.Operation != string(op.UID) || receipt.Verified != expected || (op.Spec.Action == ActionMigrate && len(receipt.Digest) != 64) {
					return fmt.Errorf("invalid worker completion receipt")
				}
				op.Status.WorkerReceipt = status.State.Terminated.Message
				return nil
			}
		}
	}
	return fmt.Errorf("completed Job has no verified worker receipt")
}

func checkWorkerSpec(op *DataOperation, job *batchv1.Job, image string) error {
	actual := job.Spec.Template.Spec
	expected := jobFor(op, image).Spec.Template.Spec
	if len(actual.Containers) != 1 || len(actual.InitContainers) != 0 || actual.ServiceAccountName != "" && actual.ServiceAccountName != "default" {
		return fmt.Errorf("operation worker process shape changed")
	}
	process, want := actual.Containers[0], expected.Containers[0]
	if !reflect.DeepEqual(process.SecurityContext, want.SecurityContext) {
		return fmt.Errorf("worker container execution identity or security restrictions changed")
	}
	security := actual.SecurityContext
	required := expected.SecurityContext
	if security == nil || !reflect.DeepEqual(security.RunAsUser, required.RunAsUser) || !reflect.DeepEqual(security.RunAsGroup, required.RunAsGroup) || !reflect.DeepEqual(security.FSGroup, required.FSGroup) || !reflect.DeepEqual(security.RunAsNonRoot, required.RunAsNonRoot) || len(security.SupplementalGroups) != 0 || actual.AutomountServiceAccountToken == nil || *actual.AutomountServiceAccountToken {
		return fmt.Errorf("worker execution identity changed")
	}
	if process.Image != want.Image || !reflect.DeepEqual(process.Command, want.Command) || len(process.Args) != 0 || len(process.Env) != 0 || len(process.EnvFrom) != 0 || !reflect.DeepEqual(process.VolumeMounts, want.VolumeMounts) || !reflect.DeepEqual(actual.Volumes, expected.Volumes) {
		return fmt.Errorf("operation worker image, commands or data mounts changed")
	}
	return nil
}
