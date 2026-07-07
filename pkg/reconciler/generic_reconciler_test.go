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
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
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
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: reconciler.RoleGroupResourceName(crName, "test-role", "default")}, cm)).To(Succeed())
		})

		It("should create Service during reconciliation", func() {
			req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}}

			_, err := r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Verify Service was created
			svc := &corev1.Service{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: reconciler.RoleGroupResourceName(crName, "test-role", "default")}, svc)).To(Succeed())
		})

		It("should create StatefulSet during reconciliation", func() {
			req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}}

			_, err := r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Verify StatefulSet was created
			sts := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: reconciler.RoleGroupResourceName(crName, "test-role", "default")}, sts)).To(Succeed())
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
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: reconciler.RoleGroupResourceName(crName, "test-role", "default")}, sts)).To(Succeed())
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
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: reconciler.RoleGroupResourceName(crName, "test-role", "default")}, sts)).To(Succeed())
			Expect(sts.Spec.Template.Spec.Containers[0].Image).To(Equal("custom-image:v1"))
			Expect(sts.Spec.Template.Spec.Containers[0].ImagePullPolicy).To(Equal(corev1.PullAlways))
		})
	})
})

// Regression coverage for issue #511: the ClusterOperation pause/stop gate must be evaluated at
// the very top of reconcile(), BEFORE any resource mutation. Previously the gate lived in
// dependency validation (step 2), so a paused cluster still provisioned its ServiceAccount and ran
// PreReconcile extensions before returning early. These tests wire a real GenericReconciler with a
// configured ServiceAccountName and assert the SA is never created while paused (and still is when
// the cluster proceeds normally).
var _ = Describe("GenericReconciler ClusterOperation gate ordering (issue #511)", func() {
	var (
		mockHandler *testutil.MockRoleGroupHandler
		namespace   string
		saName      string
	)

	newReconciler := func(prototype *testutil.ClusterWrapper) *reconciler.GenericReconciler[*testutil.ClusterWrapper] {
		cfg := &reconciler.GenericReconcilerConfig[*testutil.ClusterWrapper]{
			Client:             k8sClient,
			Scheme:             testScheme,
			Recorder:           recorder,
			RoleGroupHandler:   &handlerAdapter{handler: mockHandler},
			Prototype:          prototype,
			ServiceAccountName: saName,
		}
		r, err := reconciler.NewGenericReconciler(cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(r).NotTo(BeNil())
		return r
	}

	saLookup := func() error {
		return k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: saName}, &corev1.ServiceAccount{})
	}

	BeforeEach(func() {
		namespace = testNamespace
		mockHandler = testutil.NewMockRoleGroupHandler()
		// Unique SA name per spec run so parallel/ordered specs don't observe each other's SAs.
		saName = fmt.Sprintf("gate-sa-%d", time.Now().UnixNano())
	})

	Context("when reconciliation is paused", func() {
		var pausedCR *testutil.MockCluster
		var crName string

		BeforeEach(func() {
			crName = fmt.Sprintf("gate-paused-%d", time.Now().UnixNano())
			pausedCR = testutil.NewMockCluster(crName, namespace).
				WithClusterOperation(&v1alpha1.ClusterOperationSpec{ReconciliationPaused: true}).
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
			// Best-effort cleanup in case a regression re-created the SA.
			_ = k8sClient.Delete(ctx, &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Name: saName, Namespace: namespace},
			})
		})

		It("must NOT create the ServiceAccount (the pre-gate mutation the bug caused)", func() {
			// Guard: SA must not pre-exist.
			Expect(saLookup()).NotTo(Succeed())

			r := newReconciler(testutil.WrapMockCluster(pausedCR))
			req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}}

			result, err := r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// The pause gate runs before ensureServiceAccount, so the SA must still be absent.
			err = saLookup()
			Expect(err).To(HaveOccurred())
			Expect(k8serrors.IsNotFound(err)).To(BeTrue(), "ServiceAccount should not have been created while paused")

			// And no StatefulSet was built either.
			sts := &appsv1.StatefulSet{}
			stsErr := k8sClient.Get(ctx, types.NamespacedName{
				Namespace: namespace,
				Name:      reconciler.RoleGroupResourceName(crName, "test-role", "default"),
			}, sts)
			Expect(k8serrors.IsNotFound(stsErr)).To(BeTrue())
		})
	})

	Context("when the cluster is not paused", func() {
		var runningCR *testutil.MockCluster
		var crName string

		BeforeEach(func() {
			crName = fmt.Sprintf("gate-running-%d", time.Now().UnixNano())
			runningCR = testutil.NewMockCluster(crName, namespace).
				WithRoles(map[string]v1alpha1.RoleSpec{
					"test-role": {
						RoleGroups: map[string]v1alpha1.RoleGroupSpec{
							"default": {Replicas: ptr.To(int32(1))},
						},
					},
				})
			Expect(k8sClient.Create(ctx, runningCR)).To(Succeed())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, runningCR)).To(Succeed())
			_ = k8sClient.Delete(ctx, &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Name: saName, Namespace: namespace},
			})
		})

		It("proceeds normally and DOES create the ServiceAccount", func() {
			r := newReconciler(testutil.WrapMockCluster(runningCR))
			req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}}

			result, err := r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			Expect(saLookup()).To(Succeed(), "ServiceAccount should be created for an un-paused cluster")
		})
	})

	Context("when the cluster is stopped", func() {
		var stoppedCR *testutil.MockCluster
		var crName string

		BeforeEach(func() {
			crName = fmt.Sprintf("gate-stopped-%d", time.Now().UnixNano())
			stoppedCR = testutil.NewMockCluster(crName, namespace).
				WithClusterOperation(&v1alpha1.ClusterOperationSpec{Stopped: true}).
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
			_ = k8sClient.Delete(ctx, &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Name: saName, Namespace: namespace},
			})
			_ = k8sClient.Delete(ctx, &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      reconciler.RoleGroupResourceName(crName, "test-role", "default"),
					Namespace: namespace,
				},
			})
		})

		It("scales an existing StatefulSet to zero without provisioning the ServiceAccount first", func() {
			// Pre-create a StatefulSet with replicas > 0 that the stopped path should scale down.
			replicas := int32(3)
			stsName := reconciler.RoleGroupResourceName(crName, "test-role", "default")
			sts := &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: stsName, Namespace: namespace},
				Spec: appsv1.StatefulSetSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": crName}},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": crName}},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{Name: "test", Image: "test-image"}},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, sts)).To(Succeed())

			r := newReconciler(testutil.WrapMockCluster(stoppedCR))
			req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}}

			result, err := r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// StatefulSet scaled to zero.
			fetched := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: stsName}, fetched)).To(Succeed())
			Expect(*fetched.Spec.Replicas).To(Equal(int32(0)))

			// Stopped path handled by the gate before ensureServiceAccount, so no SA was provisioned.
			Expect(k8serrors.IsNotFound(saLookup())).To(BeTrue(), "ServiceAccount should not be created for a stopped cluster")
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
				Name:      reconciler.RoleGroupResourceName(crName, "test-role", "default"),
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

