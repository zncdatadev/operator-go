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
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/common"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("HealthManager", func() {
	var healthManager *reconciler.HealthManager
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
		healthManager = reconciler.NewHealthManager(k8sClient)
	})

	Describe("NewHealthManager", func() {
		It("should create a HealthManager with default values", func() {
			Expect(healthManager).NotTo(BeNil())
			Expect(healthManager.Client).To(Equal(k8sClient))
			Expect(healthManager.CheckInterval).To(Equal(reconciler.DefaultCheckInterval))
			Expect(healthManager.Timeout).To(Equal(reconciler.DefaultTimeout))
		})

		It("should allow custom check interval and timeout", func() {
			hm := reconciler.NewHealthManager(k8sClient)
			hm.CheckInterval = 60 * time.Second
			hm.Timeout = 120 * time.Second
			Expect(hm.CheckInterval).To(Equal(60 * time.Second))
			Expect(hm.Timeout).To(Equal(120 * time.Second))
		})
	})

	Describe("Check", func() {
		var spec *v1alpha1.GenericClusterSpec
		var status *v1alpha1.GenericClusterStatus

		BeforeEach(func() {
			spec = &v1alpha1.GenericClusterSpec{
				Roles: map[string]v1alpha1.RoleSpec{},
			}
			status = &v1alpha1.GenericClusterStatus{}
		})

		It("should handle ReconciliationPaused state", func() {
			spec.ClusterOperation = &v1alpha1.ClusterOperationSpec{
				ReconciliationPaused: true,
			}

			err := healthManager.Check(ctx, "default", "test-cluster", spec, status)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should handle Stopped state", func() {
			spec.ClusterOperation = &v1alpha1.ClusterOperationSpec{
				Stopped: true,
			}

			err := healthManager.Check(ctx, "default", "test-cluster", spec, status)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should handle empty roles", func() {
			err := healthManager.Check(ctx, "default", "test-cluster", spec, status)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should handle nil cluster operation", func() {
			spec.ClusterOperation = nil
			err := healthManager.Check(ctx, "default", "test-cluster", spec, status)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should handle role groups with no existing StatefulSet", func() {
			spec.Roles = map[string]v1alpha1.RoleSpec{
				"test-role": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{
						"default": {
							Replicas: ptr.To(int32(3)),
						},
					},
				},
			}

			err := healthManager.Check(ctx, "default", "test-cluster", spec, status)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should set available condition when all replicas are available", func() {
			spec.Roles = map[string]v1alpha1.RoleSpec{
				"test-role": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{
						"default": {Replicas: ptr.To(int32(0))}, // 0 replicas is trivially available
					},
				},
			}

			err := healthManager.Check(ctx, "default", "test-cluster", spec, status)
			Expect(err).ToNot(HaveOccurred())

			availableCond := status.GetCondition(v1alpha1.ConditionAvailable)
			Expect(availableCond).NotTo(BeNil())
		})

		It("should handle multiple roles and role groups", func() {
			spec.Roles = map[string]v1alpha1.RoleSpec{
				"role-a": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{
						"group-1": {Replicas: ptr.To(int32(0))},
						"group-2": {Replicas: ptr.To(int32(0))},
					},
				},
				"role-b": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{
						"group-1": {Replicas: ptr.To(int32(0))},
					},
				},
			}

			err := healthManager.Check(ctx, "default", "multi-cluster", spec, status)
			Expect(err).ToNot(HaveOccurred())

			// Status should be updated
			Expect(status.GetCondition(v1alpha1.ConditionAvailable)).NotTo(BeNil())
			Expect(status.GetCondition(v1alpha1.ConditionDegraded)).NotTo(BeNil())
			Expect(status.GetCondition(v1alpha1.ConditionProgressing)).NotTo(BeNil())
		})

		It("should handle ReconciliationPaused and Stopped both set", func() {
			spec.ClusterOperation = &v1alpha1.ClusterOperationSpec{
				ReconciliationPaused: true,
				Stopped:              true,
			}

			err := healthManager.Check(ctx, "default", "test-cluster", spec, status)
			Expect(err).ToNot(HaveOccurred())
			// ReconciliationPaused takes precedence
			degradedCond := status.GetCondition(v1alpha1.ConditionDegraded)
			Expect(degradedCond).NotTo(BeNil())
		})

		It("should handle cluster operation with neither paused nor stopped", func() {
			spec.ClusterOperation = &v1alpha1.ClusterOperationSpec{
				ReconciliationPaused: false,
				Stopped:              false,
			}
			spec.Roles = map[string]v1alpha1.RoleSpec{
				"test-role": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{
						"default": {Replicas: ptr.To(int32(0))},
					},
				},
			}

			err := healthManager.Check(ctx, "default", "test-cluster", spec, status)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("CheckPodHealth", func() {
		It("should return zero for non-existent pods", func() {
			total, ready, err := healthManager.CheckPodHealth(ctx, "default", map[string]string{"app": "nonexistent"})
			Expect(err).ToNot(HaveOccurred())
			Expect(total).To(Equal(0))
			Expect(ready).To(Equal(0))
		})

		It("should handle empty labels", func() {
			total, ready, err := healthManager.CheckPodHealth(ctx, "default", map[string]string{})
			Expect(err).ToNot(HaveOccurred())
			Expect(total).To(Equal(0))
			Expect(ready).To(Equal(0))
		})

		It("should handle nil labels", func() {
			total, ready, err := healthManager.CheckPodHealth(ctx, "default", nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(total).To(Equal(0))
			Expect(ready).To(Equal(0))
		})
	})

	Describe("UpdateStatusCondition", func() {
		It("should update status condition", func() {
			status := &v1alpha1.GenericClusterStatus{}
			healthManager.UpdateStatusCondition(
				status,
				v1alpha1.ConditionAvailable,
				metav1.ConditionTrue,
				v1alpha1.ReasonAvailable,
				"Cluster is available",
			)

			condition := status.GetCondition(v1alpha1.ConditionAvailable)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionTrue))
		})

		It("should update multiple conditions", func() {
			status := &v1alpha1.GenericClusterStatus{}

			healthManager.UpdateStatusCondition(
				status,
				v1alpha1.ConditionAvailable,
				metav1.ConditionTrue,
				v1alpha1.ReasonAvailable,
				"Cluster is available",
			)

			healthManager.UpdateStatusCondition(
				status,
				v1alpha1.ConditionDegraded,
				metav1.ConditionFalse,
				v1alpha1.ReasonAvailable,
				"Cluster is not degraded",
			)

			availableCond := status.GetCondition(v1alpha1.ConditionAvailable)
			Expect(availableCond).NotTo(BeNil())
			Expect(availableCond.Status).To(Equal(metav1.ConditionTrue))

			degradedCond := status.GetCondition(v1alpha1.ConditionDegraded)
			Expect(degradedCond).NotTo(BeNil())
			Expect(degradedCond.Status).To(Equal(metav1.ConditionFalse))
		})

		It("should update existing condition", func() {
			status := &v1alpha1.GenericClusterStatus{}

			// First update
			healthManager.UpdateStatusCondition(
				status,
				v1alpha1.ConditionAvailable,
				metav1.ConditionFalse,
				v1alpha1.ReasonCreating,
				"Cluster is starting",
			)

			// Second update
			healthManager.UpdateStatusCondition(
				status,
				v1alpha1.ConditionAvailable,
				metav1.ConditionTrue,
				v1alpha1.ReasonAvailable,
				"Cluster is available",
			)

			condition := status.GetCondition(v1alpha1.ConditionAvailable)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionTrue))
			Expect(condition.Reason).To(Equal(v1alpha1.ReasonAvailable))
		})

		It("should set all condition fields correctly", func() {
			status := &v1alpha1.GenericClusterStatus{}
			healthManager.UpdateStatusCondition(
				status,
				v1alpha1.ConditionProgressing,
				metav1.ConditionTrue,
				v1alpha1.ReasonProgressing,
				"Cluster is progressing",
			)

			condition := status.GetCondition(v1alpha1.ConditionProgressing)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Type).To(Equal(string(v1alpha1.ConditionProgressing)))
			Expect(condition.Status).To(Equal(metav1.ConditionTrue))
			Expect(condition.Reason).To(Equal(v1alpha1.ReasonProgressing))
			Expect(condition.Message).To(Equal("Cluster is progressing"))
		})
	})
})

