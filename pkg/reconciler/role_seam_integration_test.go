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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/config"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/testutil"
)

var _ = Describe("The role declaration and derivation seams", func() {
	ctx := context.Background()

	// reconcileWith builds a reconciler wired to the given provider/resolver and runs one pass.
	reconcileWith := func(name string, provider reconciler.RoleProvider[*testutil.MockCluster],
		resolver reconciler.RoleGroupResolver[*testutil.MockCluster],
	) (*record.FakeRecorder, string, error) {
		cr, resourceName := newResilienceCR(ctx, name)
		rec := record.NewFakeRecorder(100)
		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:            k8sClient,
			Scheme:            testScheme,
			ImageResolution:   reconciler.ImageResolution{Defaults: commonsv1alpha1.ImageSpec{Custom: "test-image:latest"}},
			Recorder:          rec,
			RoleGroupHandler:  reconciler.NewBaseRoleGroupHandler[*testutil.MockCluster](testScheme),
			RoleProvider:      provider,
			RoleGroupResolver: resolver,
			Prototype:         testutil.NewMockCluster("proto", testNamespace),
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = r.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: cr.Name}})
		return rec, resourceName, err
	}

	Context("a role the CR declares and the product does not support", func() {
		It("fails the pass, naming the unknown role and the known ones", func() {
			// Before, this built a role group with no ports, no container name and no Service,
			// and reported success — which is what a typo in a role name actually produced.
			provider := reconciler.RoleProviderFunc[*testutil.MockCluster](
				func(context.Context, client.Client, *testutil.MockCluster) (reconciler.RoleCatalog, error) {
					return reconciler.RoleCatalog{"brokers": {}}, nil // CR says "broker"
				})

			_, _, err := reconcileWith(uniqueCRName("unknown-role"), provider, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("broker"))
			Expect(err.Error()).To(ContainSubstring("brokers"))
		})
	})

	Context("a config default declared by the product", func() {
		It("reaches the pod, and the user's own value still wins per leaf", func() {
			provider := reconciler.RoleProviderFunc[*testutil.MockCluster](
				func(context.Context, client.Client, *testutil.MockCluster) (reconciler.RoleCatalog, error) {
					return reconciler.RoleCatalog{"broker": {
						ConfigDefaults: &commonsv1alpha1.RoleGroupConfigSpec{
							GracefulShutdownTimeout: ptr.To("5m"),
							Resources: &commonsv1alpha1.ResourcesSpec{
								CPU:    &commonsv1alpha1.CPUResource{Min: ptrQuantity("100m"), Max: ptrQuantity("200m")},
								Memory: &commonsv1alpha1.MemoryResource{Limit: ptrQuantity("512Mi")},
							},
						},
					}}, nil
				})

			_, resourceName, err := reconcileWith(uniqueCRName("cfg-default"), provider, nil)
			Expect(err).NotTo(HaveOccurred())

			sts := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Namespace: testNamespace, Name: resourceName}, sts)).To(Succeed())

			Expect(sts.Spec.Template.Spec.TerminationGracePeriodSeconds).To(
				HaveValue(Equal(int64(300))), "the product's grace default reached the pod")

			res := sts.Spec.Template.Spec.Containers[0].Resources
			Expect(res.Requests.Cpu().String()).To(Equal("100m"))
			Expect(res.Limits.Cpu().String()).To(Equal("200m"))
			Expect(res.Limits.Memory().String()).To(Equal("512Mi"))
		})
	})

	Context("a value derived from the effective config", func() {
		It("reaches the role group ConfigMap, which was impossible before", func() {
			// The effective config used to be computed inside the StatefulSet build, AFTER the
			// ConfigMap had already been written, so nothing derived from it could reach a config
			// file. One operator gave up and froze the answer into a literal.
			provider := reconciler.RoleProviderFunc[*testutil.MockCluster](
				func(context.Context, client.Client, *testutil.MockCluster) (reconciler.RoleCatalog, error) {
					return reconciler.RoleCatalog{"broker": {
						ConfigDefaults: &commonsv1alpha1.RoleGroupConfigSpec{
							Resources: &commonsv1alpha1.ResourcesSpec{
								Memory: &commonsv1alpha1.MemoryResource{Limit: ptrQuantity("2Gi")},
							},
						},
					}}, nil
				})

			resolver := reconciler.RoleGroupResolverFunc[*testutil.MockCluster](
				func(_ context.Context, _ client.Client, _ *testutil.MockCluster,
					rg *reconciler.RoleGroupBuildContext) (*reconciler.Contribution, error) {
					heap, ok := reconciler.HeapMB(rg.RoleGroupSpec.GetConfig(), 0.8)
					Expect(ok).To(BeTrue(), "the resolver sees the FOLDED config, not the raw CR")
					return &reconciler.Contribution{
						EnvVars: map[string]string{"HEAP_MB": fmt.Sprintf("%d", heap)},
					}, nil
				})

			_, resourceName, err := reconcileWith(uniqueCRName("derived"), provider, resolver)
			Expect(err).NotTo(HaveOccurred())

			sts := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Namespace: testNamespace, Name: resourceName}, sts)).To(Succeed())

			// 2Gi = 2048MiB, times 0.8 = 1638
			Expect(sts.Spec.Template.Spec.Containers[0].Env).To(ContainElement(
				corev1.EnvVar{Name: "HEAP_MB", Value: "1638"}))
		})
	})

	Context("a logging default declared by the product", func() {
		It("survives the fold instead of being rejected outright", func() {
			// This used to be a hard *ValidationError — "config defaults may not set logging" — and
			// the reason was real but was about the FRAMEWORK, not the product: logging was merged
			// on a second path from the CR's two levels only, so a default set here reached neither
			// consumer. Now it folds with everything else, on one path, so it reaches both.
			provider := reconciler.RoleProviderFunc[*testutil.MockCluster](
				func(context.Context, client.Client, *testutil.MockCluster) (reconciler.RoleCatalog, error) {
					return reconciler.RoleCatalog{"broker": {
						ConfigDefaults: &commonsv1alpha1.RoleGroupConfigSpec{
							Logging: &commonsv1alpha1.LoggingSpec{EnableVectorAgent: ptr.To(true)},
						},
					}}, nil
				})

			// Read the FOLDED config, which is what this stage sees; the reconciler assigns
			// MergedConfig.Logging from exactly this value immediately afterwards, and
			// MergedConfig itself is deliberately not populated yet because this stage contributes
			// to it.
			var seen *commonsv1alpha1.LoggingSpec
			resolver := reconciler.RoleGroupResolverFunc[*testutil.MockCluster](
				func(_ context.Context, _ client.Client, _ *testutil.MockCluster,
					rg *reconciler.RoleGroupBuildContext) (*reconciler.Contribution, error) {
					seen = rg.RoleGroupSpec.GetConfig().Logging
					return nil, nil
				})

			_, _, err := reconcileWith(uniqueCRName("logging-default"), provider, resolver)
			Expect(err).NotTo(HaveOccurred())

			Expect(seen).NotTo(BeNil(), "the product's logging default survived the fold")
			Expect(seen.EnableVectorAgent).To(HaveValue(BeTrue()))
		})
	})

	Context("a role group the product declares nothing for", func() {
		It("reconciles unchanged, so a product that never adopts the seam is unaffected", func() {
			_, resourceName, err := reconcileWith(uniqueCRName("no-provider"), nil, nil)
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Namespace: testNamespace, Name: resourceName}, &corev1.ConfigMap{})).To(Succeed())
		})
	})
})

