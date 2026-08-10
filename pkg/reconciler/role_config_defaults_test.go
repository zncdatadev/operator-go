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
	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/common"
	"github.com/zncdatadev/operator-go/pkg/config"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/testutil"
	"k8s.io/apimachinery/pkg/api/resource"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
)

// defaultsBuildCtx builds a context for the "worker" role with whatever config the CR states.
func defaultsBuildCtx(cfg *v1alpha1.RoleGroupConfigSpec) *reconciler.RoleGroupBuildContext {
	return &reconciler.RoleGroupBuildContext{
		ClusterName:      "test-cluster",
		ClusterNamespace: "default",
		RoleName:         "worker",
		RoleSpec:         &v1alpha1.RoleSpec{},
		RoleGroupName:    "default",
		RoleGroupSpec:    v1alpha1.RoleGroupSpec{Replicas: ptr.To(int32(3)), Config: cfg},
		MergedConfig:     &config.MergedConfig{},
		ResourceName:     reconciler.RoleGroupResourceName("test-cluster", "worker", "default"),
	}
}

func quantityPtr(s string) *resource.Quantity { q := resource.MustParse(s); return &q }

var _ = Describe("Role config defaults", func() {
	var (
		ctx    context.Context
		mockCR *testutil.MockCluster
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockCR = testutil.NewMockCluster("test-cluster", testNamespace)
	})

	newHandler := func() *reconciler.BaseRoleGroupHandler[common.ClusterInterface] {
		return reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("product:1", testScheme)
	}

	It("applies a product default when the CR states nothing", func() {
		// Three Gen 3 migrations wrote this pass by hand — "if nil, fill the product default" over
		// resources / affinity / gracefulShutdownTimeout — down to the same function names.
		handler := newHandler()
		handler.SetRoleConfigDefaults("worker", &v1alpha1.RoleGroupConfigSpec{
			GracefulShutdownTimeout: ptr.To("5m"),
		})

		resources, err := handler.BuildResources(ctx, k8sClient, mockCR, defaultsBuildCtx(nil))
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.StatefulSet.Spec.Template.Spec.TerminationGracePeriodSeconds).
			To(HaveValue(Equal(int64(300))))
	})

	It("lets the CR win over the default", func() {
		handler := newHandler()
		handler.SetRoleConfigDefaults("worker", &v1alpha1.RoleGroupConfigSpec{
			GracefulShutdownTimeout: ptr.To("5m"),
		})

		resources, err := handler.BuildResources(ctx, k8sClient, mockCR, defaultsBuildCtx(
			&v1alpha1.RoleGroupConfigSpec{GracefulShutdownTimeout: ptr.To("90s")}))
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.StatefulSet.Spec.Template.Spec.TerminationGracePeriodSeconds).
			To(HaveValue(Equal(int64(90))))
	})

	It("folds resources per LEAF, so a default survives a user who set one sibling", func() {
		// The reason this reuses MergeRoleGroupConfig rather than a struct-level nil check: a
		// user setting cpu.max would otherwise discard the product's cpu.min and memory entirely,
		// which is the same defect the role/role-group merge was fixed for.
		handler := newHandler()
		handler.SetRoleConfigDefaults("worker", &v1alpha1.RoleGroupConfigSpec{
			Resources: &v1alpha1.ResourcesSpec{
				CPU:    &v1alpha1.CPUResource{Min: quantityPtr("100m"), Max: quantityPtr("1")},
				Memory: &v1alpha1.MemoryResource{Limit: quantityPtr("1Gi")},
			},
		})

		resources, err := handler.BuildResources(ctx, k8sClient, mockCR, defaultsBuildCtx(
			&v1alpha1.RoleGroupConfigSpec{Resources: &v1alpha1.ResourcesSpec{
				CPU: &v1alpha1.CPUResource{Max: quantityPtr("4")},
			}}))
		Expect(err).NotTo(HaveOccurred())

		container := resources.StatefulSet.Spec.Template.Spec.Containers[0]
		Expect(container.Resources.Limits.Cpu().String()).To(Equal("4"), "the user's value wins")
		Expect(container.Resources.Requests.Cpu().String()).To(Equal("100m"), "the default's sibling leaf survives")
		Expect(container.Resources.Limits.Memory().String()).To(Equal("1Gi"), "so does an untouched member")
	})

	It("lets `affinity: {}` clear a default rather than inherit it", func() {
		// Affinity is the one field that does not fold per leaf — it is a scheduling POLICY, and
		// an empty one is how a single-node development group says "no anti-affinity". Folding the
		// default beneath the CR keeps that: non-nil means the user has an opinion, even an empty one.
		handler := newHandler()
		aff, err := reconciler.EncodeAffinity(reconciler.DefaultAntiAffinity(
			"test-cluster", "worker", reconciler.TopologyKeyHostname, 70))
		Expect(err).NotTo(HaveOccurred())
		handler.SetRoleConfigDefaults("worker", &v1alpha1.RoleGroupConfigSpec{Affinity: aff})

		withDefault, err := handler.BuildResources(ctx, k8sClient, mockCR, defaultsBuildCtx(nil))
		Expect(err).NotTo(HaveOccurred())
		Expect(withDefault.StatefulSet.Spec.Template.Spec.Affinity).NotTo(BeNil())
		Expect(withDefault.StatefulSet.Spec.Template.Spec.Affinity.PodAntiAffinity.
			PreferredDuringSchedulingIgnoredDuringExecution).To(HaveLen(1))

		cleared, err := handler.BuildResources(ctx, k8sClient, mockCR, defaultsBuildCtx(
			&v1alpha1.RoleGroupConfigSpec{Affinity: &k8sruntime.RawExtension{Raw: []byte(`{}`)}}))
		Expect(err).NotTo(HaveOccurred())
		// `{}` decodes to an empty corev1.Affinity, which the builder sets as-is — non-nil but
		// carrying no rule, which is what "this group has no anti-affinity" looks like in a pod spec.
		Expect(cleared.StatefulSet.Spec.Template.Spec.Affinity.PodAntiAffinity).To(BeNil(),
			"an explicitly empty affinity must not inherit the product default")
	})

	It("lets the per-call channel outrank the handler's map", func() {
		// DefaultAntiAffinity's selector names the CLUSTER, and one handler instance serves every
		// cluster the operator reconciles — so a default containing one cannot live on the handler
		// at all. This is the same hazard as Image and ContainerPorts (§4).
		handler := newHandler()
		handler.SetRoleConfigDefaults("worker", &v1alpha1.RoleGroupConfigSpec{
			GracefulShutdownTimeout: ptr.To("5m"),
		})

		buildCtx := defaultsBuildCtx(nil)
		buildCtx.ConfigDefaults = &v1alpha1.RoleGroupConfigSpec{GracefulShutdownTimeout: ptr.To("2m")}

		resources, err := handler.BuildResources(ctx, k8sClient, mockCR, buildCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.StatefulSet.Spec.Template.Spec.TerminationGracePeriodSeconds).
			To(HaveValue(Equal(int64(120))))
	})

	It("rejects a default that sets logging", func() {
		// The framework merges logging in the RECONCILER, from the CR's two levels, and has already
		// decided Vector enablement and rendered the config file before a handler default is read.
		// Honouring it here would apply to neither, so it fails instead of half-working.
		handler := newHandler()
		buildCtx := defaultsBuildCtx(nil)
		buildCtx.ConfigDefaults = &v1alpha1.RoleGroupConfigSpec{
			Logging: &v1alpha1.LoggingSpec{EnableVectorAgent: ptr.To(true)},
		}

		_, err := handler.BuildResources(ctx, k8sClient, mockCR, buildCtx)
		Expect(err).To(HaveOccurred())
		Expect(reconciler.IsValidationError(err)).To(BeTrue(), "want a *ValidationError, got %T", err)
		Expect(err.Error()).To(ContainSubstring("logging"))
		Expect(err.Error()).To(ContainSubstring("RoleGroupBuildContext.ConfigDefaults"))
	})

	It("names the handler map when THAT is where the bad default came from", func() {
		// The two defaults are set through different APIs in different files, so an error naming the
		// wrong one sends the reader to code that does not contain the problem.
		handler := newHandler()
		handler.SetRoleConfigDefaults("worker", &v1alpha1.RoleGroupConfigSpec{
			Logging: &v1alpha1.LoggingSpec{EnableVectorAgent: ptr.To(true)},
		})

		_, err := handler.BuildResources(ctx, k8sClient, mockCR, defaultsBuildCtx(nil))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(`SetRoleConfigDefaults("worker")`))
		Expect(err.Error()).NotTo(ContainSubstring("RoleGroupBuildContext.ConfigDefaults"))
	})

	It("reports a config-defaults role name the CR does not declare", func() {
		// A typo in SetRoleConfigDefaults("wroker", ...) would otherwise silently produce a role
		// group with none of the product's defaults, which is what ConfiguredRoleNames exists for.
		handler := newHandler()
		handler.SetRoleConfigDefaults("wroker", &v1alpha1.RoleGroupConfigSpec{})
		handler.SetRoleStorageMountPath("mstaer", "/data")

		Expect(handler.ConfiguredRoleNames()).To(ContainElements("wroker", "mstaer"))
	})
})

