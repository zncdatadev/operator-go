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
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/common"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/testutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Mocks", func() {
	const (
		testName      = "test-cluster"
		testNamespace = "test-namespace"
	)

	Context("MockCluster", func() {
		It("should create a new MockCluster", func() {
			cluster := testutil.NewMockCluster(testName, testNamespace)
			Expect(cluster).NotTo(BeNil())
			Expect(cluster.Name).To(Equal(testName))
			Expect(cluster.Namespace).To(Equal(testNamespace))
			Expect(cluster.Kind).To(Equal("MockCluster"))
			Expect(cluster.APIVersion).To(Equal("test.zncdata.dev/v1alpha1"))
		})

		It("should have default labels", func() {
			cluster := testutil.NewMockCluster(testName, testNamespace)
			Expect(cluster.Labels).To(HaveKey("app.kubernetes.io/name"))
			Expect(cluster.Labels["app.kubernetes.io/name"]).To(Equal(testName))
			Expect(cluster.Labels).To(HaveKey("app.kubernetes.io/instance"))
		})

		It("should set labels with WithLabels", func() {
			cluster := testutil.NewMockCluster(testName, testNamespace)
			cluster.WithLabels(map[string]string{
				"custom-label": "value",
			})
			Expect(cluster.Labels["custom-label"]).To(Equal("value"))
			// Original labels should still exist
			Expect(cluster.Labels["app.kubernetes.io/name"]).To(Equal(testName))
		})

		It("should set annotations with WithAnnotations", func() {
			cluster := testutil.NewMockCluster(testName, testNamespace)
			cluster.WithAnnotations(map[string]string{
				"custom-annotation": "value",
			})
			Expect(cluster.Annotations).NotTo(BeNil())
			Expect(cluster.Annotations["custom-annotation"]).To(Equal("value"))
		})

		It("should set roles with WithRoles", func() {
			cluster := testutil.NewMockCluster(testName, testNamespace)
			replicas := int32(3)
			roles := map[string]v1alpha1.RoleSpec{
				"worker": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{
						"default": {
							Replicas: &replicas,
						},
					},
				},
			}
			cluster.WithRoles(roles)
			Expect(cluster.Spec.Roles).To(HaveKey("worker"))
		})

		It("should set cluster operation with WithClusterOperation", func() {
			cluster := testutil.NewMockCluster(testName, testNamespace)
			op := &v1alpha1.ClusterOperationSpec{
				Stopped: true,
			}
			cluster.WithClusterOperation(op)
			Expect(cluster.Spec.ClusterOperation).NotTo(BeNil())
			Expect(cluster.Spec.ClusterOperation.Stopped).To(BeTrue())
		})

		It("should deep copy correctly", func() {
			cluster := testutil.NewMockCluster(testName, testNamespace)
			cluster.WithLabels(map[string]string{"test": "value"})

			copied := cluster.DeepCopy()
			Expect(copied).NotTo(BeNil())
			Expect(copied.Name).To(Equal(cluster.Name))
			Expect(copied.Labels["test"]).To(Equal("value"))

			// Modify original should not affect copy
			cluster.Labels["test"] = "modified"
			Expect(copied.Labels["test"]).To(Equal("value"))
		})

		It("should return nil for nil DeepCopy", func() {
			var cluster *testutil.MockCluster
			copied := cluster.DeepCopy()
			Expect(copied).To(BeNil())
		})

		It("should implement DeepCopyObject", func() {
			cluster := testutil.NewMockCluster(testName, testNamespace)
			obj := cluster.DeepCopyObject()
			Expect(obj).NotTo(BeNil())
			copied, ok := obj.(*testutil.MockCluster)
			Expect(ok).To(BeTrue())
			Expect(copied.Name).To(Equal(testName))
		})
	})

	Context("ClusterWrapper", func() {
		It("should wrap MockCluster", func() {
			cluster := testutil.NewMockCluster(testName, testNamespace)
			wrapper := testutil.WrapMockCluster(cluster)
			Expect(wrapper).NotTo(BeNil())
		})

		It("should wrap MockCluster with scheme", func() {
			cluster := testutil.NewMockCluster(testName, testNamespace)
			scheme := runtime.NewScheme()
			wrapper := testutil.WrapMockCluster(cluster, scheme)
			Expect(wrapper).NotTo(BeNil())
			Expect(wrapper.GetScheme()).To(Equal(scheme))
		})

		It("should return nil scheme when not provided", func() {
			cluster := testutil.NewMockCluster(testName, testNamespace)
			wrapper := testutil.WrapMockCluster(cluster)
			Expect(wrapper.GetScheme()).To(BeNil())
		})

		It("should implement GetObjectMeta", func() {
			cluster := testutil.NewMockCluster(testName, testNamespace)
			wrapper := testutil.WrapMockCluster(cluster)
			meta := wrapper.GetObjectMeta()
			Expect(meta).NotTo(BeNil())
			Expect(meta.Name).To(Equal(testName))
		})

		It("should implement GetUID", func() {
			cluster := testutil.NewMockCluster(testName, testNamespace)
			cluster.UID = types.UID("test-uid")
			wrapper := testutil.WrapMockCluster(cluster)
			Expect(wrapper.GetUID()).To(Equal("test-uid"))
		})

		It("should implement GetName", func() {
			cluster := testutil.NewMockCluster(testName, testNamespace)
			wrapper := testutil.WrapMockCluster(cluster)
			Expect(wrapper.GetName()).To(Equal(testName))
		})

		It("should implement GetNamespace", func() {
			cluster := testutil.NewMockCluster(testName, testNamespace)
			wrapper := testutil.WrapMockCluster(cluster)
			Expect(wrapper.GetNamespace()).To(Equal(testNamespace))
		})

		It("should implement GetLabels", func() {
			cluster := testutil.NewMockCluster(testName, testNamespace)
			wrapper := testutil.WrapMockCluster(cluster)
			labels := wrapper.GetLabels()
			Expect(labels).NotTo(BeNil())
			Expect(labels["app.kubernetes.io/name"]).To(Equal(testName))
		})

		It("should return empty map for nil labels", func() {
			cluster := &testutil.MockCluster{}
			wrapper := testutil.WrapMockCluster(cluster)
			labels := wrapper.GetLabels()
			Expect(labels).NotTo(BeNil())
			Expect(len(labels)).To(Equal(0))
		})

		It("should implement GetAnnotations", func() {
			cluster := testutil.NewMockCluster(testName, testNamespace)
			cluster.WithAnnotations(map[string]string{"test": "value"})
			wrapper := testutil.WrapMockCluster(cluster)
			annotations := wrapper.GetAnnotations()
			Expect(annotations).NotTo(BeNil())
			Expect(annotations["test"]).To(Equal("value"))
		})

		It("should return empty map for nil annotations", func() {
			cluster := &testutil.MockCluster{}
			wrapper := testutil.WrapMockCluster(cluster)
			annotations := wrapper.GetAnnotations()
			Expect(annotations).NotTo(BeNil())
			Expect(len(annotations)).To(Equal(0))
		})

		It("should implement GetSpec", func() {
			cluster := testutil.NewMockCluster(testName, testNamespace)
			wrapper := testutil.WrapMockCluster(cluster)
			spec := wrapper.GetSpec()
			Expect(spec).NotTo(BeNil())
		})

		It("should implement GetStatus", func() {
			cluster := testutil.NewMockCluster(testName, testNamespace)
			wrapper := testutil.WrapMockCluster(cluster)
			status := wrapper.GetStatus()
			Expect(status).NotTo(BeNil())
		})

		It("should implement SetStatus", func() {
			cluster := testutil.NewMockCluster(testName, testNamespace)
			wrapper := testutil.WrapMockCluster(cluster)
			newStatus := &v1alpha1.GenericClusterStatus{
				Conditions: []metav1.Condition{
					{
						Type:   "Ready",
						Status: metav1.ConditionTrue,
					},
				},
			}
			wrapper.SetStatus(newStatus)
			Expect(wrapper.GetStatus().Conditions).To(HaveLen(1))
		})

		It("should implement DeepCopyCluster", func() {
			cluster := testutil.NewMockCluster(testName, testNamespace)
			wrapper := testutil.WrapMockCluster(cluster)
			copied := wrapper.DeepCopyCluster()
			Expect(copied).NotTo(BeNil())
			Expect(copied.GetName()).To(Equal(testName))
		})

		It("should handle nil DeepCopyCluster", func() {
			var wrapper *testutil.ClusterWrapper
			copied := wrapper.DeepCopyCluster()
			Expect(copied).NotTo(BeNil())
		})

		It("should implement GetRuntimeObject", func() {
			cluster := testutil.NewMockCluster(testName, testNamespace)
			wrapper := testutil.WrapMockCluster(cluster)
			obj := wrapper.GetRuntimeObject()
			Expect(obj).NotTo(BeNil())
		})
	})

	Context("MockRoleGroupHandler", func() {
		It("should create a new MockRoleGroupHandler", func() {
			handler := testutil.NewMockRoleGroupHandler()
			Expect(handler).NotTo(BeNil())
			Expect(handler.Image).To(Equal("test-image:latest"))
		})

		It("should set image with WithImage", func() {
			handler := testutil.NewMockRoleGroupHandler()
			handler.WithImage("custom-image:v1")
			Expect(handler.Image).To(Equal("custom-image:v1"))
		})

		It("should build default resources", func() {
			handler := testutil.NewMockRoleGroupHandler()
			buildCtx := &reconciler.RoleGroupBuildContext{
				ResourceName:     testName,
				ClusterNamespace: testNamespace,
				RoleGroupName:    "default",
				RoleGroupSpec:    v1alpha1.RoleGroupSpec{},
			}
			cluster := testutil.NewMockCluster(testName, testNamespace)
			wrapper := testutil.WrapMockCluster(cluster)

			resources, err := handler.BuildResources(context.Background(), k8sClient, wrapper, buildCtx)
			Expect(err).To(BeNil())
			Expect(resources).NotTo(BeNil())
			Expect(resources.ConfigMap).NotTo(BeNil())
			Expect(resources.Service).NotTo(BeNil())
			Expect(resources.StatefulSet).NotTo(BeNil())
		})

		It("should use custom BuildResourcesFunc", func() {
			handler := testutil.NewMockRoleGroupHandler()
			customError := "custom error"
			handler.WithBuildResourcesFunc(func(ctx context.Context, k8sClient client.Client, cr *testutil.ClusterWrapper, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
				return nil, errors.New(customError)
			})

			buildCtx := &reconciler.RoleGroupBuildContext{
				ResourceName:     testName,
				ClusterNamespace: testNamespace,
			}
			cluster := testutil.NewMockCluster(testName, testNamespace)
			wrapper := testutil.WrapMockCluster(cluster)

			_, err := handler.BuildResources(context.Background(), k8sClient, wrapper, buildCtx)
			Expect(err).NotTo(BeNil())
			Expect(err.Error()).To(ContainSubstring(customError))
		})
	})

	Context("MockExtension", func() {
		It("should create a new MockExtension", func() {
			ext := testutil.NewMockExtension("test-extension")
			Expect(ext).NotTo(BeNil())
			Expect(ext.Name).To(Equal("test-extension"))
		})

		It("should implement GetExtensionName", func() {
			ext := testutil.NewMockExtension("test-extension")
			Expect(ext.GetExtensionName()).To(Equal("test-extension"))
		})

		It("should implement ClusterPreReconcile with nil func", func() {
			ext := testutil.NewMockExtension("test-extension")
			cluster := testutil.NewMockCluster(testName, testNamespace)
			wrapper := testutil.WrapMockCluster(cluster)
			err := ext.ClusterPreReconcile(context.Background(), k8sClient, wrapper)
			Expect(err).To(BeNil())
		})

		It("should implement ClusterPreReconcile with custom func", func() {
			ext := testutil.NewMockExtension("test-extension")
			called := false
			ext.WithPreReconcile(func(ctx context.Context, client client.Client, cr common.ClusterInterface) error {
				called = true
				return nil
			})
			cluster := testutil.NewMockCluster(testName, testNamespace)
			wrapper := testutil.WrapMockCluster(cluster)
			err := ext.ClusterPreReconcile(context.Background(), k8sClient, wrapper)
			Expect(err).To(BeNil())
			Expect(called).To(BeTrue())
		})

		It("should implement ClusterPostReconcile with nil func", func() {
			ext := testutil.NewMockExtension("test-extension")
			cluster := testutil.NewMockCluster(testName, testNamespace)
			wrapper := testutil.WrapMockCluster(cluster)
			err := ext.ClusterPostReconcile(context.Background(), k8sClient, wrapper)
			Expect(err).To(BeNil())
		})

		It("should implement ClusterPostReconcile with custom func", func() {
			ext := testutil.NewMockExtension("test-extension")
			called := false
			ext.WithPostReconcile(func(ctx context.Context, client client.Client, cr common.ClusterInterface) error {
				called = true
				return nil
			})
			cluster := testutil.NewMockCluster(testName, testNamespace)
			wrapper := testutil.WrapMockCluster(cluster)
			err := ext.ClusterPostReconcile(context.Background(), k8sClient, wrapper)
			Expect(err).To(BeNil())
			Expect(called).To(BeTrue())
		})

		It("should implement ClusterOnError with nil func", func() {
			ext := testutil.NewMockExtension("test-extension")
			cluster := testutil.NewMockCluster(testName, testNamespace)
			wrapper := testutil.WrapMockCluster(cluster)
			err := ext.ClusterOnError(context.Background(), k8sClient, wrapper, errors.New("test error"))
			Expect(err).To(BeNil())
		})

		It("should implement ClusterOnError with custom func", func() {
			ext := testutil.NewMockExtension("test-extension")
			called := false
			ext.WithOnError(func(ctx context.Context, client client.Client, cr common.ClusterInterface, err error) error {
				called = true
				return nil
			})
			cluster := testutil.NewMockCluster(testName, testNamespace)
			wrapper := testutil.WrapMockCluster(cluster)
			err := ext.ClusterOnError(context.Background(), k8sClient, wrapper, errors.New("test error"))
			Expect(err).To(BeNil())
			Expect(called).To(BeTrue())
		})

		It("should return error from PreReconcile func", func() {
			ext := testutil.NewMockExtension("test-extension")
			ext.WithPreReconcile(func(ctx context.Context, client client.Client, cr common.ClusterInterface) error {
				return errors.New("pre-reconcile error")
			})
			cluster := testutil.NewMockCluster(testName, testNamespace)
			wrapper := testutil.WrapMockCluster(cluster)
			err := ext.ClusterPreReconcile(context.Background(), k8sClient, wrapper)
			Expect(err).NotTo(BeNil())
			Expect(err.Error()).To(ContainSubstring("pre-reconcile error"))
		})
	})

	Context("DefaultRoleGroupResources", func() {
		It("should create default role group resources", func() {
			resources := testutil.DefaultRoleGroupResources(testName, testNamespace, "test-image:v1")
			Expect(resources).NotTo(BeNil())
			Expect(resources.ConfigMap).NotTo(BeNil())
			Expect(resources.ConfigMap.Name).To(Equal(testName))
			Expect(resources.Service).NotTo(BeNil())
			Expect(resources.Service.Name).To(Equal(testName))
			Expect(resources.StatefulSet).NotTo(BeNil())
			Expect(resources.StatefulSet.Name).To(Equal(testName))
			Expect(resources.PodDisruptionBudget).NotTo(BeNil())
			Expect(resources.PodDisruptionBudget.Name).To(Equal(testName))
		})
	})

	Context("MockClusterList", func() {
		It("should create MockClusterList", func() {
			cluster := testutil.NewMockCluster(testName, testNamespace)
			list := testutil.MockClusterList{
				Items: []testutil.MockCluster{*cluster},
			}
			Expect(len(list.Items)).To(Equal(1))
		})

		It("should implement DeepCopyObject", func() {
			cluster := testutil.NewMockCluster(testName, testNamespace)
			list := testutil.MockClusterList{
				Items: []testutil.MockCluster{*cluster},
			}
			copied := list.DeepCopyObject()
			Expect(copied).NotTo(BeNil())
			copiedList, ok := copied.(*testutil.MockClusterList)
			Expect(ok).To(BeTrue())
			Expect(len(copiedList.Items)).To(Equal(1))
		})
	})

	Context("SchemeBuilder", func() {
		It("should add MockCluster to scheme", func() {
			scheme := runtime.NewScheme()
			err := testutil.AddToScheme(scheme)
			Expect(err).To(BeNil())

			// Verify MockCluster is registered
			gvk := schema.GroupVersionKind{
				Group:   "test.zncdata.dev",
				Version: "v1alpha1",
				Kind:    "MockCluster",
			}
			obj, err := scheme.New(gvk)
			Expect(err).To(BeNil())
			Expect(obj).NotTo(BeNil())
		})
	})
})
