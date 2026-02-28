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
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/testutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Matchers", func() {
	Context("HaveCondition", func() {
		It("should match when condition exists", func() {
			conditions := []metav1.Condition{
				{
					Type:   "Ready",
					Reason: "ClusterReady",
				},
				{
					Type:   "Available",
					Reason: "ClusterAvailable",
				},
			}
			Expect(conditions).To(testutil.HaveCondition("Ready", "ClusterReady"))
		})

		It("should not match when condition type does not exist", func() {
			conditions := []metav1.Condition{
				{
					Type:   "Ready",
					Reason: "ClusterReady",
				},
			}
			Expect(conditions).NotTo(testutil.HaveCondition("NotReady", "ClusterReady"))
		})

		It("should not match when condition reason does not exist", func() {
			conditions := []metav1.Condition{
				{
					Type:   "Ready",
					Reason: "ClusterReady",
				},
			}
			Expect(conditions).NotTo(testutil.HaveCondition("Ready", "WrongReason"))
		})

		It("should return error for wrong type", func() {
			matcher := testutil.HaveCondition("Ready", "ClusterReady")
			_, err := matcher.Match("not a condition slice")
			Expect(err).NotTo(BeNil())
		})

		It("should return proper failure message", func() {
			matcher := testutil.HaveCondition("Ready", "ClusterReady")
			matcher.Match([]metav1.Condition{})
			msg := matcher.FailureMessage([]metav1.Condition{})
			Expect(msg).To(ContainSubstring("Ready"))
			Expect(msg).To(ContainSubstring("ClusterReady"))
		})

		It("should return proper negated failure message", func() {
			matcher := testutil.HaveCondition("Ready", "ClusterReady")
			msg := matcher.NegatedFailureMessage([]metav1.Condition{})
			Expect(msg).To(ContainSubstring("NOT"))
		})
	})

	Context("HaveOwnerReference", func() {
		It("should match when owner reference exists in slice", func() {
			ownerRefs := []metav1.OwnerReference{
				{
					Name: "test-owner",
					Kind: "MockCluster",
				},
			}
			Expect(ownerRefs).To(testutil.HaveOwnerReference("test-owner", "MockCluster"))
		})

		It("should not match when owner reference does not exist", func() {
			ownerRefs := []metav1.OwnerReference{
				{
					Name: "test-owner",
					Kind: "MockCluster",
				},
			}
			Expect(ownerRefs).NotTo(testutil.HaveOwnerReference("wrong-owner", "MockCluster"))
		})

		It("should match when owner reference exists in runtime.Object", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cm",
					OwnerReferences: []metav1.OwnerReference{
						{
							Name: "test-owner",
							Kind: "MockCluster",
						},
					},
				},
			}
			Expect(cm).To(testutil.HaveOwnerReference("test-owner", "MockCluster"))
		})

		It("should not match when owner reference does not exist in runtime.Object", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-cm",
					OwnerReferences: []metav1.OwnerReference{},
				},
			}
			Expect(cm).NotTo(testutil.HaveOwnerReference("test-owner", "MockCluster"))
		})

		It("should return error for invalid type", func() {
			matcher := testutil.HaveOwnerReference("test-owner", "MockCluster")
			_, err := matcher.Match(123)
			Expect(err).NotTo(BeNil())
		})

		It("should return proper failure message", func() {
			matcher := testutil.HaveOwnerReference("test-owner", "MockCluster")
			matcher.Match([]metav1.OwnerReference{})
			msg := matcher.FailureMessage([]metav1.OwnerReference{})
			Expect(msg).To(ContainSubstring("test-owner"))
			Expect(msg).To(ContainSubstring("MockCluster"))
		})

		It("should return proper negated failure message", func() {
			matcher := testutil.HaveOwnerReference("test-owner", "MockCluster")
			msg := matcher.NegatedFailureMessage([]metav1.OwnerReference{})
			Expect(msg).To(ContainSubstring("NOT"))
			Expect(msg).To(ContainSubstring("test-owner"))
			Expect(msg).To(ContainSubstring("MockCluster"))
		})
	})

	Context("BeCreatedSuccessfully", func() {
		It("should match when error is nil", func() {
			Expect(nil).To(testutil.BeCreatedSuccessfully())
		})

		It("should not match when error is not nil", func() {
			err := errors.New("test error")
			Expect(err).NotTo(testutil.BeCreatedSuccessfully())
		})

		It("should match when actual is nil interface", func() {
			matcher := testutil.BeCreatedSuccessfully()
			match, err := matcher.Match(nil)
			Expect(err).To(BeNil())
			Expect(match).To(BeTrue())
		})

		It("should return proper failure message", func() {
			matcher := testutil.BeCreatedSuccessfully()
			testErr := errors.New("test error")
			msg := matcher.FailureMessage(testErr)
			Expect(msg).To(ContainSubstring("test error"))
		})

		It("should return proper negated failure message", func() {
			matcher := testutil.BeCreatedSuccessfully()
			msg := matcher.NegatedFailureMessage(nil)
			Expect(msg).To(ContainSubstring("fail"))
		})
	})

	Context("HaveName", func() {
		It("should match when names are equal", func() {
			Expect("test-name").To(testutil.HaveName("test-name"))
		})

		It("should not match when names are different", func() {
			Expect("test-name").NotTo(testutil.HaveName("different-name"))
		})

		It("should return error for non-string type", func() {
			matcher := testutil.HaveName("test-name")
			_, err := matcher.Match(123)
			Expect(err).NotTo(BeNil())
		})

		It("should return proper failure message", func() {
			matcher := testutil.HaveName("expected-name")
			matcher.Match("actual-name")
			msg := matcher.FailureMessage("actual-name")
			Expect(msg).To(ContainSubstring("expected-name"))
			Expect(msg).To(ContainSubstring("actual-name"))
		})

		It("should return proper negated failure message", func() {
			matcher := testutil.HaveName("expected-name")
			msg := matcher.NegatedFailureMessage("expected-name")
			Expect(msg).To(ContainSubstring("NOT"))
			Expect(msg).To(ContainSubstring("expected-name"))
		})
	})

	Context("HaveLabels", func() {
		It("should match when all expected labels exist", func() {
			labels := map[string]string{
				"app":         "test",
				"component":   "server",
				"extra-label": "value",
			}
			expected := map[string]string{
				"app":       "test",
				"component": "server",
			}
			Expect(labels).To(testutil.HaveLabels(expected))
		})

		It("should not match when expected label is missing", func() {
			labels := map[string]string{
				"app": "test",
			}
			expected := map[string]string{
				"app":       "test",
				"component": "server",
			}
			Expect(labels).NotTo(testutil.HaveLabels(expected))
		})

		It("should not match when expected label has wrong value", func() {
			labels := map[string]string{
				"app": "test",
			}
			expected := map[string]string{
				"app": "wrong",
			}
			Expect(labels).NotTo(testutil.HaveLabels(expected))
		})

		It("should return error for non-map type", func() {
			matcher := testutil.HaveLabels(map[string]string{"app": "test"})
			_, err := matcher.Match("not a map")
			Expect(err).NotTo(BeNil())
		})

		It("should return proper failure message", func() {
			matcher := testutil.HaveLabels(map[string]string{"app": "test"})
			matcher.Match(map[string]string{})
			msg := matcher.FailureMessage(map[string]string{})
			Expect(msg).To(ContainSubstring("app"))
		})

		It("should return proper negated failure message", func() {
			matcher := testutil.HaveLabels(map[string]string{"app": "test"})
			msg := matcher.NegatedFailureMessage(map[string]string{"app": "test"})
			Expect(msg).To(ContainSubstring("NOT"))
			Expect(msg).To(ContainSubstring("app"))
		})
	})

	Context("HaveReplicas", func() {
		It("should match StatefulSet with expected replicas", func() {
			replicas := int32(3)
			sts := &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Replicas: &replicas,
				},
			}
			Expect(sts).To(testutil.HaveReplicas(3))
		})

		It("should not match StatefulSet with different replicas", func() {
			replicas := int32(3)
			sts := &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Replicas: &replicas,
				},
			}
			Expect(sts).NotTo(testutil.HaveReplicas(5))
		})

		It("should match Deployment with expected replicas", func() {
			replicas := int32(2)
			deploy := &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
				},
			}
			Expect(deploy).To(testutil.HaveReplicas(2))
		})

		It("should handle nil replicas (defaults to 0 or 1)", func() {
			sts := &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{},
			}
			// Default is typically 1, but matcher allows 0 or 1 for nil
			Expect(sts).To(testutil.HaveReplicas(1))
		})

		It("should return error for object without Spec field", func() {
			matcher := testutil.HaveReplicas(1)
			// Use a struct that has no Spec field - wrap string in a struct to avoid panic
			type NoSpecStruct struct {
				Name string
			}
			_, err := matcher.Match(&NoSpecStruct{Name: "test"})
			Expect(err).NotTo(BeNil())
		})

		It("should return error for object without Replicas field", func() {
			matcher := testutil.HaveReplicas(1)
			// ConfigMap has Spec but no Replicas in Spec
			_, err := matcher.Match(&corev1.ConfigMap{})
			Expect(err).NotTo(BeNil())
		})

		It("should return proper failure message", func() {
			matcher := testutil.HaveReplicas(5)
			replicas := int32(1)
			sts := &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Replicas: &replicas,
				},
			}
			matcher.Match(sts)
			msg := matcher.FailureMessage(sts)
			Expect(msg).To(ContainSubstring("5"))
		})

		It("should return proper negated failure message", func() {
			matcher := testutil.HaveReplicas(5)
			replicas := int32(5)
			sts := &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Replicas: &replicas,
				},
			}
			msg := matcher.NegatedFailureMessage(sts)
			Expect(msg).To(ContainSubstring("NOT"))
			Expect(msg).To(ContainSubstring("5"))
		})
	})
})
