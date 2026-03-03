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

package testutil_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/testutil"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("TestEnv", func() {
	Context("NewTestEnv", func() {
		It("should create a new TestEnv with default config", func() {
			env := testutil.NewTestEnv(nil)
			Expect(env).NotTo(BeNil())
			Expect(env.Env).NotTo(BeNil())
			Expect(env.Scheme).NotTo(BeNil())
			Expect(env.Ctx).NotTo(BeNil())
			Expect(env.Cancel).NotTo(BeNil())
			Expect(env.IsStarted()).To(BeFalse())
		})

		It("should create a new TestEnv with custom config", func() {
			cfg := &testutil.TestEnvConfig{
				ErrorIfCRDPathMissing: true,
			}
			env := testutil.NewTestEnv(cfg)
			Expect(env).NotTo(BeNil())
			Expect(env.Env).NotTo(BeNil())
			Expect(env.Env.ErrorIfCRDPathMissing).To(BeTrue())
		})

		It("should create a new TestEnv with custom CRD paths", func() {
			cfg := &testutil.TestEnvConfig{
				CRDDirectoryPaths: []string{"/tmp/crds"},
			}
			env := testutil.NewTestEnv(cfg)
			Expect(env).NotTo(BeNil())
			Expect(env.Env.CRDDirectoryPaths).To(HaveLen(1))
		})
	})

	Context("GetMethods", func() {
		It("should return the client", func() {
			client := testEnv.GetClient()
			Expect(client).NotTo(BeNil())
		})

		It("should return the scheme", func() {
			scheme := testEnv.GetScheme()
			Expect(scheme).NotTo(BeNil())
		})

		It("should return the config", func() {
			cfg := testEnv.GetConfig()
			Expect(cfg).NotTo(BeNil())
		})

		It("should return the context", func() {
			ctx := testEnv.GetContext()
			Expect(ctx).NotTo(BeNil())
		})

		It("should return started status", func() {
			started := testEnv.IsStarted()
			Expect(started).To(BeTrue())
		})
	})

	Context("NamespaceOperations", func() {
		It("should create a namespace", func() {
			ns, err := testEnv.CreateNamespace("test-namespace-create")
			Expect(err).ToNot(HaveOccurred())
			Expect(ns).NotTo(BeNil())
			Expect(ns.Name).To(Equal("test-namespace-create"))
		})

		It("should delete a namespace", func() {
			_, err := testEnv.CreateNamespace("test-namespace-delete")
			Expect(err).ToNot(HaveOccurred())

			err = testEnv.DeleteNamespace("test-namespace-delete")
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return error when deleting non-existent namespace", func() {
			err := testEnv.DeleteNamespace("non-existent-namespace")
			Expect(err).To(HaveOccurred())
		})
	})

	Context("Start", func() {
		It("should return nil when already started", func() {
			// testEnv is already started in BeforeSuite
			err := testEnv.Start()
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("Stop", func() {
		It("should return nil when not started", func() {
			env := testutil.NewTestEnv(nil)
			err := env.Stop()
			Expect(err).ToNot(HaveOccurred())
		})
	})
})

var _ = Describe("FakeClient", func() {
	Context("NewFakeClient", func() {
		It("should create a fake client", func() {
			client := testutil.NewFakeClient()
			Expect(client).NotTo(BeNil())
		})
	})

	Context("NewFakeClientWithObjects", func() {
		It("should create a fake client with objects", func() {
			cm := testutil.NewTestConfigMap("test-cm", "default")
			client := testutil.NewFakeClientWithObjects(cm)
			Expect(client).NotTo(BeNil())

			// Verify the object exists
			var retrieved corev1.ConfigMap
			err := client.Get(context.Background(), types.NamespacedName{Name: "test-cm", Namespace: "default"}, &retrieved)
			Expect(err).ToNot(HaveOccurred())
			Expect(retrieved.Name).To(Equal("test-cm"))
		})

		It("should create a fake client with multiple objects", func() {
			cm := testutil.NewTestConfigMap("test-cm", "default")
			svc := testutil.NewTestService("test-svc", "default")
			client := testutil.NewFakeClientWithObjects(cm, svc)
			Expect(client).NotTo(BeNil())

			// Verify both objects exist
			var retrievedCM corev1.ConfigMap
			err := client.Get(context.Background(), types.NamespacedName{Name: "test-cm", Namespace: "default"}, &retrievedCM)
			Expect(err).ToNot(HaveOccurred())

			var retrievedSvc corev1.Service
			err = client.Get(context.Background(), types.NamespacedName{Name: "test-svc", Namespace: "default"}, &retrievedSvc)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
