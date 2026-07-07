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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/testutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("EnsureDiscoveryConfigMap", func() {
	const namespace = "default"

	var (
		crName  string
		mockCR  *testutil.MockCluster
		owner   *testutil.ClusterWrapper
		getCM   func(name string) *corev1.ConfigMap
		cleanup []string
	)

	BeforeEach(func() {
		crName = fmt.Sprintf("discovery-cr-%d", time.Now().UnixNano())
		mockCR = testutil.NewMockCluster(crName, namespace)
		Expect(k8sClient.Create(ctx, mockCR)).To(Succeed())
		owner = testutil.WrapMockCluster(mockCR, testScheme)
		cleanup = nil

		getCM = func(name string) *corev1.ConfigMap {
			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, cm)).To(Succeed())
			return cm
		}
	})

	AfterEach(func() {
		// envtest has no GC controller, so owned ConfigMaps are deleted explicitly.
		for _, name := range cleanup {
			cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
			_ = k8sClient.Delete(ctx, cm)
		}
		_ = k8sClient.Delete(ctx, mockCR)
	})

	ensure := func(name string, data map[string]string, opts ...reconciler.DiscoveryConfigMapOption) error {
		cleanup = append(cleanup, name)
		return reconciler.EnsureDiscoveryConfigMap(ctx, k8sClient, testScheme, owner, name, data, opts...)
	}

	It("creates the ConfigMap with a controller owner reference and canonical labels", func() {
		Expect(ensure(crName, map[string]string{"KAFKA": "broker-0:9092"},
			reconciler.WithDiscoveryProductName("kafka"),
		)).To(Succeed())

		cm := getCM(crName)
		Expect(cm.Data).To(Equal(map[string]string{"KAFKA": "broker-0:9092"}))
		Expect(cm.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", "kafka"))
		Expect(cm.Labels).To(HaveKeyWithValue("app.kubernetes.io/instance", crName))
		Expect(cm.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "operator-go"))

		// Controller owner reference -> garbage-collected with the CR.
		Expect(cm).To(testutil.HaveOwnerReference(crName, "MockCluster"))
		controllerRef := metav1.GetControllerOf(cm)
		Expect(controllerRef).NotTo(BeNil())
		fetchedCR := &testutil.MockCluster{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: crName}, fetchedCR)).To(Succeed())
		Expect(controllerRef.UID).To(Equal(fetchedCR.UID))
	})

	It("updates the ConfigMap in place when the data changes", func() {
		Expect(ensure(crName, map[string]string{"KAFKA": "broker-0:9092", "STALE": "x"})).To(Succeed())
		created := getCM(crName)

		Expect(ensure(crName, map[string]string{"KAFKA": "broker-0:9092,broker-1:9092"})).To(Succeed())

		updated := getCM(crName)
		// Same object (updated, not recreated), data replaced wholesale: stale keys pruned.
		Expect(updated.UID).To(Equal(created.UID))
		Expect(updated.Data).To(Equal(map[string]string{"KAFKA": "broker-0:9092,broker-1:9092"}))
	})

	It("is idempotent when nothing changes", func() {
		data := map[string]string{"ZOOKEEPER": "zk-0:2181/"}
		Expect(ensure(crName, data)).To(Succeed())
		created := getCM(crName)

		Expect(ensure(crName, data)).To(Succeed())

		unchanged := getCM(crName)
		Expect(unchanged.UID).To(Equal(created.UID))
		Expect(unchanged.ResourceVersion).To(Equal(created.ResourceVersion))
	})

	It("merges extra labels but keeps the canonical labels authoritative", func() {
		Expect(ensure(crName, map[string]string{"KAFKA": "broker-0:9092"},
			reconciler.WithDiscoveryProductName("kafka"),
			reconciler.WithDiscoveryExtraLabels(map[string]string{
				reconciler.ClusterLabelKey("kafka.kubedoop.dev"): crName,
				"app.kubernetes.io/instance":                     "spoofed",
			}),
		)).To(Succeed())

		cm := getCM(crName)
		// Product identity label merged.
		Expect(cm.Labels).To(HaveKeyWithValue("kafka.kubedoop.dev/cluster", crName))
		// Canonical label wins over the conflicting extra label.
		Expect(cm.Labels).To(HaveKeyWithValue("app.kubernetes.io/instance", crName))
	})

	It("merges annotations without pruning foreign keys", func() {
		Expect(ensure(crName, map[string]string{"KAFKA": "broker-0:9092"},
			reconciler.WithDiscoveryAnnotations(map[string]string{"kubedoop.dev/note": "v1"}),
		)).To(Succeed())

		// A foreign controller adds its own annotation.
		cm := getCM(crName)
		cm.Annotations["other.io/keep"] = "yes"
		Expect(k8sClient.Update(ctx, cm)).To(Succeed())

		Expect(ensure(crName, map[string]string{"KAFKA": "broker-0:9092"},
			reconciler.WithDiscoveryAnnotations(map[string]string{"kubedoop.dev/note": "v2"}),
		)).To(Succeed())

		cm = getCM(crName)
		Expect(cm.Annotations).To(HaveKeyWithValue("kubedoop.dev/note", "v2"))
		Expect(cm.Annotations).To(HaveKeyWithValue("other.io/keep", "yes"))
	})

	It("supports suffixed ConfigMaps for the same owner (e.g. nodeport variant)", func() {
		for _, name := range []string{crName, crName + "-nodeport"} {
			Expect(ensure(name, map[string]string{"KAFKA": "node-0:31234"})).To(Succeed())
		}

		for _, name := range []string{crName, crName + "-nodeport"} {
			cm := getCM(name)
			Expect(cm).To(testutil.HaveOwnerReference(crName, "MockCluster"))
			Expect(cm.Labels).To(HaveKeyWithValue("app.kubernetes.io/instance", crName))
		}
	})

	It("rejects an empty ConfigMap name", func() {
		err := reconciler.EnsureDiscoveryConfigMap(ctx, k8sClient, testScheme, owner, "", nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("name must not be empty"))
	})
})
