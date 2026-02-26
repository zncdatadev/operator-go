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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/common"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/testutil"
	corev1 "k8s.io/api/core/v1"
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

// handlerAdapter adapts MockRoleGroupHandler to the RoleGroupHandler interface
type handlerAdapter struct {
	handler *testutil.MockRoleGroupHandler
}

// BuildResources implements reconciler.RoleGroupHandler
func (a *handlerAdapter) BuildResources(ctx context.Context, k8sClient client.Client, cr *testutil.ClusterWrapper, buildCtx *reconciler.RoleGroupBuildContext) (*reconciler.RoleGroupResources, error) {
	return a.handler.BuildResources(ctx, k8sClient, cr, buildCtx)
}

// GetContainerImage implements reconciler.RoleGroupHandler
func (a *handlerAdapter) GetContainerImage(roleName string) string {
	return a.handler.Image
}

// GetContainerPorts implements reconciler.RoleGroupHandler
func (a *handlerAdapter) GetContainerPorts(roleName, roleGroupName string) []corev1.ContainerPort {
	if ports, ok := a.handler.ContainerPorts[roleName]; ok {
		return ports
	}
	return nil
}

// GetServicePorts implements reconciler.RoleGroupHandler
func (a *handlerAdapter) GetServicePorts(roleName, roleGroupName string) []corev1.ServicePort {
	if ports, ok := a.handler.ServicePorts[roleName]; ok {
		return ports
	}
	return nil
}

// Verify interface implementations
var _ common.ClusterInterface = &testutil.ClusterWrapper{}
var _ reconciler.RoleGroupHandler[*testutil.ClusterWrapper] = &handlerAdapter{}
