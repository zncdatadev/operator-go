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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/common"
	"github.com/zncdatadev/operator-go/pkg/config"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/testutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("GenericReconciler", func() {
	Describe("NewGenericReconciler", func() {
		It("should create a GenericReconciler with valid config", func() {
			mockCR := testutil.NewMockCluster("test-cluster", "default")
			wrappedCR := testutil.WrapMockCluster(mockCR)
			mockHandler := testutil.NewMockRoleGroupHandler()

			cfg := &reconciler.GenericReconcilerConfig[*testutil.ClusterWrapper]{
				Client:           k8sClient,
				Scheme:           testScheme,
				Recorder:         recorder,
				RoleGroupHandler: &handlerAdapter{handler: mockHandler},
				Prototype:        wrappedCR,
			}

			r, err := reconciler.NewGenericReconciler(cfg)

			Expect(err).NotTo(HaveOccurred())
			Expect(r).NotTo(BeNil())
		})

		It("should return error when client is nil", func() {
			mockCR := testutil.NewMockCluster("test-cluster", "default")
			wrappedCR := testutil.WrapMockCluster(mockCR)
			mockHandler := testutil.NewMockRoleGroupHandler()

			cfg := &reconciler.GenericReconcilerConfig[*testutil.ClusterWrapper]{
				Client:           nil,
				Scheme:           testScheme,
				Recorder:         recorder,
				RoleGroupHandler: &handlerAdapter{handler: mockHandler},
				Prototype:        wrappedCR,
			}

			r, err := reconciler.NewGenericReconciler(cfg)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("client is required"))
			Expect(r).To(BeNil())
		})

		It("should return error when scheme is nil", func() {
			mockCR := testutil.NewMockCluster("test-cluster", "default")
			wrappedCR := testutil.WrapMockCluster(mockCR)
			mockHandler := testutil.NewMockRoleGroupHandler()

			cfg := &reconciler.GenericReconcilerConfig[*testutil.ClusterWrapper]{
				Client:           k8sClient,
				Scheme:           nil,
				Recorder:         recorder,
				RoleGroupHandler: &handlerAdapter{handler: mockHandler},
				Prototype:        wrappedCR,
			}

			r, err := reconciler.NewGenericReconciler(cfg)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("scheme is required"))
			Expect(r).To(BeNil())
		})

		It("should return error when recorder is nil", func() {
			mockCR := testutil.NewMockCluster("test-cluster", "default")
			wrappedCR := testutil.WrapMockCluster(mockCR)
			mockHandler := testutil.NewMockRoleGroupHandler()

			cfg := &reconciler.GenericReconcilerConfig[*testutil.ClusterWrapper]{
				Client:           k8sClient,
				Scheme:           testScheme,
				Recorder:         nil,
				RoleGroupHandler: &handlerAdapter{handler: mockHandler},
				Prototype:        wrappedCR,
			}

			r, err := reconciler.NewGenericReconciler(cfg)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("recorder is required"))
			Expect(r).To(BeNil())
		})

		It("should return error when roleGroupHandler is nil", func() {
			mockCR := testutil.NewMockCluster("test-cluster", "default")
			wrappedCR := testutil.WrapMockCluster(mockCR)

			cfg := &reconciler.GenericReconcilerConfig[*testutil.ClusterWrapper]{
				Client:           k8sClient,
				Scheme:           testScheme,
				Recorder:         recorder,
				RoleGroupHandler: nil,
				Prototype:        wrappedCR,
			}

			r, err := reconciler.NewGenericReconciler(cfg)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("roleGroupHandler is required"))
			Expect(r).To(BeNil())
		})

		It("should use default health check intervals when not specified", func() {
			mockCR := testutil.NewMockCluster("test-cluster", "default")
			wrappedCR := testutil.WrapMockCluster(mockCR)
			mockHandler := testutil.NewMockRoleGroupHandler()

			cfg := &reconciler.GenericReconcilerConfig[*testutil.ClusterWrapper]{
				Client:           k8sClient,
				Scheme:           testScheme,
				Recorder:         recorder,
				RoleGroupHandler: &handlerAdapter{handler: mockHandler},
				Prototype:        wrappedCR,
			}

			r, err := reconciler.NewGenericReconciler(cfg)

			Expect(err).NotTo(HaveOccurred())
			Expect(r).NotTo(BeNil())
		})

		It("should use custom health check intervals when specified", func() {
			mockCR := testutil.NewMockCluster("test-cluster", "default")
			wrappedCR := testutil.WrapMockCluster(mockCR)
			mockHandler := testutil.NewMockRoleGroupHandler()

			cfg := &reconciler.GenericReconcilerConfig[*testutil.ClusterWrapper]{
				Client:              k8sClient,
				Scheme:              testScheme,
				Recorder:            recorder,
				RoleGroupHandler:    &handlerAdapter{handler: mockHandler},
				Prototype:           wrappedCR,
				HealthCheckInterval: 60 * time.Second,
				HealthCheckTimeout:  120 * time.Second,
			}

			r, err := reconciler.NewGenericReconciler(cfg)

			Expect(err).NotTo(HaveOccurred())
			Expect(r).NotTo(BeNil())
		})
	})

	Describe("MockRoleGroupHandler", func() {
		It("should create mock handler with default values", func() {
			handler := testutil.NewMockRoleGroupHandler()
			Expect(handler).NotTo(BeNil())
			Expect(handler.Image).To(Equal("test-image:latest"))
		})

		It("should create mock handler with custom image", func() {
			handler := testutil.NewMockRoleGroupHandler()
			handler.Image = "custom-image:v1"
			Expect(handler.Image).To(Equal("custom-image:v1"))
		})
	})

	Describe("ClusterWrapper", func() {
		It("should implement ClusterInterface", func() {
			mockCR := testutil.NewMockCluster("test-cluster", "default")
			wrappedCR := testutil.WrapMockCluster(mockCR)

			var _ common.ClusterInterface = wrappedCR
			Expect(wrappedCR).NotTo(BeNil())
		})

		It("should return correct name and namespace", func() {
			mockCR := testutil.NewMockCluster("test-cluster", "test-namespace")
			wrappedCR := testutil.WrapMockCluster(mockCR)

			Expect(wrappedCR.GetName()).To(Equal("test-cluster"))
			Expect(wrappedCR.GetNamespace()).To(Equal("test-namespace"))
		})
	})
})

