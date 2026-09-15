package controller

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/zncdatadev/operator-go/internal/framework/pipeline"
	"github.com/zncdatadev/operator-go/pkg/framework"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	generatedtrino "github.com/zncdatadev/operator-go/internal/framework/pipeline/testinput"
)

type storageFixture struct {
	cr    *generatedtrino.TrinoCluster
	group groupSlot
	data  *pipeline.RetainedDataSlot
	sts   *appsv1.StatefulSet
	claim *corev1.PersistentVolumeClaim
	pv    *corev1.PersistentVolume
	class *storagev1.StorageClass
}

func storageJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func retainedFixture(t *testing.T) storageFixture {
	t.Helper()
	f := storageFixture{cr: controllerInput(), group: groupSlot{Role: "workers", Group: "default", Slot: slotStatefulset},
		data: &pipeline.RetainedDataSlot{Name: "data",
			RetainedData: pipeline.RetainedData{StorageClassName: "retain", Capacity: resource.MustParse("64Mi")}}}
	f.sts = retirementObjects(t, f.cr, "default", 1)[0].(*appsv1.StatefulSet)
	f.sts.Spec.PersistentVolumeClaimRetentionPolicy = &appsv1.StatefulSetPersistentVolumeClaimRetentionPolicy{
		WhenScaled:  appsv1.RetainPersistentVolumeClaimRetentionPolicyType,
		WhenDeleted: appsv1.RetainPersistentVolumeClaimRetentionPolicyType}
	f.sts.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{
		{ObjectMeta: metav1.ObjectMeta{Name: f.data.Name}, Spec: retainedClaimSpec(f.data)}}
	var err error
	f.sts, err = stampRetainedSource(f.sts, f.cr, f.group, f.data)
	if err != nil {
		t.Fatal(err)
	}
	f.claim = &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{
		Name: "data-" + f.sts.Name + "-0", Namespace: f.cr.Namespace, UID: "claim-uid",
		Annotations: map[string]string{retainedSourceAnnotation: storageJSON(t, sourceFor(f.cr, f.group, f.data))}},
		Spec: retainedClaimSpec(f.data)}
	f.claim.Spec.VolumeName = "data-pv"
	f.claim.Status.Phase = corev1.ClaimBound
	mode := corev1.PersistentVolumeFilesystem
	f.pv = &corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "data-pv", UID: "pv-uid"},
		Spec: corev1.PersistentVolumeSpec{Capacity: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("64Mi")},
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			VolumeMode:  &mode, StorageClassName: "retain",
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
			ClaimRef: &corev1.ObjectReference{
				Namespace: f.claim.Namespace, Name: f.claim.Name, UID: f.claim.UID}},
		Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeBound}}
	retain, wait := corev1.PersistentVolumeReclaimRetain, storagev1.VolumeBindingWaitForFirstConsumer
	f.class = &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "retain"},
		ReclaimPolicy: &retain, VolumeBindingMode: &wait}
	return f
}

func (f storageFixture) receipt(t *testing.T) {
	f.claim.Annotations[retainedBindingAnnotation] = storageJSON(t, retainedBinding{
		Version: 1, PVCUID: f.claim.UID, PVUID: f.pv.UID, VolumeName: f.pv.Name})
}

func (f storageFixture) desired() *appsv1.StatefulSet {
	sts := f.sts.DeepCopy()
	sts.Annotations, sts.OwnerReferences = nil, nil
	sts.Spec.VolumeClaimTemplates[0].Annotations = nil
	return sts
}

func TestRetainedFirstConsumerAndHistoricalNeverBoundDoNotBlock(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "first-creation", true: "never-bound-high-ordinal"}[existing], func(t *testing.T) {
			f := retainedFixture(t)
			objects := []client.Object{f.cr, f.class}
			if existing {
				f.claim.Name = "data-" + f.sts.Name + "-17"
				f.claim.Spec.VolumeName, f.claim.Status.Phase = "", corev1.ClaimPending
				objects = append(objects, f.claim)
			}
			c := fake.NewClientBuilder().WithScheme(controllerScheme(t)).WithObjects(objects...).Build()
			err := checkRetainedStorage(t.Context(), c, f.cr, f.group, f.desired(), f.data, nil)
			if !errors.Is(err, errStorageUnbound) {
				t.Fatalf("missing binding must only request observation: %v", err)
			}
			changed, err := applyObject(t.Context(), c, f.cr, f.desired(), c.Scheme(), &f.group, f.data)
			if err != nil || !changed {
				t.Fatalf("WFFC was prevented from creating its consumer: %v %v", changed, err)
			}
			live := &appsv1.StatefulSet{}
			applyTestGet(t, c, liveWithKey(live, f.sts))
			if live.Spec.VolumeClaimTemplates[0].Annotations[retainedSourceAnnotation] == "" {
				t.Fatal("missing stamped source")
			}
			// API-assigned identity is supplied because fake Create does not allocate it.
			live.UID = "created-sts"
			if err := c.Update(t.Context(), live); err != nil {
				t.Fatal(err)
			}
			if err := checkRetiringStorage(t.Context(), c, f.cr, f.group, live); err != nil {
				t.Fatalf("unbound claim prevented safe retain: %v", err)
			}
		})
	}
}

