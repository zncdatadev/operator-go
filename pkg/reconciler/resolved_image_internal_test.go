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

package reconciler

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
)

var _ = Describe("resolveImage", func() {
	operatorDefaults := ImageResolution{
		ProductName: "zookeeper",
		Defaults: v1alpha1.ImageSpec{
			Repo:            "quay.io/zncdatadev",
			ProductVersion:  "3.9.2",
			KubedoopVersion: "0.2.0",
		},
	}

	It("assembles the kubedoop reference from the operator's defaults alone", func() {
		out, err := resolveImage(&v1alpha1.GenericClusterSpec{}, RoleDeclaration{}, operatorDefaults)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.Reference).To(Equal("quay.io/zncdatadev/zookeeper:3.9.2-kubedoop0.2.0"))
		Expect(out.ProductVersion).To(Equal("3.9.2"))
	})

	It("lets a role declare its own version without restating the repo", func() {
		// Per-field folding, not wholesale replacement: a declaration stating only productVersion
		// must not blank the repo it never mentioned.
		decl := RoleDeclaration{Image: &v1alpha1.ImageSpec{ProductVersion: "3.8.4"}}
		out, err := resolveImage(&v1alpha1.GenericClusterSpec{}, decl, operatorDefaults)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.Reference).To(Equal("quay.io/zncdatadev/zookeeper:3.8.4-kubedoop0.2.0"))
		Expect(out.ProductVersion).To(Equal("3.8.4"))
	})

	It("lets the CR beat the role's declaration", func() {
		// A product pinning a role to a different image must not silently beat a user who pinned
		// spec.image to an air-gapped mirror — the same rule the config fold enforces.
		spec := &v1alpha1.GenericClusterSpec{Image: &v1alpha1.ImageSpec{ProductVersion: "3.9.3"}}
		decl := RoleDeclaration{Image: &v1alpha1.ImageSpec{ProductVersion: "3.8.4"}}
		out, err := resolveImage(spec, decl, operatorDefaults)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.Reference).To(Equal("quay.io/zncdatadev/zookeeper:3.9.3-kubedoop0.2.0"))
		Expect(out.ProductVersion).To(Equal("3.9.3"))
	})

	It("honours spec.image.custom verbatim", func() {
		spec := &v1alpha1.GenericClusterSpec{
			Image: &v1alpha1.ImageSpec{Custom: "internal.registry/zk:pinned"}}
		out, err := resolveImage(spec, RoleDeclaration{}, operatorDefaults)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.Reference).To(Equal("internal.registry/zk:pinned"))
	})

	Context("the pull secret", func() {
		It("survives the assembled-reference path", func() {
			res := operatorDefaults
			res.Defaults.PullSecretName = "registry-creds"
			out, err := resolveImage(&v1alpha1.GenericClusterSpec{}, RoleDeclaration{}, res)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.PullSecretName).To(Equal("registry-creds"))
		})

		It("survives the custom-reference path", func() {
			// A pull secret is a property of WHERE the image lives, not of how the reference was
			// built, so it must apply on every path. Ten product CRDs accepted pullSecretName and
			// silently stopped applying it on migration: the value was accepted, no pod carried the
			// entry, and a private-registry install failed with ImagePullBackOff naming no cause.
			spec := &v1alpha1.GenericClusterSpec{Image: &v1alpha1.ImageSpec{
				Custom:         "internal.registry/zk:pinned",
				PullSecretName: "registry-creds",
			}}
			out, err := resolveImage(spec, RoleDeclaration{}, operatorDefaults)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Reference).To(Equal("internal.registry/zk:pinned"))
			Expect(out.PullSecretName).To(Equal("registry-creds"))
		})

		It("survives when the product resolves its own images", func() {
			res := ImageResolution{Defaults: v1alpha1.ImageSpec{PullSecretName: "registry-creds"}}
			spec := &v1alpha1.GenericClusterSpec{Image: &v1alpha1.ImageSpec{Custom: "own/image:1"}}
			out, err := resolveImage(spec, RoleDeclaration{}, res)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.PullSecretName).To(Equal("registry-creds"))
		})

		It("lets the CR override the operator's", func() {
			res := operatorDefaults
			res.Defaults.PullSecretName = "operator-creds"
			spec := &v1alpha1.GenericClusterSpec{Image: &v1alpha1.ImageSpec{PullSecretName: "user-creds"}}
			out, err := resolveImage(spec, RoleDeclaration{}, res)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.PullSecretName).To(Equal("user-creds"))
		})
	})

	It("resolves the pull policy with the CR first", func() {
		res := operatorDefaults
		res.Defaults.PullPolicy = corev1.PullAlways
		out, err := resolveImage(&v1alpha1.GenericClusterSpec{}, RoleDeclaration{}, res)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.PullPolicy).To(Equal(corev1.PullAlways))

		spec := &v1alpha1.GenericClusterSpec{Image: &v1alpha1.ImageSpec{PullPolicy: corev1.PullNever}}
		out, err = resolveImage(spec, RoleDeclaration{}, res)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.PullPolicy).To(Equal(corev1.PullNever))
	})

	It("fails rather than falling back when the assembled tag is unusable", func() {
		// KubedoopVersion is commonly the operator's own build version, whose scaffold default is
		// "N/A" — the slash makes the tag unparsable. The API server does not validate
		// container.image at all, so falling back would deploy a version nobody asked for and the
		// only symptom would be InvalidImageName on a pod, long after the reconcile reported
		// success.
		res := ImageResolution{ProductName: "zookeeper", Defaults: v1alpha1.ImageSpec{
			Repo: "quay.io/zncdatadev", ProductVersion: "3.9.2", KubedoopVersion: "N/A"}}
		_, err := resolveImage(&v1alpha1.GenericClusterSpec{}, RoleDeclaration{}, res)
		Expect(err).To(HaveOccurred())
	})

	It("returns an empty reference when nobody expressed an opinion", func() {
		// A product that resolves its own images leaves ProductName empty; that path never errors.
		out, err := resolveImage(&v1alpha1.GenericClusterSpec{}, RoleDeclaration{}, ImageResolution{})
		Expect(err).NotTo(HaveOccurred())
		Expect(out.Reference).To(BeEmpty())
	})
})
