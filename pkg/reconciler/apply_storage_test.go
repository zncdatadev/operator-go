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

package reconciler_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/testutil"
)

// The data PVC is the one preserved field the pod template depends on, so preserving it alone leaves
// an incoherent StatefulSet: adding storage to a live role group used to send a template mounting a
// claim that was never created (naming a volumeMounts field the user never wrote — the Update is
// rejected on Kubernetes 1.34+, and on older servers every pod is rejected instead), and removing it
// used to be ACCEPTED on every version — claim kept, mount gone, pods rolled onto the container's
// writable layer with their PVCs still bound and mounted nowhere.
var _ = Describe("StatefulSet data PVC transitions", func() {
	ctx := context.Background()

	const (
		role            = "datanode"
		roleGroup       = "default"
		mountPath       = "/kubedoop/data"
		secondMountPath = "/kubedoop/data-archive"
	)

	storage := func() *v1alpha1.RoleGroupConfigSpec {
		return &v1alpha1.RoleGroupConfigSpec{
			Resources: &v1alpha1.ResourcesSpec{
				Storage: &v1alpha1.StorageResource{Capacity: ptr.To(resource.MustParse("1Gi"))},
			},
		}
	}

	rolesWith := func(cfg *v1alpha1.RoleGroupConfigSpec) map[string]v1alpha1.RoleSpec {
		return map[string]v1alpha1.RoleSpec{
			role: {RoleGroups: map[string]v1alpha1.RoleGroupSpec{
				roleGroup: {Replicas: ptr.To(int32(1)), Config: cfg},
			}},
		}
	}

	// newCluster creates the CR and returns it with the role group's resource name.
	newCluster := func(name string, cfg *v1alpha1.RoleGroupConfigSpec) (*testutil.MockCluster, string) {
		cr := testutil.NewMockCluster(name, testNamespace).WithRoles(rolesWith(cfg))
		Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		resourceName := reconciler.RoleGroupResourceName(name, role, roleGroup)
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, cr)
			meta := metav1.ObjectMeta{Name: resourceName, Namespace: testNamespace}
			_ = k8sClient.Delete(ctx, &appsv1.StatefulSet{ObjectMeta: meta})
			_ = k8sClient.Delete(ctx, &corev1.ConfigMap{ObjectMeta: meta})
			_ = k8sClient.Delete(ctx, &corev1.Service{ObjectMeta: metav1.ObjectMeta{
				Name: resourceName + "-headless", Namespace: testNamespace}})
		})
		return cr, resourceName
	}

	newReconcilerFor := func(rec record.EventRecorder) *reconciler.GenericReconciler[*testutil.MockCluster] {
		handler := reconciler.NewBaseRoleGroupHandler[*testutil.MockCluster]("product:1", testScheme)
		handler.StorageMountPath = mountPath
		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			Recorder:         rec,
			RoleGroupHandler: handler,
			Prototype:        testutil.NewMockCluster("proto", testNamespace),
		})
		Expect(err).NotTo(HaveOccurred())
		return r
	}

	// reconcileOnce runs one pass and requires it to succeed.
	reconcileOnce := func(r *reconciler.GenericReconciler[*testutil.MockCluster], name string) {
		GinkgoHelper()
		_, err := r.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: name}})
		Expect(err).NotTo(HaveOccurred())
	}

	// setRoleGroupConfig rewrites the role group's config on the live CR.
	setRoleGroupConfig := func(name string, cfg *v1alpha1.RoleGroupConfigSpec) {
		GinkgoHelper()
		cr := &testutil.MockCluster{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: name}, cr)).To(Succeed())
		cr.Spec.Roles = rolesWith(cfg)
		Expect(k8sClient.Update(ctx, cr)).To(Succeed())
	}

	getSTS := func(name string) *appsv1.StatefulSet {
		GinkgoHelper()
		sts := &appsv1.StatefulSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: name}, sts)).To(Succeed())
		return sts
	}

	dataMountPaths := func(sts *appsv1.StatefulSet) []string {
		var paths []string
		for _, c := range sts.Spec.Template.Spec.Containers {
			for _, m := range c.VolumeMounts {
				if m.Name == "data" {
					paths = append(paths, m.MountPath)
				}
			}
		}
		return paths
	}

	mountPathsAt := func(sts *appsv1.StatefulSet, path string) []corev1.VolumeMount {
		var found []corev1.VolumeMount
		for _, c := range sts.Spec.Template.Spec.Containers {
			for _, m := range c.VolumeMounts {
				if m.MountPath == path {
					found = append(found, m)
				}
			}
		}
		return found
	}

	dataMount := func(sts *appsv1.StatefulSet) *corev1.VolumeMount {
		for _, c := range sts.Spec.Template.Spec.Containers {
			for i, m := range c.VolumeMounts {
				if m.Name == "data" {
					return &c.VolumeMounts[i]
				}
			}
		}
		return nil
	}

	It("keeps converging when storage is added to a live role group, and says what it dropped", func() {
		name := uniqueCRName("storage-add")
		_, resourceName := newCluster(name, nil)

		rec := record.NewFakeRecorder(100)
		r := newReconcilerFor(rec)
		reconcileOnce(r, name)

		before := getSTS(resourceName)
		Expect(before.Spec.VolumeClaimTemplates).To(BeEmpty(), "the fixture must start without storage")
		Expect(dataMount(before)).To(BeNil())
		drainRecorder(rec)

		// The user adds storage to a role group that already exists. volumeClaimTemplates is
		// immutable, so the framework cannot create the claim.
		setRoleGroupConfig(name, storage())
		reconcileOnce(r, name)

		after := getSTS(resourceName)
		// Without the mount reconciliation the template mounts a claim that does not exist:
		// `volumeMounts[0].name: Not found: "data"`, over a field the user never wrote. Kubernetes
		// 1.34+ rejects the Update itself (permanently Degraded); older servers accept it and reject
		// every pod instead, so the workload silently never progresses.
		Expect(after.Spec.VolumeClaimTemplates).To(BeEmpty())
		Expect(dataMount(after)).To(BeNil(),
			"a mount for a claim the framework declined to create can never run a pod")

		Expect(drainRecorder(rec)).To(ContainElement(SatisfyAll(
			ContainSubstring("Warning"),
			ContainSubstring("ImmutableFieldIgnored"),
			ContainSubstring("spec.volumeClaimTemplates"),
		)), "converging quietly on the user's storage request being dropped is the original defect")
	})

	It("keeps the preserved claim mounted when storage is removed", func() {
		name := uniqueCRName("storage-remove")
		_, resourceName := newCluster(name, storage())

		rec := record.NewFakeRecorder(100)
		r := newReconcilerFor(rec)
		reconcileOnce(r, name)

		before := getSTS(resourceName)
		Expect(before.Spec.VolumeClaimTemplates).To(HaveLen(1), "the fixture must start with storage")
		Expect(dataMount(before)).NotTo(BeNil())
		Expect(dataMount(before).MountPath).To(Equal(mountPath))
		drainRecorder(rec)

		// The user removes the storage block. The claim template is immutable, so it stays — and
		// the pods must stay attached to it.
		setRoleGroupConfig(name, &v1alpha1.RoleGroupConfigSpec{})
		reconcileOnce(r, name)

		after := getSTS(resourceName)
		Expect(after.Spec.VolumeClaimTemplates).To(HaveLen(1), "a claim template cannot be removed in place")
		mount := dataMount(after)
		// This is the silent half: the Update is ACCEPTED either way, so nothing fails and nothing
		// alerts. Dropping the mount rolls the pods onto the container's writable layer while the
		// bound PVCs sit there mounted nowhere — a data path that disappears with every signal green.
		Expect(mount).NotTo(BeNil(), "the preserved claim must stay mounted")
		Expect(mount.MountPath).To(Equal(mountPath), "restored from the live template, not invented")

		Expect(drainRecorder(rec)).To(ContainElement(SatisfyAll(
			ContainSubstring("Warning"),
			ContainSubstring("ImmutableFieldIgnored"),
			ContainSubstring("spec.volumeClaimTemplates"),
		)))
	})

	It("restores every mount of a preserved claim, not just the first", func() {
		name := uniqueCRName("storage-multimount")
		_, resourceName := newCluster(name, storage())

		base := reconciler.NewBaseRoleGroupHandler[*testutil.MockCluster]("product:1", testScheme)
		base.StorageMountPath = mountPath
		handler := &secondDataMountHandler{BaseRoleGroupHandler: base}

		rec := record.NewFakeRecorder(100)
		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			Recorder:         rec,
			RoleGroupHandler: handler,
			Prototype:        testutil.NewMockCluster("proto", testNamespace),
		})
		Expect(err).NotTo(HaveOccurred())
		reconcileOnce(r, name)

		// Kubernetes allows one volume to be mounted several times at different paths, and products
		// do it (a data dir and a subPath'd sub-tree of the same claim).
		Expect(dataMountPaths(getSTS(resourceName))).To(ConsistOf(mountPath, secondMountPath))

		setRoleGroupConfig(name, &v1alpha1.RoleGroupConfigSpec{})
		reconcileOnce(r, name)

		// Keying the restore on the volume NAME stops after the first append and silently drops the
		// rest — the second path is as bound to the preserved claim as the first.
		Expect(dataMountPaths(getSTS(resourceName))).To(ConsistOf(mountPath, secondMountPath))
	})

	It("does not restore onto a path the desired template already uses", func() {
		name := uniqueCRName("storage-path-taken")
		_, resourceName := newCluster(name, storage())

		base := reconciler.NewBaseRoleGroupHandler[*testutil.MockCluster]("product:1", testScheme)
		base.StorageMountPath = mountPath
		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			Recorder:         record.NewFakeRecorder(100),
			RoleGroupHandler: &pathSquatterHandler{BaseRoleGroupHandler: base},
			Prototype:        testutil.NewMockCluster("proto", testNamespace),
		})
		Expect(err).NotTo(HaveOccurred())
		reconcileOnce(r, name)
		Expect(dataMountPaths(getSTS(resourceName))).To(ConsistOf(mountPath))

		// Storage goes away, and this product puts an emptyDir at the path the claim used to hold.
		// Restoring the claim's mount there anyway would put two mounts on one path — a pod spec the
		// API server rejects, which would wedge the role group in the name of rescuing it.
		setRoleGroupConfig(name, &v1alpha1.RoleGroupConfigSpec{})
		reconcileOnce(r, name)

		sts := getSTS(resourceName)
		Expect(mountPathsAt(sts, mountPath)).To(HaveLen(1), "one path, one mount")
		Expect(dataMountPaths(sts)).To(BeEmpty(), "the desired template's opinion about a path wins")
	})

	It("still reports a dropped immutable field when the write itself fails", func() {
		name := uniqueCRName("storage-event-order")
		_, resourceName := newCluster(name, nil)

		base := reconciler.NewBaseRoleGroupHandler[*testutil.MockCluster]("product:1", testScheme)
		base.StorageMountPath = mountPath
		handler := &rejectedUpdateHandler{BaseRoleGroupHandler: base}

		rec := record.NewFakeRecorder(100)
		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			Recorder:         rec,
			RoleGroupHandler: handler,
			Prototype:        testutil.NewMockCluster("proto", testNamespace),
		})
		Expect(err).NotTo(HaveOccurred())

		reconcileOnce(r, name)
		Expect(getSTS(resourceName).Spec.ServiceName).NotTo(Equal("some-other-service"))
		drainRecorder(rec)

		// From here the handler both (a) changes a preserved field and (b) builds a template the
		// API server rejects. The event explaining (a) is the one the user needs precisely when (b)
		// happens, and emitting it after the error return is what used to suppress it.
		handler.broken = true
		_, err = r.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: name}})
		Expect(err).To(HaveOccurred(), "the API server must reject this write, or the spec proves nothing")

		Expect(drainRecorder(rec)).To(ContainElement(SatisfyAll(
			ContainSubstring("ImmutableFieldIgnored"),
			ContainSubstring("spec.serviceName"),
		)))
	})
})