func liveWithKey(sts, source *appsv1.StatefulSet) *appsv1.StatefulSet {
	sts.Name, sts.Namespace = source.Name, source.Namespace
	return sts
}

func TestRetainedBindingReceiptAndSameSourceReaddition(t *testing.T) {
	f := retainedFixture(t)
	c := fake.NewClientBuilder().WithScheme(controllerScheme(t)).WithObjects(f.cr, f.class, f.sts, f.claim, f.pv).Build()
	if err := checkRetainedStorage(t.Context(), c, f.cr, f.group, f.desired(), f.data, f.sts); err != nil {
		t.Fatal(err)
	}
	observed := &corev1.PersistentVolumeClaim{}
	observed.Name, observed.Namespace = f.claim.Name, f.claim.Namespace
	applyTestGet(t, c, observed)
	var binding retainedBinding
	if err := strictReceipt(observed.Annotations[retainedBindingAnnotation], &binding); err != nil {
		t.Fatal(err)
	}
	if binding.PVCUID != f.claim.UID || binding.PVUID != f.pv.UID || binding.VolumeName != f.pv.Name ||
		len(observed.OwnerReferences) != 0 {
		t.Fatalf("incorrect binding receipt: %+v", binding)
	}
	version := observed.ResourceVersion
	if err := checkRetainedStorage(t.Context(), c, f.cr, f.group, f.desired(), f.data, f.sts); err != nil {
		t.Fatal(err)
	}
	applyTestGet(t, c, observed)
	if observed.ResourceVersion != version {
		t.Fatal("unchanged receipt was rewritten")
	}
	if err := c.Delete(t.Context(), f.sts); err != nil {
		t.Fatal(err)
	}
	changed, err := applyObject(t.Context(), c, f.cr, f.desired(), c.Scheme(), &f.group, f.data)
	if err != nil || !changed {
		t.Fatalf("retained readdition: %v %v", changed, err)
	}
}

func TestRetainedRecoveryRejectsConflictingSurvivingClaims(t *testing.T) {
	cases := map[string]func(*storageFixture){
		"new-cr-uid":           func(f *storageFixture) { f.cr.UID = "new-cr" },
		"missing-provenance":   func(f *storageFixture) { delete(f.claim.Annotations, retainedSourceAnnotation) },
		"missing-receipt":      func(f *storageFixture) { delete(f.claim.Annotations, retainedBindingAnnotation) },
		"new-pvc-uid":          func(f *storageFixture) { f.claim.UID = "replacement-pvc" },
		"new-pv-uid":           func(f *storageFixture) { f.pv.UID = "replacement-pv" },
		"foreign-binding-uid":  func(f *storageFixture) { f.pv.Spec.ClaimRef.UID = "other" },
		"foreign-binding-name": func(f *storageFixture) { f.pv.Spec.ClaimRef.Name = "other" },
		"lost-binding":         func(f *storageFixture) { f.pv.Spec.ClaimRef = nil },
		"lost-claim":           func(f *storageFixture) { f.claim.Status.Phase = corev1.ClaimLost },
		"owner-reference-noncontroller": func(f *storageFixture) {
			f.claim.OwnerReferences = []metav1.OwnerReference{{APIVersion: "v1", Kind: "Pod", Name: "old", UID: "old"}}
		},
		"pv-delete-policy": func(f *storageFixture) {
			f.pv.Spec.PersistentVolumeReclaimPolicy = corev1.PersistentVolumeReclaimDelete
		},
		"sc-delete-policy": func(f *storageFixture) {
			policy := corev1.PersistentVolumeReclaimDelete
			f.class.ReclaimPolicy = &policy
		},
		"capacity-changed": func(f *storageFixture) {
			f.data.Capacity = resource.MustParse("128Mi")
			f.sts.Spec.VolumeClaimTemplates[0].Spec = retainedClaimSpec(f.data)
		},
		"slot-renamed-after-retirement": func(f *storageFixture) {
			f.data.Name = "new-data"
			f.sts.Spec.VolumeClaimTemplates[0].Name = "new-data"
		},
		"historical-high-ordinal": func(f *storageFixture) {
			f.claim.Name = "data-" + f.sts.Name + "-37"
			f.pv.Spec.ClaimRef.Name = f.claim.Name
			f.pv.Spec.PersistentVolumeReclaimPolicy = corev1.PersistentVolumeReclaimDelete
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := retainedFixture(t)
			f.receipt(t)
			mutate(&f)
			writes := 0
			c := fake.NewClientBuilder().WithScheme(controllerScheme(t)).WithObjects(f.cr, f.class, f.claim, f.pv).
				WithInterceptorFuncs(interceptor.Funcs{Create: func(
					context.Context, client.WithWatch, client.Object, ...client.CreateOption,
				) error {
					writes++
					return nil
				}}).Build()
			changed, err := applyObject(t.Context(), c, f.cr, f.desired(), c.Scheme(), &f.group, f.data)
			if err == nil || errors.Is(err, errStorageUnbound) || changed || writes != 0 {
				t.Fatalf("unsafe recovery wrote objects: changed=%v writes=%d err=%v", changed, writes, err)
			}
		})
	}
}

