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

package testutil

import (
	"context"

	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/common"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MockCluster is a test CR that embeds ObjectMeta for client.Object compatibility.
type MockCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              v1alpha1.GenericClusterSpec   `json:"spec,omitempty"`
	Status            v1alpha1.GenericClusterStatus `json:"status,omitempty"`
}

// NewMockCluster creates a new MockCluster with default values.
func NewMockCluster(name, namespace string) *MockCluster {
	return &MockCluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "MockCluster",
			APIVersion: "test.zncdata.dev/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":     name,
				"app.kubernetes.io/instance": name,
			},
		},
		Spec: v1alpha1.GenericClusterSpec{},
	}
}

// WithLabels sets labels on the MockCluster.
func (m *MockCluster) WithLabels(labels map[string]string) *MockCluster {
	if m.Labels == nil {
		m.Labels = make(map[string]string)
	}
	for k, v := range labels {
		m.Labels[k] = v
	}
	return m
}

// WithAnnotations sets annotations on the MockCluster.
func (m *MockCluster) WithAnnotations(annotations map[string]string) *MockCluster {
	if m.Annotations == nil {
		m.Annotations = make(map[string]string)
	}
	for k, v := range annotations {
		m.Annotations[k] = v
	}
	return m
}

// WithRoles sets roles on the MockCluster.
func (m *MockCluster) WithRoles(roles map[string]v1alpha1.RoleSpec) *MockCluster {
	m.Spec.Roles = roles
	return m
}

// WithClusterOperation sets cluster operation on the MockCluster.
func (m *MockCluster) WithClusterOperation(op *v1alpha1.ClusterOperationSpec) *MockCluster {
	m.Spec.ClusterOperation = op
	return m
}

// DeepCopy creates a deep copy of MockCluster.
func (m *MockCluster) DeepCopy() *MockCluster {
	if m == nil {
		return nil
	}
	out := new(MockCluster)
	*out = *m
	out.TypeMeta = m.TypeMeta
	m.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = *m.Spec.DeepCopy()
	out.Status = *m.Status.DeepCopy()
	return out
}

// DeepCopyObject implements runtime.Object.
func (m *MockCluster) DeepCopyObject() runtime.Object {
	return m.DeepCopy()
}

// ClusterWrapper wraps MockCluster to implement common.ClusterInterface.
// This is needed because common.ClusterInterface expects GetUID() string,
// but client.Object expects GetUID() types.UID.
type ClusterWrapper struct {
	*MockCluster
	scheme *runtime.Scheme
}

// WrapMockCluster wraps a MockCluster to implement common.ClusterInterface.
// An optional scheme can be provided for tests that require scheme access.
func WrapMockCluster(m *MockCluster, scheme ...*runtime.Scheme) *ClusterWrapper {
	var s *runtime.Scheme
	if len(scheme) > 0 {
		s = scheme[0]
	}
	return &ClusterWrapper{MockCluster: m, scheme: s}
}

// GetObjectMeta implements common.ClusterInterface.
func (w *ClusterWrapper) GetObjectMeta() *metav1.ObjectMeta {
	return &w.ObjectMeta
}

// GetUID returns the UID as string for common.ClusterInterface compatibility.
func (w *ClusterWrapper) GetUID() string {
	return string(w.ObjectMeta.UID)
}

// GetName implements common.ClusterInterface.
func (w *ClusterWrapper) GetName() string {
	return w.Name
}

// GetNamespace implements common.ClusterInterface.
func (w *ClusterWrapper) GetNamespace() string {
	return w.Namespace
}

// GetLabels implements common.ClusterInterface.
func (w *ClusterWrapper) GetLabels() map[string]string {
	if w.Labels == nil {
		return make(map[string]string)
	}
	return w.Labels
}

// GetAnnotations implements common.ClusterInterface.
func (w *ClusterWrapper) GetAnnotations() map[string]string {
	if w.Annotations == nil {
		return make(map[string]string)
	}
	return w.Annotations
}

// GetSpec implements common.ClusterInterface.
func (w *ClusterWrapper) GetSpec() *v1alpha1.GenericClusterSpec {
	return &w.Spec
}

// GetStatus implements common.ClusterInterface.
func (w *ClusterWrapper) GetStatus() *v1alpha1.GenericClusterStatus {
	return &w.Status
}

// SetStatus implements common.ClusterInterface.
func (w *ClusterWrapper) SetStatus(status *v1alpha1.GenericClusterStatus) {
	w.Status = *status
}

// GetScheme implements common.ClusterInterface.
// Returns the scheme if one was provided during wrapping, nil otherwise.
func (w *ClusterWrapper) GetScheme() *runtime.Scheme {
	return w.scheme
}

// DeepCopyCluster implements common.ClusterInterface.
func (w *ClusterWrapper) DeepCopyCluster() common.ClusterInterface {
	return WrapMockCluster(w.MockCluster.DeepCopy())
}

// GetRuntimeObject implements common.ClusterInterface.
func (w *ClusterWrapper) GetRuntimeObject() runtime.Object {
	return w.MockCluster
}

