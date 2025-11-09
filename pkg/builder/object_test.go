package builder_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/constants"
)

var _ = Describe("ObjectMeta Immutability Tests", func() {
	var cli *client.Client

	BeforeEach(func() {
		cli = client.NewClient(k8sClient, &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-service",
				Namespace: "test-namespace",
			},
		})
	})

	Context("ObjectMeta.GetLabels()", func() {
		It("should return immutable labels - modifications should not affect internal state", func() {
			objMeta := builder.NewObjectMeta(
				cli,
				"test-resource",
				func(o *builder.Options) {
					o.ClusterName = "test-cluster"
				},
			)

			// Get labels
			labels1 := objMeta.GetLabels()
			Expect(labels1).To(HaveKey(constants.LabelKubernetesInstance))
			Expect(labels1[constants.LabelKubernetesInstance]).To(Equal("test-cluster"))

			// Modify the returned map
			labels1["custom-key"] = "custom-value"
			labels1[constants.LabelKubernetesInstance] = "modified-cluster"

			// Get labels again and verify the modification didn't affect internal state
			labels2 := objMeta.GetLabels()
			Expect(labels2).NotTo(HaveKey("custom-key"))
			Expect(labels2[constants.LabelKubernetesInstance]).To(Equal("test-cluster"))
		})

		It("should allow adding labels through AddLabels without affecting returned copies", func() {
			objMeta := builder.NewObjectMeta(
				cli,
				"test-resource",
			)

			// Get initial labels
			labels1 := objMeta.GetLabels()
			initialSize := len(labels1)

			// Add labels through AddLabels method
			objMeta.AddLabels(map[string]string{
				"added-label-1": "value-1",
				"added-label-2": "value-2",
			})

			// Verify labels1 (previously returned copy) is not affected
			Expect(labels1).NotTo(HaveKey("added-label-1"))
			Expect(labels1).NotTo(HaveKey("added-label-2"))
			Expect(labels1).To(HaveLen(initialSize))

			// Get new labels and verify the added labels are present
			labels2 := objMeta.GetLabels()
			Expect(labels2).To(HaveKey("added-label-1"))
			Expect(labels2["added-label-1"]).To(Equal("value-1"))
			Expect(labels2).To(HaveKey("added-label-2"))
			Expect(labels2["added-label-2"]).To(Equal("value-2"))
		})

		It("should return independent copies on each call", func() {
			objMeta := builder.NewObjectMeta(
				cli,
				"test-resource",
			)

			// Get labels twice
			labels1 := objMeta.GetLabels()
			labels2 := objMeta.GetLabels()

			// Modify first copy
			labels1["test-key"] = "test-value"

			// Verify second copy is not affected
			Expect(labels2).NotTo(HaveKey("test-key"))
		})
	})
})
