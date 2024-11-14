// Test deployment reconciler
// In the envtest test environment, use a special pod as the ownerReference of the resource.
//
// In actual development, it is preferred to implement the deployment builder according to the requirements.
// You can call other methods of deploymentBuilder to quickly build the deployment object.
// For example, adding containers, adding init containers, adding volumes, etc.
package reconciler_test

import (
	"context"
	"math/rand"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/util"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ builder.DeploymentBuilder = &TrinoCoordinatorDeploymentBuilder{}

type TrinoCoordinatorDeploymentBuilder struct {
	builder.Deployment
}

func (b *TrinoCoordinatorDeploymentBuilder) Build(ctx context.Context) (ctrlclient.Object, error) {
	trinoContainer := builder.NewContainerBuilder("coordinator", b.GetImage()).
		SetCommand([]string{"/usr/lib/trino/bin/launcher", "run"}).
		SetImagePullPolicy(b.GetImage().PullPolicy).
		Build()

	b.AddContainer(trinoContainer)

	return b.GetObject()
}

var _ = Describe("Deloyment reconciler", func() {

	Context("DeploymentReconciler test", func() {
		var resourceClient *client.Client
		var deploymentBuilder *TrinoCoordinatorDeploymentBuilder
		const name = "whoami"
		var namespace string
		ctx := context.Background()
		replcias := int32(3)

		BeforeEach(func() {
			// Define a random namespace
			namespace = "test-" + strconv.Itoa(rand.Intn(10000))
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			// Create a namespace
			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())

			fakeOwner := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "fake-owner",
					Namespace: namespace,
					UID:       types.UID("fake-uid"),
				},
			}

			resourceClient = client.NewClient(k8sClient, fakeOwner)

			// Create a deployment builder
			deploymentBuilder = &TrinoCoordinatorDeploymentBuilder{
				Deployment: *builder.NewDeployment(
					resourceClient,
					name,
					&replcias,
					util.NewImage("trino", "458", "1.0.0"),
					nil,
					nil,
				),
			}
		})

		It("Should successfully reconcile a whoami deployment", func() {
			By("Create a deployment reconciler")
			deploymentReconciler := reconciler.NewDeployment(resourceClient, deploymentBuilder, false)
			Expect(deploymentReconciler).ShouldNot(BeNil())

			By("reconcile the deployment")
			result, err := deploymentReconciler.Reconcile(ctx)
			Expect(result).ShouldNot(BeNil())
			// Expect(result.Error).Should(BeNil())
			// Expect(result.RequeueOrNot()).Should(BeTrue())
			Expect(err).Should(BeNil())
			Expect(result.IsZero()).Should(BeFalse())
			Expect(result.Requeue).Should(BeTrue())

			By("Checking the deployment spec.replicas is valid")
			deployment := &appv1.Deployment{}
			Expect(k8sClient.Get(ctx, ctrlclient.ObjectKey{Namespace: namespace, Name: name}, deployment)).Should(Succeed())
			Expect(deployment.Spec.Replicas).Should(Equal(&replcias))

			By("check the deployment is not ready")
			result, err = deploymentReconciler.Ready(ctx)
			Expect(result).ShouldNot(BeNil())
			Expect(err).Should(BeNil())
			Expect(result.IsZero()).Should(BeFalse())
			Expect(result.Requeue).Should(BeTrue())

			// Because the envtest does not handle the pod, we need to mock that the statefulset is ready
			// Mock that the deployment is ready by updating the ready replicas to 3
			By("mock the deployment is ready")
			deployment = &appv1.Deployment{}
			Expect(k8sClient.Get(ctx, ctrlclient.ObjectKey{Namespace: namespace, Name: name}, deployment)).Should(Succeed())
			deployment.Status.Replicas = replcias
			deployment.Status.ReadyReplicas = replcias
			Expect(k8sClient.Status().Update(ctx, deployment)).Should(Succeed())

			By("check the deployment is ready")
			result, err = deploymentReconciler.Ready(ctx)
			Expect(result).ShouldNot(BeNil())
			Expect(err).Should(BeNil())
			Expect(result.IsZero()).Should(BeTrue())

			By("check the container image pull policy of deployment is default value")
			deployment = &appv1.Deployment{}
			Expect(k8sClient.Get(ctx, ctrlclient.ObjectKey{Namespace: namespace, Name: name}, deployment)).Should(Succeed())
			Expect(deployment.Spec.Template.Spec.Containers).ShouldNot(BeNil())
			Expect(deployment.Spec.Template.Spec.Containers).Should(HaveLen(1))
			Expect(deployment.Spec.Template.Spec.Containers[0].ImagePullPolicy).Should(Equal(util.DefaultImagePullPolicy))
		})

		It("Should successfully reconcile deployment replicas to 0 when stopped", func() {

			By("Create a stopped deployment reconciler normal")
			deploymentReconciler := reconciler.NewDeployment(resourceClient, deploymentBuilder, false)
			Expect(deploymentReconciler).ShouldNot(BeNil())

			By("reconcile the deployment")
			result, err := deploymentReconciler.Reconcile(ctx)
			Expect(result).ShouldNot(BeNil())
			Expect(err).Should(BeNil())
			Expect(result.IsZero()).Should(BeFalse())
			Expect(result.Requeue).Should(BeTrue())

			By("checking the deployment spec replicas is valid")
			deployment := &appv1.Deployment{}
			Expect(k8sClient.Get(ctx, ctrlclient.ObjectKey{Namespace: namespace, Name: name}, deployment)).Should(Succeed())
			Expect(*deployment.Spec.Replicas).Should(BeEquivalentTo(int32(3)))

			By("Simulate reconcile again when CR is updated")
			deploymentBuilder = &TrinoCoordinatorDeploymentBuilder{
				Deployment: *builder.NewDeployment(
					resourceClient,
					name,
					&replcias,
					util.NewImage("trino", "458", "1.0.0"),
					nil,
					nil,
				),
			}
			By("create deployment reconciler with stopped is true")
			deploymentReconciler = reconciler.NewDeployment(resourceClient, deploymentBuilder, true)
			Expect(deploymentReconciler).ShouldNot(BeNil())

			By("reconcile the deployment")
			result, err = deploymentReconciler.Reconcile(ctx)
			Expect(result).ShouldNot(BeNil())
			Expect(err).Should(BeNil())
			Expect(result.IsZero()).Should(BeFalse())
			Expect(result.Requeue).Should(BeTrue())

			By("checking the deployment spec replicas is updated to 0")
			deployment = &appv1.Deployment{}
			Expect(k8sClient.Get(ctx, ctrlclient.ObjectKey{Namespace: namespace, Name: name}, deployment)).Should(Succeed())
			Expect(*deployment.Spec.Replicas).Should(BeEquivalentTo(int32(0)))

		})
	})

})
