package reconciler_test

import (
	"context"
	"math/rand"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// ClusterReconciler reconciles a GiteaCluster object
// Extends this struct to implement the reconciler,
// in addition, user can add more fields if needed
type ClusterReconciler struct {
	reconciler.BaseCluster[*GiteaClusterSpec]
	ClusterConfig *ClusterConfigSpec // add more fields in implementation
}

func NewClusterReconciler(
	client *client.Client,
	clusterInfo *reconciler.ClusterInfo,
	clusterOperation *apiv1alpha1.ClusterOperationSpec,
	spec *GiteaClusterSpec,
) *ClusterReconciler {
	return &ClusterReconciler{
		BaseCluster: *reconciler.NewBaseCluster[*GiteaClusterSpec](
			client,
			clusterInfo,
			clusterOperation,
			spec,
		),
		ClusterConfig: spec.ClusterConfig,
	}
}

// RegisterResources registers resources
// Implements this method to register resources
func (r *ClusterReconciler) RegisterResources(ctx context.Context) error {
	// If a service resource in cluster level needs to be create,
	// create a service reconciler and register it
	serviceReconciler := reconciler.NewServiceReconciler(
		r.Client,
		r.GetName(),
		r.ClusterInfo.GetLabels(),
		r.ClusterInfo.GetAnnotations(),
		[]corev1.ContainerPort{
			{
				Name:          "http",
				ContainerPort: 3000,
			},
		},
	)
	// Register resources
	r.AddResource(serviceReconciler)

	return nil
}

var _ = Describe("Cluster reconciler", func() {
	Context("ClusterReconciler test", func() {
		var resourceClient *client.Client

		clusterInfo := &reconciler.ClusterInfo{
			GVK: &metav1.GroupVersionKind{
				Group:   "fake.zncdata.dev",
				Version: "v1alpha1",
				Kind:    "GiteaCluster",
			},
			ClusterName: "fake-owner",
		}

		var namespace string
		ctx := context.Background()

		giteaCluster := &GiteaClusterSpec{}

		BeforeEach(func() {
			namespace = "test-" + strconv.Itoa(rand.Intn(10000))
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}

			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())

			fakeOwner := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      clusterInfo.GetClusterName(),
					Namespace: namespace,
					UID:       types.UID("fake-uid"),
				},
			}

			resourceClient = client.NewClient(k8sClient, fakeOwner)
		})

		AfterEach(func() {

		})

		It("should success reconcile cluster resource", func() {
			By("Create a cluster reconciler")
			clusterReconciler := NewClusterReconciler(
				resourceClient,
				clusterInfo,
				&apiv1alpha1.ClusterOperationSpec{},
				giteaCluster,
			)
			Expect(clusterReconciler).ShouldNot(BeNil())

			By("Register resources")
			Expect(clusterReconciler.RegisterResources(ctx)).Should(BeNil())

			By("Reconcile")
			result := clusterReconciler.Reconcile(ctx)
			Expect(result).ShouldNot(BeNil())
			Expect(result.Error).Should(BeNil())
			Expect(result.RequeueOrNot()).Should(BeTrue())

			By("Ready")
			result = clusterReconciler.Ready(ctx)
			Expect(result).ShouldNot(BeNil())
			Expect(result.Error).Should(BeNil())
			Expect(result.RequeueOrNot()).Should(BeFalse())

			By("Checking the service resource of cluster level")
			service := &corev1.Service{}
			serviceName := clusterInfo.GetClusterName()
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: namespace}, service)).Should(Succeed())

			By("Checking the service labels")
			Expect(service.Labels).Should(HaveKeyWithValue("app.kubernetes.io/instance", clusterInfo.GetClusterName()))

		})
	})
})
