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
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ builder.DeploymentBuilder = &FooDeploymentBuilder{}

type FooDeploymentBuilder struct {
	builder.Deployment
}

func (b *FooDeploymentBuilder) Build(ctx context.Context) (ctrlclient.Object, error) {
	containerBuilder := builder.NewContainerBuilder(
		"foo",
		"nginx",
		corev1.PullIfNotPresent,
	)

	b.AddContainer(containerBuilder.Build())

	return b.GetObject()
}

var _ = Describe("Deloyment reconciler", func() {

	Context("DeploymentReconciler test", func() {
		var resourceClient *client.Client
		var deploymentBuilder *FooDeploymentBuilder
		const name = "whoami"
		var namespace string
		ctx := context.Background()
		replcias := int32(1)

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
			deploymentBuilder = &FooDeploymentBuilder{
				Deployment: *builder.NewDeployment(
					resourceClient,
					name,
					map[string]string{"app.kubernetes.io/instance": name},
					map[string]string{"app.kubernetes.io/instance": name},
					nil,
					nil,
					nil,
					&replcias,
				),
			}
		})

		It("Should successfully reconcile a whoami deployment", func() {

			deploymentReconciler := reconciler.NewDeployment(resourceClient, name, deploymentBuilder)
			Expect(deploymentReconciler).ShouldNot(BeNil())

			result := deploymentReconciler.Reconcile(ctx)
			Expect(result).ShouldNot(BeNil())
			Expect(result.Error).Should(BeNil())
			Expect(result.RequeueOrNot()).Should(BeTrue())

			// Check the deployment is ready
			result = deploymentReconciler.Ready(ctx)
			Expect(result).ShouldNot(BeNil())
			Expect(result.Error).Should(BeNil())
			Expect(result.RequeueOrNot()).Should(BeTrue())

			// Because the envtest does not handle the pod, we need to mock that the statefulset is ready
			// Mock that the deployment is ready by updating the ready replicas to 1
			deployment := &appv1.Deployment{}
			Expect(k8sClient.Get(ctx, ctrlclient.ObjectKey{Namespace: namespace, Name: name}, deployment)).Should(Succeed())
			deployment.Status.Replicas = replcias
			deployment.Status.ReadyReplicas = replcias
			Expect(k8sClient.Status().Update(ctx, deployment)).Should(Succeed())

			result = deploymentReconciler.Ready(ctx)
			Expect(result).ShouldNot(BeNil())
			Expect(result.Error).Should(BeNil())
			Expect(result.RequeueOrNot()).Should(BeFalse())
		})

	})
})
