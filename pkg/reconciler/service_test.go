package reconciler_test

import (
	"context"
	"math/rand"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Service reconciler", func() {

	Context("ServiceReconciler test", func() {
		var resourceClient *client.Client
		const name = "whoami"
		var namespace string
		ctx := context.Background()

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

		It("should do something", func() {
			serviceReconciler := reconciler.NewServiceReconciler(
				resourceClient,
				name,
				map[string]string{"app.kubernetes.io/name": name},
				map[string]string{"app.kubernetes.io/name": name},
				[]corev1.ContainerPort{
					{
						Name:          "http",
						ContainerPort: 80,
						Protocol:      corev1.ProtocolTCP,
					},
				},
				nil,
			)
			Expect(serviceReconciler).ShouldNot(BeNil())

			result, err := serviceReconciler.Reconcile(ctx)
			Expect(result).ShouldNot(BeNil())
			Expect(err).Should(BeNil())
			Expect(result.Requeue).Should(BeTrue())

			result, err = serviceReconciler.Ready(ctx)
			Expect(result).ShouldNot(BeNil())
			Expect(err).Should(BeNil())
			Expect(result.IsZero()).Should(BeTrue())

		})
	})
})
