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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"

	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
)

// productConfig is the shape a product's own `config` block takes: the commons block embedded
// inline — the framework's extension point — plus the fields specific to this product. Modelled on
// zookeeper's real ConfigSpec, with its four fields made pointers so "unset" is representable.
type productConfig struct {
	*commonsv1alpha1.RoleGroupConfigSpec `json:",inline"`

	MyidOffset *int32  `json:"myidOffset,omitempty"`
	TickTime   *int32  `json:"tickTime,omitempty"`
	Listener   *string `json:"listenerClass,omitempty"`
}

// commonsOnlyConfig is the shape a product with no fields of its own uses.
type commonsOnlyConfig struct {
	*commonsv1alpha1.RoleGroupConfigSpec `json:",inline"`
}

// bareScalarConfig cannot express "unset" for its own field.
type bareScalarConfig struct {
	*commonsv1alpha1.RoleGroupConfigSpec `json:",inline"`
	TickTime                             int32 `json:"tickTime,omitempty"`
}

// compositeConfig has a product struct a wholesale replacement would strip siblings from.
type nestedBlock struct{ A, B *string }
type compositeConfig struct {
	*commonsv1alpha1.RoleGroupConfigSpec `json:",inline"`
	Nested                               *nestedBlock `json:"nested,omitempty"`
}

// taggedCompositeConfig opts that composite into wholesale replacement explicitly.
type taggedCompositeConfig struct {
	*commonsv1alpha1.RoleGroupConfigSpec `json:",inline"`
	Nested                               *nestedBlock `json:"nested,omitempty" kubedoop:"atomic"`
}

func foldQ(s string) *resource.Quantity { return ptr.To(resource.MustParse(s)) }

func affinityConfig(members *corev1.Affinity) *commonsv1alpha1.RoleGroupConfigSpec {
	raw, err := reconciler.EncodeAffinity(members)
	Expect(err).NotTo(HaveOccurred())
	return &commonsv1alpha1.RoleGroupConfigSpec{Affinity: raw}
}