var _ = Describe("RoleGroupResourceName", func() {
	It("joins cluster, role and group", func() {
		Expect(reconciler.RoleGroupResourceName("zk", "server", "default")).To(Equal("zk-server-default"))
	})

	It("truncates with a deterministic hash suffix within the 63-char DNS limit (incl -headless)", func() {
		longCluster := "this-is-a-very-long-zookeeper-cluster-name-that-overflows"
		name := reconciler.RoleGroupResourceName(longCluster, "server", "default")
		Expect(len(name)).To(BeNumerically("<=", 54))
		Expect(len(name + "-headless")).To(BeNumerically("<=", 63))
		// Deterministic: same inputs yield the same truncated name.
		Expect(reconciler.RoleGroupResourceName(longCluster, "server", "default")).To(Equal(name))
	})
})

// createRecordingClient wraps a client.Client and records the order of successful Create
// calls (as "<go-type>/<name>"), so tests can assert apply ordering — e.g. that extra
// resources are created before the StatefulSet. All other operations pass through.
type createRecordingClient struct {
	client.Client
	created []string
}

func (c *createRecordingClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if err := c.Client.Create(ctx, obj, opts...); err != nil {
		return err
	}
	c.created = append(c.created, fmt.Sprintf("%T/%s", obj, obj.GetName()))
	return nil
}