var _ = Describe("DefaultAntiAffinity", func() {
	It("selects on the framework's own role identity", func() {
		// The value of centralising this: those keys are framework-owned and stamped by
		// BaseRoleGroupHandler. An operator that re-types them as string literals — zookeeper's
		// Gen 3 branch does — gets a selector matching nothing the moment they change, and a
		// PREFERRED term that matches no pod is not an error, so the spread silently disappears.
		aff := reconciler.DefaultAntiAffinity("kafka-prod", "broker", reconciler.TopologyKeyHostname, 70)

		terms := aff.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution
		Expect(terms).To(HaveLen(1))
		Expect(terms[0].Weight).To(Equal(int32(70)))
		Expect(terms[0].PodAffinityTerm.TopologyKey).To(Equal("kubernetes.io/hostname"))
		Expect(terms[0].PodAffinityTerm.LabelSelector.MatchLabels).To(Equal(map[string]string{
			"app.kubernetes.io/instance":  "kafka-prod",
			"app.kubernetes.io/component": "broker",
		}))
		Expect(aff.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution).To(BeEmpty(),
			"required would leave replicas Pending forever on a cluster with fewer nodes than replicas")
	})

	It("round-trips through EncodeAffinity and the strict decoder", func() {
		// EncodeAffinity's output has to survive DecodeAffinity, which rejects unknown fields — so
		// this is the guard that the two stay inverses as corev1.Affinity evolves.
		aff := reconciler.DefaultAntiAffinity("kafka-prod", "broker", "topology.kubernetes.io/zone", 20)
		raw, err := reconciler.EncodeAffinity(aff)
		Expect(err).NotTo(HaveOccurred())

		decoded, err := reconciler.DecodeAffinity(raw)
		Expect(err).NotTo(HaveOccurred())
		Expect(decoded).To(Equal(aff))
	})

	It("encodes nil as nil, which the merge reads as no opinion", func() {
		raw, err := reconciler.EncodeAffinity(nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(raw).To(BeNil())
	})
})

var _ = Describe("Per-role storage mount path", func() {
	var (
		ctx    context.Context
		mockCR *testutil.MockCluster
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockCR = testutil.NewMockCluster("test-cluster", testNamespace)
	})

	storageCtx := func(roleName string) *reconciler.RoleGroupBuildContext {
		buildCtx := defaultsBuildCtx(&v1alpha1.RoleGroupConfigSpec{
			Resources: &v1alpha1.ResourcesSpec{
				Storage: &v1alpha1.StorageResource{Capacity: quantityPtr("10Gi")},
			},
		})
		buildCtx.RoleName = roleName
		buildCtx.ResourceName = reconciler.RoleGroupResourceName("test-cluster", roleName, "default")
		return buildCtx
	}

	It("gives one role a data PVC and withholds it from another", func() {
		// DolphinScheduler's case: `worker` needs the volume, `master` must not have it. With only a
		// handler-global StorageMountPath the migration had to choose "none", deleting the
		// worker-data volumeClaimTemplate Gen 2 already rendered.
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("product:1", testScheme)
		handler.SetRoleStorageMountPath("worker", "/kubedoop/data")

		worker, err := handler.BuildResources(ctx, k8sClient, mockCR, storageCtx("worker"))
		Expect(err).NotTo(HaveOccurred())
		Expect(worker.StatefulSet.Spec.VolumeClaimTemplates).To(HaveLen(1))

		master, err := handler.BuildResources(ctx, k8sClient, mockCR, storageCtx("master"))
		Expect(err).NotTo(HaveOccurred())
		Expect(master.StatefulSet.Spec.VolumeClaimTemplates).To(BeEmpty(),
			"a role with no entry and no global default gets no data PVC")
	})

	It("lets a role opt OUT of a handler-global default with an empty path", func() {
		// "" for a role is not "unset": it is the only way to say this role has no data PVC while
		// its siblings inherit the global one.
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("product:1", testScheme)
		handler.StorageMountPath = "/kubedoop/data"
		handler.SetRoleStorageMountPath("master", "")

		worker, err := handler.BuildResources(ctx, k8sClient, mockCR, storageCtx("worker"))
		Expect(err).NotTo(HaveOccurred())
		Expect(worker.StatefulSet.Spec.VolumeClaimTemplates).To(HaveLen(1), "inherits the global default")

		master, err := handler.BuildResources(ctx, k8sClient, mockCR, storageCtx("master"))
		Expect(err).NotTo(HaveOccurred())
		Expect(master.StatefulSet.Spec.VolumeClaimTemplates).To(BeEmpty())
	})

	It("mounts the per-role path on the primary container", func() {
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("product:1", testScheme)
		handler.StorageMountPath = "/kubedoop/data"
		handler.SetRoleStorageMountPath("worker", "/var/lib/worker")

		worker, err := handler.BuildResources(ctx, k8sClient, mockCR, storageCtx("worker"))
		Expect(err).NotTo(HaveOccurred())

		mounts := worker.StatefulSet.Spec.Template.Spec.Containers[0].VolumeMounts
		paths := make([]string, 0, len(mounts))
		for _, m := range mounts {
			paths = append(paths, m.MountPath)
		}
		Expect(paths).To(ContainElement("/var/lib/worker"))
		Expect(paths).NotTo(ContainElement("/kubedoop/data"))
	})

	It("keeps the handler-global path for roles with no entry", func() {
		handler := reconciler.NewBaseRoleGroupHandler[common.ClusterInterface]("product:1", testScheme)
		handler.StorageMountPath = "/kubedoop/data"

		resources, err := handler.BuildResources(ctx, k8sClient, mockCR, storageCtx("worker"))
		Expect(err).NotTo(HaveOccurred())
		Expect(resources.StatefulSet.Spec.VolumeClaimTemplates).To(HaveLen(1))
		Expect(resources.StatefulSet.Spec.Template.Spec.Containers[0].VolumeMounts).To(
			ContainElement(HaveField("MountPath", "/kubedoop/data")))
	})
})