var _ = Describe("FoldCommonConfig (the framework's half)", func() {
	It("folds resources per LEAF, so overriding cpu.max keeps the product's cpu.min", func() {
		out, err := reconciler.FoldCommonConfig(
			&commonsv1alpha1.RoleGroupConfigSpec{Resources: &commonsv1alpha1.ResourcesSpec{
				CPU:    &commonsv1alpha1.CPUResource{Min: foldQ("100m"), Max: foldQ("200m")},
				Memory: &commonsv1alpha1.MemoryResource{Limit: foldQ("512Mi")},
			}},
			&commonsv1alpha1.RoleGroupConfigSpec{Resources: &commonsv1alpha1.ResourcesSpec{
				CPU: &commonsv1alpha1.CPUResource{Max: foldQ("4")},
			}},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.Resources.CPU.Max.String()).To(Equal("4"), "user wins")
		Expect(out.Resources.CPU.Min.String()).To(Equal("100m"), "sibling leaf survives")
		Expect(out.Resources.Memory.Limit.String()).To(Equal("512Mi"), "sibling struct survives")
	})

	It("folds affinity per MEMBER, so clearing one keeps a sibling the product set", func() {
		// Wholesale replacement could not express this: dropping a spreading rule also nuked a
		// rack-awareness nodeAffinity declared beside it.
		productDefault := affinityConfig(&corev1.Affinity{
			PodAffinity: &corev1.PodAffinity{
				PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
					reconciler.PreferredAffinityTerm(20, reconciler.TopologyKeyHostname,
						reconciler.ClusterSelectorLabels("zk")),
				}},
			PodAntiAffinity: &corev1.PodAntiAffinity{
				PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
					reconciler.PreferredAffinityTerm(70, reconciler.TopologyKeyHostname,
						reconciler.RoleSelectorLabels("zk", "server")),
				}},
		})
		// A single-node development group clears ONLY the spread.
		userClears := affinityConfig(&corev1.Affinity{PodAntiAffinity: &corev1.PodAntiAffinity{}})

		out, err := reconciler.FoldCommonConfig(productDefault, userClears)
		Expect(err).NotTo(HaveOccurred())

		decoded, err := reconciler.DecodeAffinity(out.Affinity)
		Expect(err).NotTo(HaveOccurred())
		Expect(decoded.PodAntiAffinity).To(BeNil(), "the member the user emptied is cleared")
		Expect(decoded.PodAffinity).NotTo(BeNil(), "the sibling member survives")
		Expect(decoded.PodAffinity.PreferredDuringSchedulingIgnoredDuringExecution).To(HaveLen(1))
	})

	It("treats `affinity: {}` as stating nothing, so a pruning artifact cannot delete a default", func() {
		// `resources` is structural in the generated CRD, so the API server prunes a mistyped
		// `resources.cpu.maxx` down to an empty `cpu` object. Under "empty clears" that typo would
		// silently delete the product's scheduling policy.
		productDefault := affinityConfig(&corev1.Affinity{
			PodAntiAffinity: &corev1.PodAntiAffinity{
				PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
					reconciler.PreferredAffinityTerm(70, reconciler.TopologyKeyHostname,
						reconciler.RoleSelectorLabels("zk", "server")),
				}}})

		out, err := reconciler.FoldCommonConfig(productDefault, affinityConfig(&corev1.Affinity{}))
		Expect(err).NotTo(HaveOccurred())
		decoded, err := reconciler.DecodeAffinity(out.Affinity)
		Expect(err).NotTo(HaveOccurred())
		Expect(decoded).NotTo(BeNil())
		Expect(decoded.PodAntiAffinity).NotTo(BeNil(), "an empty affinity inherits, it does not clear")
	})

	It("rejects an affinity that does not strictly decode", func() {
		// `nodeAffinty` — one letter short — passed admission and decoded into an empty Affinity,
		// so the pods were scheduled anywhere with no event and no status change.
		_, err := reconciler.FoldCommonConfig(&commonsv1alpha1.RoleGroupConfigSpec{
			Affinity: &k8sruntime.RawExtension{Raw: []byte(`{"nodeAffinty":{}}`)},
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("nodeAffinty"))
	})

	It("folds logging on this path, so a product logging default is honoured", func() {
		out, err := reconciler.FoldCommonConfig(
			&commonsv1alpha1.RoleGroupConfigSpec{
				Logging: &commonsv1alpha1.LoggingSpec{EnableVectorAgent: ptr.To(true)}},
			&commonsv1alpha1.RoleGroupConfigSpec{},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.Logging).NotTo(BeNil())
		Expect(out.Logging.EnableVectorAgent).To(HaveValue(BeTrue()))
	})

	It("skips nil layers and never returns nil", func() {
		out, err := reconciler.FoldCommonConfig(nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(out).NotTo(BeNil())
	})

	It("leaves the framework floors to consumption time, below the bottom layer", func() {
		out, err := reconciler.FoldCommonConfig()
		Expect(err).NotTo(HaveOccurred())
		Expect(out.GracefulShutdownTimeout).To(BeNil(), "the fold states nothing")
		Expect(out.GetGracefulShutdownTimeout()).To(Equal(commonsv1alpha1.DefaultGracefulShutdownTimeout))
	})

	It("does not alias its inputs, so a resolved value cannot write back into the live CR", func() {
		role := &commonsv1alpha1.RoleGroupConfigSpec{GracefulShutdownTimeout: ptr.To("5m")}
		out, err := reconciler.FoldCommonConfig(role)
		Expect(err).NotTo(HaveOccurred())
		*out.GracefulShutdownTimeout = "999h"
		Expect(role.GracefulShutdownTimeout).To(HaveValue(Equal("5m")))
	})
})

