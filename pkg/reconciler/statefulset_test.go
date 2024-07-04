package reconciler_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
)

var _ builder.StatefulSetBuilder = &FooStatefulSetBuilder{}

type FooStatefulSetBuilder struct {
	builder.StatefulSet
}

func (b *FooStatefulSetBuilder) Build(ctx context.Context) (ctrlclient.Object, error) {
	containerBuilder := builder.NewContainerBuilder(
		"foo",
		"nginx",
		corev1.PullIfNotPresent,
	)

	b.AddContainer(containerBuilder.Build())

	return b.GetObject()
}

var _ = Describe("Statefulset", func() {

	Context("StatefulsetReconciler test", func() {

		var resourceClient *client.Client
		var StatefulSetBuilder builder.StatefulSetBuilder
		const CRName = "default"
		ctx := context.Background()

		BeforeEach(func() {

			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      CRName,
					Namespace: CRName,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  CRName,
							Image: "nginx",
						},
					},
				},
			}

			// Create a pod
			Expect(k8sClient.Create(ctx, pod)).Should(Succeed())

			pod = &corev1.Pod{}

			// Get the pod
			Expect(k8sClient.Get(ctx, ctrlclient.ObjectKey{Namespace: CRName, Name: CRName}, pod)).Should(Succeed())

			resourceClient = &client.Client{
				Client: k8sClient,
				// Fake owner reference
				// In tests, we do not need to prepare CRD
				OwnerReference: pod,
			}

			StatefulSetBuilder = &FooStatefulSetBuilder{
				StatefulSet: *builder.NewStatefulSetBuilder(
					resourceClient,
					CRName,
					map[string]string{
						"app.kubernetes.io/instance": CRName,
					},
					nil,
					nil,
					nil,
					nil,
					nil,
				),
			}
		})

		It("Should successfully reconcile a whoami statefulset", func() {
			statusfulSetReconciler := reconciler.NewStatefulSet(
				resourceClient,
				&reconciler.ResourceReconcilerOptions{
					Name: CRName,
				},
				StatefulSetBuilder,
			)

			Expect(statusfulSetReconciler).ShouldNot(BeNil())

			result := statusfulSetReconciler.Reconcile(ctx)
			Expect(result).ShouldNot(BeNil())

			Expect(result.Error).Should(BeNil())

			result = statusfulSetReconciler.Ready(ctx)

			Expect(result).ShouldNot(BeNil())
			Expect(result.Error).Should(BeNil())

		})
	})

})
