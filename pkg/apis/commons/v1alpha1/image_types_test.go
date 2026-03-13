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

package v1alpha1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
)

var _ = Describe("ImageSpec", func() {

	Describe("GetImage", func() {
		Context("when Custom is set", func() {
			It("returns the custom image reference directly, ignoring other fields", func() {
				spec := &v1alpha1.ImageSpec{
					Custom:          "my-registry.com/ns/product:custom-tag",
					Repo:            "quay.io/kubedoop",
					ProductVersion:  "3.4.1",
					KubedoopVersion: "0.2.0",
				}
				Expect(spec.GetImage("product")).To(Equal("my-registry.com/ns/product:custom-tag"))
			})
		})

		Context("when Custom is not set", func() {
			It("constructs the image from Repo, productName, ProductVersion and KubedoopVersion", func() {
				spec := &v1alpha1.ImageSpec{
					Repo:            "quay.io/kubedoop",
					ProductVersion:  "3.4.1",
					KubedoopVersion: "0.2.0",
				}
				Expect(spec.GetImage("trino")).To(Equal("quay.io/kubedoop/trino:3.4.1-kubedoop0.2.0"))
			})

			It("uses the provided productName in the image path", func() {
				spec := &v1alpha1.ImageSpec{
					Repo:            "quay.io/kubedoop",
					ProductVersion:  "3.0.0",
					KubedoopVersion: "0.1.0",
				}
				Expect(spec.GetImage("hive")).To(Equal("quay.io/kubedoop/hive:3.0.0-kubedoop0.1.0"))
			})

			It("returns empty string when Repo is empty", func() {
				spec := &v1alpha1.ImageSpec{ProductVersion: "3.0.0", KubedoopVersion: "0.1.0"}
				Expect(spec.GetImage("trino")).To(Equal(""))
			})

			It("returns empty string when productName is empty", func() {
				spec := &v1alpha1.ImageSpec{Repo: "quay.io/kubedoop", ProductVersion: "3.0.0"}
				Expect(spec.GetImage("")).To(Equal(""))
			})

			It("omits kubedoop suffix when KubedoopVersion is empty", func() {
				spec := &v1alpha1.ImageSpec{
					Repo:           "quay.io/kubedoop",
					ProductVersion: "3.0.0",
				}
				Expect(spec.GetImage("trino")).To(Equal("quay.io/kubedoop/trino:3.0.0"))
			})
		})
	})

	Describe("GetPullPolicy", func() {
		It("returns the configured pull policy when set", func() {
			spec := &v1alpha1.ImageSpec{PullPolicy: corev1.PullAlways}
			Expect(spec.GetPullPolicy()).To(Equal(corev1.PullAlways))
		})

		It("defaults to IfNotPresent when PullPolicy is empty", func() {
			spec := &v1alpha1.ImageSpec{}
			Expect(spec.GetPullPolicy()).To(Equal(corev1.PullIfNotPresent))
		})
	})

	Describe("DeepCopy", func() {
		It("creates an independent copy that does not share state", func() {
			original := &v1alpha1.ImageSpec{
				Custom:          "my-registry.com/product:1.0.0",
				Repo:            "quay.io/kubedoop",
				ProductVersion:  "1.0.0",
				KubedoopVersion: "0.1.0",
				PullPolicy:      corev1.PullIfNotPresent,
			}
			copy := original.DeepCopy()
			Expect(copy).To(Equal(original))

			// Mutate original and verify copy is unaffected
			original.Custom = "changed"
			Expect(copy.Custom).To(Equal("my-registry.com/product:1.0.0"))
		})
	})

	Describe("GenericClusterSpec.Image", func() {
		It("is optional and defaults to nil", func() {
			spec := &v1alpha1.GenericClusterSpec{}
			Expect(spec.Image).To(BeNil())
		})

		It("can be set and deep copied correctly", func() {
			spec := &v1alpha1.GenericClusterSpec{
				Image: &v1alpha1.ImageSpec{
					Repo:            "quay.io/kubedoop",
					ProductVersion:  "3.4.1",
					KubedoopVersion: "0.2.0",
					PullPolicy:      corev1.PullIfNotPresent,
				},
			}
			copied := spec.DeepCopy()
			Expect(copied.Image).NotTo(BeNil())
			Expect(copied.Image.ProductVersion).To(Equal("3.4.1"))

			// Verify independence
			spec.Image.ProductVersion = "9.9.9"
			Expect(copied.Image.ProductVersion).To(Equal("3.4.1"))
		})
	})
})
