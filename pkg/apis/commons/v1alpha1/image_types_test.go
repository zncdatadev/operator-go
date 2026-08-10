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
	"strings"

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

var _ = Describe("ResolveImage", func() {
	defaults := v1alpha1.ImageSpec{
		Repo:            "quay.io/zncdatadev",
		ProductVersion:  "4.0.1",
		KubedoopVersion: "0.0.0-dev",
	}

	It("fills what the user left empty, so a bare productVersion resolves", func() {
		// The case that made three operators hand-roll image resolution. Kubedoop publishes only
		// the "-kubedoop<version>" tag, and GetImage appends that suffix only when the USER wrote
		// kubedoopVersion — so a spec carrying just productVersion produced
		// "quay.io/zncdatadev/hive:4.0.1", which does not exist in the registry.
		spec := &v1alpha1.ImageSpec{ProductVersion: "4.1.0"}
		Expect(spec.ResolveImage("hive", defaults)).
			To(Equal("quay.io/zncdatadev/hive:4.1.0-kubedoop0.0.0-dev"))
	})

	It("resolves entirely from defaults when the user states nothing", func() {
		Expect((&v1alpha1.ImageSpec{}).ResolveImage("hive", defaults)).
			To(Equal("quay.io/zncdatadev/hive:4.0.1-kubedoop0.0.0-dev"))
	})

	It("treats a nil spec as an empty one", func() {
		var spec *v1alpha1.ImageSpec
		Expect(spec.ResolveImage("hive", defaults)).
			To(Equal("quay.io/zncdatadev/hive:4.0.1-kubedoop0.0.0-dev"))
	})

	It("ignores pullPolicy when deciding whether the user stated anything", func() {
		// pullPolicy carries a CRD default, so it is filled the moment `image: {}` exists. Counting
		// it would make every stored spec look like a user opinion.
		spec := &v1alpha1.ImageSpec{PullPolicy: corev1.PullAlways}
		Expect(spec.ResolveImage("hive", v1alpha1.ImageSpec{Custom: "pinned:1"})).To(Equal("pinned:1"))
	})

	It("lets the spec's custom win outright", func() {
		spec := &v1alpha1.ImageSpec{Custom: "my-registry/hive:local", ProductVersion: "9.9.9"}
		Expect(spec.ResolveImage("hive", defaults)).To(Equal("my-registry/hive:local"))
	})

	It("does not let a default custom discard a version the user asked for", func() {
		// The whole point of this change is that a user's stated version reaches the container.
		// A product pinning ImageDefaults.Custom must not silently undo that.
		spec := &v1alpha1.ImageSpec{ProductVersion: "4.1.0"}
		withCustomDefault := defaults
		withCustomDefault.Custom = "pinned/hive:frozen"
		Expect(spec.ResolveImage("hive", withCustomDefault)).
			To(Equal("quay.io/zncdatadev/hive:4.1.0-kubedoop0.0.0-dev"))
	})

	It("uses a default custom when the user states nothing", func() {
		Expect((&v1alpha1.ImageSpec{}).ResolveImage("hive", v1alpha1.ImageSpec{Custom: "pinned/hive:frozen"})).
			To(Equal("pinned/hive:frozen"))
	})

	It("errors, naming the missing field, rather than resolving to nothing", func() {
		// Returning "" here is what let a caller fall back to its static image and run a version
		// nobody asked for, with no error and no event.
		_, err := (&v1alpha1.ImageSpec{ProductVersion: "4.1.0"}).ResolveImage("hive", v1alpha1.ImageSpec{})
		Expect(err).To(MatchError(ContainSubstring("repo is unset")))
		Expect(err).To(MatchError(ContainSubstring("hive")))
	})

	It("says nothing when nobody expressed an opinion", func() {
		image, err := (&v1alpha1.ImageSpec{}).ResolveImage("hive", v1alpha1.ImageSpec{})
		Expect(err).NotTo(HaveOccurred())
		Expect(image).To(BeEmpty(), "the caller falls back to whatever it would have used")
	})

	It("still resolves a default custom with no product name", func() {
		// A fully qualified reference needs no repository path segment, from either layer. The
		// field docs said only spec.image.custom survived an empty ProductName, which was the
		// design before it was narrowed — an untested sentence is how that drift survives.
		image, err := (&v1alpha1.ImageSpec{}).ResolveImage("", v1alpha1.ImageSpec{Custom: "pinned/hive:frozen"})
		Expect(err).NotTo(HaveOccurred())
		Expect(image).To(Equal("pinned/hive:frozen"))
	})

	It("writes its error for whoever is editing the CR, not for the SDK", func() {
		// A product's validating webhook forwards this verbatim to a `kubectl apply`, where
		// SDK-internal vocabulary is noise: nobody editing a CR knows what an ImageDefaults is.
		_, err := (&v1alpha1.ImageSpec{ProductVersion: "4.1.0"}).ResolveImage("hive", v1alpha1.ImageSpec{})
		Expect(err).To(MatchError(ContainSubstring("spec.image")))
		Expect(err.Error()).NotTo(ContainSubstring("ImageDefaults"))
		Expect(err.Error()).NotTo(ContainSubstring("handler"))
	})

	It("resolves only custom when no product name is given, without erroring", func() {
		// An empty product name means the caller resolves images itself — the shape hive and
		// zookeeper use today. Erroring here would break every one of their clusters, and it is
		// the caller's own arrangement rather than a user mistake.
		image, err := (&v1alpha1.ImageSpec{Repo: "r", ProductVersion: "1"}).ResolveImage("", defaults)
		Expect(err).NotTo(HaveOccurred())
		Expect(image).To(BeEmpty())

		Expect((&v1alpha1.ImageSpec{Custom: "pinned:1"}).ResolveImage("", defaults)).To(Equal("pinned:1"))
	})

	It("rejects a kubedoopVersion that cannot appear in an image tag", func() {
		// The shipped failure: the documented wiring is ImageDefaults.KubedoopVersion =
		// version.BuildVersion, and the scaffold's dev default for that variable is "N/A". The "/"
		// makes the tag unparsable, so every pod of a development build was InvalidImageName while
		// the reconcile reported success — the API server does not validate container.image at all,
		// so nothing between here and the kubelet would have caught it.
		broken := defaults
		broken.KubedoopVersion = "N/A"
		image, err := (&v1alpha1.ImageSpec{}).ResolveImage("hive", broken)
		Expect(err).To(HaveOccurred())
		Expect(image).To(BeEmpty(), "an unusable reference must never be returned")
		Expect(err).To(MatchError(ContainSubstring("kubedoopVersion")))
		Expect(err).To(MatchError(ContainSubstring("N/A")))
		// The value is the operator's, not the CR's, so the message says where to look.
		Expect(err).To(MatchError(ContainSubstring("build version")))
	})

	It("rejects a productVersion that cannot open an image tag", func() {
		// A tag may not start with '.' or '-', though both are legal inside one. Naming the field
		// matters: the two halves of the tag come from different places (a CR and the operator's
		// own defaults), so "bad tag" alone would not say whose value to fix.
		_, err := (&v1alpha1.ImageSpec{ProductVersion: ".4.1.0"}).ResolveImage("hive", defaults)
		Expect(err).To(MatchError(ContainSubstring("productVersion")))
		Expect(err).To(MatchError(ContainSubstring(".4.1.0")))
	})

	It("rejects an assembled tag longer than a tag may be", func() {
		long := defaults
		long.KubedoopVersion = strings.Repeat("v", 200)
		_, err := (&v1alpha1.ImageSpec{}).ResolveImage("hive", long)
		Expect(err).To(MatchError(ContainSubstring("128")))
	})

	It("leaves a custom reference alone", func() {
		// custom is the user's verbatim reference, whole. The framework assembles nothing there, so
		// it validates nothing: a wrong value is the user's own visible mistake, while a tag built
		// from two layers is a mistake the CR may not even contain.
		image, err := (&v1alpha1.ImageSpec{Custom: "my-registry/hive:N/A"}).ResolveImage("hive", defaults)
		Expect(err).NotTo(HaveOccurred())
		Expect(image).To(Equal("my-registry/hive:N/A"))
	})

	It("accepts the tag characters real product versions use", func() {
		// Control: '.', '-' and '_' are all legal inside a tag, and pre-release versions use them.
		ok := defaults
		ok.KubedoopVersion = "0.0.0-dev_1.2"
		Expect((&v1alpha1.ImageSpec{ProductVersion: "4.1.0-rc.1"}).ResolveImage("hive", ok)).
			To(Equal("quay.io/zncdatadev/hive:4.1.0-rc.1-kubedoop0.0.0-dev_1.2"))
	})

	It("keeps GetImage in step by delegating to it", func() {
		spec := &v1alpha1.ImageSpec{Repo: "quay.io/kubedoop", ProductVersion: "3.4.1", KubedoopVersion: "0.2.0"}
		Expect(spec.GetImage("trino")).To(Equal("quay.io/kubedoop/trino:3.4.1-kubedoop0.2.0"))
		Expect((&v1alpha1.ImageSpec{ProductVersion: "3.4.1"}).GetImage("trino")).To(BeEmpty(),
			"the legacy form still reports nothing rather than the reason")
	})
})