// MockRoleGroupHandler is a test implementation of RoleGroupHandler.
type MockRoleGroupHandler struct {
	BuildResourcesFunc func(ctx context.Context, k8sClient client.Client, cr *ClusterWrapper, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error)
	Image              string
	ContainerPorts     map[string][]corev1.ContainerPort
	ServicePorts       map[string][]corev1.ServicePort
}

// NewMockRoleGroupHandler creates a new MockRoleGroupHandler.
func NewMockRoleGroupHandler() *MockRoleGroupHandler {
	return &MockRoleGroupHandler{
		Image:          "test-image:latest",
		ContainerPorts: make(map[string][]corev1.ContainerPort),
		ServicePorts:   make(map[string][]corev1.ServicePort),
	}
}

// WithImage sets the image on the MockRoleGroupHandler.
func (h *MockRoleGroupHandler) WithImage(image string) *MockRoleGroupHandler {
	h.Image = image
	return h
}

// WithBuildResourcesFunc sets the BuildResourcesFunc.
func (h *MockRoleGroupHandler) WithBuildResourcesFunc(fn func(ctx context.Context, k8sClient client.Client, cr *ClusterWrapper, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error)) *MockRoleGroupHandler {
	h.BuildResourcesFunc = fn
	return h
}

// BuildResources builds resources using the mock handler.
func (h *MockRoleGroupHandler) BuildResources(ctx context.Context, k8sClient client.Client, cr *ClusterWrapper, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
	if h.BuildResourcesFunc != nil {
		return h.BuildResourcesFunc(ctx, k8sClient, cr, buildCtx)
	}

	// Return default resources
	return &reconciler.RoleGroupResources{
		ConfigMap: NewTestConfigMap(buildCtx.ResourceName, buildCtx.ClusterNamespace),
		Service:   NewTestService(buildCtx.ResourceName, buildCtx.ClusterNamespace),
		StatefulSet: NewTestStatefulSetBuilder(buildCtx.ResourceName, buildCtx.ClusterNamespace).
			WithImage(h.Image, corev1.PullIfNotPresent).
			Build(),
	}, nil
}

// MockExtension is a test implementation of ClusterExtension.
type MockExtension struct {
	PreReconcileFunc  func(ctx context.Context, client client.Client, cr common.ClusterInterface) error
	PostReconcileFunc func(ctx context.Context, client client.Client, cr common.ClusterInterface) error
	OnErrorFunc       func(ctx context.Context, client client.Client, cr common.ClusterInterface, err error) error
	Name              string
}

// NewMockExtension creates a new MockExtension.
func NewMockExtension(name string) *MockExtension {
	return &MockExtension{
		Name: name,
	}
}

// WithPreReconcile sets the PreReconcile function.
func (e *MockExtension) WithPreReconcile(fn func(ctx context.Context, client client.Client, cr common.ClusterInterface) error) *MockExtension {
	e.PreReconcileFunc = fn
	return e
}

// WithPostReconcile sets the PostReconcile function.
func (e *MockExtension) WithPostReconcile(fn func(ctx context.Context, client client.Client, cr common.ClusterInterface) error) *MockExtension {
	e.PostReconcileFunc = fn
	return e
}

// WithOnError sets the OnError function.
func (e *MockExtension) WithOnError(fn func(ctx context.Context, client client.Client, cr common.ClusterInterface, err error) error) *MockExtension {
	e.OnErrorFunc = fn
	return e
}

// GetExtensionName returns the extension name.
func (e *MockExtension) GetExtensionName() string {
	return e.Name
}

// ClusterPreReconcile implements ClusterExtension.
func (e *MockExtension) ClusterPreReconcile(ctx context.Context, client client.Client, cr common.ClusterInterface) error {
	if e.PreReconcileFunc != nil {
		return e.PreReconcileFunc(ctx, client, cr)
	}
	return nil
}

// ClusterPostReconcile implements ClusterExtension.
func (e *MockExtension) ClusterPostReconcile(ctx context.Context, client client.Client, cr common.ClusterInterface) error {
	if e.PostReconcileFunc != nil {
		return e.PostReconcileFunc(ctx, client, cr)
	}
	return nil
}

// ClusterOnError implements ClusterExtension.
func (e *MockExtension) ClusterOnError(ctx context.Context, client client.Client, cr common.ClusterInterface, err error) error {
	if e.OnErrorFunc != nil {
		return e.OnErrorFunc(ctx, client, cr, err)
	}
	return nil
}

// DefaultRoleGroupResources returns a RoleGroupResources with all standard resources.
func DefaultRoleGroupResources(name, namespace, image string) *reconciler.RoleGroupResources {
	maxUnavailable := intstr.FromInt(1)
	return &reconciler.RoleGroupResources{
		ConfigMap:   NewTestConfigMap(name, namespace),
		Service:     NewTestService(name, namespace),
		StatefulSet: NewTestStatefulSetBuilder(name, namespace).WithImage(image, corev1.PullIfNotPresent).Build(),
		PodDisruptionBudget: &policyv1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: policyv1.PodDisruptionBudgetSpec{
				MaxUnavailable: &maxUnavailable,
			},
		},
	}
}

// Verify interface implementations
var _ common.ClusterInterface = &ClusterWrapper{}
