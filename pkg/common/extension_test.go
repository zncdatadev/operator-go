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

package common_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/common"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("ExtensionRegistry", func() {
	var registry *common.ExtensionRegistry

	BeforeEach(func() {
		registry = common.GetExtensionRegistry()
		// Clear registry for each test
		registry.Clear()
	})

	Describe("RegisterClusterExtension", func() {
		It("should register an extension", func() {
			ext := &mockClusterExtension{name: "test-extension"}
			registry.RegisterClusterExtension(ext)

			extensions := registry.GetClusterExtensions()
			Expect(extensions).To(HaveLen(1))
		})
	})

	Describe("ExecuteClusterPreReconcile", func() {
		It("should execute PreReconcile hooks", func() {
			executed := false
			ext := &mockClusterExtension{
				name: "test-extension",
				preReconcileFunc: func(ctx context.Context, client client.Client, cr common.ClusterInterface) error {
					executed = true
					return nil
				},
			}
			registry.RegisterClusterExtension(ext)

			err := registry.ExecuteClusterPreReconcile(context.Background(), k8sClient, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(executed).To(BeTrue())
		})

		It("should return error when PreReconcile hook fails", func() {
			ext := &mockClusterExtension{
				name: "test-extension",
				preReconcileFunc: func(ctx context.Context, client client.Client, cr common.ClusterInterface) error {
					return errors.New("hook error")
				},
			}
			registry.RegisterClusterExtension(ext)

			err := registry.ExecuteClusterPreReconcile(context.Background(), k8sClient, nil)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("ExecuteClusterPostReconcile", func() {
		It("should execute PostReconcile hooks", func() {
			executed := false
			ext := &mockClusterExtension{
				name: "test-extension",
				postReconcileFunc: func(ctx context.Context, client client.Client, cr common.ClusterInterface) error {
					executed = true
					return nil
				},
			}
			registry.RegisterClusterExtension(ext)

			err := registry.ExecuteClusterPostReconcile(context.Background(), k8sClient, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(executed).To(BeTrue())
		})
	})

	Describe("HasClusterExtensions", func() {
		It("should return false when no extensions registered", func() {
			Expect(registry.HasClusterExtensions()).To(BeFalse())
		})

		It("should return true when extensions registered", func() {
			ext := &mockClusterExtension{name: "test-extension"}
			registry.RegisterClusterExtension(ext)
			Expect(registry.HasClusterExtensions()).To(BeTrue())
		})
	})

	Describe("Count", func() {
		It("should return 0 when no extensions registered", func() {
			Expect(registry.Count()).To(Equal(0))
		})

		It("should return correct count", func() {
			ext := &mockClusterExtension{name: "test-extension"}
			registry.RegisterClusterExtension(ext)
			Expect(registry.Count()).To(Equal(1))
		})
	})
})

// mockClusterExtension is a test implementation of ClusterExtension
type mockClusterExtension struct {
	name              string
	preReconcileFunc  func(ctx context.Context, client client.Client, cr common.ClusterInterface) error
	postReconcileFunc func(ctx context.Context, client client.Client, cr common.ClusterInterface) error
	onErrorFunc       func(ctx context.Context, client client.Client, cr common.ClusterInterface, err error) error
}

func (e *mockClusterExtension) Name() string {
	return e.name
}

func (e *mockClusterExtension) PreReconcile(ctx context.Context, client client.Client, cr common.ClusterInterface) error {
	if e.preReconcileFunc != nil {
		return e.preReconcileFunc(ctx, client, cr)
	}
	return nil
}

func (e *mockClusterExtension) PostReconcile(ctx context.Context, client client.Client, cr common.ClusterInterface) error {
	if e.postReconcileFunc != nil {
		return e.postReconcileFunc(ctx, client, cr)
	}
	return nil
}

func (e *mockClusterExtension) OnReconcileError(ctx context.Context, client client.Client, cr common.ClusterInterface, reconcileErr error) error {
	if e.onErrorFunc != nil {
		return e.onErrorFunc(ctx, client, cr, reconcileErr)
	}
	return nil
}