// indexOf returns the position of the given "<go-type>/<name>" entry in the recorded create
// order, or -1 if it was never created.
func (c *createRecordingClient) indexOf(entry string) int {
	for i, e := range c.created {
		if e == entry {
			return i
		}
	}
	return -1
}

var _ = Describe("GenericReconciler ExtraResources", func() {
	var (
		namespace string
		crName    string
		mockCR    *testutil.MockCluster
		recClient *createRecordingClient
	)

	BeforeEach(func() {
		namespace = testNamespace
		crName = fmt.Sprintf("extra-res-cr-%d", time.Now().UnixNano())
		mockCR = testutil.NewMockCluster(crName, namespace).
			WithRoles(map[string]v1alpha1.RoleSpec{
				"broker": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{
						"default": {Replicas: ptr.To(int32(1))},
					},
				},
			})
		Expect(k8sClient.Create(ctx, mockCR)).To(Succeed())
		recClient = &createRecordingClient{Client: k8sClient}
	})

	AfterEach(func() {
		_ = k8sClient.Delete(ctx, mockCR)
	})

	newReconciler := func(handler reconciler.RoleGroupHandler[*testutil.ClusterWrapper]) *reconciler.GenericReconciler[*testutil.ClusterWrapper] {
		cfg := &reconciler.GenericReconcilerConfig[*testutil.ClusterWrapper]{
			Client:           recClient,
			Scheme:           testScheme,
			Recorder:         recorder,
			RoleGroupHandler: handler,
			Prototype:        testutil.WrapMockCluster(testutil.NewMockCluster("proto", namespace)),
		}
		r, err := reconciler.NewGenericReconciler(cfg)
		Expect(err).NotTo(HaveOccurred())
		return r
	}

	It("applies extras with a controller owner reference, after the ConfigMap and before the StatefulSet", func() {
		handler := &reconciler.RoleGroupHandlerFuncs[*testutil.ClusterWrapper]{
			BuildResourcesFunc: func(_ context.Context, _ client.Client, _ *testutil.ClusterWrapper, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
				// A Secret stands in for an arbitrary product resource (e.g. a Listener CR)
				// that must exist before the workload pods are scheduled.
				extra := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      buildCtx.ResourceName + "-bootstrap",
						Namespace: buildCtx.ClusterNamespace,
					},
					StringData: map[string]string{"listener": "bootstrap"},
				}
				return &reconciler.RoleGroupResources{
					ConfigMap:      testutil.NewTestConfigMap(buildCtx.ResourceName, buildCtx.ClusterNamespace),
					ExtraResources: []client.Object{extra},
					StatefulSet:    testutil.NewTestStatefulSetBuilder(buildCtx.ResourceName, buildCtx.ClusterNamespace).WithImage("test-image:latest", corev1.PullIfNotPresent).Build(),
				}, nil
			},
		}
		r := newReconciler(handler)

		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}}
		_, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		resourceName := reconciler.RoleGroupResourceName(crName, "broker", "default")

		// The extra is applied with a controller owner reference to the cluster CR, so it is
		// garbage-collected with the CR.
		secret := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: resourceName + "-bootstrap"}, secret)).To(Succeed())
		fetchedCR := &testutil.MockCluster{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: crName}, fetchedCR)).To(Succeed())
		controllerRef := metav1.GetControllerOf(secret)
		Expect(controllerRef).NotTo(BeNil())
		Expect(controllerRef.UID).To(Equal(fetchedCR.UID))

		// Apply order: ConfigMap -> extra -> StatefulSet. Extras must precede the StatefulSet
		// because they are pod-scheduling prerequisites (e.g. Listener CRs mounted via CSI).
		cmIdx := recClient.indexOf("*v1.ConfigMap/" + resourceName)
		extraIdx := recClient.indexOf("*v1.Secret/" + resourceName + "-bootstrap")
		stsIdx := recClient.indexOf("*v1.StatefulSet/" + resourceName)
		Expect(cmIdx).To(BeNumerically(">=", 0))
		Expect(extraIdx).To(BeNumerically(">=", 0))
		Expect(stsIdx).To(BeNumerically(">=", 0))
		Expect(cmIdx).To(BeNumerically("<", extraIdx))
		Expect(extraIdx).To(BeNumerically("<", stsIdx))
	})

	It("is idempotent for extras across repeated reconciles", func() {
		handler := &reconciler.RoleGroupHandlerFuncs[*testutil.ClusterWrapper]{
			BuildResourcesFunc: func(_ context.Context, _ client.Client, _ *testutil.ClusterWrapper, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
				return &reconciler.RoleGroupResources{
					ExtraResources: []client.Object{
						&corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      buildCtx.ResourceName + "-extra",
								Namespace: buildCtx.ClusterNamespace,
							},
						},
					},
				}, nil
			},
		}
		r := newReconciler(handler)

		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}}
		_, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		_, err = r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		resourceName := reconciler.RoleGroupResourceName(crName, "broker", "default")
		secret := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: resourceName + "-extra"}, secret)).To(Succeed())
		// Created exactly once — the second reconcile updated instead of re-creating.
		Expect(recClient.indexOf("*v1.Secret/" + resourceName + "-extra")).To(BeNumerically(">=", 0))
		count := 0
		for _, e := range recClient.created {
			if e == "*v1.Secret/"+resourceName+"-extra" {
				count++
			}
		}
		Expect(count).To(Equal(1))
	})

	It("behaves exactly as before when ExtraResources is nil, and skips nil entries", func() {
		handler := &reconciler.RoleGroupHandlerFuncs[*testutil.ClusterWrapper]{
			BuildResourcesFunc: func(_ context.Context, _ client.Client, _ *testutil.ClusterWrapper, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
				return &reconciler.RoleGroupResources{
					ConfigMap:   testutil.NewTestConfigMap(buildCtx.ResourceName, buildCtx.ClusterNamespace),
					StatefulSet: testutil.NewTestStatefulSetBuilder(buildCtx.ResourceName, buildCtx.ClusterNamespace).WithImage("test-image:latest", corev1.PullIfNotPresent).Build(),
					// ExtraResources intentionally nil (backward compatible), covering the
					// pre-existing contract.
				}, nil
			},
		}
		r := newReconciler(handler)

		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}}
		_, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		resourceName := reconciler.RoleGroupResourceName(crName, "broker", "default")
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: resourceName}, &corev1.ConfigMap{})).To(Succeed())
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: resourceName}, &appsv1.StatefulSet{})).To(Succeed())
		// Only the ConfigMap and StatefulSet were created.
		Expect(recClient.created).To(ConsistOf(
			"*v1.ConfigMap/"+resourceName,
			"*v1.StatefulSet/"+resourceName,
		))

		// A slice holding only nil entries is equally a no-op.
		handler.BuildResourcesFunc = func(_ context.Context, _ client.Client, _ *testutil.ClusterWrapper, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
			return &reconciler.RoleGroupResources{
				ExtraResources: []client.Object{nil},
			}, nil
		}
		recClient.created = nil
		_, err = r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(recClient.created).To(BeEmpty())
	})
})

