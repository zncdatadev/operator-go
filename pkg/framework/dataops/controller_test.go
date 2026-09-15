package dataops

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func testController(t *testing.T, action string) (*reconciler, *DataOperation, *DataAsset) {
	t.Helper()
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{corev1.AddToScheme, appsv1.AddToScheme, batchv1.AddToScheme, storagev1.AddToScheme, AddToScheme} {
		if err := add(scheme); err != nil {
			t.Fatal(err)
		}
	}
	gvk := schema.GroupVersionKind{Group: "example.test", Version: "v1", Kind: "Cluster"}
	scheme.AddKnownTypeWithName(gvk, &unstructured.Unstructured{})
	source := Source{Version: 1, CRUID: "source-cr", Role: "worker", Group: "default", Slot: "data", StorageClass: "retained", Capacity: "64Mi"}
	identity := DataIdentity{Cluster: ClusterRef{APIVersion: "example.test/v1", Kind: "Cluster", Name: "old", UID: "source-cr"}, ClaimName: "data-old-worker-default-0", Source: source, Binding: Binding{Version: 1, PVCUID: "source-pvc", PVUID: "source-pv", VolumeName: "old-pv"}}
	asset := &DataAsset{ObjectMeta: metav1.ObjectMeta{Name: "asset", Namespace: "test", UID: "asset-uid"}, Spec: identity}
	encodedSource, _ := json.Marshal(source)
	encodedBinding, _ := json.Marshal(identity.Binding)
	class := "retained"
	mode := corev1.PersistentVolumeFilesystem
	claim := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: identity.ClaimName, Namespace: "test", UID: identity.Binding.PVCUID, Annotations: map[string]string{SourceAnnotation: string(encodedSource), BindingAnnotation: string(encodedBinding), AssetAnnotation: asset.Name}}, Spec: corev1.PersistentVolumeClaimSpec{VolumeName: "old-pv", StorageClassName: &class, VolumeMode: &mode, AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}, Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("64Mi")}}}, Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound}}
	pv := &corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "old-pv", UID: "source-pv"}, Spec: corev1.PersistentVolumeSpec{PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain, StorageClassName: class, ClaimRef: &corev1.ObjectReference{Name: claim.Name, Namespace: claim.Namespace, UID: claim.UID}}, Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeBound}}
	target := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "example.test/v1", "kind": "Cluster", "metadata": map[string]any{"name": "new", "namespace": "test", "uid": "target-cr"}, "spec": map[string]any{"clusterConfig": map[string]any{"reconciliationPaused": true}}}}
	retain := corev1.PersistentVolumeReclaimRetain
	storageClass := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: class}, ReclaimPolicy: &retain}
	op := &DataOperation{ObjectMeta: metav1.ObjectMeta{Name: "operation", Namespace: "test", UID: "operation-uid"}, Spec: OperationSpec{WorkerIdentity: WorkerIdentity{UID: 1000, GID: 1000}, Action: action, AssetName: asset.Name, AssetUID: asset.UID, Source: identity, SourceCluster: ClusterRef{APIVersion: "example.test/v1", Kind: "Cluster", Name: "old", UID: source.CRUID}}}
	if action != ActionDestroy {
		next := source
		next.CRUID = "target-cr"
		op.Spec.Target = &Target{Cluster: ClusterRef{APIVersion: "example.test/v1", Kind: "Cluster", Name: "new", UID: "target-cr"}, ClaimName: "data-new-worker-default-0", Source: next}
	}
	op.Spec.Approval = Approval(op.Spec)
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&DataAsset{}, &DataOperation{}, &corev1.PersistentVolumeClaim{}, &corev1.PersistentVolume{}, &batchv1.Job{}, &corev1.Pod{}).WithObjects(asset, claim, pv, target, storageClass, op).WithInterceptorFuncs(interceptor.Funcs{Create: func(ctx context.Context, c client.WithWatch, obj client.Object, options ...client.CreateOption) error {
		if obj.GetUID() == "" {
			obj.SetUID(types.UID("uid-" + obj.GetName()))
		}
		return c.Create(ctx, obj, options...)
	}}).Build()
	return &reconciler{Client: c, WorkerImage: "python:test"}, op, asset
}
func tick(t *testing.T, r *reconciler, op *DataOperation) {
	t.Helper()
	restarted := &reconciler{Client: r.Client, WorkerImage: r.WorkerImage}
	if _, err := restarted.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(op)}); err != nil {
		t.Fatal(err)
	}
	if err := r.Client.Get(t.Context(), client.ObjectKeyFromObject(op), op); err != nil {
		t.Fatal(err)
	}
}
func TestAdoptRebindPersistsAcrossEveryRestart(t *testing.T) {
	r, op, asset := testController(t, ActionAdopt)
	for i := 0; i < 5; i++ {
		tick(t, r, op)
		if op.Status.Message != "" {
			t.Fatal(op.Status.Message)
		}
	}
	claim := &corev1.PersistentVolumeClaim{}
	if err := r.Client.Get(t.Context(), client.ObjectKey{Namespace: op.Namespace, Name: op.Spec.Target.ClaimName}, claim); err != nil {
		t.Fatal(err)
	}
	claim.Status.Phase = corev1.ClaimBound
	if err := r.Client.Status().Update(t.Context(), claim); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		tick(t, r, op)
	}
	if op.Status.Phase != "Complete" {
		t.Fatalf("%+v", op.Status)
	}
	if err := r.Client.Get(t.Context(), client.ObjectKeyFromObject(asset), asset); err != nil {
		t.Fatal(err)
	}
	if len(asset.Status.History) != 1 || asset.Status.Current.Binding.PVUID != "source-pv" || asset.Status.Current.Source.CRUID != "target-cr" {
		t.Fatalf("%+v", asset.Status)
	}
	if err := r.Client.Get(t.Context(), client.ObjectKeyFromObject(claim), claim); err != nil {
		t.Fatal(err)
	}
	if err := EnsureAsset(t.Context(), r.Client, claim, op.Spec.Target.Cluster); err != nil {
		t.Fatal(err)
	}
	tick(t, r, op)
	if len(asset.Status.History) != 1 {
		t.Fatal("duplicated history")
	}
}
func finishWorker(t *testing.T, r *reconciler, op *DataOperation) {
	t.Helper()
	job := &batchv1.Job{}
	if err := r.Client.Get(t.Context(), client.ObjectKey{Namespace: op.Namespace, Name: jobName(op)}, job); err != nil {
		t.Fatal(err)
	}
	job.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}
	if err := r.Client.Status().Update(t.Context(), job); err != nil {
		t.Fatal(err)
	}
	yes := true
	verified := "empty-filesystem"
	if op.Spec.Action == ActionMigrate {
		verified = "sha256-tree"
	}
	receipt := `{"operation":"` + string(op.UID) + `","verified":"` + verified + `","digest":"` + strings.Repeat("a", 64) + `"}`
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "worker", Namespace: op.Namespace, OwnerReferences: []metav1.OwnerReference{{APIVersion: "batch/v1", Kind: "Job", Name: job.Name, UID: job.UID, Controller: &yes}}}, Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{Name: "data", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 0, Message: receipt}}}}}}
	if err := r.Client.Create(t.Context(), pod); err != nil {
		t.Fatal(err)
	}
}
func TestDestroyRequiresVerifiedEraseBeforeDeletingIdentities(t *testing.T) {
	r, op, asset := testController(t, ActionDestroy)
	for i := 0; i < 3; i++ {
		tick(t, r, op)
	}
	if op.Status.Phase != "Erase" {
		t.Fatalf("%+v", op.Status)
	}
	claim := &corev1.PersistentVolumeClaim{}
	if err := r.Client.Get(t.Context(), client.ObjectKey{Namespace: op.Namespace, Name: op.Spec.Source.ClaimName}, claim); err != nil {
		t.Fatal("deleted before worker completion", err)
	}
	finishWorker(t, r, op)
	finishDestruction(t, r, op)
	if op.Status.Phase != "Complete" {
		t.Fatalf("%+v", op.Status)
	}
	if err := r.Client.Get(t.Context(), client.ObjectKeyFromObject(asset), asset); err != nil {
		t.Fatal(err)
	}
	if !asset.Status.Destroyed || len(asset.Status.History) != 1 || !strings.Contains(asset.Status.History[0].Verification, "empty-filesystem") {
		t.Fatalf("%+v", asset.Status)
	}
}
func TestOperationRefusesConsumerAndChangedApproval(t *testing.T) {
	r, op, _ := testController(t, ActionDestroy)
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foreign", Namespace: op.Namespace}, Spec: corev1.PodSpec{Volumes: []corev1.Volume{{Name: "data", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: op.Spec.Source.ClaimName}}}}}}
	if err := r.Client.Create(t.Context(), pod); err != nil {
		t.Fatal(err)
	}
	tick(t, r, op)
	if !strings.Contains(op.Status.Message, "consumer Pod") {
		t.Fatalf("%+v", op.Status)
	}
	op.Spec.Approval = "wrong"
	if err := r.Client.Update(t.Context(), op); err != nil {
		t.Fatal(err)
	}
	tick(t, r, op)
	if !strings.Contains(op.Status.Message, "approval") {
		t.Fatalf("%+v", op.Status)
	}
}
func TestWorkerCopiesVerifiesAndErasesActualBytes(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	source, target := t.TempDir(), t.TempDir()
	if err = os.Chmod(target, 0770); err != nil {
		t.Fatal(err)
	}
	// A worker with fsGroup write access does not own the provisioner's volume
	// root. Model its metadata protection while executing the real copy script.
	guardedScript := `import shutil, os, sys
original_copystat = shutil.copystat
def protected_copystat(source, destination, **kwargs):
    if os.path.abspath(destination) == os.path.abspath(sys.argv[4]):
        raise PermissionError("provisioner-owned volume root")
    return original_copystat(source, destination, **kwargs)
shutil.copystat = protected_copystat
` + workerScript
	if err = os.Mkdir(filepath.Join(source, "nested"), 0750); err != nil {
		t.Fatal(err)
	}
	data := []byte(strings.Repeat("retained-marker\x00", 1000))
	if err = os.WriteFile(filepath.Join(source, "nested", "data"), data, 0640); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink("nested/data", filepath.Join(source, "link")); err != nil {
		t.Fatal(err)
	}
	run := func(action string) {
		t.Helper()
		out, err := exec.CommandContext(t.Context(), python, "-c", guardedScript, action, "test-operation", source, target).CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %s", err, out)
		}
	}
	run(ActionMigrate)
	run(ActionMigrate)
	info, err := os.Stat(target)
	if err != nil || info.Mode().Perm() != 0770 {
		t.Fatal("migration changed the provisioner-owned mount root", err)
	}
	copied, err := os.ReadFile(filepath.Join(target, "nested", "data"))
	if err != nil || string(copied) != string(data) {
		t.Fatal("copied bytes differ", err)
	}
	run(ActionDestroy)
	entries, err := os.ReadDir(source)
	if err != nil || len(entries) != 0 {
		t.Fatal("erase did not remove bytes", err)
	}
}

