package productlogging_test

import (
	"k8s.io/apimachinery/pkg/api/resource"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/productlogging"
)

var _ = Describe("CalculateLogVolumeSizeLimit", func() {
	Context("when maxLogFilesSize contains multiple quantities", func() {
		It("should correctly sum all quantities", func() {
			// given
			quantities := []resource.Quantity{
				resource.MustParse("1Gi"),
				resource.MustParse("2Gi"),
				resource.MustParse("3Gi"),
			}

			// when
			result := productlogging.CalculateLogVolumeSizeLimit(quantities)

			// then
			expected := resource.MustParse("18Gi")
			Expect(result.Cmp(expected)).To(Equal(0))
		})
	})

	// when maxLogFilesSize is Mibi
	// the limit is calculated by summing up all the given sizes, scaling the result to MEBI and multiplying it by 3.0
	// the result is then ceiled to avoid bulky numbers due to floating-point arithmetic
	Context("when maxLogFilesSize is Mibi", func() {
		It("should return the quantity multiplied by 3", func() {
			// given
			quantities := []resource.Quantity{
				resource.MustParse("1Mi"),
				resource.MustParse("2Mi"),
				resource.MustParse("3Mi"),
			}

			// when
			result := productlogging.CalculateLogVolumeSizeLimit(quantities)

			// then
			expected := resource.MustParse("18Mi")

			Expect(result.Cmp(expected)).To(Equal(0))

		})

	})

	Context("when maxLogFilesSize is an empty slice", func() {
		It("should return a zero quantity", func() {
			// given
			quantities := []resource.Quantity{}

			// when
			result := productlogging.CalculateLogVolumeSizeLimit(quantities)

			// then
			expected := resource.Quantity{}
			Expect(result.Cmp(expected)).To(Equal(0))
		})
	})

	Context("when maxLogFilesSize contains a single quantity", func() {
		It("should return the quantity multiplied by 3", func() {
			// given
			quantities := []resource.Quantity{
				resource.MustParse("1Gi"),
			}

			// when
			result := productlogging.CalculateLogVolumeSizeLimit(quantities)

			// then
			expected := resource.MustParse("3Gi")
			Expect(result.Cmp(expected)).To(Equal(0))
		})
	})
})