// secondDataMountHandler mounts the data claim a second time, which Kubernetes allows and products
// do. It only adds the mount while the claim template exists, so a role group that loses its storage
// block produces a desired template with no data mount at all — exactly the state the restore has to
// repair, for BOTH paths.
type secondDataMountHandler struct {
	*reconciler.BaseRoleGroupHandler[*testutil.MockCluster]
}

func (h *secondDataMountHandler) BuildResources(
	ctx context.Context,
	c client.Client,
	cr *testutil.MockCluster,
	buildCtx *reconciler.RoleGroupBuildContext,
) (*reconciler.RoleGroupResources, error) {
	res, err := h.BaseRoleGroupHandler.BuildResources(ctx, c, cr, buildCtx)
	if err != nil || res.StatefulSet == nil || len(res.StatefulSet.Spec.VolumeClaimTemplates) == 0 {
		return res, err
	}
	containers := res.StatefulSet.Spec.Template.Spec.Containers
	containers[0].VolumeMounts = append(containers[0].VolumeMounts,
		corev1.VolumeMount{Name: "data", MountPath: "/kubedoop/data-archive", SubPath: "archive"})
	return res, nil
}

// pathSquatterHandler puts an emptyDir at the data mount path once the storage block is gone — a
// product moving that directory back into the container filesystem. Two mounts at one path is a pod
// spec the API server rejects, so the restore has to yield to it.
type pathSquatterHandler struct {
	*reconciler.BaseRoleGroupHandler[*testutil.MockCluster]
}