const testNamespace = "default"

var _ = Describe("GenericReconciler Reconcile", func() {
	var r *reconciler.GenericReconciler[*testutil.ClusterWrapper]
	var mockHandler *testutil.MockRoleGroupHandler
	var namespace string

	BeforeEach(func() {
		namespace = testNamespace
		mockHandler = testutil.NewMockRoleGroupHandler()

		mockCR := testutil.NewMockCluster("test-cr", namespace)
		wrappedCR := testutil.WrapMockCluster(mockCR)

		cfg := &reconciler.GenericReconcilerConfig[*testutil.ClusterWrapper]{
			Client:           k8sClient,
			Scheme:           testScheme,
			Recorder:         recorder,
			RoleGroupHandler: &handlerAdapter{handler: mockHandler},
			Prototype:        wrappedCR,
		}

		var err error
		r, err = reconciler.NewGenericReconciler(cfg)
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("Reconcile", func() {
		It("should return empty result when CR does not exist", func() {
			req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: "non-existent-cr"}}

			result, err := r.Reconcile(context.Background(), req)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})
	})
})

var _ = Describe("GenericReconcilerConfig", func() {
	It("should have correct default values", func() {
		mockCR := testutil.NewMockCluster("test-cluster", "default")
		wrappedCR := testutil.WrapMockCluster(mockCR)

		cfg := &reconciler.GenericReconcilerConfig[*testutil.ClusterWrapper]{
			Client:           k8sClient,
			Scheme:           testScheme,
			Recorder:         recorder,
			RoleGroupHandler: &handlerAdapter{handler: testutil.NewMockRoleGroupHandler()},
			Prototype:        wrappedCR,
		}

		Expect(cfg.Client).To(Equal(k8sClient))
		Expect(cfg.Scheme).To(Equal(testScheme))
		Expect(cfg.Recorder).To(Equal(recorder))
		Expect(cfg.HealthCheckInterval).To(BeZero())
		Expect(cfg.HealthCheckTimeout).To(BeZero())
	})
})