const healthTestNamespace = "default"

var _ = Describe("HealthManager with StatefulSet", func() {
	var healthManager *reconciler.HealthManager
	var ctx context.Context
	var namespace string

	BeforeEach(func() {
		ctx = context.Background()
		namespace = healthTestNamespace
		healthManager = reconciler.NewHealthManager(k8sClient)
	})

	Describe("Check with actual StatefulSet", func() {
		It("should detect available StatefulSet", func() {
			// Create a StatefulSet with ready replicas
			replicas := int32(2)
			sts := &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "health-test-available",
					Namespace: namespace,
				},
				Spec: appsv1.StatefulSetSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "health-test"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "health-test"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "test",
									Image: "test-image",
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, sts)).To(Succeed())

			// Update status to simulate ready replicas
			sts.Status.Replicas = 2
			sts.Status.ReadyReplicas = 2
			sts.Status.CurrentReplicas = 2
			sts.Status.UpdatedReplicas = 2
			sts.Status.CurrentRevision = "v1"
			sts.Status.UpdateRevision = "v1"
			Expect(k8sClient.Status().Update(ctx, sts)).To(Succeed())

			spec := &v1alpha1.GenericClusterSpec{
				Roles: map[string]v1alpha1.RoleSpec{
					"test-role": {
						RoleGroups: map[string]v1alpha1.RoleGroupSpec{
							"available": {Replicas: ptr.To(int32(2))},
						},
					},
				},
			}
			status := &v1alpha1.GenericClusterStatus{}

			err := healthManager.Check(ctx, namespace, "health-test", spec, status)
			Expect(err).ToNot(HaveOccurred())

			Expect(k8sClient.Delete(ctx, sts)).To(Succeed())
		})

		It("should detect progressing StatefulSet", func() {
			replicas := int32(2)
			sts := &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "health-test-progressing",
					Namespace: namespace,
				},
				Spec: appsv1.StatefulSetSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "health-test-prog"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "health-test-prog"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "test",
									Image: "test-image",
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, sts)).To(Succeed())

			// Update status to simulate progressing state
			sts.Status.Replicas = 2
			sts.Status.ReadyReplicas = 1
			sts.Status.CurrentReplicas = 1
			sts.Status.UpdatedReplicas = 2
			sts.Status.CurrentRevision = "v1"
			sts.Status.UpdateRevision = "v2"
			Expect(k8sClient.Status().Update(ctx, sts)).To(Succeed())

			spec := &v1alpha1.GenericClusterSpec{
				Roles: map[string]v1alpha1.RoleSpec{
					"test-role": {
						RoleGroups: map[string]v1alpha1.RoleGroupSpec{
							"progressing": {Replicas: ptr.To(int32(2))},
						},
					},
				},
			}
			status := &v1alpha1.GenericClusterStatus{}

			err := healthManager.Check(ctx, namespace, "health-test", spec, status)
			Expect(err).ToNot(HaveOccurred())

			Expect(k8sClient.Delete(ctx, sts)).To(Succeed())
		})
	})
})