var _ = Describe("FoldProductConfig (the product's half)", func() {
	It("is presence-wins per field, so a product default survives an unrelated user field", func() {
		// The defect this exists to prevent, and the one five of nine hand-written downstream
		// merges get wrong: an either/or rule applied to a composite drops the sibling fields the
		// upper layer did not restate.
		out, err := reconciler.FoldProductConfig(
			&productConfig{ // the product's own default
				MyidOffset: ptr.To[int32](1),
				TickTime:   ptr.To[int32](2000),
				Listener:   ptr.To("cluster-internal"),
			},
			&productConfig{TickTime: ptr.To[int32](3000)}, // the user states ONE field
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.TickTime).To(HaveValue(Equal(int32(3000))), "user wins")
		Expect(out.MyidOffset).To(HaveValue(Equal(int32(1))), "sibling default survives")
		Expect(out.Listener).To(HaveValue(Equal("cluster-internal")), "sibling default survives")
	})

	It("folds three layers, role group over role over the product's default", func() {
		out, err := reconciler.FoldProductConfig(
			&productConfig{MyidOffset: ptr.To[int32](1), TickTime: ptr.To[int32](2000), Listener: ptr.To("a")},
			&productConfig{TickTime: ptr.To[int32](3000), Listener: ptr.To("b")},
			&productConfig{Listener: ptr.To("c")},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.MyidOffset).To(HaveValue(Equal(int32(1))))
		Expect(out.TickTime).To(HaveValue(Equal(int32(3000))))
		Expect(out.Listener).To(HaveValue(Equal("c")))
	})

	It("never lets a product layer sit ON TOP of the user's value", func() {
		// Two shipped operators get this backwards and silently discard what the user asked for.
		out, err := reconciler.FoldProductConfig(
			&productConfig{Listener: ptr.To("product-default")},
			&productConfig{Listener: ptr.To("user-chose-this")},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.Listener).To(HaveValue(Equal("user-chose-this")))
	})

	It("SKIPS the embedded commons block, which the framework resolves separately", func() {
		// Folding it here as well would compute one answer in two places — the defect that made
		// the previous seam's result unreadable.
		out, err := reconciler.FoldProductConfig(
			&productConfig{RoleGroupConfigSpec: &commonsv1alpha1.RoleGroupConfigSpec{
				GracefulShutdownTimeout: ptr.To("5m")}},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.RoleGroupConfigSpec).To(BeNil(),
			"the commons half is the framework's; read it from the build context")
	})

	It("skips nil layers and never returns nil", func() {
		out, err := reconciler.FoldProductConfig[productConfig](nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(out).NotTo(BeNil())
		Expect(out.TickTime).To(BeNil())
	})

	It("does not alias any input", func() {
		role := &productConfig{TickTime: ptr.To[int32](2000)}
		out, err := reconciler.FoldProductConfig(role)
		Expect(err).NotTo(HaveOccurred())
		*out.TickTime = 9999
		Expect(role.TickTime).To(HaveValue(Equal(int32(2000))))
	})

	It("refuses to fold a shape it cannot handle honestly, rather than guessing", func() {
		_, err := reconciler.FoldProductConfig(&bareScalarConfig{TickTime: 2000})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("TickTime"))
	})
})

var _ = Describe("ValidateProductConfigType", func() {
	It("accepts the shape a real product uses", func() {
		Expect(reconciler.ValidateProductConfigType[productConfig]()).To(Succeed())
	})

	It("accepts a config block that adds nothing to the commons half", func() {
		// A product with no fields of its own never calls FoldProductConfig at all — the framework
		// resolves everything through FoldCommonConfig. But its ConfigSpec is still a legal shape,
		// and validating it must not trip over the commons block's own composite fields.
		Expect(reconciler.ValidateProductConfigType[commonsOnlyConfig]()).To(Succeed())
	})

	It("refuses a bare scalar, which cannot express `unset`", func() {
		err := reconciler.ValidateProductConfigType[bareScalarConfig]()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("TickTime"))
		Expect(err.Error()).To(ContainSubstring("bare"))
	})

	It("refuses a product composite, which wholesale replacement would strip siblings from", func() {
		err := reconciler.ValidateProductConfigType[compositeConfig]()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("Nested"))
		Expect(err.Error()).To(ContainSubstring("composite"))
	})

	It("accepts that same composite when the product opts into wholesale replacement", func() {
		Expect(reconciler.ValidateProductConfigType[taggedCompositeConfig]()).To(Succeed())
	})
})