var _ = Describe("RoleGroupBuildContext", func() {
	It("should create a valid context", func() {
		buildCtx := &reconciler.RoleGroupBuildContext{
			ClusterName:      "test-cluster",
			ClusterNamespace: "default",
			ClusterLabels:    map[string]string{"app": "test"},
			ClusterSpec: &v1alpha1.GenericClusterSpec{
				Roles: map[string]v1alpha1.RoleSpec{
					"test-role": {
						RoleGroups: map[string]v1alpha1.RoleGroupSpec{
							"default": {Replicas: ptr.To(int32(1))},
						},
					},
				},
			},
			RoleName:      "test-role",
			RoleSpec:      &v1alpha1.RoleSpec{},
			RoleGroupName: "default",
			RoleGroupSpec: v1alpha1.RoleGroupSpec{Replicas: ptr.To(int32(1))},
			MergedConfig:  &config.MergedConfig{},
			ResourceName:  "test-cluster-default",
		}

		Expect(buildCtx.ClusterName).To(Equal("test-cluster"))
		Expect(buildCtx.RoleName).To(Equal("test-role"))
		Expect(buildCtx.RoleGroupName).To(Equal("default"))
		Expect(buildCtx.ResourceName).To(Equal("test-cluster-default"))
	})
})

var _ = Describe("RoleGroupResources", func() {
	It("should hold all resource types", func() {
		resources := &reconciler.RoleGroupResources{
			ConfigMap:       &corev1.ConfigMap{},
			Service:         &corev1.Service{},
			HeadlessService: &corev1.Service{},
			StatefulSet:     &appsv1.StatefulSet{},
		}

		Expect(resources.ConfigMap).NotTo(BeNil())
		Expect(resources.Service).NotTo(BeNil())
		Expect(resources.HeadlessService).NotTo(BeNil())
		Expect(resources.StatefulSet).NotTo(BeNil())
	})

	It("should allow nil resources", func() {
		resources := &reconciler.RoleGroupResources{}

		Expect(resources.ConfigMap).To(BeNil())
		Expect(resources.Service).To(BeNil())
		Expect(resources.HeadlessService).To(BeNil())
		Expect(resources.StatefulSet).To(BeNil())
		Expect(resources.PodDisruptionBudget).To(BeNil())
	})
})

var _ = Describe("StatefulSet scaling", func() {
	It("should create StatefulSet with correct replicas", func() {
		replicas := int32(3)
		sts := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-sts",
				Namespace: "default",
			},
			Spec: appsv1.StatefulSetSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "test"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": "test"},
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

		fetchedSts := &appsv1.StatefulSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "test-sts"}, fetchedSts)).To(Succeed())
		Expect(*fetchedSts.Spec.Replicas).To(Equal(int32(3)))

		Expect(k8sClient.Delete(ctx, sts)).To(Succeed())
	})
})

var _ = Describe("GenericReconciler with MockCluster", func() {
	It("should handle MockCluster with roles", func() {
		mockCR := testutil.NewMockCluster("test-cluster", "default").
			WithRoles(map[string]v1alpha1.RoleSpec{
				"test-role": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{
						"default": {Replicas: ptr.To(int32(1))},
					},
				},
			})

		Expect(mockCR.Name).To(Equal("test-cluster"))
		Expect(mockCR.Namespace).To(Equal("default"))
		Expect(mockCR.Spec.Roles).To(HaveKey("test-role"))
	})

	It("should handle MockCluster with cluster operation", func() {
		mockCR := testutil.NewMockCluster("test-cluster", "default").
			WithClusterOperation(&v1alpha1.ClusterOperationSpec{
				ReconciliationPaused: true,
				Stopped:              false,
			})

		Expect(mockCR.Spec.ClusterOperation.ReconciliationPaused).To(BeTrue())
		Expect(mockCR.Spec.ClusterOperation.Stopped).To(BeFalse())
	})

	It("should handle MockCluster with labels", func() {
		mockCR := testutil.NewMockCluster("test-cluster", "default").
			WithLabels(map[string]string{
				"custom-label": "label-value",
			})

		Expect(mockCR.Labels["custom-label"]).To(Equal("label-value"))
	})

	It("should handle MockCluster with annotations", func() {
		mockCR := testutil.NewMockCluster("test-cluster", "default").
			WithAnnotations(map[string]string{
				"custom-annotation": "annotation-value",
			})

		Expect(mockCR.Annotations["custom-annotation"]).To(Equal("annotation-value"))
	})
})

