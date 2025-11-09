package reconciler_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/zncdatadev/operator-go/pkg/constants"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
)

const testCustomValue = "custom-value"

var _ = Describe("Info Immutability Tests", func() {
	var (
		gvk *metav1.GroupVersionKind
	)

	BeforeEach(func() {
		gvk = &metav1.GroupVersionKind{
			Group:   "test.example.com",
			Version: "v1",
			Kind:    "TestCluster",
		}
	})

	Context("ClusterInfo.GetLabels()", func() {
		It("should return immutable labels - modifications should not affect internal state", func() {
			clusterInfo := reconciler.ClusterInfo{
				GVK:         gvk,
				ClusterName: "test-cluster",
			}

			// Get labels
			labels1 := clusterInfo.GetLabels()
			Expect(labels1).To(HaveKey(constants.LabelKubernetesInstance))
			Expect(labels1[constants.LabelKubernetesInstance]).To(Equal("test-cluster"))

			// Modify the returned map
			labels1["custom-key"] = "custom-value"

			// Get labels again and verify the modification didn't affect internal state
			labels2 := clusterInfo.GetLabels()
			Expect(labels2).NotTo(HaveKey("custom-key"))
			Expect(labels2[constants.LabelKubernetesInstance]).To(Equal("test-cluster"))
		})

		It("should allow adding labels through AddLabel without affecting returned copies", func() {
			clusterInfo := reconciler.ClusterInfo{
				GVK:         gvk,
				ClusterName: "test-cluster",
			}

			// Get initial labels
			labels1 := clusterInfo.GetLabels()
			initialSize := len(labels1)

			// Add a label through AddLabel method
			clusterInfo.AddLabel("added-label", "added-value")

			// Verify labels1 (previously returned copy) is not affected
			Expect(labels1).NotTo(HaveKey("added-label"))
			Expect(labels1).To(HaveLen(initialSize))

			// Get new labels and verify the added label is present
			labels2 := clusterInfo.GetLabels()
			Expect(labels2).To(HaveKey("added-label"))
			Expect(labels2["added-label"]).To(Equal("added-value"))
		})
	})

	Context("RoleInfo.GetLabels()", func() {
		It("should return immutable labels - modifications should not affect internal state", func() {
			roleInfo := reconciler.RoleInfo{
				ClusterInfo: reconciler.ClusterInfo{
					GVK:         gvk,
					ClusterName: "test-cluster",
				},
				RoleName: "test-role",
			}

			// Get labels
			labels1 := roleInfo.GetLabels()
			Expect(labels1).To(HaveKey(constants.LabelKubernetesComponent))
			Expect(labels1[constants.LabelKubernetesComponent]).To(Equal("test-role"))

			// Modify the returned map
			labels1["custom-key"] = testCustomValue

			// Get labels again and verify the modification didn't affect internal state
			labels2 := roleInfo.GetLabels()
			Expect(labels2).NotTo(HaveKey("custom-key"))
			Expect(labels2[constants.LabelKubernetesComponent]).To(Equal("test-role"))
		})
	})

	Context("RoleGroupInfo.GetLabels()", func() {
		It("should return immutable labels - modifications should not affect internal state", func() {
			roleGroupInfo := reconciler.RoleGroupInfo{
				RoleInfo: reconciler.RoleInfo{
					ClusterInfo: reconciler.ClusterInfo{
						GVK:         gvk,
						ClusterName: "test-cluster",
					},
					RoleName: "test-role",
				},
				RoleGroupName: "test-group",
			}

			// Get labels
			labels1 := roleGroupInfo.GetLabels()
			Expect(labels1).To(HaveKey(constants.LabelKubernetesRoleGroup))
			Expect(labels1[constants.LabelKubernetesRoleGroup]).To(Equal("test-group"))

			// Modify the returned map
			labels1["custom-key"] = testCustomValue
			labels1[constants.LabelKubernetesRoleGroup] = "modified-group"

			// Get labels again and verify the modification didn't affect internal state
			labels2 := roleGroupInfo.GetLabels()
			Expect(labels2).NotTo(HaveKey("custom-key"))
			Expect(labels2[constants.LabelKubernetesRoleGroup]).To(Equal("test-group"))
		})
	})
})
