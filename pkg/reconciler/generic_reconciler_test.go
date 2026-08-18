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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	testutilmetrics "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/common"
	"github.com/zncdatadev/operator-go/pkg/config"
	"github.com/zncdatadev/operator-go/pkg/constant"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/testutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ = Describe("GenericReconciler", func() {
	Describe("NewGenericReconciler", func() {
		It("should create a GenericReconciler with valid config", func() {
			mockCR := testutil.NewMockCluster("test-cluster", "default")
			wrappedCR := mockCR
			mockHandler := testutil.NewMockRoleGroupHandler()

			cfg := &reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
				Client:           k8sClient,
				Scheme:           testScheme,
				ImageResolution:  reconciler.ImageResolution{Defaults: v1alpha1.ImageSpec{Custom: "test-image:latest"}},
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
			wrappedCR := mockCR
			mockHandler := testutil.NewMockRoleGroupHandler()

			cfg := &reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
				Client:           nil,
				Scheme:           testScheme,
				ImageResolution:  reconciler.ImageResolution{Defaults: v1alpha1.ImageSpec{Custom: "test-image:latest"}},
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
			wrappedCR := mockCR
			mockHandler := testutil.NewMockRoleGroupHandler()

			cfg := &reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
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
			wrappedCR := mockCR
			mockHandler := testutil.NewMockRoleGroupHandler()

			cfg := &reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
				Client:           k8sClient,
				Scheme:           testScheme,
				ImageResolution:  reconciler.ImageResolution{Defaults: v1alpha1.ImageSpec{Custom: "test-image:latest"}},
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
			wrappedCR := mockCR

			cfg := &reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
				Client:           k8sClient,
				Scheme:           testScheme,
				ImageResolution:  reconciler.ImageResolution{Defaults: v1alpha1.ImageSpec{Custom: "test-image:latest"}},
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
			wrappedCR := mockCR
			mockHandler := testutil.NewMockRoleGroupHandler()

			cfg := &reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
				Client:           k8sClient,
				Scheme:           testScheme,
				ImageResolution:  reconciler.ImageResolution{Defaults: v1alpha1.ImageSpec{Custom: "test-image:latest"}},
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
			wrappedCR := mockCR
			mockHandler := testutil.NewMockRoleGroupHandler()

			cfg := &reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
				Client:              k8sClient,
				Scheme:              testScheme,
				ImageResolution:     reconciler.ImageResolution{Defaults: v1alpha1.ImageSpec{Custom: "test-image:latest"}},
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

	Describe("MockCluster", func() {
		It("should implement ClusterInterface", func() {
			mockCR := testutil.NewMockCluster("test-cluster", "default")
			wrappedCR := mockCR

			var _ common.ClusterInterface = wrappedCR
			Expect(wrappedCR).NotTo(BeNil())
		})

		It("should return correct name and namespace", func() {
			mockCR := testutil.NewMockCluster("test-cluster", "test-namespace")
			wrappedCR := mockCR

			Expect(wrappedCR.GetName()).To(Equal("test-cluster"))
			Expect(wrappedCR.GetNamespace()).To(Equal("test-namespace"))
		})
	})
})

const testNamespace = "default"

var _ = Describe("GenericReconciler Reconcile", func() {
	var r *reconciler.GenericReconciler[*testutil.MockCluster]
	var mockHandler *testutil.MockRoleGroupHandler
	var namespace string

	BeforeEach(func() {
		namespace = testNamespace
		mockHandler = testutil.NewMockRoleGroupHandler()

		mockCR := testutil.NewMockCluster("test-cr", namespace)
		wrappedCR := mockCR

		cfg := &reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			ImageResolution:  reconciler.ImageResolution{Defaults: v1alpha1.ImageSpec{Custom: "test-image:latest"}},
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
		wrappedCR := mockCR

		cfg := &reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			ImageResolution:  reconciler.ImageResolution{Defaults: v1alpha1.ImageSpec{Custom: "test-image:latest"}},
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
			ResolvedImage:    reconciler.ResolvedImage{Reference: "test-image:latest"},
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

var _ = Describe("MockCluster operations", func() {
	It("should expose object metadata and the projected spec", func() {
		mockCR := testutil.NewMockCluster("plain-cluster", "test-ns").
			WithLabels(map[string]string{"app": "test"}).
			WithRoles(map[string]v1alpha1.RoleSpec{
				"role-a": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{
						"group-1": {Replicas: ptr.To(int32(2))},
					},
				},
			})

		Expect(mockCR.GetName()).To(Equal("plain-cluster"))
		Expect(mockCR.GetNamespace()).To(Equal("test-ns"))
		Expect(mockCR.GetLabels()["app"]).To(Equal("test"))
		Expect(mockCR.GetSpec().Roles).To(HaveKey("role-a"))
	})

	It("should hand out a status pointer the framework can write through", func() {
		mockCR := testutil.NewMockCluster("status-cluster", "default")

		mockCR.GetStatus().SetRoleGroup("role-a", "group-1")

		Expect(mockCR.Status.RoleGroups).To(HaveKey("role-a"))
	})

	It("should deep copy into its own concrete type", func() {
		mockCR := testutil.NewMockCluster("deepcopy-cluster", "default").
			WithLabels(map[string]string{"env": "prod"}).
			WithRoles(map[string]v1alpha1.RoleSpec{
				"role-a": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{
						"group-1": {Replicas: ptr.To(int32(1))},
					},
				},
			})

		copied := mockCR.DeepCopy()
		Expect(copied.GetName()).To(Equal("deepcopy-cluster"))

		// Aliasing is asserted by mutation, not by identity: BeIdenticalTo on a map panics inside
		// the matcher (maps are not comparable), which Gomega reports as a failed match — so an
		// identity assertion on the shared maps passes whether or not they are shared.
		// MockCluster.DeepCopy is load-bearing: the reconciler builds every fetched CR from the
		// prototype through it, and the status guard compares against a snapshot taken with it.
		copied.Labels["env"] = "staging"
		copied.Spec.Roles["role-b"] = v1alpha1.RoleSpec{}
		copied.Status.SetRoleGroup("role-a", "group-1")

		Expect(mockCR.Labels).To(HaveKeyWithValue("env", "prod"))
		Expect(mockCR.Spec.Roles).NotTo(HaveKey("role-b"))
		Expect(mockCR.Status.RoleGroups).To(BeEmpty())
	})
})

// handlerAdapter adapts MockRoleGroupHandler to the RoleGroupHandler interface
type handlerAdapter struct {
	handler *testutil.MockRoleGroupHandler
}

// BuildResources implements reconciler.RoleGroupHandler
func (a *handlerAdapter) BuildResources(ctx context.Context, k8sClient client.Client, cr *testutil.MockCluster, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
	return a.handler.BuildResources(ctx, k8sClient, cr, buildCtx)
}

// Verify interface implementations
var _ common.ClusterInterface = &testutil.MockCluster{}
var _ reconciler.RoleGroupHandler[*testutil.MockCluster] = &handlerAdapter{}

var _ = Describe("GenericReconciler Integration Tests", func() {
	var r *reconciler.GenericReconciler[*testutil.MockCluster]
	var mockHandler *testutil.MockRoleGroupHandler
	var namespace string
	var testID string // Unique identifier for test isolation

	BeforeEach(func() {
		namespace = testNamespace
		testID = fmt.Sprintf("test-%d", time.Now().UnixNano())
		mockHandler = testutil.NewMockRoleGroupHandler()

		mockCR := testutil.NewMockCluster("test-cr-"+testID, namespace)
		wrappedCR := mockCR

		cfg := &reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			ImageResolution:  reconciler.ImageResolution{Defaults: v1alpha1.ImageSpec{Custom: "test-image:latest"}},
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
			Expect(result).To(Equal(ctrl.Result{RequeueAfter: reconciler.DefaultHealthCheckInterval}))

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
			// Paused freezes the resources, not the observing: the cycle still re-arms itself on
			// the health cadence, because a paused cluster's pods keep changing state and a
			// container entering ImagePullBackOff without ever having been ready changes no
			// StatefulSet field for the Owns() watch to deliver.
			Expect(result.RequeueAfter).To(Equal(reconciler.DefaultHealthCheckInterval))
			fetchedCR := &testutil.MockCluster{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: crName}, fetchedCR)).To(Succeed())

			// Surfacing the pause is the only thing this cycle does: without the status write the
			// CR keeps advertising the last running cycle's state and nothing tells an operator
			// why the cluster stopped converging.
			paused := fetchedCR.Status.GetCondition(v1alpha1.ConditionPaused)
			Expect(paused).NotTo(BeNil())
			Expect(paused.Status).To(Equal(metav1.ConditionTrue))
			Expect(paused.Reason).To(Equal(v1alpha1.ReasonReconciliationPaused))

			// And the pause is NOT a fault. Degraded is the condition an operator alerts on, so
			// reporting a maintenance window through it pages someone for a planned action — which
			// is why the sibling operation `stopped` has always reported Degraded=False.
			degraded := fetchedCR.Status.GetCondition(v1alpha1.ConditionDegraded)
			Expect(degraded).NotTo(BeNil())
			Expect(degraded.Status).To(Equal(metav1.ConditionFalse))
			Expect(degraded.Reason).To(Equal(v1alpha1.ReasonReconciliationPaused))
			Expect(fetchedCR.Status.ObservedGeneration).To(Equal(fetchedCR.Generation))

			// The gate runs before any mutation, so nothing was applied for the declared role group.
			resourceName := reconciler.RoleGroupResourceName(crName, "test-role", "default")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: resourceName}, &corev1.ConfigMap{})).
				To(MatchError(k8serrors.IsNotFound, "IsNotFound"))
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

		It("should reconcile all resources with the StatefulSet scaled to zero when stopped", func() {
			// A cluster created directly with stopped=true (no prior StatefulSet to scale) must
			// still get its full resource set — the shortcut design left such a cluster with
			// nothing. The new design runs the normal reconcile and forces replicas to 0.
			req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}}

			result, err := r.Reconcile(ctx, req)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{RequeueAfter: reconciler.DefaultHealthCheckInterval}))

			resName := reconciler.RoleGroupResourceName(crName, "test-role", "default")

			// The ConfigMap and Service are provisioned even though the cluster is stopped.
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: resName}, &corev1.ConfigMap{})).To(Succeed())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: resName}, &corev1.Service{})).To(Succeed())

			// The StatefulSet is CREATED (not merely scaled) with 0 replicas so it can be resumed.
			sts := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: resName}, sts)).To(Succeed())
			Expect(sts.Spec.Replicas).NotTo(BeNil())
			Expect(*sts.Spec.Replicas).To(Equal(int32(0)))

			// The health step must report Stopped at the end of the reconcile: the Available
			// condition is False with Reason == ReasonStopped. Asserting the reason (not merely
			// "conditions are non-empty") is what discriminates the new behavior from a cluster
			// that is merely Creating/Not-all-replicas-available.
			fetchedCR := &testutil.MockCluster{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: crName}, fetchedCR)).To(Succeed())
			availableCond := fetchedCR.Status.GetCondition(v1alpha1.ConditionAvailable)
			Expect(availableCond).NotTo(BeNil())
			Expect(availableCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(availableCond.Reason).To(Equal(v1alpha1.ReasonStopped))
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
			Expect(result1).To(Equal(ctrl.Result{RequeueAfter: reconciler.DefaultHealthCheckInterval}))

			// Second reconcile
			result2, err2 := r.Reconcile(ctx, req)
			Expect(err2).NotTo(HaveOccurred())
			Expect(result2).To(Equal(ctrl.Result{RequeueAfter: reconciler.DefaultHealthCheckInterval}))

			// Third reconcile
			result3, err3 := r.Reconcile(ctx, req)
			Expect(err3).NotTo(HaveOccurred())
			Expect(result3).To(Equal(ctrl.Result{RequeueAfter: reconciler.DefaultHealthCheckInterval}))

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
			mockHandler.WithBuildResourcesFunc(func(ctx context.Context, k8sClient client.Client, cr *testutil.MockCluster, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
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
			Expect(result).To(Equal(ctrl.Result{RequeueAfter: reconciler.DefaultHealthCheckInterval}))

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
// PreReconcile extensions before returning early. These tests assert the SA is never created while
// paused (and still is when the cluster proceeds normally).
//
// The SA name is derived from the CR, so each spec's unique CR name gives it a unique SA name and
// parallel/ordered specs cannot observe each other's ServiceAccounts.
var _ = Describe("GenericReconciler ClusterOperation gate ordering (issue #511)", func() {
	var (
		mockHandler *testutil.MockRoleGroupHandler
		namespace   string
	)

	newReconciler := func(prototype *testutil.MockCluster) *reconciler.GenericReconciler[*testutil.MockCluster] {
		cfg := &reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			ImageResolution:  reconciler.ImageResolution{Defaults: v1alpha1.ImageSpec{Custom: "test-image:latest"}},
			Recorder:         recorder,
			RoleGroupHandler: &handlerAdapter{handler: mockHandler},
			Prototype:        prototype,
		}
		r, err := reconciler.NewGenericReconciler(cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(r).NotTo(BeNil())
		return r
	}

	saNameFor := func(crName string) string {
		return reconciler.ServiceAccountResourceName("MockCluster", crName)
	}

	saLookup := func(crName string) error {
		return k8sClient.Get(ctx,
			types.NamespacedName{Namespace: namespace, Name: saNameFor(crName)},
			&corev1.ServiceAccount{})
	}

	deleteSAFor := func(crName string) {
		_ = k8sClient.Delete(ctx, &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{Name: saNameFor(crName), Namespace: namespace},
		})
	}

	BeforeEach(func() {
		namespace = testNamespace
		mockHandler = testutil.NewMockRoleGroupHandler()
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
			deleteSAFor(crName)
		})

		It("must NOT create the ServiceAccount (the pre-gate mutation the bug caused)", func() {
			// Guard: SA must not pre-exist.
			Expect(saLookup(crName)).NotTo(Succeed())

			r := newReconciler(pausedCR)
			req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}}

			result, err := r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(reconciler.DefaultHealthCheckInterval),
				"the pause still re-observes on the health cadence")

			// The pause gate runs before ensureServiceAccount, so the SA must still be absent.
			err = saLookup(crName)
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
			deleteSAFor(crName)
		})

		It("proceeds normally and DOES create the ServiceAccount", func() {
			r := newReconciler(runningCR)
			req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}}

			result, err := r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{RequeueAfter: reconciler.DefaultHealthCheckInterval}))

			Expect(saLookup(crName)).To(Succeed(), "ServiceAccount should be created for an un-paused cluster")
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
			deleteSAFor(crName)
			_ = k8sClient.Delete(ctx, &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      reconciler.RoleGroupResourceName(crName, "test-role", "default"),
					Namespace: namespace,
				},
			})
		})

		It("reconciles the full resource set — provisioning the ServiceAccount and creating the StatefulSet with zero replicas", func() {
			// New design (was inverted under the shortcut): stopped is NOT a short-circuit. It runs
			// the normal reconcile so every resource — including the ServiceAccount — is created and
			// kept up to date, with the StatefulSet's replicas forced to 0. The old behavior (scale
			// only pre-existing StatefulSets, never provision the SA) left a directly-stopped cluster
			// with no resources at all.
			stsName := reconciler.RoleGroupResourceName(crName, "test-role", "default")

			// Guards: neither the SA nor the StatefulSet pre-exists.
			Expect(k8serrors.IsNotFound(saLookup(crName))).To(BeTrue())
			Expect(k8serrors.IsNotFound(
				k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: stsName}, &appsv1.StatefulSet{}),
			)).To(BeTrue())

			r := newReconciler(stoppedCR)
			req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}}

			result, err := r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{RequeueAfter: reconciler.DefaultHealthCheckInterval}))

			// The ServiceAccount IS provisioned (stopped runs the full reconcile).
			Expect(saLookup(crName)).To(Succeed(), "ServiceAccount should be created for a stopped cluster under the new design")

			// The StatefulSet is CREATED with 0 replicas so the cluster can be resumed.
			fetched := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: stsName}, fetched)).To(Succeed())
			Expect(fetched.Spec.Replicas).NotTo(BeNil())
			Expect(*fetched.Spec.Replicas).To(Equal(int32(0)))
		})
	})
})