func (h *pathSquatterHandler) BuildResources(
	ctx context.Context,
	c client.Client,
	cr *testutil.MockCluster,
	buildCtx *reconciler.RoleGroupBuildContext,
) (*reconciler.RoleGroupResources, error) {
	res, err := h.BaseRoleGroupHandler.BuildResources(ctx, c, cr, buildCtx)
	if err != nil || res.StatefulSet == nil || len(res.StatefulSet.Spec.VolumeClaimTemplates) > 0 {
		return res, err
	}
	spec := &res.StatefulSet.Spec.Template.Spec
	spec.Volumes = append(spec.Volumes, corev1.Volume{
		Name:         "scratch",
		VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
	})
	spec.Containers[0].VolumeMounts = append(spec.Containers[0].VolumeMounts,
		corev1.VolumeMount{Name: "scratch", MountPath: "/kubedoop/data"})
	return res, nil
}

// rejectedUpdateHandler builds a role group whose StatefulSet Update the API server refuses, while
// also asking to change a preserved immutable field.
type rejectedUpdateHandler struct {
	*reconciler.BaseRoleGroupHandler[*testutil.MockCluster]
	broken bool
}

func (h *rejectedUpdateHandler) BuildResources(
	ctx context.Context,
	c client.Client,
	cr *testutil.MockCluster,
	buildCtx *reconciler.RoleGroupBuildContext,
) (*reconciler.RoleGroupResources, error) {
	res, err := h.BaseRoleGroupHandler.BuildResources(ctx, c, cr, buildCtx)
	if err != nil || !h.broken {
		return res, err
	}
	// A preserved field the framework will refuse to change...
	res.StatefulSet.Spec.ServiceName = "some-other-service"
	// ...on an object the API server will refuse to store. A negative replica count is rejected by
	// plain field validation on every Kubernetes version this SDK is tested against; a pod template
	// mounting a volume that does not exist is NOT — servers before 1.34 accept that on update, so a
	// spec built on it passes for the wrong reason on half the matrix.
	res.StatefulSet.Spec.Replicas = ptr.To(int32(-1))
	return res, nil
}