func TestMigrationCopiesToNewBindingAndRetainsOperableSourceHistory(t *testing.T) {
	r, op, asset := testController(t, ActionMigrate)
	for i := 0; i < 3; i++ {
		tick(t, r, op)
	}
	if op.Status.Phase != "Copy" || op.Status.Target == nil {
		t.Fatalf("%+v", op.Status)
	}
	target := &corev1.PersistentVolumeClaim{}
	if err := r.Client.Get(t.Context(), client.ObjectKey{Namespace: op.Namespace, Name: op.Spec.Target.ClaimName}, target); err != nil {
		t.Fatal(err)
	}
	target.Spec.VolumeName = "new-pv"
	if err := r.Client.Update(t.Context(), target); err != nil {
		t.Fatal(err)
	}
	target.Status.Phase = corev1.ClaimBound
	if err := r.Client.Status().Update(t.Context(), target); err != nil {
		t.Fatal(err)
	}
	pv := &corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "new-pv", UID: "new-pv-uid"}, Spec: corev1.PersistentVolumeSpec{PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain, StorageClassName: "retained", ClaimRef: &corev1.ObjectReference{Name: target.Name, Namespace: target.Namespace, UID: target.UID}}, Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeBound}}
	if err := r.Client.Create(t.Context(), pv); err != nil {
		t.Fatal(err)
	}
	finishWorker(t, r, op)
	for i := 0; i < 7; i++ {
		tick(t, r, op)
	}
	if op.Status.Phase != phaseComplete {
		t.Fatalf("%+v", op.Status)
	}
	if err := r.Client.Get(t.Context(), client.ObjectKeyFromObject(asset), asset); err != nil {
		t.Fatal(err)
	}
	if asset.Status.Current.Binding.PVUID != "new-pv-uid" || len(asset.Status.RetiredCopies) != 1 || !strings.Contains(asset.Status.History[0].Verification, "sha256-tree") {
		t.Fatalf("%+v", asset.Status)
	}
	old := &corev1.PersistentVolumeClaim{}
	if err := r.Client.Get(t.Context(), client.ObjectKey{Namespace: op.Namespace, Name: op.Spec.Source.ClaimName}, old); err != nil {
		t.Fatal("migration deleted source before explicit authorization", err)
	}
	destroy := &DataOperation{ObjectMeta: metav1.ObjectMeta{Name: "destroy-copy", Namespace: op.Namespace, UID: "destroy-copy-uid"}, Spec: OperationSpec{WorkerIdentity: WorkerIdentity{UID: 1000, GID: 1000}, Action: ActionDestroy, AssetName: asset.Name, AssetUID: asset.UID, Source: op.Spec.Source, SourceCluster: op.Spec.SourceCluster}}
	destroy.Spec.Approval = Approval(destroy.Spec)
	if err := r.Client.Create(t.Context(), destroy); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		tick(t, r, destroy)
	}
	finishWorker(t, r, destroy)
	finishDestruction(t, r, destroy)
	if destroy.Status.Phase != phaseComplete {
		t.Fatalf("%+v", destroy.Status)
	}
	if err := r.Client.Get(t.Context(), client.ObjectKeyFromObject(asset), asset); err != nil {
		t.Fatal(err)
	}
	if asset.Status.Destroyed || len(asset.Status.RetiredCopies) != 0 || asset.Status.Current.Binding.PVUID != "new-pv-uid" {
		t.Fatalf("destroying retired copy affected current data: %+v", asset.Status)
	}
}