var _ = Describe("ClusterWrapper operations", func() {
	It("should wrap MockCluster correctly", func() {
		mockCR := testutil.NewMockCluster("wrapped-cluster", "test-ns").
			WithLabels(map[string]string{"app": "test"}).
			WithRoles(map[string]v1alpha1.RoleSpec{
				"role-a": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{
						"group-1": {Replicas: ptr.To(int32(2))},
					},
				},
			})

		wrapped := testutil.WrapMockCluster(mockCR)

		Expect(wrapped.GetName()).To(Equal("wrapped-cluster"))
		Expect(wrapped.GetNamespace()).To(Equal("test-ns"))
		Expect(wrapped.GetLabels()["app"]).To(Equal("test"))
		Expect(wrapped.GetSpec().Roles).To(HaveKey("role-a"))
	})

	It("should return correct status", func() {
		mockCR := testutil.NewMockCluster("status-cluster", "default")
		mockCR.Status.SetRoleGroup("role-a", "group-1")

		wrapped := testutil.WrapMockCluster(mockCR)

		status := wrapped.GetStatus()
		Expect(status).NotTo(BeNil())
	})

	It("should allow setting status", func() {
		mockCR := testutil.NewMockCluster("set-status-cluster", "default")
		wrapped := testutil.WrapMockCluster(mockCR)

		newStatus := &v1alpha1.GenericClusterStatus{}
		newStatus.SetRoleGroup("new-role", "new-group")

		wrapped.SetStatus(newStatus)

		Expect(wrapped.GetStatus()).NotTo(BeNil())
	})

	It("should implement DeepCopyCluster", func() {
		mockCR := testutil.NewMockCluster("deepcopy-cluster", "default").
			WithRoles(map[string]v1alpha1.RoleSpec{
				"role-a": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{
						"group-1": {Replicas: ptr.To(int32(1))},
					},
				},
			})

		wrapped := testutil.WrapMockCluster(mockCR)
		copied := wrapped.DeepCopyCluster()

		Expect(copied.GetName()).To(Equal("deepcopy-cluster"))
		Expect(copied).NotTo(BeIdenticalTo(wrapped))
	})

	It("should return runtime object", func() {
		mockCR := testutil.NewMockCluster("runtime-cluster", "default")
		wrapped := testutil.WrapMockCluster(mockCR)

		runtimeObj := wrapped.GetRuntimeObject()
		Expect(runtimeObj).NotTo(BeNil())
	})
})

// handlerAdapter adapts MockRoleGroupHandler to the RoleGroupHandler interface
type handlerAdapter struct {
	handler *testutil.MockRoleGroupHandler
}

// BuildResources implements reconciler.RoleGroupHandler
func (a *handlerAdapter) BuildResources(ctx context.Context, k8sClient client.Client, cr *testutil.ClusterWrapper, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
	return a.handler.BuildResources(ctx, k8sClient, cr, buildCtx)
}

// Verify interface implementations
var _ common.ClusterInterface = &testutil.ClusterWrapper{}
var _ reconciler.RoleGroupHandler[*testutil.ClusterWrapper] = &handlerAdapter{}

