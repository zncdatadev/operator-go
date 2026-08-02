/*
Copyright 2024 ZNCDataDev.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package reconciler_test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/constant"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/sidecar"
	"github.com/zncdatadev/operator-go/pkg/testutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("EnsureGeneratedSecret", func() {
	const namespace = "default"

	var (
		crName     string
		secretName string
		mockCR     *testutil.MockCluster
		getSecret  func() *corev1.Secret
	)

	BeforeEach(func() {
		stamp := time.Now().UnixNano()
		crName = fmt.Sprintf("gensecret-cr-%d", stamp)
		secretName = fmt.Sprintf("gensecret-%d", stamp)
		mockCR = testutil.NewMockCluster(crName, namespace)
		Expect(k8sClient.Create(ctx, mockCR)).To(Succeed())

		getSecret = func() *corev1.Secret {
			s := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: secretName}, s)).To(Succeed())
			return s
		}
		DeferCleanup(func() {
			// envtest runs no GC controller, so the owned Secret is removed explicitly.
			_ = k8sClient.Delete(ctx, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: namespace}})
			_ = k8sClient.Delete(ctx, mockCR)
		})
	})

	// fixedValue, not "constant": that name would shadow the constant package below.
	fixedValue := func(v string) func() (string, error) {
		return func() (string, error) { return v, nil }
	}

	It("creates the secret with the generated values", func() {
		secret, err := reconciler.EnsureGeneratedSecret(ctx, k8sClient, testScheme, mockCR, secretName,
			map[string]func() (string, error){"cookie-secret": fixedValue("first-value")})
		Expect(err).NotTo(HaveOccurred())
		Expect(secret.Data).To(HaveKeyWithValue("cookie-secret", []byte("first-value")))
		Expect(getSecret().Data).To(HaveKeyWithValue("cookie-secret", []byte("first-value")))
		Expect(getSecret().Type).To(Equal(corev1.SecretTypeOpaque))
	})

	It("never rewrites a value that already exists", func() {
		// The whole reason this helper exists. Everything else the SDK applies converges on a
		// desired state; here convergence IS the failure — a fresh oauth2-proxy cookie secret on
		// every pass rolls the pods and logs every user out.
		_, err := reconciler.EnsureGeneratedSecret(ctx, k8sClient, testScheme, mockCR, secretName,
			map[string]func() (string, error){"cookie-secret": fixedValue("first-value")})
		Expect(err).NotTo(HaveOccurred())

		for range 3 {
			_, err := reconciler.EnsureGeneratedSecret(ctx, k8sClient, testScheme, mockCR, secretName,
				map[string]func() (string, error){"cookie-secret": fixedValue("SECOND-value")})
			Expect(err).NotTo(HaveOccurred())
		}

		Expect(getSecret().Data).To(HaveKeyWithValue("cookie-secret", []byte("first-value")))
	})

	It("does not call a generator for a key that already exists", func() {
		// Steady state must be side-effect free: a generator may reach an external KMS, and
		// "never rewrites" would be hollow if the value were produced and then discarded.
		var calls atomic.Int32
		// Erroring on the second call rather than just counting: the assertion then rides on the
		// helper's own error path, so "was not called" cannot pass by an off-by-one in the test.
		counting := func() (string, error) {
			if n := calls.Add(1); n > 1 {
				return "", fmt.Errorf("generator called %d times; it must run only for an absent key", n)
			}
			return "value", nil
		}

		_, err := reconciler.EnsureGeneratedSecret(ctx, k8sClient, testScheme, mockCR, secretName,
			map[string]func() (string, error){"cookie-secret": counting})
		Expect(err).NotTo(HaveOccurred())
		Expect(calls.Load()).To(Equal(int32(1)))

		_, err = reconciler.EnsureGeneratedSecret(ctx, k8sClient, testScheme, mockCR, secretName,
			map[string]func() (string, error){"cookie-secret": counting})
		Expect(err).NotTo(HaveOccurred(), "a second call to the generator would have failed this")
		Expect(calls.Load()).To(Equal(int32(1)), "the second pass must not generate anything")
	})

	It("fills a key that went missing, leaving its siblings alone", func() {
		// A partial restore from backup, or a hand-edit. Providers fail the reconcile on a
		// missing key, so without this the only recovery is deleting the whole Secret — which
		// rotates every OTHER key too, logging out every user to fix one.
		_, err := reconciler.EnsureGeneratedSecret(ctx, k8sClient, testScheme, mockCR, secretName,
			map[string]func() (string, error){
				"cookie-secret": fixedValue("keep-me"),
				"client-secret": fixedValue("original"),
			})
		Expect(err).NotTo(HaveOccurred())

		live := getSecret()
		delete(live.Data, "client-secret")
		Expect(k8sClient.Update(ctx, live)).To(Succeed())

		_, err = reconciler.EnsureGeneratedSecret(ctx, k8sClient, testScheme, mockCR, secretName,
			map[string]func() (string, error){
				"cookie-secret": fixedValue("SHOULD-NOT-APPEAR"),
				"client-secret": fixedValue("restored"),
			})
		Expect(err).NotTo(HaveOccurred())

		data := getSecret().Data
		Expect(data).To(HaveKeyWithValue("client-secret", []byte("restored")), "the lost key is filled")
		Expect(data).To(HaveKeyWithValue("cookie-secret", []byte("keep-me")), "the surviving key is untouched")
	})

	It("sets a controller owner reference so the secret is collected with the CR", func() {
		_, err := reconciler.EnsureGeneratedSecret(ctx, k8sClient, testScheme, mockCR, secretName,
			map[string]func() (string, error){"cookie-secret": fixedValue("v")})
		Expect(err).NotTo(HaveOccurred())

		refs := getSecret().OwnerReferences
		Expect(refs).To(HaveLen(1))
		Expect(refs[0].Name).To(Equal(crName))
		Expect(refs[0].Controller).To(HaveValue(BeTrue()))
	})

	It("applies the canonical labels over any extra ones", func() {
		_, err := reconciler.EnsureGeneratedSecret(ctx, k8sClient, testScheme, mockCR, secretName,
			map[string]func() (string, error){"cookie-secret": fixedValue("v")},
			reconciler.WithGeneratedSecretProductName("trino"),
			reconciler.WithGeneratedSecretExtraLabels(map[string]string{
				"team":                            "data",
				constant.LabelKubernetesManagedBy: "somebody-else",
			}),
		)
		Expect(err).NotTo(HaveOccurred())

		labels := getSecret().Labels
		Expect(labels).To(HaveKeyWithValue("team", "data"))
		Expect(labels).To(HaveKeyWithValue(constant.LabelKubernetesName, "trino"))
		Expect(labels).To(HaveKeyWithValue(constant.LabelKubernetesInstance, crName))
		Expect(labels).To(HaveKeyWithValue(constant.LabelKubernetesManagedBy, "operator-go"),
			"a framework-owned label must not be overridable")
	})

	It("accumulates repeated label and annotation options", func() {
		// The sibling WithDiscoveryExtraLabels merges, and these two helpers sit side by side with
		// parallel option names — replacing here would silently drop the earlier call's keys.
		_, err := reconciler.EnsureGeneratedSecret(ctx, k8sClient, testScheme, mockCR, secretName,
			map[string]func() (string, error){"cookie-secret": fixedValue("v")},
			reconciler.WithGeneratedSecretExtraLabels(map[string]string{"team": "data"}),
			reconciler.WithGeneratedSecretExtraLabels(map[string]string{"tier": "backend"}),
			reconciler.WithGeneratedSecretAnnotations(map[string]string{"note": "first"}),
			reconciler.WithGeneratedSecretAnnotations(map[string]string{"owner": "platform"}),
		)
		Expect(err).NotTo(HaveOccurred())

		live := getSecret()
		Expect(live.Labels).To(HaveKeyWithValue("team", "data"))
		Expect(live.Labels).To(HaveKeyWithValue("tier", "backend"))
		Expect(live.Annotations).To(HaveKeyWithValue("note", "first"))
		Expect(live.Annotations).To(HaveKeyWithValue("owner", "platform"))
	})

	It("does not re-set the immutable type on an existing secret", func() {
		_, err := reconciler.EnsureGeneratedSecret(ctx, k8sClient, testScheme, mockCR, secretName,
			map[string]func() (string, error){"cookie-secret": fixedValue("v")},
			reconciler.WithGeneratedSecretType(corev1.SecretTypeOpaque))
		Expect(err).NotTo(HaveOccurred())

		// A caller that changes its mind must not wedge every later reconcile: `type` is
		// immutable, so writing it on update would make the API server reject the pass forever.
		_, err = reconciler.EnsureGeneratedSecret(ctx, k8sClient, testScheme, mockCR, secretName,
			map[string]func() (string, error){"cookie-secret": fixedValue("v")},
			reconciler.WithGeneratedSecretType(corev1.SecretTypeBasicAuth))
		Expect(err).NotTo(HaveOccurred())
		Expect(getSecret().Type).To(Equal(corev1.SecretTypeOpaque))
	})

	It("reports which key failed to generate", func() {
		_, err := reconciler.EnsureGeneratedSecret(ctx, k8sClient, testScheme, mockCR, secretName,
			map[string]func() (string, error){
				"cookie-secret": func() (string, error) { return "", fmt.Errorf("kms unavailable") },
			})
		Expect(err).To(MatchError(ContainSubstring("cookie-secret")))
		Expect(err).To(MatchError(ContainSubstring("kms unavailable")))
	})

	It("settles on one value under concurrent ensures", func() {
		// Two reconcilers racing to create it must not leave two different values behind — the
		// AlreadyExists loser re-reads rather than failing, so whoever won is what everyone uses.
		const workers = 4
		values := make([]string, workers)
		var wg sync.WaitGroup
		for i := range workers {
			wg.Add(1)
			go func() {
				defer GinkgoRecover()
				defer wg.Done()
				secret, err := reconciler.EnsureGeneratedSecret(ctx, k8sClient, testScheme, mockCR, secretName,
					map[string]func() (string, error){"cookie-secret": fixedValue(fmt.Sprintf("worker-%d", i))})
				Expect(err).NotTo(HaveOccurred())
				values[i] = string(secret.Data["cookie-secret"])
			}()
		}
		wg.Wait()

		stored := string(getSecret().Data["cookie-secret"])
		for i := range workers {
			Expect(values[i]).To(Equal(stored), "every caller must observe the one stored value")
		}
	})

	It("works with the cookie secret generator the framework ships", func() {
		// The shipped case end to end: GenerateCookieSecret's doc says call it once and store the
		// result, and this is the "once".
		_, err := reconciler.EnsureGeneratedSecret(ctx, k8sClient, testScheme, mockCR, secretName,
			map[string]func() (string, error){"cookie-secret": sidecar.GenerateCookieSecret})
		Expect(err).NotTo(HaveOccurred())

		first := getSecret().Data["cookie-secret"]
		Expect(first).NotTo(BeEmpty())

		_, err = reconciler.EnsureGeneratedSecret(ctx, k8sClient, testScheme, mockCR, secretName,
			map[string]func() (string, error){"cookie-secret": sidecar.GenerateCookieSecret})
		Expect(err).NotTo(HaveOccurred())
		Expect(getSecret().Data["cookie-secret"]).To(Equal(first),
			"a second pass must not rotate the session key")
	})

	It("rejects the arguments it cannot act on", func() {
		_, err := reconciler.EnsureGeneratedSecret(ctx, k8sClient, testScheme, mockCR, secretName, nil)
		Expect(err).To(MatchError(ContainSubstring("at least one key generator")))

		_, err = reconciler.EnsureGeneratedSecret(ctx, k8sClient, testScheme, mockCR, "",
			map[string]func() (string, error){"k": fixedValue("v")})
		Expect(err).To(MatchError(ContainSubstring("name must not be empty")))
	})
})
