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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/testutil"
	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("Builder", func() {
	const (
		testName      = "test-resource"
		testNamespace = "test-namespace"
	)

	Context("NewTestConfigMap", func() {
		It("should create a test ConfigMap", func() {
			cm := testutil.NewTestConfigMap(testName, testNamespace)
			Expect(cm).NotTo(BeNil())
			Expect(cm.Name).To(Equal(testName))
			Expect(cm.Namespace).To(Equal(testNamespace))
			Expect(cm.Data).To(HaveKey("test-key"))
			Expect(cm.Data["test-key"]).To(Equal("test-value"))
			Expect(cm.Labels).To(HaveKey("app.kubernetes.io/name"))
		})
	})

	Context("NewTestConfigMapWithData", func() {
		It("should create a ConfigMap with custom data", func() {
			data := map[string]string{
				"custom-key1": "value1",
				"custom-key2": "value2",
			}
			cm := testutil.NewTestConfigMapWithData(testName, testNamespace, data)
			Expect(cm).NotTo(BeNil())
			Expect(cm.Name).To(Equal(testName))
			Expect(cm.Namespace).To(Equal(testNamespace))
			Expect(cm.Data).To(Equal(data))
		})
	})

	Context("NewTestService", func() {
		It("should create a test Service", func() {
			svc := testutil.NewTestService(testName, testNamespace)
			Expect(svc).NotTo(BeNil())
			Expect(svc.Name).To(Equal(testName))
			Expect(svc.Namespace).To(Equal(testNamespace))
			Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
			Expect(svc.Spec.Ports).ToNot(BeEmpty())
			Expect(svc.Spec.Ports[0].Name).To(Equal("http"))
			Expect(svc.Spec.Ports[0].Port).To(Equal(int32(8080)))
			Expect(svc.Labels).To(HaveKey("app.kubernetes.io/name"))
		})
	})

	Context("NewTestHeadlessService", func() {
		It("should create a headless Service", func() {
			svc := testutil.NewTestHeadlessService(testName, testNamespace)
			Expect(svc).NotTo(BeNil())
			Expect(svc.Name).To(Equal(testName + "-headless"))
			Expect(svc.Namespace).To(Equal(testNamespace))
			Expect(svc.Spec.ClusterIP).To(Equal("None"))
			Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
		})
	})

	Context("NewTestServiceWithPorts", func() {
		It("should create a Service with custom ports", func() {
			ports := []corev1.ServicePort{
				{
					Name: "http",
					Port: 80,
				},
				{
					Name: "https",
					Port: 443,
				},
			}
			svc := testutil.NewTestServiceWithPorts(testName, testNamespace, ports)
			Expect(svc).NotTo(BeNil())
			Expect(svc.Spec.Ports).To(HaveLen(2))
			Expect(svc.Spec.Ports[0].Port).To(Equal(int32(80)))
			Expect(svc.Spec.Ports[1].Port).To(Equal(int32(443)))
		})
	})

	Context("NewTestStatefulSet", func() {
		It("should create a test StatefulSet", func() {
			sts := testutil.NewTestStatefulSet(testName, testNamespace)
			Expect(sts).NotTo(BeNil())
			Expect(sts.Name).To(Equal(testName))
			Expect(sts.Namespace).To(Equal(testNamespace))
			Expect(*sts.Spec.Replicas).To(Equal(int32(1)))
			Expect(sts.Spec.ServiceName).To(Equal(testName + "-headless"))
			Expect(sts.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(sts.Spec.Template.Spec.Containers[0].Name).To(Equal(testName))
			Expect(sts.Spec.Template.Spec.Containers[0].Image).To(Equal("test-image:latest"))
		})
	})

	Context("NewTestStatefulSetBuilder", func() {
		It("should create a StatefulSetBuilder", func() {
			builder := testutil.NewTestStatefulSetBuilder(testName, testNamespace)
			Expect(builder).NotTo(BeNil())

			sts := builder.Build()
			Expect(sts).NotTo(BeNil())
			Expect(sts.Name).To(Equal(testName))
			Expect(sts.Namespace).To(Equal(testNamespace))
		})
	})

	Context("NewTestServiceBuilder", func() {
		It("should create a ServiceBuilder", func() {
			builder := testutil.NewTestServiceBuilder(testName, testNamespace)
			Expect(builder).NotTo(BeNil())

			svc := builder.Build()
			Expect(svc).NotTo(BeNil())
			Expect(svc.Name).To(Equal(testName))
			Expect(svc.Namespace).To(Equal(testNamespace))
		})
	})

	Context("NewTestConfigMapBuilder", func() {
		It("should create a ConfigMapBuilder", func() {
			builder := testutil.NewTestConfigMapBuilder(testName, testNamespace)
			Expect(builder).NotTo(BeNil())

			cm := builder.Build()
			Expect(cm).NotTo(BeNil())
			Expect(cm.Name).To(Equal(testName))
			Expect(cm.Namespace).To(Equal(testNamespace))
		})
	})

	Context("NewTestPDB", func() {
		It("should create a test PodDisruptionBudget", func() {
			pdb := testutil.NewTestPDB(testName, testNamespace)
			Expect(pdb).NotTo(BeNil())
			Expect(pdb.Name).To(Equal(testName))
			Expect(pdb.Namespace).To(Equal(testNamespace))
			Expect(pdb.Spec.MaxUnavailable.IntVal).To(Equal(int32(1)))
			Expect(pdb.Labels).To(HaveKey("app.kubernetes.io/name"))
		})
	})

	Context("NewTestPDBWithMaxUnavailable", func() {
		It("should create a PDB with maxUnavailable", func() {
			pdb := testutil.NewTestPDBWithMaxUnavailable(testName, testNamespace, 2)
			Expect(pdb).NotTo(BeNil())
			Expect(pdb.Spec.MaxUnavailable.IntVal).To(Equal(int32(2)))
		})
	})

	Context("NewTestMergedConfig", func() {
		It("should create a test MergedConfig", func() {
			cfg := testutil.NewTestMergedConfig()
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.EnvVars).To(HaveKey("TEST_VAR"))
			Expect(cfg.EnvVars["TEST_VAR"]).To(Equal("test-value"))
			Expect(cfg.CliArgs).ToNot(BeEmpty())
		})
	})

	Context("NewTestMergedConfigWithEnv", func() {
		It("should create a MergedConfig with custom env vars", func() {
			envVars := map[string]string{
				"CUSTOM_VAR1": "value1",
				"CUSTOM_VAR2": "value2",
			}
			cfg := testutil.NewTestMergedConfigWithEnv(envVars)
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.EnvVars["CUSTOM_VAR1"]).To(Equal("value1"))
			Expect(cfg.EnvVars["CUSTOM_VAR2"]).To(Equal("value2"))
			// Original TEST_VAR should still exist
			Expect(cfg.EnvVars["TEST_VAR"]).To(Equal("test-value"))
		})
	})

	Context("NewTestRoleGroupSpec", func() {
		It("should create a test RoleGroupSpec", func() {
			replicas := int32(3)
			spec := testutil.NewTestRoleGroupSpec(replicas)
			Expect(spec).NotTo(BeNil())
			Expect(*spec.Replicas).To(Equal(replicas))
		})
	})

	Context("NewTestNamespace", func() {
		It("should create a test Namespace", func() {
			ns := testutil.NewTestNamespace(testName)
			Expect(ns).NotTo(BeNil())
			Expect(ns.Name).To(Equal(testName))
			Expect(ns.Labels).To(HaveKey("app.kubernetes.io/managed-by"))
			Expect(ns.Labels["app.kubernetes.io/managed-by"]).To(Equal("test"))
		})
	})

	Context("NewTestPod", func() {
		It("should create a test Pod", func() {
			pod := testutil.NewTestPod(testName, testNamespace)
			Expect(pod).NotTo(BeNil())
			Expect(pod.Name).To(Equal(testName))
			Expect(pod.Namespace).To(Equal(testNamespace))
			Expect(pod.Spec.Containers).To(HaveLen(1))
			Expect(pod.Spec.Containers[0].Name).To(Equal(testName))
			Expect(pod.Spec.Containers[0].Image).To(Equal("test-image:latest"))
		})
	})

	Context("NewTestSecret", func() {
		It("should create a test Secret", func() {
			secret := testutil.NewTestSecret(testName, testNamespace)
			Expect(secret).NotTo(BeNil())
			Expect(secret.Name).To(Equal(testName))
			Expect(secret.Namespace).To(Equal(testNamespace))
			Expect(secret.Data).To(HaveKey("username"))
			Expect(secret.Data).To(HaveKey("password"))
			Expect(string(secret.Data["username"])).To(Equal("test-user"))
			Expect(string(secret.Data["password"])).To(Equal("test-password"))
		})
	})
})