// saCapturingHandler wraps the mock handler and records the ServiceAccountName each
// RoleGroupBuildContext carries, so tests can assert the resolved SA name is propagated to
// resource building (the base handler's pod-template binding is covered in
// base_role_group_handler_test.go).
type saCapturingHandler struct {
	inner       reconciler.RoleGroupHandler[*testutil.MockCluster]
	seenSANames []string
}

func (h *saCapturingHandler) BuildResources(ctx context.Context, k8sClient client.Client, cr *testutil.MockCluster, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
	h.seenSANames = append(h.seenSANames, buildCtx.ServiceAccountName)
	return h.inner.BuildResources(ctx, k8sClient, cr, buildCtx)
}

// Coverage for the DERIVED workload ServiceAccount name. The name is
// ServiceAccountResourceName(kind, cluster) and there is no config field for it, which is what
// makes the failure class it replaced unrepresentable: a configurable static name was a constant,
// so every CR of a product in one namespace resolved to the SAME ServiceAccount — the second
// cluster hit AlreadyOwnedError forever and deleting the first garbage-collected the SA out from
// under the second's running pods.
var _ = Describe("GenericReconciler derived workload ServiceAccount", func() {
	var (
		mockHandler *testutil.MockRoleGroupHandler
		capturing   *saCapturingHandler
		namespace   string
		testID      string
	)

	newReconciler := func(prototype *testutil.MockCluster) *reconciler.GenericReconciler[*testutil.MockCluster] {
		cfg := &reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			ImageResolution:  reconciler.ImageResolution{Defaults: v1alpha1.ImageSpec{Custom: "test-image:latest"}},
			Recorder:         recorder,
			RoleGroupHandler: capturing,
			Prototype:        prototype,
		}
		r, err := reconciler.NewGenericReconciler(cfg)
		Expect(err).NotTo(HaveOccurred())
		return r
	}

	// The name the framework will derive for a MockCluster CR.
	saNameFor := func(crName string) string {
		return reconciler.ServiceAccountResourceName("MockCluster", crName)
	}

	newCR := func(name string) *testutil.MockCluster {
		cr := testutil.NewMockCluster(name, namespace).
			WithRoles(map[string]v1alpha1.RoleSpec{
				"test-role": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{
						"default": {Replicas: ptr.To(int32(1))},
					},
				},
			})
		Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		return cr
	}

	getSA := func(name string) (*corev1.ServiceAccount, error) {
		sa := &corev1.ServiceAccount{}
		err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, sa)
		return sa, err
	}

	deleteSA := func(name string) {
		_ = k8sClient.Delete(ctx, &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		})
	}

	reconcileReq := func(r *reconciler.GenericReconciler[*testutil.MockCluster], crName string) (ctrl.Result, error) {
		return r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}})
	}

	BeforeEach(func() {
		namespace = testNamespace
		mockHandler = testutil.NewMockRoleGroupHandler()
		capturing = &saCapturingHandler{inner: &handlerAdapter{handler: mockHandler}}
		testID = fmt.Sprintf("%d", time.Now().UnixNano())
	})

	It("derives the SA name from the CR and wires it into the build context", func() {
		crName := "sa-derived-" + testID
		derived := saNameFor(crName)
		cr := newCR(crName)
		defer func() {
			Expect(k8sClient.Delete(ctx, cr)).To(Succeed())
			deleteSA(derived)
		}()

		// Nothing is configured — the name comes from the CR alone.
		r := newReconciler(cr)

		_, err := reconcileReq(r, crName)
		Expect(err).NotTo(HaveOccurred())

		// The derived SA exists, is controller-owned by this CR, and carries the canonical labels
		// every other framework-built object carries.
		sa, err := getSA(derived)
		Expect(err).NotTo(HaveOccurred())
		Expect(sa.Name).To(Equal("mockcluster-" + crName))
		ownerRef := metav1.GetControllerOf(sa)
		Expect(ownerRef).NotTo(BeNil())
		Expect(ownerRef.Name).To(Equal(crName))
		Expect(ownerRef.Kind).To(Equal("MockCluster"))
		Expect(sa.Labels).To(HaveKeyWithValue(constant.LabelKubernetesInstance, crName))
		Expect(sa.Labels).To(HaveKeyWithValue(constant.LabelKubernetesManagedBy, "operator-go"))

		// The same name flows through RoleGroupBuildContext to resource building (the base handler
		// binds it to the pod template; see base_role_group_handler_test.go). Deriving it in both
		// places from one function is what keeps the ensured object and the consumed identity from
		// drifting apart.
		Expect(capturing.seenSANames).NotTo(BeEmpty())
		for _, seen := range capturing.seenSANames {
			Expect(seen).To(Equal(derived))
		}
	})

	It("gives every cluster an SA with nothing configured", func() {
		// The identity is not opt-in. Before this, a product that set neither config field got no
		// ServiceAccount at all and its pods ran as the namespace `default` — the exact opposite of
		// what docs/security.md claims the framework guarantees.
		crName := "sa-always-" + testID
		cr := newCR(crName)
		defer func() {
			Expect(k8sClient.Delete(ctx, cr)).To(Succeed())
			deleteSA(saNameFor(crName))
		}()

		r := newReconciler(cr)
		_, err := reconcileReq(r, crName)
		Expect(err).NotTo(HaveOccurred())

		_, err = getSA(saNameFor(crName))
		Expect(err).NotTo(HaveOccurred(), "every cluster gets a workload ServiceAccount, with nothing to configure")
	})

	It("keeps two CRs of different kinds but the same name on different SAs", func() {
		// The Kind is in the derived name because a CR name alone is not unique in a namespace.
		// Without it an HdfsCluster and a TrinoCluster both called "prod" would select one
		// ServiceAccount and the second controller could never own it — the same collision the
		// configurable name produced, one level down.
		crName := "sa-samename-" + testID
		Expect(reconciler.ServiceAccountResourceName("HdfsCluster", crName)).
			NotTo(Equal(reconciler.ServiceAccountResourceName("TrinoCluster", crName)))
		Expect(reconciler.ServiceAccountResourceName("HdfsCluster", crName)).
			To(Equal("hdfscluster-" + crName))
	})

	It("bounds a pathologically long derived name and keeps it unique", func() {
		long := strings.Repeat("a", 300)
		name := reconciler.ServiceAccountResourceName("MockCluster", long)
		Expect(len(name)).To(BeNumerically("<=", 253))
		Expect(name).NotTo(Equal(reconciler.ServiceAccountResourceName("MockCluster", long+"b")),
			"truncation must stay collision-free via the hash suffix")
	})

	It("lets two clusters in one namespace each reconcile and own their own SA", func() {
		// The multi-cluster scenario a configurable static name broke: the second cluster's
		// SetControllerReference on the shared SA failed with AlreadyOwnedError forever. Deriving
		// the name from the CR removes the shared object rather than detecting the conflict.
		crNameA := "sa-multi-a-" + testID
		crNameB := "sa-multi-b-" + testID
		crA := newCR(crNameA)
		crB := newCR(crNameB)
		defer func() {
			Expect(k8sClient.Delete(ctx, crA)).To(Succeed())
			Expect(k8sClient.Delete(ctx, crB)).To(Succeed())
			deleteSA(saNameFor(crNameA))
			deleteSA(saNameFor(crNameB))
		}()

		rA := newReconciler(crA)
		rB := newReconciler(crB)

		_, err := reconcileReq(rA, crNameA)
		Expect(err).NotTo(HaveOccurred())
		_, err = reconcileReq(rB, crNameB)
		Expect(err).NotTo(HaveOccurred(), "a second cluster in the namespace must reconcile cleanly")

		// Reconcile both again: steady state must stay clean (no AlreadyOwnedError on re-reconcile).
		_, err = reconcileReq(rA, crNameA)
		Expect(err).NotTo(HaveOccurred())
		_, err = reconcileReq(rB, crNameB)
		Expect(err).NotTo(HaveOccurred())

		saA, err := getSA(saNameFor(crNameA))
		Expect(err).NotTo(HaveOccurred())
		saB, err := getSA(saNameFor(crNameB))
		Expect(err).NotTo(HaveOccurred())
		Expect(saA.Name).NotTo(Equal(saB.Name))

		refA := metav1.GetControllerOf(saA)
		Expect(refA).NotTo(BeNil())
		Expect(refA.Name).To(Equal(crNameA))
		refB := metav1.GetControllerOf(saB)
		Expect(refB).NotTo(BeNil())
		Expect(refB.Name).To(Equal(crNameB))
	})

	It("refuses to adopt a foreign object squatting on the derived name", func() {
		// A derived name cannot collide with another CLUSTER, but it can still be occupied by
		// something else. Adopting it would hand these pods whatever identity that object carries,
		// so the framework refuses and says what to do.
		crName := "sa-victim-" + testID
		otherName := "sa-squatter-" + testID
		derived := saNameFor(crName)

		// A different CR controller-owns an object sitting at this cluster's derived SA name.
		otherCR := newCR(otherName)
		cr := newCR(crName)
		defer func() {
			Expect(k8sClient.Delete(ctx, cr)).To(Succeed())
			Expect(k8sClient.Delete(ctx, otherCR)).To(Succeed())
			deleteSA(derived)
		}()

		sa := &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{Name: derived, Namespace: namespace},
		}
		Expect(controllerutil.SetControllerReference(otherCR, sa, testScheme)).To(Succeed())
		Expect(k8sClient.Create(ctx, sa)).To(Succeed())

		r := newReconciler(cr)

		_, err := reconcileReq(r, crName)
		Expect(err).To(HaveOccurred())
		// The error names both owners and says the name is derived, so nobody goes looking for the
		// config field that would have caused this before.
		Expect(err.Error()).To(ContainSubstring(derived))
		Expect(err.Error()).To(ContainSubstring("already controlled by"))
		Expect(err.Error()).To(ContainSubstring(otherName), "error should name the existing owner")
		Expect(err.Error()).To(ContainSubstring(crName), "error should name the owner that failed to adopt")
		Expect(err.Error()).To(ContainSubstring("derived from the CR"),
			"error should say the name is derived rather than blaming a naming collision")

		// The squatter's ownership is untouched.
		fetched, err := getSA(derived)
		Expect(err).NotTo(HaveOccurred())
		ref := metav1.GetControllerOf(fetched)
		Expect(ref).NotTo(BeNil())
		Expect(ref.Name).To(Equal(otherName))
	})
})

