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
	clusterName string,
	clusterOperation *apiv1alpha1.ClusterOperationSpec,
	spec *GiteaClusterSpec,
) *ClusterReconciler {
	return &ClusterReconciler{
		BaseCluster: *reconciler.NewBaseCluster[*GiteaClusterSpec](
			client,
			clusterName,
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
		map[string]string{"app.kubernetes.io/name": r.GetName()},
		map[string]string{"app.kubernetes.io/name": r.GetName()},
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
		const name = "whoami"
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
					Name:      "fake-owner",
					Namespace: namespace,
					UID:       types.UID("fake-uid"),
				},
			}

			resourceClient = client.NewClient(k8sClient, fakeOwner)
		})

		AfterEach(func() {

		})

		It("should pass", func() {
			clusterReconciler := NewClusterReconciler(
				resourceClient,
				name,
				&apiv1alpha1.ClusterOperationSpec{},
				giteaCluster,
			)

			Expect(clusterReconciler).ShouldNot(BeNil())

			Expect(clusterReconciler.RegisterResources(ctx)).Should(BeNil())

			result := clusterReconciler.Reconcile(ctx)
			Expect(result).ShouldNot(BeNil())
			Expect(result.Error).Should(BeNil())
			Expect(result.RequeueOrNot()).Should(BeTrue())

		})
	})
})
