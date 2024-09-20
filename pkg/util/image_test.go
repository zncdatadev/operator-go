package util_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/util"
	v1 "k8s.io/api/core/v1"
)

var _ = Describe("GetImageTag", func() {
	var (
		image util.Image
		tag   func() string
	)

	BeforeEach(func() {
		tag = func() string {
			image, err := image.GetImageWithTag()
			Expect(err).ShouldNot(HaveOccurred())
			return image
		}
	})

	It("should return the custom tag if provided", func() {
		image = util.Image{
			Custom:          "myrepo/myimage:latest",
			ProductName:     "myproduct",
			KubedoopVersion: "1.0",
			ProductVersion:  "1.0.0",
		}
		Expect(tag()).Should(Equal("myrepo/myimage:latest"))
	})

	It("should return the default repository and tag if not provided", func() {
		image = util.Image{
			ProductName:     "myproduct",
			KubedoopVersion: "1.0",
			ProductVersion:  "1.0.0",
		}
		Expect(tag()).Should(Equal("quay.io/zncdatadev/myproduct:1.0.0-kubedoop1.0"))
	})

	It("should return the custom repository and tag if provided", func() {
		image = util.Image{
			Repo:            "example.com",
			ProductName:     "myproduct",
			KubedoopVersion: "1.0",
			ProductVersion:  "1.0.0",
		}
		Expect(tag()).Should(Equal("example.com/myproduct:1.0.0-kubedoop1.0"))
	})
})

var _ = Describe("GetPullPolicy", func() {
	var (
		image  util.Image
		policy func() v1.PullPolicy
	)

	BeforeEach(func() {
		policy = func() v1.PullPolicy {
			return image.GetPullPolicy()
		}
	})

	It("should return the existing PullPolicy when it is not nil", func() {
		pullPolicy := v1.PullAlways
		image = util.Image{
			PullPolicy: pullPolicy,
		}
		Expect(policy()).Should(Equal(pullPolicy))
	})

	It("should return PullIfNotPresent when PullPolicy is nil", func() {
		image = util.Image{}
		expected := v1.PullIfNotPresent
		Expect(policy()).Should(Equal(expected))
	})
})