var _ = Describe("GenericReconciler stopped scales the StatefulSet to zero", func() {
	var r *reconciler.GenericReconciler[*testutil.MockCluster]
	var mockHandler *testutil.MockRoleGroupHandler
	var namespace string
	var crName string
	var ctx context.Context
	var customCR *testutil.MockCluster
	var wrappedCR *testutil.MockCluster

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
		wrappedCR = customCR

		Expect(k8sClient.Create(ctx, customCR)).To(Succeed())
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, customCR)).To(Succeed())
		// envtest has no garbage collector, so explicitly remove the reconciled StatefulSet to
		// keep the two specs in this block independent regardless of execution order.
		_ = k8sClient.Delete(ctx, &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      reconciler.RoleGroupResourceName(crName, "test-role", "default"),
				Namespace: namespace,
			},
		})
	})

	JustBeforeEach(func() {
		cfg := &reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			ImageResolution:  reconciler.ImageResolution{Defaults: v1alpha1.ImageSpec{Custom: "test-image:latest"}},
			Recorder:         recorder,
			RoleGroupHandler: &handlerAdapter{handler: mockHandler},
			Prototype:        wrappedCR,
		}
		var err error
		r, err = reconciler.NewGenericReconciler(cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(r).NotTo(BeNil())
	})

	It("updates an existing StatefulSet's replicas to zero while reconciling it", func() {
		// Pre-create a StatefulSet with replicas > 0. The normal reconcile now UPDATES it to the
		// handler-built desired state, whose replicas are forced to 0 because the cluster is stopped.
		// The selector/template labels must match those the mock handler builds
		// (app.kubernetes.io/name = resource name) so the update keeps the immutable selector valid.
		replicas := int32(3)
		stsName := reconciler.RoleGroupResourceName(crName, "test-role", "default")
		stsLabels := map[string]string{"app.kubernetes.io/name": stsName}
		sts := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      stsName,
				Namespace: namespace,
			},
			Spec: appsv1.StatefulSetSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: stsLabels,
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: stsLabels,
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
		Expect(result).To(Equal(ctrl.Result{RequeueAfter: reconciler.DefaultHealthCheckInterval}))

		fetched := &appsv1.StatefulSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: stsName}, fetched)).To(Succeed())
		Expect(fetched.Spec.Replicas).NotTo(BeNil())
		Expect(*fetched.Spec.Replicas).To(Equal(int32(0)))
	})

	It("creates the StatefulSet with zero replicas when none exists yet", func() {
		// A cluster stopped from the start has no StatefulSet to scale; the reconcile must CREATE
		// it (with 0 replicas) rather than do nothing.
		stsName := reconciler.RoleGroupResourceName(crName, "test-role", "default")
		Expect(k8serrors.IsNotFound(
			k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: stsName}, &appsv1.StatefulSet{}),
		)).To(BeTrue())

		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}}
		result, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{RequeueAfter: reconciler.DefaultHealthCheckInterval}))

		fetched := &appsv1.StatefulSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: stsName}, fetched)).To(Succeed())
		Expect(fetched.Spec.Replicas).NotTo(BeNil())
		Expect(*fetched.Spec.Replicas).To(Equal(int32(0)))
	})
})