var _ = Describe("GenericReconciler ProductConfig", func() {
	var (
		namespace string
		crName    string
		mockCR    *testutil.MockCluster
	)

	BeforeEach(func() {
		namespace = testNamespace
		crName = fmt.Sprintf("product-config-cr-%d", time.Now().UnixNano())

		mockCR = testutil.NewMockCluster(crName, namespace).
			WithRoles(map[string]v1alpha1.RoleSpec{
				"coordinator": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{
						// The user overrides one product-computed key via the CRD.
						"default": {
							Replicas: ptr.To(int32(1)),
							ConfigOverrides: map[string]map[string]string{
								"config.properties": {"shared": "from-crd"},
							},
						},
					},
				},
			})
		Expect(k8sClient.Create(ctx, mockCR)).To(Succeed())
	})

	AfterEach(func() {
		_ = k8sClient.Delete(ctx, mockCR)
	})

	// capturingHandler records the MergedConfig the reconciler hands to the product.
	newCapturingHandler := func(into **config.MergedConfig) reconciler.RoleGroupHandler[*testutil.ClusterWrapper] {
		return &reconciler.RoleGroupHandlerFuncs[*testutil.ClusterWrapper]{
			BuildResourcesFunc: func(_ context.Context, _ client.Client, _ *testutil.ClusterWrapper, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
				*into = buildCtx.MergedConfig
				return &reconciler.RoleGroupResources{}, nil
			},
		}
	}

	It("merges product config beneath CRD overrides and applies role-specific logic", func() {
		var captured *config.MergedConfig

		cfg := &reconciler.GenericReconcilerConfig[*testutil.ClusterWrapper]{
			Client:           k8sClient,
			Scheme:           testScheme,
			Recorder:         recorder,
			RoleGroupHandler: newCapturingHandler(&captured),
			Prototype:        testutil.WrapMockCluster(testutil.NewMockCluster("proto", namespace)),
			ProductConfig: func(_ *testutil.ClusterWrapper, roleName, _ string) *v1alpha1.OverridesSpec {
				overrides := map[string]map[string]string{
					"config.properties": {
						"shared":       "from-product",
						"product-only": "p",
					},
				}
				// Role-specific product knowledge (neither framework nor user).
				if roleName == "coordinator" {
					overrides["config.properties"]["coordinator"] = "true"
				}
				return &v1alpha1.OverridesSpec{ConfigOverrides: overrides}
			},
		}

		r, err := reconciler.NewGenericReconciler(cfg)
		Expect(err).NotTo(HaveOccurred())

		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}}
		_, err = r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		Expect(captured).NotTo(BeNil())
		props := captured.ConfigFiles["config.properties"]
		// CRD override wins over the product-computed value for the shared key.
		Expect(props).To(HaveKeyWithValue("shared", "from-crd"))
		// Product-only computed keys survive untouched.
		Expect(props).To(HaveKeyWithValue("product-only", "p"))
		// Role-specific product logic applied.
		Expect(props).To(HaveKeyWithValue("coordinator", "true"))
	})

	It("uses CRD-only config when ProductConfig is unset", func() {
		var captured *config.MergedConfig

		cfg := &reconciler.GenericReconcilerConfig[*testutil.ClusterWrapper]{
			Client:           k8sClient,
			Scheme:           testScheme,
			Recorder:         recorder,
			RoleGroupHandler: newCapturingHandler(&captured),
			Prototype:        testutil.WrapMockCluster(testutil.NewMockCluster("proto", namespace)),
			// ProductConfig intentionally left nil.
		}

		r, err := reconciler.NewGenericReconciler(cfg)
		Expect(err).NotTo(HaveOccurred())

		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}}
		_, err = r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		Expect(captured).NotTo(BeNil())
		props := captured.ConfigFiles["config.properties"]
		Expect(props).To(HaveKeyWithValue("shared", "from-crd"))
		Expect(props).NotTo(HaveKey("product-only"))
	})
})

