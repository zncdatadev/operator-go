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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/common"
	"github.com/zncdatadev/operator-go/pkg/constant"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/testutil"
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
					Name:      reconciler.RoleGroupResourceName("health-test", "test-role", "available"),
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
					Name:      reconciler.RoleGroupResourceName("health-test", "test-role", "progressing"),
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

	It("runs the service health check under the configured timeout", func() {
		var deadline time.Time
		var hasDeadline bool
		hm := reconciler.NewHealthManager(k8sClient).
			WithServiceHealthCheck(common.ServiceHealthCheckFunc(func(checkCtx context.Context, _ client.Client, _, _ string) (bool, error) {
				deadline, hasDeadline = checkCtx.Deadline()
				return true, nil
			}))
		hm.Timeout = 2 * time.Second

		Expect(hm.Check(ctx, "default", "test-cluster", spec, status)).To(Succeed())
		// Without a deadline a hanging probe would pin the reconcile worker forever.
		Expect(hasDeadline).To(BeTrue())
		Expect(deadline).To(BeTemporally("~", time.Now().Add(2*time.Second), time.Second))
	})

	It("leaves the context unbounded when the timeout is disabled", func() {
		var hasDeadline bool
		hm := reconciler.NewHealthManager(k8sClient).
			WithServiceHealthCheck(common.ServiceHealthCheckFunc(func(checkCtx context.Context, _ client.Client, _, _ string) (bool, error) {
				_, hasDeadline = checkCtx.Deadline()
				return true, nil
			}))
		hm.Timeout = 0

		Expect(hm.Check(ctx, "default", "test-cluster", spec, status)).To(Succeed())
		Expect(hasDeadline).To(BeFalse())
	})
})

var _ = Describe("HealthManager role group aggregation", func() {
	var ctx context.Context
	var status *v1alpha1.GenericClusterStatus
	var namespace string

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "default"
		status = &v1alpha1.GenericClusterStatus{}
	})

	It("reports Available=False when a role group's StatefulSet cannot be read", func() {
		spec := &v1alpha1.GenericClusterSpec{
			Roles: map[string]v1alpha1.RoleSpec{
				"missing-role": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{
						"default": {Replicas: ptr.To(int32(3))},
					},
				},
			},
		}

		hm := reconciler.NewHealthManager(k8sClient)
		Expect(hm.Check(ctx, namespace, "health-missing-sts", spec, status)).To(Succeed())

		// A StatefulSet that does not exist yet has no available replicas: claiming
		// Available=True next to Degraded=True would misreport the cluster as usable.
		available := status.GetCondition(v1alpha1.ConditionAvailable)
		Expect(available).NotTo(BeNil())
		Expect(available.Status).To(Equal(metav1.ConditionFalse))

		degraded := status.GetCondition(v1alpha1.ConditionDegraded)
		Expect(degraded).NotTo(BeNil())
		Expect(degraded.Status).To(Equal(metav1.ConditionTrue))
		// The message names the offender instead of a generic "some replicas are unhealthy".
		Expect(degraded.Message).To(ContainSubstring("missing-role/default"))
	})

	It("keeps a role group scaled to zero out of the Degraded condition", func() {
		clusterName := "health-zero-replicas"
		resourceName := reconciler.RoleGroupResourceName(clusterName, "worker", "default")
		sts := testutil.NewTestStatefulSet(resourceName, namespace)
		sts.Spec.Replicas = ptr.To(int32(0))
		Expect(k8sClient.Create(ctx, sts)).To(Succeed())
		DeferCleanup(func() {
			Expect(k8sClient.Delete(ctx, sts)).To(Succeed())
		})

		spec := &v1alpha1.GenericClusterSpec{
			Roles: map[string]v1alpha1.RoleSpec{
				"worker": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{
						"default": {Replicas: ptr.To(int32(0))},
					},
				},
			},
		}

		hm := reconciler.NewHealthManager(k8sClient)
		Expect(hm.Check(ctx, namespace, clusterName, spec, status)).To(Succeed())

		// 0 ready of 0 desired is exactly what was asked for, not a degradation.
		degraded := status.GetCondition(v1alpha1.ConditionDegraded)
		Expect(degraded).NotTo(BeNil())
		Expect(degraded.Status).To(Equal(metav1.ConditionFalse))

		available := status.GetCondition(v1alpha1.ConditionAvailable)
		Expect(available).NotTo(BeNil())
		Expect(available.Status).To(Equal(metav1.ConditionTrue))
	})
})