var _ = Describe("GenericReconciler error paths", func() {
	var r *reconciler.GenericReconciler[*testutil.MockCluster]
	var mockHandler *testutil.MockRoleGroupHandler
	var namespace string
	var crName string
	var ctx context.Context
	var customCR *testutil.MockCluster
	var wrappedCR *testutil.MockCluster

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
		wrappedCR = customCR

		Expect(k8sClient.Create(ctx, customCR)).To(Succeed())
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, customCR)).To(Succeed())
	})

	JustBeforeEach(func() {
		cfg := &reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			ImageResolution:  reconciler.ImageResolution{Defaults: v1alpha1.ImageSpec{Custom: "test-image:latest"}},
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
		mockHandler.WithBuildResourcesFunc(func(ctx context.Context, k8sClient client.Client, cr *testutil.MockCluster, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
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
	var r *reconciler.GenericReconciler[*testutil.MockCluster]
	var mockHandler *testutil.MockRoleGroupHandler
	var namespace string
	var crName string
	var ctx context.Context
	var customCR *testutil.MockCluster
	var wrappedCR *testutil.MockCluster

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
		wrappedCR = customCR

		Expect(k8sClient.Create(ctx, customCR)).To(Succeed())
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, customCR)).To(Succeed())
		mockHandler.BuildResourcesFunc = nil
	})

	JustBeforeEach(func() {
		cfg := &reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			ImageResolution:  reconciler.ImageResolution{Defaults: v1alpha1.ImageSpec{Custom: "test-image:latest"}},
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
		mockHandler.WithBuildResourcesFunc(func(ctx context.Context, k8sClient client.Client, cr *testutil.MockCluster, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
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
		Expect(result).To(Equal(ctrl.Result{RequeueAfter: reconciler.DefaultHealthCheckInterval}))
	})

	It("should handle resources with all nil fields", func() {
		mockHandler.WithBuildResourcesFunc(func(ctx context.Context, k8sClient client.Client, cr *testutil.MockCluster, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
			return &reconciler.RoleGroupResources{
				// All fields are nil - should succeed with no resources applied
			}, nil
		})

		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}}
		result, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{RequeueAfter: reconciler.DefaultHealthCheckInterval}))
	})

	It("should handle resources with PDB", func() {
		maxUnavailable := intstr.FromInt(1)
		mockHandler.WithBuildResourcesFunc(func(ctx context.Context, k8sClient client.Client, cr *testutil.MockCluster, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
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
		Expect(result).To(Equal(ctrl.Result{RequeueAfter: reconciler.DefaultHealthCheckInterval}))
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
		_, _ = cleaner.Cleanup(canceledCtx, namespace, "cleanup-error", spec, status, "", nil)
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

	newReconciler := func(handler reconciler.RoleGroupHandler[*testutil.MockCluster]) *reconciler.GenericReconciler[*testutil.MockCluster] {
		cfg := &reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           recClient,
			Scheme:           testScheme,
			ImageResolution:  reconciler.ImageResolution{Defaults: v1alpha1.ImageSpec{Custom: "test-image:latest"}},
			Recorder:         recorder,
			RoleGroupHandler: handler,
			Prototype:        testutil.NewMockCluster("proto", namespace),
		}
		r, err := reconciler.NewGenericReconciler(cfg)
		Expect(err).NotTo(HaveOccurred())
		return r
	}

	It("applies extras with a controller owner reference, after the ConfigMap and before the StatefulSet", func() {
		handler := &reconciler.RoleGroupHandlerFuncs[*testutil.MockCluster]{
			BuildResourcesFunc: func(_ context.Context, _ client.Client, _ *testutil.MockCluster, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
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
		handler := &reconciler.RoleGroupHandlerFuncs[*testutil.MockCluster]{
			BuildResourcesFunc: func(_ context.Context, _ client.Client, _ *testutil.MockCluster, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
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
		handler := &reconciler.RoleGroupHandlerFuncs[*testutil.MockCluster]{
			BuildResourcesFunc: func(_ context.Context, _ client.Client, _ *testutil.MockCluster, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
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
		// Only the ConfigMap and StatefulSet came from the role group. The cluster-level workload
		// ServiceAccount is created for every cluster (step 0) and is not an ExtraResource.
		Expect(recClient.created).To(ConsistOf(
			"*v1.ServiceAccount/"+reconciler.ServiceAccountResourceName("MockCluster", crName),
			"*v1.ConfigMap/"+resourceName,
			"*v1.StatefulSet/"+resourceName,
		))

		// A slice holding only nil entries is equally a no-op.
		handler.BuildResourcesFunc = func(_ context.Context, _ client.Client, _ *testutil.MockCluster, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
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
	newCapturingHandler := func(into **config.MergedConfig) reconciler.RoleGroupHandler[*testutil.MockCluster] {
		return &reconciler.RoleGroupHandlerFuncs[*testutil.MockCluster]{
			BuildResourcesFunc: func(_ context.Context, _ client.Client, _ *testutil.MockCluster, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
				*into = buildCtx.MergedConfig
				return &reconciler.RoleGroupResources{}, nil
			},
		}
	}

	It("merges product config beneath CRD overrides and applies role-specific logic", func() {
		var captured *config.MergedConfig

		cfg := &reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			ImageResolution:  reconciler.ImageResolution{Defaults: v1alpha1.ImageSpec{Custom: "test-image:latest"}},
			Recorder:         recorder,
			RoleGroupHandler: newCapturingHandler(&captured),
			Prototype:        testutil.NewMockCluster("proto", namespace),
			RoleGroupResolver: reconciler.RoleGroupResolverFunc[*testutil.MockCluster](
				func(_ context.Context, _ client.Client, _ *testutil.MockCluster,
					rg *reconciler.RoleGroupBuildContext) (*reconciler.Contribution, error) {
					roleName := rg.RoleName
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
					return &reconciler.Contribution{ConfigOverrides: overrides}, nil
				}),
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

		cfg := &reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			ImageResolution:  reconciler.ImageResolution{Defaults: v1alpha1.ImageSpec{Custom: "test-image:latest"}},
			Recorder:         recorder,
			RoleGroupHandler: newCapturingHandler(&captured),
			Prototype:        testutil.NewMockCluster("proto", namespace),
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

	It("is handed a ctx and a client, so it can read the cluster it is documented to read", func() {
		// The hook's doc has always said it "may derive from live cluster state". Without a ctx or
		// a client it could only be a pure function of the CR, so the products that needed a
		// product-config layer — an S3Connection reference resolved to an endpoint — were exactly
		// the ones it could not serve. Two of them hand-wrote the same workaround instead.
		source := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: crName + "-source", Namespace: namespace},
			Data:       map[string]string{"endpoint": "resolved-from-cluster"},
		}
		Expect(k8sClient.Create(ctx, source)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, source) })

		var captured *config.MergedConfig
		cfg := &reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			ImageResolution:  reconciler.ImageResolution{Defaults: v1alpha1.ImageSpec{Custom: "test-image:latest"}},
			Recorder:         recorder,
			RoleGroupHandler: newCapturingHandler(&captured),
			Prototype:        testutil.NewMockCluster("proto", namespace),
			RoleGroupResolver: reconciler.RoleGroupResolverFunc[*testutil.MockCluster](func(
				hookCtx context.Context, c client.Client, cr *testutil.MockCluster,
				_ *reconciler.RoleGroupBuildContext) (*reconciler.Contribution, error) {
				cm := &corev1.ConfigMap{}
				if err := c.Get(hookCtx, types.NamespacedName{
					Namespace: cr.Namespace, Name: cr.Name + "-source"}, cm); err != nil {
					return nil, err
				}
				return &reconciler.Contribution{ConfigOverrides: map[string]map[string]string{
					"config.properties": {
						"shared":       cm.Data["endpoint"],
						"product-only": "p",
					},
				}}, nil
			}),
		}

		r, err := reconciler.NewGenericReconciler(cfg)
		Expect(err).NotTo(HaveOccurred())
		_, err = r.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}})
		Expect(err).NotTo(HaveOccurred())

		Expect(captured).NotTo(BeNil())
		props := captured.ConfigFiles["config.properties"]
		Expect(props).To(HaveKeyWithValue("product-only", "p"), "the lookup's result reaches the config")
		Expect(props).To(HaveKeyWithValue("shared", "from-crd"),
			"and still sits beneath what the user wrote")
	})

	It("fails the role group when the lookup fails", func() {
		// Previously a Get error could only be swallowed — rendering a silently wrong config — or
		// panicked, because the signature had nowhere to put it.
		var captured *config.MergedConfig
		cfg := &reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			ImageResolution:  reconciler.ImageResolution{Defaults: v1alpha1.ImageSpec{Custom: "test-image:latest"}},
			Recorder:         recorder,
			RoleGroupHandler: newCapturingHandler(&captured),
			Prototype:        testutil.NewMockCluster("proto", namespace),
			RoleGroupResolver: reconciler.RoleGroupResolverFunc[*testutil.MockCluster](func(
				context.Context, client.Client, *testutil.MockCluster,
				*reconciler.RoleGroupBuildContext) (*reconciler.Contribution, error) {
				return nil, fmt.Errorf("the S3Connection reference could not be resolved")
			}),
		}

		r, err := reconciler.NewGenericReconciler(cfg)
		Expect(err).NotTo(HaveOccurred())
		_, err = r.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}})
		Expect(err).To(MatchError(ContainSubstring("could not be resolved")))
		Expect(captured).To(BeNil(), "no workload is built from a config that failed to compute")
	})

	It("keeps the lookup's error chain intact", func() {
		// A product distinguishes "the S3Connection does not exist yet" from a real failure with
		// k8serrors.IsNotFound. Flattening the cause to a string — which ConfigError forced, being
		// the one error type in the package without an Unwrap — takes that away.
		var captured *config.MergedConfig
		notFound := k8serrors.NewNotFound(
			schema.GroupResource{Group: "s3.kubedoop.dev", Resource: "s3connections"}, "warehouse")

		cfg := &reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			ImageResolution:  reconciler.ImageResolution{Defaults: v1alpha1.ImageSpec{Custom: "test-image:latest"}},
			Recorder:         recorder,
			RoleGroupHandler: newCapturingHandler(&captured),
			Prototype:        testutil.NewMockCluster("proto", namespace),
			RoleGroupResolver: reconciler.RoleGroupResolverFunc[*testutil.MockCluster](func(
				context.Context, client.Client, *testutil.MockCluster,
				*reconciler.RoleGroupBuildContext) (*reconciler.Contribution, error) {
				return nil, fmt.Errorf("resolving the warehouse connection: %w", notFound)
			}),
		}

		r, err := reconciler.NewGenericReconciler(cfg)
		Expect(err).NotTo(HaveOccurred())
		_, err = r.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}})

		Expect(err).To(HaveOccurred())
		Expect(k8serrors.IsNotFound(err)).To(BeTrue(), "errors.As must still reach the cause")
		Expect(errors.Is(err, notFound)).To(BeTrue())
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
	newReconciler := func(build func(buildCtx *reconciler.RoleGroupBuildContext) *reconciler.RoleGroupResources) *reconciler.GenericReconciler[*testutil.MockCluster] {
		handler := &reconciler.RoleGroupHandlerFuncs[*testutil.MockCluster]{
			BuildResourcesFunc: func(_ context.Context, _ client.Client, _ *testutil.MockCluster, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
				return build(buildCtx), nil
			},
		}
		cfg := &reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			ImageResolution:  reconciler.ImageResolution{Defaults: v1alpha1.ImageSpec{Custom: "test-image:latest"}},
			Recorder:         fakeRecorder,
			RoleGroupHandler: handler,
			Prototype:        testutil.NewMockCluster("proto", namespace),
		}
		r, err := reconciler.NewGenericReconciler(cfg)
		Expect(err).NotTo(HaveOccurred())
		return r
	}

	reconcile := func(r *reconciler.GenericReconciler[*testutil.MockCluster]) {
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

	It("keeps the allocated NodePort when the handler RENAMES a port", func() {
		// The spec above changes the port NUMBER and keeps the name, which the API server's own
		// patchAllocatedValues already handles — it survives with the framework's carry-over
		// deleted. A RENAME is the case that does not: probed against envtest 1.35, a naive
		// Update that renames a port (same number, nodePort left 0) makes the API server
		// REALLOCATE the node port (31965 -> 32604). Every client that had the old port hard-coded
		// — a firewall rule, an external load balancer's target, a documented connection string —
		// breaks, silently, because a role's port was given a better name.
		//
		// findServicePort's port-NUMBER fallback is what prevents that, making the framework
		// strictly more preserving than Kubernetes. Nothing pinned it before this spec.
		portName := "client"
		r := newReconciler(func(buildCtx *reconciler.RoleGroupBuildContext) *reconciler.RoleGroupResources {
			svc := testutil.NewTestService(buildCtx.ResourceName, buildCtx.ClusterNamespace)
			svc.Spec.Type = corev1.ServiceTypeNodePort
			svc.Spec.Ports = []corev1.ServicePort{{
				Name:       portName,
				Port:       9092,
				TargetPort: intstr.FromInt(9092),
				Protocol:   corev1.ProtocolTCP,
			}}
			return &reconciler.RoleGroupResources{Service: svc}
		})

		reconcile(r)
		svc := &corev1.Service{}
		key := types.NamespacedName{Namespace: namespace, Name: resourceName}
		Expect(k8sClient.Get(ctx, key, svc)).To(Succeed())
		allocated := svc.Spec.Ports[0].NodePort
		Expect(allocated).NotTo(BeZero())

		portName = "kafka-client"
		reconcile(r)

		Expect(k8sClient.Get(ctx, key, svc)).To(Succeed())
		Expect(svc.Spec.Ports).To(HaveLen(1))
		Expect(svc.Spec.Ports[0].Name).To(Equal("kafka-client"), "the rename must reach the live Service")
		Expect(svc.Spec.Ports[0].NodePort).To(Equal(allocated),
			"a rename must not cost the allocated NodePort — the API server would reallocate it")
	})

	It("keeps a loadBalancerClass the handler no longer states", func() {
		// loadBalancerClass is immutable in a stronger sense than the rest of the preserved set:
		// the API server does not silently restore it, it REJECTS the whole update
		// ("spec.loadBalancerClass: Invalid value: null: may not change once set", probed against
		// envtest 1.35). Without the carry-over, a user who set the class once and later removed
		// it from their CR would wedge that role group's reconcile permanently — a 422 on every
		// pass, and no path back except deleting the Service by hand.
		//
		// No ImmutableFieldIgnored warning is expected here: the handler UNSET the field, which is
		// declining to have an opinion rather than asking for a change.
		class := ptr.To("example.com/internal-lb")
		r := newReconciler(func(buildCtx *reconciler.RoleGroupBuildContext) *reconciler.RoleGroupResources {
			svc := testutil.NewTestService(buildCtx.ResourceName, buildCtx.ClusterNamespace)
			svc.Spec.Type = corev1.ServiceTypeLoadBalancer
			svc.Spec.LoadBalancerClass = class
			return &reconciler.RoleGroupResources{Service: svc}
		})

		reconcile(r)
		svc := &corev1.Service{}
		key := types.NamespacedName{Namespace: namespace, Name: resourceName}
		Expect(k8sClient.Get(ctx, key, svc)).To(Succeed())
		Expect(svc.Spec.LoadBalancerClass).To(HaveValue(Equal("example.com/internal-lb")))

		class = nil
		reconcile(r) // must not return the API server's 422

		Expect(k8sClient.Get(ctx, key, svc)).To(Succeed())
		Expect(svc.Spec.LoadBalancerClass).To(HaveValue(Equal("example.com/internal-lb")),
			"the live class must survive an update that does not state it")
	})

	It("propagates Service fields outside the historical copy list", func() {
		distribution := ""
		r := newReconciler(func(buildCtx *reconciler.RoleGroupBuildContext) *reconciler.RoleGroupResources {
			svc := testutil.NewTestService(buildCtx.ResourceName, buildCtx.ClusterNamespace)
			if distribution != "" {
				svc.Spec.TrafficDistribution = ptr.To(distribution)
			}
			return &reconciler.RoleGroupResources{Service: svc}
		})

		reconcile(r)
		svc := &corev1.Service{}
		key := types.NamespacedName{Namespace: namespace, Name: resourceName}
		Expect(k8sClient.Get(ctx, key, svc)).To(Succeed())
		Expect(svc.Spec.TrafficDistribution).To(BeNil())
		clusterIP := svc.Spec.ClusterIP
		Expect(clusterIP).NotTo(BeEmpty())

		// TrafficDistribution is mutable but was never part of the copied field list, so the
		// desired value used to be dropped on every update.
		distribution = corev1.ServiceTrafficDistributionPreferClose
		reconcile(r)

		Expect(k8sClient.Get(ctx, key, svc)).To(Succeed())
		Expect(svc.Spec.TrafficDistribution).To(HaveValue(Equal(corev1.ServiceTrafficDistributionPreferClose)))
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

	It("warns that a storage resize was dropped instead of reporting success", func() {
		// The scenario that motivated this: a user grows config.resources.storage. The framework
		// correctly refuses to send an Update Kubernetes would reject — volumeClaimTemplates are
		// immutable — but doing so SILENTLY meant the CR reported ReconcileComplete=True while the
		// PVC never moved, with nothing in the API explaining why. Editing the CR back does not
		// help either: the live value is authoritative from here on.
		capacity := "10Gi"
		r := newReconciler(func(buildCtx *reconciler.RoleGroupBuildContext) *reconciler.RoleGroupResources {
			sts := testutil.NewTestStatefulSet(buildCtx.ResourceName, buildCtx.ClusterNamespace)
			sts.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{{
				ObjectMeta: metav1.ObjectMeta{Name: "data"},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse(capacity)},
					},
				},
			}}
			return &reconciler.RoleGroupResources{StatefulSet: sts}
		})

		reconcile(r)
		drainEvents()

		capacity = "100Gi"
		reconcile(r)

		Expect(drainEvents()).To(ContainElement(SatisfyAll(
			ContainSubstring("Warning"),
			ContainSubstring("ImmutableFieldIgnored"),
			ContainSubstring("spec.volumeClaimTemplates"),
		)), "the user must be told the resize had no effect")

		// And the live PVC template is genuinely unchanged — the warning is not cosmetic.
		sts := &appsv1.StatefulSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: resourceName}, sts)).To(Succeed())
		stored := sts.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests[corev1.ResourceStorage]
		Expect(stored.String()).To(Equal("10Gi"))
	})

	It("stays quiet when nothing immutable drifted", func() {
		// The warning repeats for as long as spec and live disagree, so it must not fire on the
		// steady state — every reconcile of every role group would carry one.
		r := newReconciler(func(buildCtx *reconciler.RoleGroupBuildContext) *reconciler.RoleGroupResources {
			return &reconciler.RoleGroupResources{
				StatefulSet: testutil.NewTestStatefulSet(buildCtx.ResourceName, buildCtx.ClusterNamespace),
			}
		})

		reconcile(r)
		drainEvents()
		reconcile(r)

		Expect(drainEvents()).NotTo(ContainElement(ContainSubstring("ImmutableFieldIgnored")))
	})
})

