// Test statefulset reconciler
package reconciler_test

import (
	"context"
	"math/rand"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/util"
)

var _ builder.StatefulSetBuilder = &FooStatefulSetBuilder{}

type FooStatefulSetBuilder struct {
	builder.StatefulSet
}

func (b *FooStatefulSetBuilder) Build(ctx context.Context) (ctrlclient.Object, error) {
	containerBuilder := builder.NewContainer(
		"foo",
		"nginx",
	)

	b.AddContainer(containerBuilder.Build())

	return b.GetObject()
}

var _ = Describe("Statefulset reconciler", func() {

	Context("StatefulsetReconciler test", func() {

		var resourceClient *client.Client
		var statefulSetBuilder builder.StatefulSetBuilder
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

			statefulSetBuilder = &FooStatefulSetBuilder{
				StatefulSet: *builder.NewStatefulSetBuilder(
					resourceClient,
					name,
					&replcias,
					&util.Image{
						StackVersion:   "1.0.0",
						ProductVersion: "458",
						ProductName:    "nginx",
					},
					builder.WorkloadOptions{},
				),
			}
		})

		It("Should successfully reconcile a whoami statefulset", func() {
			By("Create a statefulset reconciler")
			statusfulSetReconciler := reconciler.NewStatefulSet(resourceClient, name, statefulSetBuilder, false)
			Expect(statusfulSetReconciler).ShouldNot(BeNil())

			By("Reconcile the statefulset")
			result := statusfulSetReconciler.Reconcile(ctx)
			Expect(result).ShouldNot(BeNil())
			Expect(result.Error).Should(BeNil())
			Expect(result.RequeueOrNot()).Should(BeTrue())

			By("Checking the statefulset spec.replicas is valid")
			statefulSet := &appv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, ctrlclient.ObjectKey{Namespace: namespace, Name: name}, statefulSet)).Should(Succeed())
			Expect(*statefulSet.Spec.Replicas).Should(Equal(replcias))

			By("Check the statefulset is ready or not")
			result = statusfulSetReconciler.Ready(ctx)
			Expect(result).ShouldNot(BeNil())
			Expect(result.Error).Should(BeNil())
			Expect(result.RequeueOrNot()).Should(BeTrue())

			// Because of the envtest do not handle the pod, we need to mock the statefulset is ready
			// mock the statefulset is ready, update the ready replicas to 3
			By("Mock the statefulset is ready")
			statefulSet = &appv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, ctrlclient.ObjectKey{Namespace: namespace, Name: name}, statefulSet)).Should(Succeed())
			statefulSet.Status.Replicas = replcias
			statefulSet.Status.ReadyReplicas = replcias
			Expect(k8sClient.Status().Update(ctx, statefulSet)).Should(Succeed())

			By("Check the statefulset is ready or not")
			result = statusfulSetReconciler.Ready(ctx)
			Expect(result).ShouldNot(BeNil())
			Expect(result.Error).Should(BeNil())
			Expect(result.RequeueOrNot()).Should(BeFalse())

		})

		It("Should successfully reconcile a stopped whoami statefulset", func() {

			By("Create a stopped statefulset reconciler")
			statusfulSetReconciler := reconciler.NewStatefulSet(resourceClient, name, statefulSetBuilder, false)
			Expect(statusfulSetReconciler).ShouldNot(BeNil())

			By("Reconcile the statefulset")
			result := statusfulSetReconciler.Reconcile(ctx)
			Expect(result).ShouldNot(BeNil())
			Expect(result.Error).Should(BeNil())
			Expect(result.RequeueOrNot()).Should(BeTrue())

			By("Checking the statefulset spec replicas is valid")
			statefulSet := &appv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, ctrlclient.ObjectKey{Namespace: namespace, Name: name}, statefulSet)).Should(Succeed())
			Expect(*statefulSet.Spec.Replicas).Should(BeEquivalentTo(int32(3)))

			By("Simulate reconcile again when CR is updated")
			statefulSetBuilder = &FooStatefulSetBuilder{
				StatefulSet: *builder.NewStatefulSetBuilder(
					resourceClient,
					name,
					&replcias,
					&util.Image{
						StackVersion:   "1.0.0",
						ProductVersion: "458",
						ProductName:    "nginx",
					},
					builder.WorkloadOptions{},
				),
			}

			By("Update the statefulset spec replicas when cluster is stopped")
			statusfulSetReconciler = reconciler.NewStatefulSet(resourceClient, name, statefulSetBuilder, true)
			Expect(statusfulSetReconciler).ShouldNot(BeNil())

			By("Reconcile the statefulset")
			result = statusfulSetReconciler.Reconcile(ctx)
			Expect(result).ShouldNot(BeNil())
			Expect(result.Error).Should(BeNil())
			Expect(result.RequeueOrNot()).Should(BeTrue()) // When the cluster is stopped, the statefulset should not be reconciled

			By("Checking the statefulset spec replicas is updated")
			statefulSet = &appv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, ctrlclient.ObjectKey{Namespace: namespace, Name: name}, statefulSet)).Should(Succeed())
			Expect(*statefulSet.Spec.Replicas).Should(BeEquivalentTo(int32(0)))
		})
	})

})