func TestRetainedConsumerAndBindingTransitions(t *testing.T) {
	for _, scenario := range []string{"own", "foreign", "old-terminating",
		"claim-deleting", "binding-incomplete", "missing-recorded-pv"} {
		t.Run(scenario, func(t *testing.T) {
			f := retainedFixture(t)
			objects := []client.Object{f.cr, f.class, f.sts, f.claim, f.pv}
			var want error
			conflict := false
			switch scenario {
			case "own", "foreign", "old-terminating":
				controller := true
				pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
					Name: f.sts.Name + "-0", Namespace: f.cr.Namespace,
					OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "StatefulSet",
						Name: f.sts.Name, UID: f.sts.UID, Controller: &controller}},
				}, Spec: corev1.PodSpec{Volumes: []corev1.Volume{{Name: "data",
					VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: f.claim.Name,
					}},
				}}}}

				if scenario != "own" {
					pod.OwnerReferences[0].UID = "old-sts"
					conflict = true
				}
				if scenario == "old-terminating" {
					now := metav1.Now()
					pod.DeletionTimestamp = &now
					pod.Finalizers = []string{"test/finalizer"}
					want = errStoragePending
					conflict = false
				}
				objects = append(objects, pod)
			case "claim-deleting":
				now := metav1.Now()
				f.claim.DeletionTimestamp = &now
				f.claim.Finalizers = []string{"kubernetes.io/pvc-protection"}
				want = errStoragePending
			case "binding-incomplete":
				f.pv.Spec.ClaimRef.UID = ""
				want = errStoragePending
			case "missing-recorded-pv":
				f.receipt(t)
				objects = objects[:4]
				conflict = true
			}
			c := fake.NewClientBuilder().WithScheme(controllerScheme(t)).WithObjects(objects...).Build()
			err := checkRetainedStorage(t.Context(), c, f.cr, f.group, f.desired(), f.data, f.sts)
			if want != nil {
				if !errors.Is(err, want) {
					t.Fatalf("want pending: %v", err)
				}
			} else if conflict {
				if err == nil || errors.Is(err, errStoragePending) {
					t.Fatalf("want conflict: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRetainedReceiptRetryRechecksClaimIdentity(t *testing.T) {
	f := retainedFixture(t)
	calls := 0
	c := fake.NewClientBuilder().WithScheme(controllerScheme(t)).WithObjects(f.cr, f.class, f.sts, f.claim, f.pv).
		WithInterceptorFuncs(interceptor.Funcs{Update: func(ctx context.Context, c client.WithWatch,
			obj client.Object, opts ...client.UpdateOption,
		) error {
			if claim, ok := obj.(*corev1.PersistentVolumeClaim); ok {
				calls++
				current := claim.DeepCopy()
				if err := c.Get(ctx, client.ObjectKeyFromObject(claim), current); err != nil {
					return err
				}
				current.UID = "replacement"
				if err := c.Update(ctx, current); err != nil {
					return err
				}
				return apierrors.NewConflict(schema.GroupResource{Resource: "persistentvolumeclaims"},
					claim.Name, errors.New("race"))
			}
			return c.Update(ctx, obj, opts...)
		}}).Build()
	err := checkRetainedStorage(t.Context(), c, f.cr, f.group, f.desired(), f.data, f.sts)
	if err == nil || !strings.Contains(err.Error(), "identity changed") || calls != 1 {
		t.Fatalf("receipt retry failed to recheck identity: %v calls=%d", err, calls)
	}
}

func TestRetainedRetirementBlocksPolicyBeforeScaleAndRetainsClaims(t *testing.T) {
	for _, unsafe := range []bool{false, true} {
		t.Run(map[bool]string{false: "retain", true: "delete-policy"}[unsafe], func(t *testing.T) {
			f := retainedFixture(t)
			f.receipt(t)
			if unsafe {
				f.sts.Spec.PersistentVolumeClaimRetentionPolicy.WhenScaled = appsv1.DeletePersistentVolumeClaimRetentionPolicyType
			}
			c := fake.NewClientBuilder().WithScheme(controllerScheme(t)).
				WithObjects(f.cr, f.class, f.sts, f.claim, f.pv).WithStatusSubresource(&appsv1.StatefulSet{}).Build()
			r := newTestReconciler(c, c.Scheme(), testFacts{})
			_, err := r.retireGroup(t.Context(), f.cr, f.group)
			live := liveWithKey(&appsv1.StatefulSet{}, f.sts)
			applyTestGet(t, c, live)
			if unsafe {
				if err == nil || *live.Spec.Replicas != 1 {
					t.Fatalf("delete policy was changed/scaled: %v", err)
				}
				return
			}
			if err != nil || *live.Spec.Replicas != 0 {
				t.Fatalf("retain scale failed: %v", err)
			}
			live.Status = appsv1.StatefulSetStatus{ObservedGeneration: live.Generation}
			if err := c.Status().Update(t.Context(), live); err != nil {
				t.Fatal(err)
			}
			if phase, err := r.retireGroup(t.Context(), f.cr, f.group); err != nil || phase != "deleting statefulset" {
				t.Fatalf("retirement: %s %v", phase, err)
			}
			claim := f.claim.DeepCopy()
			applyTestGet(t, c, claim)
			if claim.UID != f.claim.UID || len(claim.OwnerReferences) != 0 {
				t.Fatal("retirement changed retained claim identity/ownership")
			}
		})
	}
}

func TestRetainedRawPVCDefaultDenialAndReservedVCTReceipts(t *testing.T) {
	f := retainedFixture(t)
	for _, scenario := range []string{"vct", "pod-pvc", "ephemeral", "source-spoof", "binding-spoof"} {
		t.Run(scenario, func(t *testing.T) {
			desired := f.desired()
			switch scenario {
			case "pod-pvc", "ephemeral":
				desired.Spec.VolumeClaimTemplates = nil
				desired.Spec.PersistentVolumeClaimRetentionPolicy = nil
				v := corev1.Volume{Name: "data"}
				if scenario == "pod-pvc" {
					v.PersistentVolumeClaim = &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "external"}
				} else {
					v.Ephemeral = &corev1.EphemeralVolumeSource{}
				}
				desired.Spec.Template.Spec.Volumes = []corev1.Volume{v}
			case "source-spoof":
				desired.Spec.VolumeClaimTemplates[0].Annotations = map[string]string{retainedSourceAnnotation: "user"}
			case "binding-spoof":
				desired.Spec.VolumeClaimTemplates[0].Annotations = map[string]string{retainedBindingAnnotation: "user"}
			}
			c := fake.NewClientBuilder().WithScheme(controllerScheme(t)).Build()
			if changed, err := ApplyObject(t.Context(), c, f.cr, desired, c.Scheme()); err == nil || changed {
				t.Fatalf("undeclared PVC accepted: %v %v", changed, err)
			}
		})
	}
}

func TestRetainedPreflightBlocksAllGroupWrites(t *testing.T) {
	f := retainedFixture(t)
	f.receipt(t)
	delete(f.claim.Annotations, retainedSourceAnnotation)
	writes := 0
	c := fake.NewClientBuilder().WithScheme(controllerScheme(t)).WithObjects(f.cr, f.class, f.claim, f.pv).
		WithInterceptorFuncs(interceptor.Funcs{Create: func(
			context.Context, client.WithWatch, client.Object, ...client.CreateOption,
		) error {
			writes++
			return nil
		}}).Build()
	r := newTestReconciler(c, c.Scheme(), testFacts{})
	plan := pipeline.ResourcePlan[testConfig, testClusterConfig, testFacts]{
		ClusterOutput: framework.ClusterOutput{State: framework.ClusterOutputReady},
		Groups: []pipeline.BuiltGroup[testConfig, testClusterConfig, testFacts]{
			{Outcome: framework.GroupOutcome{Group: framework.GroupIdentity{Role: "workers", Name: "default", Replicas: 1}},
				Resources: &pipeline.GroupResources{StatefulSet: *f.desired(), RetainedData: f.data}},
		},
	}
	status := framework.ReconcileStatus{}
	pending, failures := r.applyPlan(t.Context(), f.cr, plan, &status)
	if pending || len(failures) == 0 || writes != 0 || status.Groups[0].Applied {
		t.Fatalf("unsafe group reached its first write: pending=%v writes=%d errors=%v", pending, writes, failures)
	}
}

func TestRetainedUnboundPlanAppliesAndPolls(t *testing.T) {
	f := retainedFixture(t)
	c := fake.NewClientBuilder().WithScheme(controllerScheme(t)).WithObjects(f.cr, f.class).Build()
	objects := retirementObjects(t, f.cr, "default", 1)
	for _, object := range objects {
		object.SetAnnotations(nil)
		object.SetOwnerReferences(nil)
	}
	resources := &pipeline.GroupResources{StatefulSet: *f.desired(), RetainedData: f.data,
		ConfigMap: *objects[3].(*corev1.ConfigMap), Service: *objects[1].(*corev1.Service),
		HeadlessService: *objects[2].(*corev1.Service)}
	plan := pipeline.ResourcePlan[testConfig, testClusterConfig, testFacts]{
		ClusterOutput: framework.ClusterOutput{State: framework.ClusterOutputReady},
		Groups: []pipeline.BuiltGroup[testConfig, testClusterConfig, testFacts]{
			{Outcome: framework.GroupOutcome{Group: framework.GroupIdentity{Role: "workers", Name: "default", Replicas: 1}},
				Resources: resources},
		},
	}
	r := newTestReconciler(c, c.Scheme(), testFacts{})
	status := framework.ReconcileStatus{}
	pending, failures := r.applyPlan(t.Context(), f.cr, plan, &status)
	if !pending || len(failures) != 0 || !status.Groups[0].Applied {
		t.Fatalf("first consumer must be applied and promptly observed: %v %v %+v", pending, failures, status.Groups)
	}
}

func TestRetainedReceiptRetryRechecksGroupReceipt(t *testing.T) {
	f := retainedFixture(t)
	calls := 0
	c := fake.NewClientBuilder().WithScheme(controllerScheme(t)).WithObjects(f.cr, f.class, f.sts, f.claim, f.pv).
		WithInterceptorFuncs(interceptor.Funcs{Update: func(ctx context.Context, c client.WithWatch,
			obj client.Object, opts ...client.UpdateOption,
		) error {
			if _, ok := obj.(*corev1.PersistentVolumeClaim); ok {
				calls++
				sts := liveWithKey(&appsv1.StatefulSet{}, f.sts)
				if err := c.Get(ctx, client.ObjectKeyFromObject(sts), sts); err != nil {
					return err
				}
				delete(sts.Annotations, GroupSlotAnnotation)
				if err := c.Update(ctx, sts); err != nil {
					return err
				}
				return apierrors.NewConflict(schema.GroupResource{Resource: "persistentvolumeclaims"},
					obj.GetName(), errors.New("race"))
			}
			return c.Update(ctx, obj, opts...)
		}}).Build()
	err := checkRetainedStorage(t.Context(), c, f.cr, f.group, f.desired(), f.data, f.sts)
	if err == nil || !strings.Contains(err.Error(), "group slot") || calls != 1 {
		t.Fatalf("receipt retry accepted changed group provenance: %v calls=%d", err, calls)
	}
	claim := f.claim.DeepCopy()
	applyTestGet(t, c, claim)
	if _, written := claim.Annotations[retainedBindingAnnotation]; written {
		t.Fatal("unsafe receipt persisted")
	}
}