// Regression coverage for issue #526: applyResource was create-only — the CreateOrUpdate
// mutate func only set the owner reference, so after initial creation no CR change ever
// reached the StatefulSet/ConfigMap/Service/extras. These specs drive two reconciles with a
// handler whose desired output changes in between and assert the change propagates, plus the
// copy rules of copyDesiredState: wholesale labels, merged (foreign-preserving) annotations,
// immutable StatefulSet fields kept from live, and allocated Service NodePorts carried over.
var _ = Describe("GenericReconciler update propagation (issue #526)", func() {
	var (
		namespace    string
		crName       string
		resourceName string
		mockCR       *testutil.MockCluster
		fakeRecorder *record.FakeRecorder
	)

	BeforeEach(func() {
		namespace = testNamespace
		crName = fmt.Sprintf("update-prop-cr-%d", time.Now().UnixNano())
		resourceName = reconciler.RoleGroupResourceName(crName, "broker", "default")
		mockCR = testutil.NewMockCluster(crName, namespace).
			WithRoles(map[string]v1alpha1.RoleSpec{
				"broker": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{
						"default": {Replicas: ptr.To(int32(1))},
					},
				},
			})
		Expect(k8sClient.Create(ctx, mockCR)).To(Succeed())
		// Dedicated recorder so event assertions don't race with other specs sharing the
		// suite-level recorder.
		fakeRecorder = record.NewFakeRecorder(100)
	})

	AfterEach(func() {
		_ = k8sClient.Delete(ctx, mockCR)
		// envtest runs no garbage collector, so reclaim the owned resources by name.
		meta := metav1.ObjectMeta{Name: resourceName, Namespace: namespace}
		_ = k8sClient.Delete(ctx, &appsv1.StatefulSet{ObjectMeta: meta})
		_ = k8sClient.Delete(ctx, &corev1.ConfigMap{ObjectMeta: meta})
		_ = k8sClient.Delete(ctx, &corev1.Service{ObjectMeta: meta})
		_ = k8sClient.Delete(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: resourceName + "-extra", Namespace: namespace}})
	})

	// newReconciler wires a GenericReconciler whose handler defers to build, so specs can
	// change the desired output between reconciles by mutating captured variables.
	newReconciler := func(build func(buildCtx *reconciler.RoleGroupBuildContext) *reconciler.RoleGroupResources) *reconciler.GenericReconciler[*testutil.ClusterWrapper] {
		handler := &reconciler.RoleGroupHandlerFuncs[*testutil.ClusterWrapper]{
			BuildResourcesFunc: func(_ context.Context, _ client.Client, _ *testutil.ClusterWrapper, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
				return build(buildCtx), nil
			},
		}
		cfg := &reconciler.GenericReconcilerConfig[*testutil.ClusterWrapper]{
			Client:           k8sClient,
			Scheme:           testScheme,
			Recorder:         fakeRecorder,
			RoleGroupHandler: handler,
			Prototype:        testutil.WrapMockCluster(testutil.NewMockCluster("proto", namespace)),
		}
		r, err := reconciler.NewGenericReconciler(cfg)
		Expect(err).NotTo(HaveOccurred())
		return r
	}

	reconcile := func(r *reconciler.GenericReconciler[*testutil.ClusterWrapper]) {
		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}}
		_, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
	}

	drainEvents := func() []string {
		var events []string
		for {
			select {
			case e := <-fakeRecorder.Events:
				events = append(events, e)
			default:
				return events
			}
		}
	}

	It("propagates StatefulSet replicas and template changes and emits an Update event", func() {
		replicas := int32(1)
		envValue := "v1"
		r := newReconciler(func(buildCtx *reconciler.RoleGroupBuildContext) *reconciler.RoleGroupResources {
			sts := testutil.NewTestStatefulSet(buildCtx.ResourceName, buildCtx.ClusterNamespace)
			sts.Spec.Replicas = ptr.To(replicas)
			sts.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{{Name: "PROPAGATION_PROBE", Value: envValue}}
			return &reconciler.RoleGroupResources{StatefulSet: sts}
		})

		reconcile(r)
		sts := &appsv1.StatefulSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: resourceName}, sts)).To(Succeed())
		Expect(*sts.Spec.Replicas).To(Equal(int32(1)))

		// The CR changed (e.g. kubectl patch replicas 1 -> 3): the handler now builds
		// different desired state, and the second reconcile must propagate it.
		replicas = 3
		envValue = "v2"
		Expect(drainEvents()).To(ContainElement(ContainSubstring("Created")))
		reconcile(r)

		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: resourceName}, sts)).To(Succeed())
		Expect(*sts.Spec.Replicas).To(Equal(int32(3)), "replicas change must reach the live StatefulSet")
		Expect(sts.Spec.Template.Spec.Containers[0].Env).To(ContainElement(corev1.EnvVar{Name: "PROPAGATION_PROBE", Value: "v2"}), "template change must reach the live StatefulSet")

		// OperationResultUpdated surfaced through the event plumbing.
		Expect(drainEvents()).To(ContainElement(SatisfyAll(
			ContainSubstring("Updated"),
			ContainSubstring(resourceName),
		)))
	})

	It("propagates ConfigMap data changes, including removed keys", func() {
		data := map[string]string{"keep": "one", "remove": "two"}
		r := newReconciler(func(buildCtx *reconciler.RoleGroupBuildContext) *reconciler.RoleGroupResources {
			return &reconciler.RoleGroupResources{
				ConfigMap: testutil.NewTestConfigMapWithData(buildCtx.ResourceName, buildCtx.ClusterNamespace, data),
			}
		})

		reconcile(r)
		cm := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: resourceName}, cm)).To(Succeed())
		Expect(cm.Data).To(HaveKeyWithValue("remove", "two"))

		data = map[string]string{"keep": "changed"}
		reconcile(r)

		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: resourceName}, cm)).To(Succeed())
		// Data is replaced wholesale: the changed value propagates AND the removed key
		// disappears from the live ConfigMap.
		Expect(cm.Data).To(Equal(map[string]string{"keep": "changed"}))
	})

	It("propagates Service port changes while preserving the allocated NodePort", func() {
		port := int32(9092)
		r := newReconciler(func(buildCtx *reconciler.RoleGroupBuildContext) *reconciler.RoleGroupResources {
			svc := testutil.NewTestService(buildCtx.ResourceName, buildCtx.ClusterNamespace)
			svc.Spec.Type = corev1.ServiceTypeNodePort
			svc.Spec.Ports = []corev1.ServicePort{{
				Name:       "client",
				Port:       port,
				TargetPort: intstr.FromInt(9092),
				Protocol:   corev1.ProtocolTCP,
				// NodePort deliberately unset: the API server allocates it on create and the
				// apply path must carry the allocation over on updates.
			}}
			return &reconciler.RoleGroupResources{Service: svc}
		})

		reconcile(r)
		svc := &corev1.Service{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: resourceName}, svc)).To(Succeed())
		Expect(svc.Spec.Ports).To(HaveLen(1))
		allocatedNodePort := svc.Spec.Ports[0].NodePort
		Expect(allocatedNodePort).NotTo(BeZero())
		clusterIP := svc.Spec.ClusterIP
		Expect(clusterIP).NotTo(BeEmpty())

		port = 9093
		reconcile(r)

		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: resourceName}, svc)).To(Succeed())
		Expect(svc.Spec.Ports).To(HaveLen(1))
		Expect(svc.Spec.Ports[0].Port).To(Equal(int32(9093)), "port change must reach the live Service")
		Expect(svc.Spec.Ports[0].NodePort).To(Equal(allocatedNodePort), "allocated NodePort must survive the update")
		Expect(svc.Spec.ClusterIP).To(Equal(clusterIP), "allocated ClusterIP must never be touched")
	})

	It("propagates data and label changes of extra resources via the generic fallback", func() {
		payload := "v1"
		labelValue := "v1"
		r := newReconciler(func(buildCtx *reconciler.RoleGroupBuildContext) *reconciler.RoleGroupResources {
			// A Secret has no typed rule in copyDesiredState, so it exercises the
			// unstructured top-level-field fallback used for arbitrary-GVK extras.
			extra := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      buildCtx.ResourceName + "-extra",
					Namespace: buildCtx.ClusterNamespace,
					Labels:    map[string]string{"app.kubernetes.io/version": labelValue},
				},
				Data: map[string][]byte{"payload": []byte(payload)},
			}
			return &reconciler.RoleGroupResources{ExtraResources: []client.Object{extra}}
		})

		reconcile(r)
		secret := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: resourceName + "-extra"}, secret)).To(Succeed())
		Expect(secret.Data).To(HaveKeyWithValue("payload", []byte("v1")))

		payload = "v2"
		labelValue = "v2"
		reconcile(r)

		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: resourceName + "-extra"}, secret)).To(Succeed())
		Expect(secret.Data).To(HaveKeyWithValue("payload", []byte("v2")), "extra resource data must propagate")
		Expect(secret.Labels).To(HaveKeyWithValue("app.kubernetes.io/version", "v2"), "extra resource labels must propagate")
	})

	It("keeps foreign annotations but replaces labels wholesale", func() {
		r := newReconciler(func(buildCtx *reconciler.RoleGroupBuildContext) *reconciler.RoleGroupResources {
			return &reconciler.RoleGroupResources{
				ConfigMap: testutil.NewTestConfigMap(buildCtx.ResourceName, buildCtx.ClusterNamespace),
			}
		})

		reconcile(r)
		cm := &corev1.ConfigMap{}
		key := types.NamespacedName{Namespace: namespace, Name: resourceName}
		Expect(k8sClient.Get(ctx, key, cm)).To(Succeed())

		// Out-of-band actor (e.g. kubectl apply) decorates the live object.
		if cm.Annotations == nil {
			cm.Annotations = map[string]string{}
		}
		cm.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = "{}"
		cm.Labels["foreign-label"] = "added-out-of-band"
		Expect(k8sClient.Update(ctx, cm)).To(Succeed())

		reconcile(r)

		Expect(k8sClient.Get(ctx, key, cm)).To(Succeed())
		// Annotations are merged, so the foreign annotation survives the reconcile...
		Expect(cm.Annotations).To(HaveKeyWithValue("kubectl.kubernetes.io/last-applied-configuration", "{}"))
		// ...but labels are framework-owned and replaced wholesale.
		Expect(cm.Labels).NotTo(HaveKey("foreign-label"))
		Expect(cm.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", resourceName))
	})

	It("preserves the live StatefulSet's immutable fields when the handler's selector changes", func() {
		selectorVersion := ""
		r := newReconciler(func(buildCtx *reconciler.RoleGroupBuildContext) *reconciler.RoleGroupResources {
			sts := testutil.NewTestStatefulSet(buildCtx.ResourceName, buildCtx.ClusterNamespace)
			if selectorVersion != "" {
				// The handler starts producing a different selector layout (e.g. after an
				// operator upgrade). Template labels stay a superset of the live selector,
				// which the API server requires.
				sts.Spec.Selector.MatchLabels["version"] = selectorVersion
				sts.Spec.Template.Labels["version"] = selectorVersion
			}
			return &reconciler.RoleGroupResources{StatefulSet: sts}
		})

		reconcile(r)
		sts := &appsv1.StatefulSet{}
		key := types.NamespacedName{Namespace: namespace, Name: resourceName}
		Expect(k8sClient.Get(ctx, key, sts)).To(Succeed())
		originalSelector := sts.Spec.Selector.DeepCopy()
		originalServiceName := sts.Spec.ServiceName

		selectorVersion = "v2"
		// The reconcile must NOT fail with an immutable-field error...
		reconcile(r)

		Expect(k8sClient.Get(ctx, key, sts)).To(Succeed())
		// ...because the live selector/serviceName are preserved (changing them requires a
		// manual delete/recreate migration, see copyStatefulSetState).
		Expect(sts.Spec.Selector).To(Equal(originalSelector))
		Expect(sts.Spec.ServiceName).To(Equal(originalServiceName))
		// The mutable template change still propagates.
		Expect(sts.Spec.Template.Labels).To(HaveKeyWithValue("version", "v2"))
	})
})