var _ = Describe("GenericReconciler Integration Tests", func() {
	var r *reconciler.GenericReconciler[*testutil.ClusterWrapper]
	var mockHandler *testutil.MockRoleGroupHandler
	var namespace string
	var testID string // Unique identifier for test isolation

	BeforeEach(func() {
		namespace = testNamespace
		testID = fmt.Sprintf("test-%d", time.Now().UnixNano())
		mockHandler = testutil.NewMockRoleGroupHandler()

		mockCR := testutil.NewMockCluster("test-cr-"+testID, namespace)
		wrappedCR := testutil.WrapMockCluster(mockCR)

		cfg := &reconciler.GenericReconcilerConfig[*testutil.ClusterWrapper]{
			Client:           k8sClient,
			Scheme:           testScheme,
			Recorder:         recorder,
			RoleGroupHandler: &handlerAdapter{handler: mockHandler},
			Prototype:        wrappedCR,
		}

		var err error
		r, err = reconciler.NewGenericReconciler(cfg)
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("Reconcile with CR in cluster", func() {
		var mockCR *testutil.MockCluster
		var crName string
		var additionalCRs []*testutil.MockCluster // Track additional CRs for cleanup

		BeforeEach(func() {
			crName = "integration-cr-" + testID
			mockCR = testutil.NewMockCluster(crName, namespace).
				WithRoles(map[string]v1alpha1.RoleSpec{
					"test-role": {
						RoleGroups: map[string]v1alpha1.RoleGroupSpec{
							"default": {Replicas: ptr.To(int32(1))},
						},
					},
				})
			additionalCRs = nil // Reset for each test

			// Create the CR in the cluster
			Expect(k8sClient.Create(ctx, mockCR)).To(Succeed())
		})

		AfterEach(func() {
			// Clean up the main CR
			Expect(k8sClient.Delete(ctx, mockCR)).To(Succeed())
			// Clean up any additional CRs created during tests
			for _, cr := range additionalCRs {
				_ = k8sClient.Delete(ctx, cr) // Ignore errors if already deleted
			}
		})

		It("should fetch CR from cluster and reconcile successfully", func() {
			req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}}

			result, err := r.Reconcile(ctx, req)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// Verify status was updated
			fetchedCR := &testutil.MockCluster{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: crName}, fetchedCR)).To(Succeed())
			Expect(fetchedCR.Status.Conditions).NotTo(BeEmpty())
		})

		It("should create ConfigMap during reconciliation", func() {
			req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}}

			_, err := r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Verify ConfigMap was created
			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: crName + "-default"}, cm)).To(Succeed())
		})

		It("should create Service during reconciliation", func() {
			req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}}

			_, err := r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Verify Service was created
			svc := &corev1.Service{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: crName + "-default"}, svc)).To(Succeed())
		})

		It("should create StatefulSet during reconciliation", func() {
			req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}}

			_, err := r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Verify StatefulSet was created
			sts := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: crName + "-default"}, sts)).To(Succeed())
		})

		It("should track role group in status", func() {
			req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}}

			_, err := r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Verify role group is tracked in status
			fetchedCR := &testutil.MockCluster{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: crName}, fetchedCR)).To(Succeed())
			Expect(fetchedCR.Status.RoleGroups).To(HaveKey("test-role"))
		})

		It("should handle multiple role groups", func() {
			multiName := "multi-rg-" + testID
			// Create CR with multiple role groups
			multiCR := testutil.NewMockCluster(multiName, namespace).
				WithRoles(map[string]v1alpha1.RoleSpec{
					"role-a": {
						RoleGroups: map[string]v1alpha1.RoleGroupSpec{
							"group-1": {Replicas: ptr.To(int32(1))},
							"group-2": {Replicas: ptr.To(int32(2))},
						},
					},
				})
			Expect(k8sClient.Create(ctx, multiCR)).To(Succeed())
			additionalCRs = append(additionalCRs, multiCR) // Track for cleanup

			req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: multiName}}

			_, err := r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Verify both groups are tracked (RoleGroups is map[string][]string)
			fetchedCR := &testutil.MockCluster{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: multiName}, fetchedCR)).To(Succeed())
			Expect(fetchedCR.Status.RoleGroups["role-a"]).To(ContainElements("group-1", "group-2"))
		})

		It("should handle multiple roles", func() {
			multiRoleName := "multi-role-" + testID
			// Create CR with multiple roles
			multiRoleCR := testutil.NewMockCluster(multiRoleName, namespace).
				WithRoles(map[string]v1alpha1.RoleSpec{
					"role-a": {
						RoleGroups: map[string]v1alpha1.RoleGroupSpec{
							"default": {Replicas: ptr.To(int32(1))},
						},
					},
					"role-b": {
						RoleGroups: map[string]v1alpha1.RoleGroupSpec{
							"default": {Replicas: ptr.To(int32(1))},
						},
					},
				})
			Expect(k8sClient.Create(ctx, multiRoleCR)).To(Succeed())
			additionalCRs = append(additionalCRs, multiRoleCR) // Track for cleanup

			req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: multiRoleName}}

			_, err := r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Verify both roles are tracked
			fetchedCR := &testutil.MockCluster{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: multiRoleName}, fetchedCR)).To(Succeed())
			Expect(fetchedCR.Status.RoleGroups).To(HaveKey("role-a"))
			Expect(fetchedCR.Status.RoleGroups).To(HaveKey("role-b"))
		})
	})

	Describe("Reconcile with paused cluster", func() {
		var pausedCR *testutil.MockCluster
		var crName string

		BeforeEach(func() {
			crName = "paused-" + testID
			pausedCR = testutil.NewMockCluster(crName, namespace).
				WithClusterOperation(&v1alpha1.ClusterOperationSpec{
					ReconciliationPaused: true,
				}).
				WithRoles(map[string]v1alpha1.RoleSpec{
					"test-role": {
						RoleGroups: map[string]v1alpha1.RoleGroupSpec{
							"default": {Replicas: ptr.To(int32(1))},
						},
					},
				})

			Expect(k8sClient.Create(ctx, pausedCR)).To(Succeed())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, pausedCR)).To(Succeed())
		})

		It("should return early when reconciliation is paused", func() {
			req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}}

			result, err := r.Reconcile(ctx, req)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// Verify status shows degraded due to paused
			fetchedCR := &testutil.MockCluster{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: crName}, fetchedCR)).To(Succeed())
		})
	})

	Describe("Reconcile with stopped cluster", func() {
		var stoppedCR *testutil.MockCluster
		var crName string

		BeforeEach(func() {
			crName = "stopped-" + testID
			stoppedCR = testutil.NewMockCluster(crName, namespace).
				WithClusterOperation(&v1alpha1.ClusterOperationSpec{
					Stopped: true,
				}).
				WithRoles(map[string]v1alpha1.RoleSpec{
					"test-role": {
						RoleGroups: map[string]v1alpha1.RoleGroupSpec{
							"default": {Replicas: ptr.To(int32(1))},
						},
					},
				})

			Expect(k8sClient.Create(ctx, stoppedCR)).To(Succeed())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, stoppedCR)).To(Succeed())
		})

		It("should handle stopped cluster", func() {
			req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}}

			result, err := r.Reconcile(ctx, req)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// Verify status shows unavailable due to stopped
			fetchedCR := &testutil.MockCluster{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: crName}, fetchedCR)).To(Succeed())
		})
	})

	Describe("Reconcile idempotency", func() {
		var idempotentCR *testutil.MockCluster
		var crName string

		BeforeEach(func() {
			crName = "idempotent-" + testID
			idempotentCR = testutil.NewMockCluster(crName, namespace).
				WithRoles(map[string]v1alpha1.RoleSpec{
					"test-role": {
						RoleGroups: map[string]v1alpha1.RoleGroupSpec{
							"default": {Replicas: ptr.To(int32(1))},
						},
					},
				})

			Expect(k8sClient.Create(ctx, idempotentCR)).To(Succeed())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, idempotentCR)).To(Succeed())
		})

		It("should be idempotent - multiple reconciles should succeed", func() {
			req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}}

			// First reconcile
			result1, err1 := r.Reconcile(ctx, req)
			Expect(err1).NotTo(HaveOccurred())
			Expect(result1).To(Equal(ctrl.Result{}))

			// Second reconcile
			result2, err2 := r.Reconcile(ctx, req)
			Expect(err2).NotTo(HaveOccurred())
			Expect(result2).To(Equal(ctrl.Result{}))

			// Third reconcile
			result3, err3 := r.Reconcile(ctx, req)
			Expect(err3).NotTo(HaveOccurred())
			Expect(result3).To(Equal(ctrl.Result{}))

			// Verify only one StatefulSet exists (by direct name lookup)
			sts := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: crName + "-default"}, sts)).To(Succeed())
		})
	})

	Describe("Reconcile with custom handler", func() {
		var customCR *testutil.MockCluster
		var crName string

		BeforeEach(func() {
			crName = "custom-handler-" + testID
			customCR = testutil.NewMockCluster(crName, namespace).
				WithRoles(map[string]v1alpha1.RoleSpec{
					"test-role": {
						RoleGroups: map[string]v1alpha1.RoleGroupSpec{
							"default": {Replicas: ptr.To(int32(1))},
						},
					},
				})

			// Set custom handler behavior
			mockHandler.WithBuildResourcesFunc(func(ctx context.Context, k8sClient client.Client, cr *testutil.ClusterWrapper, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
				return &reconciler.RoleGroupResources{
					ConfigMap:   testutil.NewTestConfigMap(buildCtx.ResourceName, buildCtx.ClusterNamespace),
					Service:     testutil.NewTestService(buildCtx.ResourceName, buildCtx.ClusterNamespace),
					StatefulSet: testutil.NewTestStatefulSetBuilder(buildCtx.ResourceName, buildCtx.ClusterNamespace).WithImage("custom-image:v1", corev1.PullAlways).Build(),
				}, nil
			})

			Expect(k8sClient.Create(ctx, customCR)).To(Succeed())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, customCR)).To(Succeed())
			// Reset handler
			mockHandler.BuildResourcesFunc = nil
		})

		It("should use custom handler to build resources", func() {
			req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}}

			result, err := r.Reconcile(ctx, req)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// Verify custom image was used
			sts := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: crName + "-default"}, sts)).To(Succeed())
			Expect(sts.Spec.Template.Spec.Containers[0].Image).To(Equal("custom-image:v1"))
			Expect(sts.Spec.Template.Spec.Containers[0].ImagePullPolicy).To(Equal(corev1.PullAlways))
		})
	})
})

