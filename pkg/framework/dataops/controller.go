package dataops

import (
	"context"
	"fmt"
	"time"

	apiequality "k8s.io/apimachinery/pkg/api/equality"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Options struct{ WorkerImage string }

// +kubebuilder:object:generate=false
type reconciler struct {
	Client      client.Client
	WorkerImage string
}

func Register(manager ctrl.Manager, options Options) error {
	if options.WorkerImage == "" {
		return fmt.Errorf("data operations require an explicit worker image")
	}
	if err := AddToScheme(manager.GetScheme()); err != nil {
		return err
	}
	if err := batchv1.AddToScheme(manager.GetScheme()); err != nil {
		return err
	}
	direct, err := client.New(manager.GetConfig(), client.Options{Scheme: manager.GetScheme(), Mapper: manager.GetRESTMapper()})
	if err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(manager).Named("framework-data-operations").For(&DataOperation{}).Owns(&batchv1.Job{}).Complete(&reconciler{Client: direct, WorkerImage: options.WorkerImage})
}
func (r *reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	op := &DataOperation{}
	if err := r.Client.Get(ctx, request.NamespacedName, op); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if op.Status.Phase == phaseComplete {
		return ctrl.Result{}, nil
	}
	before := op.Status.DeepCopy()
	err := r.advance(ctx, op)
	if err != nil {
		op.Status.Message = err.Error()
	} else {
		op.Status.Message = ""
	}
	if !apiequality.Semantic.DeepEqual(*before, op.Status) {
		if writeErr := r.Client.Status().Update(ctx, op); writeErr != nil {
			return ctrl.Result{}, writeErr
		}
	}
	if err != nil {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
}
func (r *reconciler) advance(ctx context.Context, op *DataOperation) error {
	digest := Approval(op.Spec)
	if op.Spec.Approval != digest || (op.Status.SpecDigest != "" && op.Status.SpecDigest != digest) {
		return fmt.Errorf("operation approval does not bind the current immutable intent")
	}
	if !op.DeletionTimestamp.IsZero() {
		return fmt.Errorf("operation deletion requested; execution suspended with retained locks")
	}
	if err := validateIntent(op); err != nil {
		return err
	}
	asset := &DataAsset{}
	if err := r.Client.Get(ctx, client.ObjectKey{Namespace: op.Namespace, Name: op.Spec.AssetName}, asset); err != nil {
		return err
	}
	if asset.UID != op.Spec.AssetUID {
		return fmt.Errorf("asset UID changed")
	}
	if asset.Annotations[LockAnnotation] != "" && asset.Annotations[LockAnnotation] != string(op.UID) {
		return fmt.Errorf("asset is locked by another operation")
	}
	if op.Status.Phase == "" {
		current := asset.Spec
		if asset.Status.Current != nil {
			current = *asset.Status.Current
		}
		historicalDestroy := allowsHistoricalDestroy(asset, op)
		if !historicalDestroy && (asset.Status.Destroyed || current != op.Spec.Source) {
			return fmt.Errorf("authorized source differs from asset current identity")
		}
		if err := r.checkQuiescent(ctx, op); err != nil {
			return err
		}
		if _, _, err := r.sourcePair(ctx, op, false); err != nil {
			return err
		}
		if asset.Annotations == nil {
			asset.Annotations = map[string]string{}
		}
		asset.Annotations[LockAnnotation] = string(op.UID)
		if err := r.Client.Update(ctx, asset); err != nil {
			return err
		}
		op.Status.SpecDigest = digest
		op.Status.Phase = phaseLocked
		return nil
	}
	if op.Status.Phase == phaseRecord {
		for _, entry := range asset.Status.History {
			if entry.OperationUID == op.UID {
				return r.record(ctx, op, asset)
			}
		}
	}
	if asset.Annotations[LockAnnotation] != string(op.UID) {
		return fmt.Errorf("operation lost asset lock")
	}
	if op.Status.Phase == phaseRecord {
		return r.record(ctx, op, asset)
	}
	if err := r.checkQuiescent(ctx, op); err != nil {
		return err
	}
	switch op.Spec.Action {
	case ActionMigrate:
		return r.migrate(ctx, op)
	case ActionAdopt:
		return r.adopt(ctx, op)
	case ActionDestroy:
		return r.destroy(ctx, op)
	}
	return fmt.Errorf("unsupported action")
}
func (r *reconciler) migrate(ctx context.Context, op *DataOperation) error {
	if _, _, err := r.sourcePair(ctx, op, false); err != nil {
		return err
	}
	switch op.Status.Phase {
	case phaseLocked:
		if _, err := r.targetClaim(ctx, op, ""); err != nil {
			return err
		}
		op.Status.Phase = phaseCopy
		return nil
	case phaseCopy:
		complete, err := r.runJob(ctx, op)
		if err != nil {
			return err
		}
		if !complete {
			return nil
		}
		op.Status.Phase = phaseBindTarget
		return nil
	case phaseBindTarget:
		target, err := r.finishTarget(ctx, op)
		if err != nil {
			return err
		}
		op.Status.Target = target
		op.Status.Phase = phaseRecord
		return nil
	}
	return fmt.Errorf("unknown migration phase %s", op.Status.Phase)
}
func (r *reconciler) adopt(ctx context.Context, op *DataOperation) error {
	switch op.Status.Phase {
	case phaseLocked:
		if _, _, err := r.sourcePair(ctx, op, false); err != nil {
			return err
		}
		if op.Spec.Target.ClaimName == op.Spec.Source.ClaimName {
			op.Status.Phase = phaseBindTarget
			return nil
		}
		if _, err := r.targetClaim(ctx, op, op.Spec.Source.Binding.VolumeName); err != nil {
			return err
		}
		op.Status.Phase = "ReleaseSource"
		return nil
	case "ReleaseSource":
		claim, _, err := r.sourcePair(ctx, op, true)
		if err != nil {
			return err
		}
		if claim != nil {
			return r.deleteExact(ctx, claim)
		}
		op.Status.Phase = "Rebind"
		return nil
	case "Rebind":
		claim, err := r.targetClaim(ctx, op, op.Spec.Source.Binding.VolumeName)
		if err != nil {
			return err
		}
		_, pv, err := r.sourcePair(ctx, op, true)
		if err != nil {
			return err
		}
		ref := pv.Spec.ClaimRef
		if ref != nil && ref.UID != op.Spec.Source.Binding.PVCUID && ref.UID != claim.UID {
			return fmt.Errorf("PV claimed by another identity")
		}
		if ref == nil || ref.UID != claim.UID {
			pv.Spec.ClaimRef = &corev1.ObjectReference{APIVersion: "v1", Kind: "PersistentVolumeClaim", Name: claim.Name, Namespace: claim.Namespace, UID: claim.UID}
			if err = r.Client.Update(ctx, pv); err != nil {
				return err
			}
		}
		op.Status.Phase = phaseBindTarget
		return nil
	case phaseBindTarget:
		target, err := r.finishTarget(ctx, op)
		if err != nil {
			return err
		}
		op.Status.Target = target
		op.Status.Phase = phaseRecord
		return nil
	}
	return fmt.Errorf("unknown adoption phase %s", op.Status.Phase)
}
func (r *reconciler) destroy(ctx context.Context, op *DataOperation) error {
	switch op.Status.Phase {
	case phaseLocked:
		if _, _, err := r.sourcePair(ctx, op, false); err != nil {
			return err
		}
		op.Status.Phase = phaseErase
		return nil
	case phaseErase:
		if _, _, err := r.sourcePair(ctx, op, false); err != nil {
			return err
		}
		complete, err := r.runJob(ctx, op)
		if err != nil {
			return err
		}
		if !complete {
			return nil
		}
		op.Status.Phase = "DeleteClaim"
		return nil
	case "DeleteClaim":
		claim, _, err := r.sourcePair(ctx, op, true)
		if err != nil {
			return err
		}
		if claim != nil {
			return r.deleteExact(ctx, claim)
		}
		op.Status.Phase = phaseDeleteVolume
		return nil
	case phaseDeleteVolume, phaseReclaimVolume:
		pv := &corev1.PersistentVolume{}
		err := r.Client.Get(ctx, client.ObjectKey{Name: op.Spec.Source.Binding.VolumeName}, pv)
		if apierrors.IsNotFound(err) {
			if op.Status.Phase != phaseReclaimVolume {
				return fmt.Errorf("volume disappeared before backend reclamation was persisted; completion is unknown")
			}
			op.Status.Phase = phaseRecord
			return nil
		}
		if err != nil {
			return err
		}
		if pv.UID != op.Spec.Source.Binding.PVUID || len(pv.OwnerReferences) != 0 {
			return fmt.Errorf("volume identity changed before backend reclamation")
		}
		if pv.Spec.PersistentVolumeReclaimPolicy == corev1.PersistentVolumeReclaimDelete {
			if pv.Annotations[LockAnnotation] != string(op.UID) {
				return fmt.Errorf("volume deletion policy changed without this operation receipt")
			}
			op.Status.Phase = phaseReclaimVolume
			return nil
		}
		if err := checkPV(pv, op.Spec.Source); err != nil {
			return err
		}
		if op.Status.WorkerReceipt == "" {
			return fmt.Errorf("backend reclamation requires persisted erase verification")
		}
		if pv.Annotations == nil {
			pv.Annotations = map[string]string{}
		}
		pv.Annotations[LockAnnotation] = string(op.UID)
		pv.Spec.PersistentVolumeReclaimPolicy = corev1.PersistentVolumeReclaimDelete
		if err := r.Client.Update(ctx, pv); err != nil {
			return err
		}
		op.Status.Phase = phaseReclaimVolume
		// The actual storage provisioner must delete its backend volume and the
		// PV. Deleting a Retain PV object alone would silently orphan storage.
		return nil

	}
	return fmt.Errorf("unknown destruction phase %s", op.Status.Phase)
}
func (r *reconciler) deleteExact(ctx context.Context, object client.Object) error {
	uid, rv := object.GetUID(), object.GetResourceVersion()
	return client.IgnoreNotFound(r.Client.Delete(ctx, object, &client.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid, ResourceVersion: &rv}}))
}
func (r *reconciler) record(ctx context.Context, op *DataOperation, asset *DataAsset) error {
	found := false
	for _, entry := range asset.Status.History {
		if entry.OperationUID == op.UID {
			found = true
		}
	}
	if !found {
		asset.Status.History = append(asset.Status.History, History{OperationUID: op.UID, Action: op.Spec.Action, From: op.Spec.Source, To: op.Status.Target, Completed: metav1.Now(), Verification: verification(op)})
		if op.Spec.Action == ActionDestroy {
			current := asset.Spec
			if asset.Status.Current != nil {
				current = *asset.Status.Current
			}
			if current == op.Spec.Source {
				asset.Status.Destroyed = true
			}
			remaining := make([]DataIdentity, 0, len(asset.Status.RetiredCopies))
			for _, copy := range asset.Status.RetiredCopies {
				if copy != op.Spec.Source {
					remaining = append(remaining, copy)
				}
			}
			asset.Status.RetiredCopies = remaining
		} else {
			if op.Spec.Action == ActionMigrate {
				asset.Status.RetiredCopies = append(asset.Status.RetiredCopies, op.Spec.Source)
			}
			asset.Status.Current = op.Status.Target
			asset.Status.Destroyed = false
		}
		if err := r.Client.Status().Update(ctx, asset); err != nil {
			return err
		}
	}
	delete(asset.Annotations, LockAnnotation)
	if err := r.Client.Update(ctx, asset); err != nil {
		return err
	}
	now := metav1.Now()
	op.Status.Completed = &now
	op.Status.Phase = phaseComplete
	return nil
}

func verification(op *DataOperation) string {
	if op.Spec.Action == ActionAdopt {
		return "same-pv-uid"
	}
	return op.Status.WorkerReceipt
}

func allowsHistoricalDestroy(asset *DataAsset, op *DataOperation) bool {
	if op.Spec.Action != ActionDestroy {
		return false
	}
	for _, copy := range asset.Status.RetiredCopies {
		if copy == op.Spec.Source {
			return true
		}
	}
	return false
}
