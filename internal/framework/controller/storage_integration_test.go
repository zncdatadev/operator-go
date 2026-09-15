package controller

import (
	"reflect"
	"testing"

	generatedtrino "github.com/zncdatadev/operator-go/internal/framework/pipeline/testinput"
	"github.com/zncdatadev/operator-go/pkg/framework"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// The API server persists the source/binding records and enforces VCT defaults.
// PV/PVC Bound status below is an explicit fixture observation: envtest runs no
// binder/provisioner and proves neither a mount nor retained filesystem contents.
func integrationRetainedAPI(t *testing.T, c client.Client, scheme *runtime.Scheme) {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "retained-api-"}}
	if err := c.Create(t.Context(), ns); err != nil {
		t.Fatal(err)
	}
	retain, wait := corev1.PersistentVolumeReclaimRetain, storagev1.VolumeBindingWaitForFirstConsumer
	class := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: ns.Name}, Provisioner: "example.invalid/fixture",
		ReclaimPolicy: &retain, VolumeBindingMode: &wait}
	if err := c.Create(t.Context(), class); err != nil {
		t.Fatal(err)
	}
	one := int32(1)
	kind := "persistent"
	lower, upper := resource.MustParse("32Mi"), resource.MustParse("64Mi")
	cr := &generatedtrino.TrinoCluster{ObjectMeta: metav1.ObjectMeta{Name: "retained", Namespace: ns.Name},
		Spec: generatedtrino.SpecInput{Workers: &generatedtrino.RoleInput{
			Config: &generatedtrino.ConfigInput{Resources: &generatedtrino.ConfigInputResources{
				Storage: &generatedtrino.ConfigInputResourcesStorage{Type: &kind,
					StorageClassName: &class.Name, Capacity: &lower}}},
			RoleGroups: map[string]generatedtrino.RoleGroupInput{"default": {Replicas: &one,
				Config: &generatedtrino.ConfigInput{Resources: &generatedtrino.ConfigInputResources{
					Storage: &generatedtrino.ConfigInputResourcesStorage{Capacity: &upper}}}}}}}}
	if err := c.Create(t.Context(), cr); err != nil {
		t.Fatal(err)
	}
	r := newTestReconciler(c, scheme, testFacts{})
	generate := r.Definition.GenerateGroup
	r.Definition.GenerateGroup = func(in framework.EffectiveInput[testConfig, testClusterConfig, testFacts]) (
		framework.RuntimeDescription, error,
	) {
		result, err := generate(in)
		result.Directories = []framework.Directory{{Name: "data", Data: true}}
		result.Main.Access = []framework.DirectoryAccess{{Directory: "data", MountPath: "/data"}}
		return result, err
	}
	request := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(cr)}
	reconcile := func() {
		t.Helper()
		if _, err := r.Reconcile(t.Context(), request); err != nil {
			t.Fatal(err)
		}
	}
	reconcile()
	sts := workloadAPI(t, c, cr, 1)
	if len(sts.Spec.VolumeClaimTemplates) != 1 {
		t.Fatal("missing retained template")
	}
	var source retainedSource
	if err := strictReceipt(sts.Spec.VolumeClaimTemplates[0].Annotations[retainedSourceAnnotation], &source); err != nil {
		t.Fatal(err)
	}
	if source.CRUID != cr.UID || source.Capacity != "64Mi" || source.Slot != "data" {
		t.Fatalf("wrong persisted source: %+v", source)
	}
	version := sts.ResourceVersion
	reconcile()
	applyTestGet(t, c, sts)
	if sts.ResourceVersion != version {
		t.Fatal("API-defaulted retained template was rewritten")
	}
	template := sts.Spec.VolumeClaimTemplates[0].DeepCopy()
	claim := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "data-" + sts.Name + "-0",
		Namespace:   cr.Namespace,
		Annotations: template.Annotations}, Spec: template.Spec}
	claim.Spec.VolumeName = "pv-" + ns.Name
	if err := c.Create(t.Context(), claim); err != nil {
		t.Fatal(err)
	}
	mode := corev1.PersistentVolumeFilesystem
	pv := &corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: claim.Spec.VolumeName},
		Spec: corev1.PersistentVolumeSpec{
			Capacity:    corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("64Mi")},
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			VolumeMode:  &mode, StorageClassName: class.Name,
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
			ClaimRef:                      &corev1.ObjectReference{Namespace: claim.Namespace, Name: claim.Name, UID: claim.UID},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				HostPath: &corev1.HostPathVolumeSource{Path: "/api-test-only"},
			},
		}}
	if err := c.Create(t.Context(), pv); err != nil {
		t.Fatal(err)
	}
	pv.Status.Phase = corev1.VolumeBound
	if err := c.Status().Update(t.Context(), pv); err != nil {
		t.Fatal(err)
	}
	claim.Status.Phase = corev1.ClaimBound
	if err := c.Status().Update(t.Context(), claim); err != nil {
		t.Fatal(err)
	}
	reconcile()
	applyTestGet(t, c, claim)
	receipt, err := recordedBinding(claim)
	if err != nil || receipt == nil {
		t.Fatalf("binding was not observed: %v", err)
	}
	if receipt.PVCUID != claim.UID || receipt.PVUID != pv.UID || receipt.VolumeName != pv.Name {
		t.Fatalf("wrong binding identities: %+v", receipt)
	}
	oldClaim, oldPV, oldSTS := claim.DeepCopy(), pv.DeepCopy(), sts.UID
	groupInput := cr.DeepCopy().Spec.Workers.RoleGroups["default"]
	updateOperationAPI(t, c, cr, func(current *generatedtrino.TrinoCluster) { current.Spec.Workers.RoleGroups = nil })
	reconcile()
	sts = workloadAPI(t, c, cr, 0)
	observeZeroAPI(t, c, sts)
	for range 5 {
		reconcile()
	}
	applyTestGet(t, c, claim)
	applyTestGet(t, c, pv)
	if claim.UID != oldClaim.UID || pv.UID != oldPV.UID || !reflect.DeepEqual(claim.Spec, oldClaim.Spec) ||
		!claim.DeletionTimestamp.IsZero() || !pv.DeletionTimestamp.IsZero() {
		t.Fatal("retirement changed retained storage")
	}
	updateOperationAPI(t, c, cr, func(current *generatedtrino.TrinoCluster) {
		current.Spec.Workers.RoleGroups = map[string]generatedtrino.RoleGroupInput{"default": groupInput}
	})
	reconcile()
	replacement := workloadAPI(t, c, cr, 1)
	applyTestGet(t, c, claim)
	applyTestGet(t, c, pv)
	if replacement.UID == oldSTS || claim.UID != oldClaim.UID || pv.UID != oldPV.UID {
		t.Fatal("same-source readd failed")
	}
	if err := c.Delete(t.Context(), cr); err != nil {
		t.Fatal(err)
	}
}