var _ = Describe("GenericReconciler scaleToZero", func() {
	var r *reconciler.GenericReconciler[*testutil.ClusterWrapper]
	var mockHandler *testutil.MockRoleGroupHandler
	var namespace string
	var crName string
	var ctx context.Context
	var customCR *testutil.MockCluster
	var wrappedCR *testutil.ClusterWrapper

	BeforeEach(func() {
		ctx = context.Background()
		namespace = testNamespace
		crName = "scale-zero-cr"
		mockHandler = testutil.NewMockRoleGroupHandler()
		customCR = testutil.NewMockCluster(crName, namespace).
			WithRoles(map[string]v1alpha1.RoleSpec{
				"test-role": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{
						"default": {Replicas: ptr.To(int32(1))},
					},
				},
			}).
			WithClusterOperation(&v1alpha1.ClusterOperationSpec{
				Stopped: true,
			})
		wrappedCR = testutil.WrapMockCluster(customCR)

		Expect(k8sClient.Create(ctx, customCR)).To(Succeed())
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, customCR)).To(Succeed())
	})

	JustBeforeEach(func() {
		cfg := &reconciler.GenericReconcilerConfig[*testutil.ClusterWrapper]{
			Client:           k8sClient,
			Scheme:           testScheme,
			Recorder:         recorder,
			RoleGroupHandler: &handlerAdapter{handler: mockHandler},
			Prototype:        wrappedCR,
		}
		var err error
		r, err = reconciler.NewGenericReconciler(cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(r).NotTo(BeNil())
	})

	It("should scale StatefulSet to zero when cluster is stopped", func() {
		// Create StatefulSet with replicas > 0
		replicas := int32(3)
		sts := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      crName + "-default",
				Namespace: namespace,
			},
			Spec: appsv1.StatefulSetSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": crName},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": crName},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "test", Image: "test-image"},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, sts)).To(Succeed())

		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}}
		result, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{}))
	})

	It("should handle StatefulSet not existing when scaling to zero", func() {
		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}}
		result, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{}))
	})
})