var _ = Describe("HealthManager CheckPodHealth with Pods", func() {
	var healthManager *reconciler.HealthManager
	var ctx context.Context
	var namespace string

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "default"
		healthManager = reconciler.NewHealthManager(k8sClient)
	})

	It("should count ready pods correctly", func() {
		// Create pods
		pod1 := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "health-pod-1",
				Namespace: namespace,
				Labels:    map[string]string{"app": "health-test-pods"},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "test", Image: "test-image"},
				},
			},
		}
		Expect(k8sClient.Create(ctx, pod1)).To(Succeed())

		pod2 := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "health-pod-2",
				Namespace: namespace,
				Labels:    map[string]string{"app": "health-test-pods"},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "test", Image: "test-image"},
				},
			},
		}
		Expect(k8sClient.Create(ctx, pod2)).To(Succeed())

		// Update pod statuses
		pod1.Status.Phase = corev1.PodRunning
		pod1.Status.Conditions = []corev1.PodCondition{
			{
				Type:   corev1.PodReady,
				Status: corev1.ConditionTrue,
			},
		}
		Expect(k8sClient.Status().Update(ctx, pod1)).To(Succeed())

		pod2.Status.Phase = corev1.PodRunning
		pod2.Status.Conditions = []corev1.PodCondition{
			{
				Type:   corev1.PodReady,
				Status: corev1.ConditionFalse,
			},
		}
		Expect(k8sClient.Status().Update(ctx, pod2)).To(Succeed())

		// Check pod health
		total, ready, err := healthManager.CheckPodHealth(ctx, namespace, map[string]string{"app": "health-test-pods"})
		Expect(err).ToNot(HaveOccurred())
		Expect(total).To(Equal(2))
		Expect(ready).To(Equal(1))

		Expect(k8sClient.Delete(ctx, pod1)).To(Succeed())
		Expect(k8sClient.Delete(ctx, pod2)).To(Succeed())
	})

	It("should return 0 ready for non-running pods", func() {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "health-pod-pending",
				Namespace: namespace,
				Labels:    map[string]string{"app": "health-test-pending"},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "test", Image: "test-image"},
				},
			},
		}
		Expect(k8sClient.Create(ctx, pod)).To(Succeed())

		pod.Status.Phase = corev1.PodPending
		Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())

		total, ready, err := healthManager.CheckPodHealth(ctx, namespace, map[string]string{"app": "health-test-pending"})
		Expect(err).ToNot(HaveOccurred())
		Expect(total).To(Equal(1))
		Expect(ready).To(Equal(0))

		Expect(k8sClient.Delete(ctx, pod)).To(Succeed())
	})

	It("should return 0 ready for running pod without PodReady condition", func() {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "health-pod-no-condition",
				Namespace: namespace,
				Labels:    map[string]string{"app": "health-test-no-cond"},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "test", Image: "test-image"},
				},
			},
		}
		Expect(k8sClient.Create(ctx, pod)).To(Succeed())

		pod.Status.Phase = corev1.PodRunning
		pod.Status.Conditions = []corev1.PodCondition{
			{
				Type:   corev1.PodScheduled,
				Status: corev1.ConditionTrue,
			},
		}
		Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())

		total, ready, err := healthManager.CheckPodHealth(ctx, namespace, map[string]string{"app": "health-test-no-cond"})
		Expect(err).ToNot(HaveOccurred())
		Expect(total).To(Equal(1))
		Expect(ready).To(Equal(0))

		Expect(k8sClient.Delete(ctx, pod)).To(Succeed())
	})
})