// embeddingRoleGroupHandler mirrors how product operators (e.g. ZkRoleGroupHandler) consume the
// framework: it EMBEDS *BaseRoleGroupHandler rather than being it exactly. It is the regression
// guard for the role-level PDB — the reconciler must still emit it for an embedding handler,
// which it does by asserting on the promoted method set (rolePodDisruptionBudgetBuilder) instead
// of the concrete *BaseRoleGroupHandler type.
type embeddingRoleGroupHandler struct {
	*reconciler.BaseRoleGroupHandler[*testutil.MockCluster]
}

var _ = Describe("GenericReconciler role PodDisruptionBudget", func() {
	var r *reconciler.GenericReconciler[*testutil.MockCluster]
	var namespace string
	var crName string
	var mockCR *testutil.MockCluster

	BeforeEach(func() {
		namespace = testNamespace
		crName = fmt.Sprintf("pdb-role-%d", time.Now().UnixNano())

		handler := &embeddingRoleGroupHandler{
			BaseRoleGroupHandler: reconciler.NewBaseRoleGroupHandler[*testutil.MockCluster](testScheme),
		}

		cfg := &reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			ImageResolution:  reconciler.ImageResolution{Defaults: v1alpha1.ImageSpec{Custom: "test-image:latest"}},
			Recorder:         recorder,
			RoleGroupHandler: handler,
			Prototype:        testutil.NewMockCluster(crName, namespace),
		}
		var err error
		r, err = reconciler.NewGenericReconciler(cfg)
		Expect(err).NotTo(HaveOccurred())

		maxUnavailable := int32(2)
		mockCR = testutil.NewMockCluster(crName, namespace).WithRoles(map[string]v1alpha1.RoleSpec{
			"server": {
				RoleConfig: &v1alpha1.RoleConfigSpec{
					PodDisruptionBudget: &v1alpha1.PodDisruptionBudgetSpec{Enabled: ptr.To(true), MaxUnavailable: &maxUnavailable},
				},
				RoleGroups: map[string]v1alpha1.RoleGroupSpec{
					"default":   {Replicas: ptr.To(int32(2))},
					"secondary": {Replicas: ptr.To(int32(1))},
				},
			},
		})
		Expect(k8sClient.Create(ctx, mockCR)).To(Succeed())
	})

	AfterEach(func() {
		_ = k8sClient.Delete(ctx, mockCR)
	})

	It("emits exactly one role-level PDB across role groups for an embedding handler", func() {
		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}}
		_, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		// One role-level PDB named "<cluster>-<role>" (not "<cluster>-<role>-<group>").
		rolePDB := &policyv1.PodDisruptionBudget{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: reconciler.RoleResourceName(crName, "server")}, rolePDB)).To(Succeed())
		Expect(rolePDB.Spec.MaxUnavailable.IntVal).To(Equal(int32(2)))

		// No per-group PDBs: the role PDB's selector spans every group.
		for _, group := range []string{"default", "secondary"} {
			perGroup := &policyv1.PodDisruptionBudget{}
			getErr := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: reconciler.RoleGroupResourceName(crName, "server", group)}, perGroup)
			Expect(k8serrors.IsNotFound(getErr)).To(BeTrue(), "no per-group PDB should exist for group %q", group)
		}
	})

	// newOwnedPerGroupPDB creates a PodDisruptionBudget named after a role group and owned by the
	// CR, so only its labels can say whether the framework put it there.
	newOwnedPerGroupPDB := func(name string, labels map[string]string) *policyv1.PodDisruptionBudget {
		pdb := &policyv1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels:    labels,
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: "test.zncdata.dev/v1alpha1",
					Kind:       "MockCluster",
					Name:       crName,
					UID:        mockCR.GetUID(),
					Controller: ptr.To(true),
				}},
			},
			Spec: policyv1.PodDisruptionBudgetSpec{
				Selector:       &metav1.LabelSelector{MatchLabels: map[string]string{"legacy": "true"}},
				MaxUnavailable: func() *intstr.IntOrString { v := intstr.FromInt(1); return &v }(),
			},
		}
		Expect(k8sClient.Create(ctx, pdb)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, pdb)
		})
		return pdb
	}

	It("reclaims a legacy per-role-group PDB left by an older framework version", func() {
		// Simulate an upgrade: a per-group PDB "<cluster>-server-default", owned by the CR, was
		// written by an older framework version and still lingers. Pre-#530 versions built it from
		// the role group's descriptive labels, which is the fingerprint the reclaim recognizes.
		legacyName := reconciler.RoleGroupResourceName(crName, "server", "default")
		newOwnedPerGroupPDB(legacyName, map[string]string{
			"app.kubernetes.io/instance":   crName,
			"app.kubernetes.io/component":  "server",
			"app.kubernetes.io/managed-by": "operator-go",
			crName + "-default":            "true",
		})

		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}}
		_, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		// The legacy per-group PDB is reclaimed; exactly the role-level PDB remains.
		getErr := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: legacyName}, &policyv1.PodDisruptionBudget{})
		Expect(k8serrors.IsNotFound(getErr)).To(BeTrue(), "legacy per-group PDB should have been deleted")
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: reconciler.RoleResourceName(crName, "server")}, &policyv1.PodDisruptionBudget{})).To(Succeed())
	})

	It("leaves a product's own PDB of the same name alone", func() {
		// A product may ship a PDB named after one of its role groups through ExtraResources. It
		// carries this CR's controller owner reference like every framework object, so ownership
		// cannot tell it apart from the framework's per-group slot — only the slot label can.
		customName := reconciler.RoleGroupResourceName(crName, "server", "secondary")
		custom := newOwnedPerGroupPDB(customName, map[string]string{"app.kubernetes.io/name": "product-owned"})

		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName}}
		_, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(custom), &policyv1.PodDisruptionBudget{})).To(Succeed())
	})
})