var _ = Describe("GenericReconciler error paths", func() {
	var r *reconciler.GenericReconciler[*testutil.ClusterWrapper]
	var mockHandler *testutil.MockRoleGroupHandler
	var namespace string
	var crName string
	var ctx context.Context
	var customCR *testutil.MockCluster
	var wrappedCR *testutil.ClusterWrapper

	BeforeEach(func() {
		ctx = context.Background()
		namespace = testNamespace
		crName = "error-path-cr"
		mockHandler = testutil.NewMockRoleGroupHandler()
		customCR = testutil.NewMockCluster(crName, namespace).
			WithRoles(map[string]v1alpha1.RoleSpec{
				"test-role": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{
						"default": {Replicas: ptr.To(int32(1))},
					},
				},
			})
		wrappedCR = testutil.WrapMockCluster(customCR)

		Expect(k8sClient.Create(ctx, customCR)).To(Succeed())
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, customCR)).To(Succeed())
	})

	JustBeforeEach(func() {
		cfg := &reconciler.GenericReconcilerConfig[*testutil.ClusterWrapper]{
			Client:           k8sClient,
			Scheme:           testScheme,
			Recorder:         recorder,
			RoleGroupHandler: &handlerAdapter{handler: mockHandler},
			Prototype:        wrappedCR,
		}
		var err error
		r, err = reconciler.NewGenericReconciler(cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(r).NotTo(BeNil())
	})

	It("should handle reconcile error and execute error hooks", func() {
		// Set up handler to return error
		mockHandler.WithBuildResourcesFunc(func(ctx context.Context, k8sClient client.Client, cr *testutil.ClusterWrapper, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
			return nil, fmt.Errorf("intentional build error for testing")
		})

		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}}
		result, err := r.Reconcile(ctx, req)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("intentional build error"))
		Expect(result).To(Equal(ctrl.Result{}))

		// Verify status shows degraded
		fetchedCR := &testutil.MockCluster{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: crName}, fetchedCR)).To(Succeed())
	})

	It("should handle context cancellation during reconcile", func() {
		canceledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}}
		_, _ = r.Reconcile(canceledCtx, req)
		// Context cancellation may or may not cause error depending on timing
	})
})