var _ = Describe("pull policy", func() {
	It("is nil-safe", func() {
		// spec.image is an optional pointer, so a CR declaring no image reaches this as nil. It
		// used to panic, and only surfaced once ImageDefaults made a nil spec resolve to a real
		// image instead of stopping earlier.
		var spec *v1alpha1.ImageSpec
		Expect(spec.GetPullPolicy()).To(Equal(corev1.PullIfNotPresent))
		Expect(spec.ResolvedPullPolicy(v1alpha1.ImageSpec{})).To(Equal(corev1.PullIfNotPresent))
	})

	It("layers like the image does: user, then product, then IfNotPresent", func() {
		defaults := v1alpha1.ImageSpec{PullPolicy: corev1.PullNever}
		Expect((&v1alpha1.ImageSpec{PullPolicy: corev1.PullAlways}).ResolvedPullPolicy(defaults)).
			To(Equal(corev1.PullAlways))
		Expect((&v1alpha1.ImageSpec{}).ResolvedPullPolicy(defaults)).To(Equal(corev1.PullNever))
		Expect((&v1alpha1.ImageSpec{}).ResolvedPullPolicy(v1alpha1.ImageSpec{})).
			To(Equal(corev1.PullIfNotPresent))
	})
})

var _ = Describe("ResolvedProductVersion", func() {
	defaults := v1alpha1.ImageSpec{Repo: "quay.io/zncdatadev", ProductVersion: "4.0.1"}

	It("reports the version the resolved image runs", func() {
		Expect((&v1alpha1.ImageSpec{ProductVersion: "4.1.0"}).ResolvedProductVersion(defaults)).To(Equal("4.1.0"))
		Expect((&v1alpha1.ImageSpec{}).ResolvedProductVersion(defaults)).To(Equal("4.0.1"),
			"the default is what the pods actually run, so the label must say so")
	})

	It("keeps the user's declaration alongside a custom reference", func() {
		// `custom` replaces the image REFERENCE; productVersion remains the user's statement of
		// which product version that reference is. Nothing else knows, and they do.
		spec := &v1alpha1.ImageSpec{Custom: "my-registry/hive:local", ProductVersion: "9.9.9"}
		Expect(spec.ResolvedProductVersion(defaults)).To(Equal("9.9.9"))

		Expect((&v1alpha1.ImageSpec{Custom: "my-registry/hive:local"}).ResolvedProductVersion(defaults)).
			To(BeEmpty(), "with no declaration there is nothing to publish")
	})

	It("is empty when the custom reference came from defaults rather than the spec", func() {
		// A product's default productVersion says nothing about an image it did not build.
		Expect((&v1alpha1.ImageSpec{}).ResolvedProductVersion(
			v1alpha1.ImageSpec{Custom: "pinned:1", ProductVersion: "4.0.1"})).To(BeEmpty())
	})
})