func TestFailedWorkerRequiresExplicitRetryAndPreservesJobEvidence(t *testing.T) {
	r, op, _ := testController(t, ActionDestroy)
	for i := 0; i < 3; i++ {
		tick(t, r, op)
	}
	job := &batchv1.Job{}
	if err := r.Client.Get(t.Context(), client.ObjectKey{Namespace: op.Namespace, Name: jobName(op)}, job); err != nil {
		t.Fatal(err)
	}
	job.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Reason: "DeadlineExceeded"}}
	if err := r.Client.Status().Update(t.Context(), job); err != nil {
		t.Fatal(err)
	}
	tick(t, r, op)
	if !strings.Contains(op.Status.Message, "data-retry") || op.Status.Attempt != 0 {
		t.Fatalf("%+v", op.Status)
	}
	op.Annotations = map[string]string{RetryAnnotation: "1"}
	if err := r.Client.Update(t.Context(), op); err != nil {
		t.Fatal(err)
	}
	tick(t, r, op)
	tick(t, r, op)
	if op.Status.Attempt != 1 || op.Status.JobUID == job.UID || op.Status.JobUID == "" {
		t.Fatalf("%+v", op.Status)
	}
	if err := r.Client.Get(t.Context(), client.ObjectKeyFromObject(job), job); err != nil {
		t.Fatal("failed Job evidence lost", err)
	}
}

func TestLedgerRefusesLostOriginalClaim(t *testing.T) {
	r, op, _ := testController(t, ActionDestroy)
	if err := CheckHistory(t.Context(), r.Client, op.Namespace, op.Spec.Source.Source, nil); err == nil {
		t.Fatal("lost data was treated as first creation")
	}
}

func finishDestruction(t *testing.T, r *reconciler, op *DataOperation) {
	t.Helper()
	for i := 0; i < 12; i++ {
		tick(t, r, op)
		if op.Status.Phase == "ReclaimVolume" {
			pv := &corev1.PersistentVolume{}
			if err := r.Client.Get(t.Context(), client.ObjectKey{Name: op.Spec.Source.Binding.VolumeName}, pv); err != nil {
				t.Fatal(err)
			}
			if pv.Spec.PersistentVolumeReclaimPolicy != corev1.PersistentVolumeReclaimDelete || pv.Annotations[LockAnnotation] != string(op.UID) {
				t.Fatal("reclaim was not bound to the exact authorized operation")
			}
			// A fake client has no provisioner. Its completion is explicitly simulated;
			// the live harness must observe real backend reclamation before completion.
			if err := r.Client.Delete(t.Context(), pv); err != nil {
				t.Fatal(err)
			}
		}
	}
}