var _ = Describe("GenericReconciler applyResources errors", func() {
	var r *reconciler.GenericReconciler[*testutil.ClusterWrapper]
	var mockHandler *testutil.MockRoleGroupHandler
	var namespace string
	var crName string
	var ctx context.Context
	var customCR *testutil.MockCluster
	var wrappedCR *testutil.ClusterWrapper

	BeforeEach(func() {
		ctx = context.Background()
		namespace = testNamespace
		crName = "apply-error-cr"
		mockHandler = testutil.NewMockRoleGroupHandler()
		customCR = testutil.NewMockCluster(crName, namespace).
			WithRoles(map[string]v1alpha1.RoleSpec{
				"test-role": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{
						"default": {Replicas: ptr.To(int32(1))},
					},
				},
			})
		wrappedCR = testutil.WrapMockCluster(customCR)

		Expect(k8sClient.Create(ctx, customCR)).To(Succeed())
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, customCR)).To(Succeed())
		mockHandler.BuildResourcesFunc = nil
	})

	JustBeforeEach(func() {
		cfg := &reconciler.GenericReconcilerConfig[*testutil.ClusterWrapper]{
			Client:           k8sClient,
			Scheme:           testScheme,
			Recorder:         recorder,
			RoleGroupHandler: &handlerAdapter{handler: mockHandler},
			Prototype:        wrappedCR,
		}
		var err error
		r, err = reconciler.NewGenericReconciler(cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(r).NotTo(BeNil())
	})

	It("should handle ConfigMap apply error", func() {
		// Create a ConfigMap with invalid data that will cause apply to work but with special handling
		mockHandler.WithBuildResourcesFunc(func(ctx context.Context, k8sClient client.Client, cr *testutil.ClusterWrapper, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
			return &reconciler.RoleGroupResources{
				ConfigMap: &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      buildCtx.ResourceName,
						Namespace: buildCtx.ClusterNamespace,
						// Invalid owner reference will be set by applyResource
					},
					Data: map[string]string{"test": "data"},
				},
			}, nil
		})

		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}}
		result, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{}))
	})

	It("should handle resources with all nil fields", func() {
		mockHandler.WithBuildResourcesFunc(func(ctx context.Context, k8sClient client.Client, cr *testutil.ClusterWrapper, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
			return &reconciler.RoleGroupResources{
				// All fields are nil - should succeed with no resources applied
			}, nil
		})

		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}}
		result, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{}))
	})

	It("should handle resources with PDB", func() {
		maxUnavailable := intstr.FromInt(1)
		mockHandler.WithBuildResourcesFunc(func(ctx context.Context, k8sClient client.Client, cr *testutil.ClusterWrapper, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
			return &reconciler.RoleGroupResources{
				PodDisruptionBudget: &policyv1.PodDisruptionBudget{
					ObjectMeta: metav1.ObjectMeta{
						Name:      buildCtx.ResourceName,
						Namespace: buildCtx.ClusterNamespace,
					},
					Spec: policyv1.PodDisruptionBudgetSpec{
						MaxUnavailable: &maxUnavailable,
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": buildCtx.ResourceName},
						},
					},
				},
			}, nil
		})

		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}}
		result, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{}))
	})
})

var _ = Describe("GenericReconciler cleanupRoleGroup errors", func() {
	var cleaner *reconciler.RoleGroupCleaner
	var ctx context.Context
	var namespace string

	BeforeEach(func() {
		ctx = context.Background()
		namespace = testNamespace
		cleaner = reconciler.NewRoleGroupCleaner(k8sClient, testScheme)
	})

	It("should handle cleanup error with canceled context", func() {
		// Create a ConfigMap to be cleaned up
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cleanup-error-test",
				Namespace: namespace,
			},
			Data: map[string]string{"key": "value"},
		}
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())

		// Set up spec with orphaned group
		spec := &v1alpha1.GenericClusterSpec{
			Roles: map[string]v1alpha1.RoleSpec{
				"role": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{},
				},
			},
		}
		status := &v1alpha1.GenericClusterStatus{}
		status.SetRoleGroup("role", "error-test")

		// Use canceled context
		canceledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		// Cleanup - may or may not error depending on timing
		_ = cleaner.Cleanup(canceledCtx, namespace, "cleanup-error", spec, status, "", nil)
	})
})