func ptrQuantity(s string) *resource.Quantity {
	return ptr.To(resource.MustParse(s))
}

var _ = Describe("What the declaration may and may not beat", func() {
	ctx := context.Background()

	build := func(decl reconciler.RoleDeclaration, merged *config.MergedConfig) *appsv1.StatefulSet {
		handler := reconciler.NewBaseRoleGroupHandler[*testutil.MockCluster](testScheme)
		res, err := handler.BuildResources(ctx, k8sClient,
			testutil.NewMockCluster("decl-precedence", testNamespace),
			&reconciler.RoleGroupBuildContext{
				ClusterName:      "decl-precedence",
				ClusterNamespace: testNamespace,
				ClusterSpec:      &commonsv1alpha1.GenericClusterSpec{},
				RoleName:         "broker",
				RoleSpec:         &commonsv1alpha1.RoleSpec{},
				RoleGroupName:    "default",
				RoleGroupSpec:    commonsv1alpha1.RoleGroupSpec{Replicas: ptr.To(int32(1))},
				MergedConfig:     merged,
				ResourceName:     reconciler.RoleGroupResourceName("decl-precedence", "broker", "default"),
				ResolvedImage:    reconciler.ResolvedImage{Reference: "test-image:latest"},
				Declaration:      decl,
			})
		Expect(err).NotTo(HaveOccurred())
		return res.StatefulSet
	}

	It("lets a user's envOverrides beat a declared env var of the same name", func() {
		// THE guard against the defect the deleted container callback embodied. That callback ran on
		// the ASSEMBLED container, after the merged env had been written into it, so a product
		// setting env there silently deleted what the user wrote. A declaration must be beneath.
		//
		// Kubernetes resolves a duplicate env name to the LAST entry, so this asserts on ordering,
		// not on absence.
		sts := build(
			reconciler.RoleDeclaration{Env: []corev1.EnvVar{{Name: "JAVA_OPTS", Value: "from-product"}}},
			&config.MergedConfig{EnvVars: map[string]string{"JAVA_OPTS": "from-user"}},
		)

		env := sts.Spec.Template.Spec.Containers[0].Env
		var last string
		for _, e := range env {
			if e.Name == "JAVA_OPTS" {
				last = e.Value
			}
		}
		Expect(last).To(Equal("from-user"), "the user's envOverrides must win")
	})

	It("still delivers a declared env var the user did not restate", func() {
		sts := build(
			reconciler.RoleDeclaration{Env: []corev1.EnvVar{
				{Name: "POD_NAME", ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"}}},
			}},
			&config.MergedConfig{EnvVars: map[string]string{"OTHER": "x"}},
		)

		// A valueFrom is the case this channel exists for: the override map is map[string]string
		// and cannot express one at all.
		Expect(sts.Spec.Template.Spec.Containers[0].Env).To(ContainElement(
			HaveField("Name", "POD_NAME")))
	})

	It("names a declared data volume, rather than always calling it \"data\"", func() {
		// The claim template's name is immutable and preserved by the apply path, so a product that
		// named its volume something else could not have fixed it in place afterwards. The field was
		// accepted and documented for a while before anything read it.
		handler := reconciler.NewBaseRoleGroupHandler[*testutil.MockCluster](testScheme)
		res, err := handler.BuildResources(ctx, k8sClient,
			testutil.NewMockCluster("decl-volume", testNamespace),
			&reconciler.RoleGroupBuildContext{
				ClusterName:      "decl-volume",
				ClusterNamespace: testNamespace,
				ClusterSpec:      &commonsv1alpha1.GenericClusterSpec{},
				RoleName:         "broker",
				RoleSpec:         &commonsv1alpha1.RoleSpec{},
				RoleGroupName:    "default",
				RoleGroupSpec: commonsv1alpha1.RoleGroupSpec{
					Replicas: ptr.To(int32(1)),
					// Whether the role HAS a volume is declared; how big it is comes from the
					// effective config, so a user can size it.
					Config: &commonsv1alpha1.RoleGroupConfigSpec{
						Resources: &commonsv1alpha1.ResourcesSpec{
							Storage: &commonsv1alpha1.StorageResource{Capacity: ptrQuantity("5Gi")},
						},
					},
				},
				MergedConfig:  &config.MergedConfig{},
				ResourceName:  reconciler.RoleGroupResourceName("decl-volume", "broker", "default"),
				ResolvedImage: reconciler.ResolvedImage{Reference: "test-image:latest"},
				Declaration: reconciler.RoleDeclaration{DataVolume: &reconciler.DataVolume{
					Name: "journal", MountPath: "/kubedoop/journal"}},
			})
		Expect(err).NotTo(HaveOccurred())

		Expect(res.StatefulSet.Spec.VolumeClaimTemplates).To(HaveLen(1))
		Expect(res.StatefulSet.Spec.VolumeClaimTemplates[0].Name).To(Equal("journal"))
		// The mount must carry the same name, or the pod references a volume that does not exist.
		Expect(res.StatefulSet.Spec.Template.Spec.Containers[0].VolumeMounts).To(ContainElement(
			SatisfyAll(HaveField("Name", "journal"), HaveField("MountPath", "/kubedoop/journal"))))
	})

	It("applies a declared Command and Lifecycle to the primary container", func() {
		sts := build(
			reconciler.RoleDeclaration{
				Command: []string{"/bin/bash", "-c", "start.sh"},
				Lifecycle: &corev1.Lifecycle{PreStop: &corev1.LifecycleHandler{
					Sleep: &corev1.SleepAction{Seconds: 5}}},
			},
			&config.MergedConfig{},
		)

		c := sts.Spec.Template.Spec.Containers[0]
		Expect(c.Command).To(Equal([]string{"/bin/bash", "-c", "start.sh"}))
		// A sleep action had no route through the builder's three narrow lifecycle helpers.
		Expect(c.Lifecycle).NotTo(BeNil())
		Expect(c.Lifecycle.PreStop.Sleep).NotTo(BeNil())
	})
})
