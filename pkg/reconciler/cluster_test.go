package reconciler_test

import (
	"context"
	"math/rand"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/constants"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// ClusterReconciler reconciles a TrinoCluster object
// Extends this struct to implement the reconciler,
// in addition, user can add more fields if needed
type ClusterReconciler struct {
	reconciler.BaseCluster[*TrinoClusterSpec]
	ClusterConfig *ClusterConfigSpec // add more fields in implementation
}

func NewClusterReconciler(
	client *client.Client,
	clusterInfo reconciler.ClusterInfo,
	clusterOperation *commonsv1alpha1.ClusterOperationSpec,
	spec *TrinoClusterSpec,
) *ClusterReconciler {
	return &ClusterReconciler{
		BaseCluster: *reconciler.NewBaseCluster[*TrinoClusterSpec](
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
		r.GetClient(),
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

	role := NewRoleReconciler(
		r.GetClient(),
		reconciler.RoleInfo{
			ClusterInfo: r.ClusterInfo,
			RoleName:    "coordinator",
		},
		r.ClusterOperation,
		r.ClusterConfig,
		*r.Spec.Coordinator,
	)

	if err := role.RegisterResources(ctx); err != nil {
		return err
	}

	r.AddResource(role)

	return nil
}

var _ = Describe("Cluster reconciler", func() {

	clusterOperation := &commonsv1alpha1.ClusterOperationSpec{
		Stopped: false,
	}

	Context("ClusterReconciler test", func() {
		var resourceClient *client.Client

		clusterInfo := reconciler.ClusterInfo{
			GVK: &metav1.GroupVersionKind{
				Group:   "fake.zncdata.dev",
				Version: "v1alpha1",
				Kind:    "TrinoCluster",
			},
			ClusterName: "fake-owner",
		}

		var namespace string
		ctx := context.Background()

		var trinoCluster *TrinoClusterSpec

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

			trinoCluster = &TrinoClusterSpec{
				ClusterConfig: &ClusterConfigSpec{
					ListenerClass: "default",
				},
				Coordinator: &CoordinatorSpec{
					RoleGroups: map[string]TrinoRoleGroupSpec{
						"default": {
							Replicas: &[]int32{1}[0],
						},
					},
				},
			}
		})

		AfterEach(func() {

		})

		It("should success reconcile cluster resource", func() {
			By("Create a cluster reconciler")
			clusterReconciler := NewClusterReconciler(
				resourceClient,
				clusterInfo,
				clusterOperation,
				trinoCluster,
			)
			Expect(clusterReconciler).ShouldNot(BeNil())

			By("Register resources")
			Expect(clusterReconciler.RegisterResources(ctx)).Should(BeNil())

			By("Reconcile")
			Eventually(func() bool {
				result := clusterReconciler.Reconcile(ctx)
				return result.RequeueOrNot()
			}, time.Second*15, time.Microsecond*100).Should(BeFalse())

			By("Checking the service resource of cluster level")
			service := &corev1.Service{}
			serviceName := clusterInfo.GetClusterName()
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: namespace}, service)).Should(Succeed())

			By("Checking the service labels")
			Expect(service.Labels).Should(HaveKeyWithValue(constants.LabelKubernetesInstance, clusterInfo.GetClusterName()))
			Expect(service.Labels).ShouldNot(HaveKey(constants.LabelKubernetesRoleGroup))
			Expect(service.Labels).ShouldNot(HaveKey(constants.LabelKubernetesComponent))

			By("Checking Deployment resource of coordinator")
			coordinatorDeployment := &appv1.Deployment{}
			coordinatorName := clusterInfo.GetClusterName() + "-coordinator" + "-default"
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: coordinatorName, Namespace: namespace}, coordinatorDeployment)).Should(Succeed())

		})
	})
})