var _ = Describe("Degraded is a fault signal, not a progress signal", func() {
	// These specs pin the distinction the three conditions are supposed to draw: Available answers
	// "can it serve", Progressing answers "is it changing", Degraded answers "must a human look".
	// Degraded used to be derived from replica counts, so every rolling update and every scale-down
	// reported it — which makes the one condition worth alerting on useless.
	var counter int
	// Each spec gets its own namespace. These specs create pods, and a spec elsewhere in the suite
	// asserts that the shared `default` namespace holds none — an assumption about global state that
	// is not this block's to break.
	var ns string

	BeforeEach(func() {
		counter++
		ns = fmt.Sprintf("health-deg-%d", counter)
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: ns},
		})).To(Succeed())
	})

	// makeRoleGroup creates a role group StatefulSet and writes the status a real controller would.
	// envtest runs no controller-manager, so the status subresource is the input under test.
	makeRoleGroup := func(cluster string, specReplicas int32, st appsv1.StatefulSetStatus) {
		name := reconciler.RoleGroupResourceName(cluster, "worker", "default")
		sts := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: appsv1.StatefulSetSpec{
				Replicas:    ptr.To(specReplicas),
				ServiceName: name + "-headless",
				Selector:    &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": name}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "m", Image: "busybox"}}},
				},
			},
		}
		Expect(k8sClient.Create(ctx, sts)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, sts) })
		sts.Status = st
		Expect(k8sClient.Status().Update(ctx, sts)).To(Succeed())
	}

	// makePod creates a pod carrying the cluster's identity labels with the given container state,
	// which is how findFailingPods sees it.
	makePod := func(cluster, name string, waiting *corev1.ContainerStateWaiting, unschedulable bool) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns,
				Labels: map[string]string{
					constant.LabelKubernetesInstance:  cluster,
					constant.LabelKubernetesManagedBy: "operator-go",
				},
			},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "m", Image: "busybox"}}},
		}
		Expect(k8sClient.Create(ctx, pod)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, pod) })

		if waiting != nil {
			pod.Status.ContainerStatuses = []corev1.ContainerStatus{
				{Name: "m", State: corev1.ContainerState{Waiting: waiting}},
			}
		}
		if unschedulable {
			pod.Status.Phase = corev1.PodPending
			pod.Status.Conditions = []corev1.PodCondition{{
				Type: corev1.PodScheduled, Status: corev1.ConditionFalse,
				Reason: corev1.PodReasonUnschedulable, Message: "0/3 nodes are available",
			}}
		}
		Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())
	}

	// checkInto runs the health pass over a one-role-group cluster, writing into the status the
	// caller supplies. Threading ONE status through several passes is what a real CR does, and is
	// the only way to tell "the condition was set to False" from "the condition was never written":
	// a fresh status makes IsX() false either way.
	checkInto := func(status *v1alpha1.GenericClusterStatus, cluster string, specReplicas int32,
		op *v1alpha1.ClusterOperationSpec) *v1alpha1.GenericClusterStatus {
		spec := &v1alpha1.GenericClusterSpec{
			ClusterOperation: op,
			Roles: map[string]v1alpha1.RoleSpec{
				"worker": {RoleGroups: map[string]v1alpha1.RoleGroupSpec{
					"default": {Replicas: ptr.To(specReplicas)},
				}},
			},
		}
		Expect(reconciler.NewHealthManager(k8sClient).
			Check(ctx, ns, cluster, spec, status)).To(Succeed())
		return status
	}

	// check runs a single pass over a fresh status.
	check := func(cluster string, specReplicas int32, op *v1alpha1.ClusterOperationSpec) *v1alpha1.GenericClusterStatus {
		return checkInto(&v1alpha1.GenericClusterStatus{}, cluster, specReplicas, op)
	}

	newCluster := func() string {
		counter++
		return fmt.Sprintf("deg-%d", counter)
	}

	It("does not report Degraded during a rolling update", func() {
		// One pod recreated, revisions differ: Available=False and Progressing=True are the honest
		// answers, and Degraded=True on top of them was the whole problem — every image bump made
		// the cluster look broken for the length of the rollout.
		cluster := newCluster()
		makeRoleGroup(cluster, 3, appsv1.StatefulSetStatus{
			Replicas: 3, ReadyReplicas: 2, CurrentReplicas: 2, UpdatedReplicas: 1,
			CurrentRevision: "rev1", UpdateRevision: "rev2",
		})

		status := check(cluster, 3, nil)
		Expect(status.IsProgressing()).To(BeTrue())
		Expect(status.IsAvailable()).To(BeFalse())
		Expect(status.IsDegraded()).To(BeFalse(), "a rollout in flight is not a fault")
	})

	It("does not report Degraded — or Unavailable — during a scale-down", func() {
		// The extra pods are still ready while they terminate, so readyReplicas EXCEEDS the desired
		// count. The old `readyReplicas == expected` test called that unhealthy, and with revisions
		// matching there was not even a Progressing=True to hint that anything was in flight.
		cluster := newCluster()
		makeRoleGroup(cluster, 3, appsv1.StatefulSetStatus{
			Replicas: 5, ReadyReplicas: 5, CurrentReplicas: 5, UpdatedReplicas: 5,
			CurrentRevision: "rev1", UpdateRevision: "rev1",
		})

		status := check(cluster, 3, nil)
		Expect(status.IsAvailable()).To(BeTrue(), "more ready replicas than asked for is not unavailable")
		Expect(status.IsDegraded()).To(BeFalse())
	})

	DescribeTable("reports Degraded for a pod the operator cannot help",
		func(reason string) {
			// State-based, not time-based: this fires even though the StatefulSet is still
			// Progressing, which is what keeps a STUCK rollout detectable after the change above.
			cluster := newCluster()
			makeRoleGroup(cluster, 3, appsv1.StatefulSetStatus{
				Replicas: 3, ReadyReplicas: 0, CurrentReplicas: 0, UpdatedReplicas: 3,
				CurrentRevision: "rev1", UpdateRevision: "rev2",
			})
			makePod(cluster, cluster+"-pod-0", &corev1.ContainerStateWaiting{Reason: reason}, false)

			status := check(cluster, 3, nil)
			Expect(status.IsProgressing()).To(BeTrue(), "the rollout is genuinely in flight")
			Expect(status.IsDegraded()).To(BeTrue(), "and it is genuinely stuck")
			cond := status.GetCondition(v1alpha1.ConditionDegraded)
			Expect(cond.Reason).To(Equal(v1alpha1.ReasonPodFailure))
			Expect(cond.Message).To(ContainSubstring(cluster+"-pod-0"), "the message must name the pod")
			Expect(cond.Message).To(ContainSubstring(reason), "and why it is stuck")
		},
		Entry("CrashLoopBackOff", "CrashLoopBackOff"),
		Entry("ImagePullBackOff", "ImagePullBackOff"),
		Entry("InvalidImageName", "InvalidImageName"),
		Entry("CreateContainerConfigError", "CreateContainerConfigError"),
	)

	It("reports Degraded for a pod that cannot be scheduled", func() {
		cluster := newCluster()
		makeRoleGroup(cluster, 1, appsv1.StatefulSetStatus{
			Replicas: 1, ReadyReplicas: 0, CurrentReplicas: 1, UpdatedReplicas: 1,
			CurrentRevision: "rev1", UpdateRevision: "rev1",
		})
		makePod(cluster, cluster+"-pod-0", nil, true)

		status := check(cluster, 1, nil)
		Expect(status.IsDegraded()).To(BeTrue())
		Expect(status.GetCondition(v1alpha1.ConditionDegraded).Message).
			To(ContainSubstring(corev1.PodReasonUnschedulable))
	})

	It("ignores a pod that is already being deleted", func() {
		// Deleting a crash-looping pod is the normal way to clear one, and the pod keeps reporting
		// CrashLoopBackOff while it terminates. Counting it would keep the cluster Degraded for the
		// length of its grace period, blaming the operator's user for the fix they just applied.
		cluster := newCluster()
		makeRoleGroup(cluster, 1, appsv1.StatefulSetStatus{
			Replicas: 1, ReadyReplicas: 0, CurrentReplicas: 1, UpdatedReplicas: 1,
			CurrentRevision: "rev1", UpdateRevision: "rev1",
		})

		name := cluster + "-pod-0"
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns,
				// A finalizer is what keeps the object readable with a deletionTimestamp; without
				// one envtest removes it immediately and there is nothing to observe.
				Finalizers: []string{"test.zncdata.dev/hold"},
				Labels: map[string]string{
					constant.LabelKubernetesInstance:  cluster,
					constant.LabelKubernetesManagedBy: "operator-go",
				},
			},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "m", Image: "busybox"}}},
		}
		Expect(k8sClient.Create(ctx, pod)).To(Succeed())
		DeferCleanup(func() {
			pod.Finalizers = nil
			_ = k8sClient.Update(ctx, pod)
			_ = k8sClient.Delete(ctx, pod)
		})
		pod.Status.ContainerStatuses = []corev1.ContainerStatus{
			{Name: "m", State: corev1.ContainerState{
				Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"},
			}},
		}
		Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())

		// Sanity: while it is alive, it IS a failure.
		Expect(check(cluster, 1, nil).IsDegraded()).To(BeTrue())

		Expect(k8sClient.Delete(ctx, pod)).To(Succeed())
		fresh := &corev1.Pod{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(pod), fresh)).To(Succeed())
		Expect(fresh.DeletionTimestamp).NotTo(BeNil(), "the finalizer must hold it in Terminating")

		Expect(check(cluster, 1, nil).IsDegraded()).To(BeFalse(),
			"a pod on its way out is not a fault")
	})

	It("ignores a transient startup state", func() {
		// ContainerCreating is what every healthy pod looks like for its first seconds. Counting it
		// would put every rollout straight back into Degraded.
		cluster := newCluster()
		makeRoleGroup(cluster, 1, appsv1.StatefulSetStatus{
			Replicas: 1, ReadyReplicas: 0, CurrentReplicas: 1, UpdatedReplicas: 1,
			CurrentRevision: "rev1", UpdateRevision: "rev1",
		})
		makePod(cluster, cluster+"-pod-0", &corev1.ContainerStateWaiting{Reason: "ContainerCreating"}, false)

		status := check(cluster, 1, nil)
		Expect(status.IsAvailable()).To(BeFalse())
		Expect(status.IsDegraded()).To(BeFalse())
	})

	It("reports a paused cluster as Paused, not Degraded, and keeps the rest fresh", func() {
		// Pausing is an administrator's decision. The old behavior reported Degraded=True AND
		// returned before any other condition was written, so a cluster paused mid-rollout kept
		// advertising Progressing=True from the last running cycle forever.
		cluster := newCluster()
		makeRoleGroup(cluster, 3, appsv1.StatefulSetStatus{
			Replicas: 3, ReadyReplicas: 3, CurrentReplicas: 3, UpdatedReplicas: 3,
			CurrentRevision: "rev1", UpdateRevision: "rev1",
		})

		status := check(cluster, 3, &v1alpha1.ClusterOperationSpec{ReconciliationPaused: true})
		Expect(status.IsPaused()).To(BeTrue())
		Expect(status.IsDegraded()).To(BeFalse())
		Expect(status.GetCondition(v1alpha1.ConditionDegraded).Reason).
			To(Equal(v1alpha1.ReasonReconciliationPaused))
		// Observed, not left stale.
		Expect(status.IsAvailable()).To(BeTrue())
		Expect(status.GetCondition(v1alpha1.ConditionProgressing)).NotTo(BeNil())
	})

	It("clears Paused once the pause is lifted", func() {
		// A condition written in only one direction is a condition that goes stale: the CR would
		// keep advertising a pause that ended. The two passes share ONE status, so this fails if
		// the un-paused pass merely leaves the condition alone.
		cluster := newCluster()
		makeRoleGroup(cluster, 1, appsv1.StatefulSetStatus{
			Replicas: 1, ReadyReplicas: 1, CurrentReplicas: 1, UpdatedReplicas: 1,
			CurrentRevision: "rev1", UpdateRevision: "rev1",
		})

		status := &v1alpha1.GenericClusterStatus{}
		checkInto(status, cluster, 1, &v1alpha1.ClusterOperationSpec{ReconciliationPaused: true})
		Expect(status.IsPaused()).To(BeTrue())

		checkInto(status, cluster, 1, nil)
		Expect(status.IsPaused()).To(BeFalse())
		cond := status.GetCondition(v1alpha1.ConditionPaused)
		Expect(cond).NotTo(BeNil(), "written to False, not left absent")
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
	})

	It("does not leave a stale Progressing behind when a cluster is paused mid-rollout", func() {
		// The old paused branch returned before writing anything else, so a cluster paused during a
		// rollout kept advertising Progressing=True for the whole pause. Same status threaded
		// through both passes, so only a genuine re-evaluation clears it.
		cluster := newCluster()
		makeRoleGroup(cluster, 3, appsv1.StatefulSetStatus{
			Replicas: 3, ReadyReplicas: 2, CurrentReplicas: 2, UpdatedReplicas: 1,
			CurrentRevision: "rev1", UpdateRevision: "rev2",
		})

		status := &v1alpha1.GenericClusterStatus{}
		checkInto(status, cluster, 3, nil)
		Expect(status.IsProgressing()).To(BeTrue(), "the rollout is in flight")

		// The rollout finishes while the cluster is paused.
		name := reconciler.RoleGroupResourceName(cluster, "worker", "default")
		sts := &appsv1.StatefulSet{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, sts)).To(Succeed())
		sts.Status = appsv1.StatefulSetStatus{
			Replicas: 3, ReadyReplicas: 3, CurrentReplicas: 3, UpdatedReplicas: 3,
			CurrentRevision: "rev2", UpdateRevision: "rev2",
		}
		Expect(k8sClient.Status().Update(ctx, sts)).To(Succeed())

		checkInto(status, cluster, 3, &v1alpha1.ClusterOperationSpec{ReconciliationPaused: true})
		Expect(status.IsPaused()).To(BeTrue())
		Expect(status.IsProgressing()).To(BeFalse(), "observed while paused, not left stale")
		Expect(status.IsAvailable()).To(BeTrue())
	})

	It("still reports Degraded when a role group's StatefulSet is missing entirely", func() {
		// Nothing to read means the object the operator applied is gone: a real fault, and one that
		// no pod state would reveal.
		status := check(newCluster(), 1, nil)
		Expect(status.IsDegraded()).To(BeTrue())
		Expect(status.GetCondition(v1alpha1.ConditionDegraded).Reason).
			To(Equal(v1alpha1.ReasonWorkloadUnreadable))
	})
})

