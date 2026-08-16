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
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"

	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
)

// aliasProbeOpts is a product's own composite, tagged atomic — a shape the fold explicitly permits.
// Its map and slice are what a by-value struct copy leaves shared.
type aliasProbeOpts struct {
	Tags map[string]string
	SANs []string
}

type aliasProbeConfig struct {
	*commonsv1alpha1.RoleGroupConfigSpec `json:",inline"`

	Env  []corev1.EnvVar
	Opts *aliasProbeOpts `kubedoop:"atomic"`
	Raw  *k8sruntime.RawExtension
}

var _ = Describe("FoldProductConfig never aliases a layer", func() {
	// The doc promises "never aliasing the live object in the informer cache", and the CR's role
	// and role group levels ARE that object. A fold result that shares memory with them turns an
	// ordinary mutation by product code into a process-wide cache corruption: every other
	// controller in the binary, and every later reconcile, reads the mutated CR.
	//
	// Struct values were the hole. A struct assigned by value copies its fields by value, so every
	// pointer, slice and map inside it stayed shared.

	It("deep-copies a pointer reached through a slice of structs", func() {
		// []corev1.EnvVar is the everyday case: EnvVar is a struct and ValueFrom is a pointer.
		layer := &aliasProbeConfig{
			Env: []corev1.EnvVar{{
				Name:      "POD_NAME",
				ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"}},
			}},
		}
		got, err := reconciler.FoldProductConfig(nil, layer, nil)
		Expect(err).NotTo(HaveOccurred())

		got.Env[0].ValueFrom.FieldRef.FieldPath = "metadata.namespace"
		Expect(layer.Env[0].ValueFrom.FieldRef.FieldPath).To(Equal("metadata.name"),
			"mutating the fold result must not write back into the layer")
	})

	It("deep-copies the map and slice inside an atomic composite", func() {
		layer := &aliasProbeConfig{
			Opts: &aliasProbeOpts{Tags: map[string]string{"k": "v"}, SANs: []string{"a"}},
		}
		got, err := reconciler.FoldProductConfig(nil, layer, nil)
		Expect(err).NotTo(HaveOccurred())

		got.Opts.Tags["k"] = "MUTATED"
		got.Opts.SANs[0] = "MUTATED"
		Expect(layer.Opts.Tags).To(HaveKeyWithValue("k", "v"))
		Expect(layer.Opts.SANs[0]).To(Equal("a"))
	})

	It("deep-copies the byte slice inside an allowlisted opaque leaf", func() {
		// RawExtension is on the allowlist AND is what `affinity` is carried in, so this is the
		// leaf most likely to be folded in practice.
		layer := &aliasProbeConfig{Raw: &k8sruntime.RawExtension{Raw: []byte(`{"a":1}`)}}
		got, err := reconciler.FoldProductConfig(layer)
		Expect(err).NotTo(HaveOccurred())

		got.Raw.Raw[2] = 'X'
		Expect(string(layer.Raw.Raw)).To(Equal(`{"a":1}`))
	})
})

// namedCommonConfig has a NAMED field of the framework's own type, beside the embedded one. A
// per-sidecar resources block is spelled exactly this way, so it is not a contrived shape.
type namedCommonConfig struct {
	*commonsv1alpha1.RoleGroupConfigSpec `json:",inline"`

	Sidecar *commonsv1alpha1.RoleGroupConfigSpec
	Foo     *string
}

var _ = Describe("the fold skips the EMBEDDED commons block, not every field of its type", func() {
	It("rejects a named field of that type rather than dropping it in silence", func() {
		// Matching by type alone made this field invisible: the validator accepted the shape and
		// the fold discarded the value, so a product shipped a config block that was silently
		// always empty. It is a composite like any other, so V5 applies.
		err := reconciler.ValidateProductConfigType[namedCommonConfig]()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("Sidecar"), "names the field")

		_, foldErr := reconciler.FoldProductConfig(&namedCommonConfig{Foo: ptr.To("bar")})
		Expect(foldErr).To(HaveOccurred(), "the fold refuses the shape too, rather than half-folding")
	})

	It("still skips the embedded one, which the framework owns", func() {
		// FoldCommonConfig resolves that half; folding it here as well would compute one answer in
		// two places, which is the defect the whole two-owner split exists to prevent.
		type okConfig struct {
			*commonsv1alpha1.RoleGroupConfigSpec `json:",inline"`
			TickTime                             *int32
		}
		Expect(reconciler.ValidateProductConfigType[okConfig]()).To(Succeed())

		got, err := reconciler.FoldProductConfig(&okConfig{
			RoleGroupConfigSpec: &commonsv1alpha1.RoleGroupConfigSpec{GracefulShutdownTimeout: ptr.To("99s")},
			TickTime:            ptr.To(int32(2000)),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(got.RoleGroupConfigSpec).To(BeNil(), "the framework's half is not folded here")
		Expect(*got.TickTime).To(Equal(int32(2000)))
	})
})