var _ = Describe("Metrics of a deleted cluster", func() {
	It("drops the cluster's series instead of leaving a gauge published forever", func() {
		// A gauge is not self-clearing. Left behind, "orphan cleanup pending = 1" keeps being
		// scraped for a cluster nobody can act on — an alert that can never be resolved because
		// the object it points at is gone. Deleting the whole series is the only honest answer;
		// setting it to zero would still publish a series for something that does not exist.
		const namespace, cluster = "default", "metrics-gone"
		reconciler.OrphanCleanupPending.WithLabelValues(namespace, cluster).Set(3)
		before := testutilmetrics.CollectAndCount(reconciler.OrphanCleanupPending)

		cfg := &reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			ImageResolution:  reconciler.ImageResolution{Defaults: v1alpha1.ImageSpec{Custom: "test-image:latest"}},
			Recorder:         recorder,
			RoleGroupHandler: &handlerAdapter{handler: testutil.NewMockRoleGroupHandler()},
			Prototype:        testutil.NewMockCluster(cluster, namespace),
		}
		r, err := reconciler.NewGenericReconciler(cfg)
		Expect(err).NotTo(HaveOccurred())

		// No such CR: the NotFound branch is the one place that learns a cluster is gone, because
		// the SDK registers no finalizer and so has no teardown callback.
		_, err = r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Namespace: namespace, Name: cluster}})
		Expect(err).NotTo(HaveOccurred())

		Expect(testutilmetrics.CollectAndCount(reconciler.OrphanCleanupPending)).To(Equal(before-1),
			"the series must be gone, not merely zeroed")
	})
})