var _ = Describe("HealthManager Default constants", func() {
	It("should have correct default check interval", func() {
		Expect(reconciler.DefaultCheckInterval).To(Equal(120 * time.Second))
	})

	It("should have correct default timeout", func() {
		Expect(reconciler.DefaultTimeout).To(Equal(300 * time.Second))
	})
})

var _ = Describe("HealthManager with ServiceHealthCheck", func() {
	var ctx context.Context
	var spec *v1alpha1.GenericClusterSpec
	var status *v1alpha1.GenericClusterStatus

	BeforeEach(func() {
		ctx = context.Background()
		spec = &v1alpha1.GenericClusterSpec{}
		status = &v1alpha1.GenericClusterStatus{}
	})

	It("sets ServiceHealthy=true when service check returns healthy", func() {
		hm := reconciler.NewHealthManager(k8sClient).
			WithServiceHealthCheck(common.ServiceHealthCheckFunc(func(_ context.Context, _ client.Client, _, _ string) (bool, error) {
				return true, nil
			}))

		err := hm.Check(ctx, "default", "test-cluster", spec, status)
		Expect(err).NotTo(HaveOccurred())

		cond := status.GetCondition(v1alpha1.ConditionServiceHealthy)
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionTrue))
	})

	It("sets ServiceHealthy=false and Degraded=true when service check returns unhealthy", func() {
		hm := reconciler.NewHealthManager(k8sClient).
			WithServiceHealthCheck(common.AlwaysUnhealthy)

		err := hm.Check(ctx, "default", "test-cluster", spec, status)
		Expect(err).NotTo(HaveOccurred())

		cond := status.GetCondition(v1alpha1.ConditionServiceHealthy)
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))

		degraded := status.GetCondition(v1alpha1.ConditionDegraded)
		Expect(degraded).NotTo(BeNil())
		Expect(degraded.Status).To(Equal(metav1.ConditionTrue))
	})

	It("sets Degraded=true when service check returns an error", func() {
		hm := reconciler.NewHealthManager(k8sClient).
			WithServiceHealthCheck(common.ServiceHealthCheckFunc(func(_ context.Context, _ client.Client, _, _ string) (bool, error) {
				return false, errors.New("connection refused")
			}))

		err := hm.Check(ctx, "default", "test-cluster", spec, status)
		Expect(err).NotTo(HaveOccurred())

		degraded := status.GetCondition(v1alpha1.ConditionDegraded)
		Expect(degraded).NotTo(BeNil())
		Expect(degraded.Status).To(Equal(metav1.ConditionTrue))
		Expect(degraded.Message).To(ContainSubstring("connection refused"))
	})

	It("skips service health check when not configured", func() {
		hm := reconciler.NewHealthManager(k8sClient) // no WithServiceHealthCheck

		err := hm.Check(ctx, "default", "test-cluster", spec, status)
		Expect(err).NotTo(HaveOccurred())

		// ServiceHealthy condition should not be set
		cond := status.GetCondition(v1alpha1.ConditionServiceHealthy)
		Expect(cond).To(BeNil())
	})

	It("uses CompositeHealthCheck combining multiple checks", func() {
		called := []string{}
		check1 := common.ServiceHealthCheckFunc(func(_ context.Context, _ client.Client, _, name string) (bool, error) {
			called = append(called, "check1")
			return true, nil
		})
		check2 := common.ServiceHealthCheckFunc(func(_ context.Context, _ client.Client, _, name string) (bool, error) {
			called = append(called, "check2")
			return true, nil
		})

		hm := reconciler.NewHealthManager(k8sClient).
			WithServiceHealthCheck(common.NewCompositeHealthCheck(check1, check2))

		err := hm.Check(ctx, "default", "test-cluster", spec, status)
		Expect(err).NotTo(HaveOccurred())
		Expect(called).To(Equal([]string{"check1", "check2"}))

		cond := status.GetCondition(v1alpha1.ConditionServiceHealthy)
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionTrue))
	})
})