var _ = Describe("Available names the right kind of problem", func() {
	// The two ways a role group can be unavailable read differently: one has replica counts to
	// compare, the other has no StatefulSet to read. Calling the second "fewer ready replicas than
	// desired" sends an operator looking for numbers that do not exist.
	var ns string
	var counter int

	BeforeEach(func() {
		counter++
		ns = fmt.Sprintf("health-avail-%d", counter)
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: ns},
		})).To(Succeed())
	})

	check := func(cluster string, groups map[string]v1alpha1.RoleGroupSpec) *v1alpha1.GenericClusterStatus {
		status := &v1alpha1.GenericClusterStatus{}
		Expect(reconciler.NewHealthManager(k8sClient).Check(ctx, ns, cluster,
			&v1alpha1.GenericClusterSpec{Roles: map[string]v1alpha1.RoleSpec{
				"worker": {RoleGroups: groups},
			}}, status)).To(Succeed())
		return status
	}

	It("says the StatefulSet is unreadable rather than short of replicas", func() {
		status := check("avail-a", map[string]v1alpha1.RoleGroupSpec{"default": {Replicas: ptr.To(int32(3))}})

		cond := status.GetCondition(v1alpha1.ConditionAvailable)
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(v1alpha1.ReasonWorkloadUnreadable))
		Expect(cond.Message).To(ContainSubstring("StatefulSet could not be read"))
		Expect(cond.Message).NotTo(ContainSubstring("fewer ready replicas"),
			"there are no replica counts to compare")
	})

	It("reports both kinds when both occur", func() {
		// One role group readable but short of replicas, one absent entirely.
		name := reconciler.RoleGroupResourceName("avail-b", "worker", "short")
		sts := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: appsv1.StatefulSetSpec{
				Replicas:    ptr.To(int32(3)),
				ServiceName: name + "-headless",
				Selector:    &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": name}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "m", Image: "busybox"}}},
				},
			},
		}
		Expect(k8sClient.Create(ctx, sts)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, sts) })
		sts.Status = appsv1.StatefulSetStatus{
			Replicas: 3, ReadyReplicas: 1, CurrentReplicas: 3, UpdatedReplicas: 3,
			CurrentRevision: "rev1", UpdateRevision: "rev1",
		}
		Expect(k8sClient.Status().Update(ctx, sts)).To(Succeed())

		status := check("avail-b", map[string]v1alpha1.RoleGroupSpec{
			"short":   {Replicas: ptr.To(int32(3))},
			"missing": {Replicas: ptr.To(int32(1))},
		})

		cond := status.GetCondition(v1alpha1.ConditionAvailable)
		Expect(cond.Reason).To(Equal(v1alpha1.ReasonPodsNotReady))
		Expect(cond.Message).To(ContainSubstring("fewer ready replicas than desired: worker/short"))
		Expect(cond.Message).To(ContainSubstring("StatefulSet could not be read: worker/missing"))
	})
})
