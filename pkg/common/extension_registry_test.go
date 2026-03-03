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

// MockClusterExtension is a mock implementation of ClusterExtension for testing
type MockClusterExtension struct {
	NameFunc             func() string
	PreReconcileFunc     func(ctx context.Context, client client.Client, cr common.ClusterInterface) error
	PostReconcileFunc    func(ctx context.Context, client client.Client, cr common.ClusterInterface) error
	OnReconcileErrorFunc func(ctx context.Context, client client.Client, cr common.ClusterInterface, reconcileErr error) error
}

func (m *MockClusterExtension) Name() string {
	if m.NameFunc != nil {
		return m.NameFunc()
	}
	return "mock-cluster-extension"
}

func (m *MockClusterExtension) PreReconcile(ctx context.Context, client client.Client, cr common.ClusterInterface) error {
	if m.PreReconcileFunc != nil {
		return m.PreReconcileFunc(ctx, client, cr)
	}
	return nil
}

func (m *MockClusterExtension) PostReconcile(ctx context.Context, client client.Client, cr common.ClusterInterface) error {
	if m.PostReconcileFunc != nil {
		return m.PostReconcileFunc(ctx, client, cr)
	}
	return nil
}

func (m *MockClusterExtension) OnReconcileError(ctx context.Context, client client.Client, cr common.ClusterInterface, reconcileErr error) error {
	if m.OnReconcileErrorFunc != nil {
		return m.OnReconcileErrorFunc(ctx, client, cr, reconcileErr)
	}
	return nil
}

// MockRoleExtension is a mock implementation of RoleExtension for testing
type MockRoleExtension struct {
	NameFunc          func() string
	PreReconcileFunc  func(ctx context.Context, client client.Client, cr common.ClusterInterface, roleName string) error
	PostReconcileFunc func(ctx context.Context, client client.Client, cr common.ClusterInterface, roleName string) error
}

func (m *MockRoleExtension) Name() string {
	if m.NameFunc != nil {
		return m.NameFunc()
	}
	return "mock-role-extension"
}

func (m *MockRoleExtension) PreReconcile(ctx context.Context, client client.Client, cr common.ClusterInterface, roleName string) error {
	if m.PreReconcileFunc != nil {
		return m.PreReconcileFunc(ctx, client, cr, roleName)
	}
	return nil
}

func (m *MockRoleExtension) PostReconcile(ctx context.Context, client client.Client, cr common.ClusterInterface, roleName string) error {
	if m.PostReconcileFunc != nil {
		return m.PostReconcileFunc(ctx, client, cr, roleName)
	}
	return nil
}

// MockRoleGroupExtension is a mock implementation of RoleGroupExtension for testing
type MockRoleGroupExtension struct {
	NameFunc          func() string
	PreReconcileFunc  func(ctx context.Context, client client.Client, cr common.ClusterInterface, roleName, roleGroupName string) error
	PostReconcileFunc func(ctx context.Context, client client.Client, cr common.ClusterInterface, roleName, roleGroupName string) error
}

func (m *MockRoleGroupExtension) Name() string {
	if m.NameFunc != nil {
		return m.NameFunc()
	}
	return "mock-rolegroup-extension"
}

func (m *MockRoleGroupExtension) PreReconcile(ctx context.Context, client client.Client, cr common.ClusterInterface, roleName, roleGroupName string) error {
	if m.PreReconcileFunc != nil {
		return m.PreReconcileFunc(ctx, client, cr, roleName, roleGroupName)
	}
	return nil
}

func (m *MockRoleGroupExtension) PostReconcile(ctx context.Context, client client.Client, cr common.ClusterInterface, roleName, roleGroupName string) error {
	if m.PostReconcileFunc != nil {
		return m.PostReconcileFunc(ctx, client, cr, roleName, roleGroupName)
	}
	return nil
}

var _ = Describe("ExtensionRegistry", func() {
	var registry *common.ExtensionRegistry

	BeforeEach(func() {
		registry = common.GetExtensionRegistry()
		registry.Clear()
	})

	AfterEach(func() {
		registry.Clear()
	})

	Describe("GetExtensionRegistry", func() {
		It("should return the global registry singleton", func() {
			r1 := common.GetExtensionRegistry()
			r2 := common.GetExtensionRegistry()
			Expect(r1).To(Equal(r2))
		})
	})

	Describe("ResetExtensionRegistry", func() {
		It("should reset the global registry", func() {
			registry.RegisterClusterExtension(&MockClusterExtension{})
			Expect(registry.Count()).To(BeNumerically(">", 0))

			common.ResetExtensionRegistry()
			registry = common.GetExtensionRegistry()
			Expect(registry.Count()).To(Equal(0))
		})
	})

	Describe("RegisterClusterExtension", func() {
		It("should register a cluster extension", func() {
			ext := &MockClusterExtension{}
			registry.RegisterClusterExtension(ext)
			Expect(registry.HasClusterExtensions()).To(BeTrue())
			Expect(registry.Count()).To(Equal(1))
		})
	})

	Describe("RegisterClusterExtensionWithPriority", func() {
		It("should register a cluster extension with priority", func() {
			ext := &MockClusterExtension{}
			registry.RegisterClusterExtensionWithPriority(ext, common.PriorityHigh)
			Expect(registry.HasClusterExtensions()).To(BeTrue())
		})

		It("should sort extensions by priority", func() {
			lowExt := &MockClusterExtension{NameFunc: func() string { return "low" }}
			highExt := &MockClusterExtension{NameFunc: func() string { return "high" }}

			registry.RegisterClusterExtensionWithPriority(lowExt, common.PriorityLow)
			registry.RegisterClusterExtensionWithPriority(highExt, common.PriorityHigh)

			extensions := registry.GetClusterExtensions()
			Expect(extensions[0].Name()).To(Equal("high"))
			Expect(extensions[1].Name()).To(Equal("low"))
		})
	})

	Describe("RegisterRoleExtension", func() {
		It("should register a role extension", func() {
			ext := &MockRoleExtension{}
			registry.RegisterRoleExtension(ext)
			Expect(registry.HasRoleExtensions()).To(BeTrue())
			Expect(registry.Count()).To(Equal(1))
		})
	})

	Describe("RegisterRoleExtensionWithPriority", func() {
		It("should register a role extension with priority", func() {
			ext := &MockRoleExtension{}
			registry.RegisterRoleExtensionWithPriority(ext, common.PriorityHigh)
			Expect(registry.HasRoleExtensions()).To(BeTrue())
		})
	})

	Describe("RegisterRoleGroupExtension", func() {
		It("should register a role group extension", func() {
			ext := &MockRoleGroupExtension{}
			registry.RegisterRoleGroupExtension(ext)
			Expect(registry.HasRoleGroupExtensions()).To(BeTrue())
			Expect(registry.Count()).To(Equal(1))
		})
	})

	Describe("RegisterRoleGroupExtensionWithPriority", func() {
		It("should register a role group extension with priority", func() {
			ext := &MockRoleGroupExtension{}
			registry.RegisterRoleGroupExtensionWithPriority(ext, common.PriorityHigh)
			Expect(registry.HasRoleGroupExtensions()).To(BeTrue())
		})
	})

	Describe("GetClusterExtensions", func() {
		It("should return all cluster extensions", func() {
			ext1 := &MockClusterExtension{NameFunc: func() string { return "ext1" }}
			ext2 := &MockClusterExtension{NameFunc: func() string { return "ext2" }}

			registry.RegisterClusterExtension(ext1)
			registry.RegisterClusterExtension(ext2)

			extensions := registry.GetClusterExtensions()
			Expect(extensions).To(HaveLen(2))
		})

		It("should return empty slice when no extensions registered", func() {
			extensions := registry.GetClusterExtensions()
			Expect(extensions).To(BeEmpty())
		})
	})

	Describe("GetRoleExtensions", func() {
		It("should return all role extensions", func() {
			ext1 := &MockRoleExtension{NameFunc: func() string { return "role-ext1" }}
			ext2 := &MockRoleExtension{NameFunc: func() string { return "role-ext2" }}

			registry.RegisterRoleExtension(ext1)
			registry.RegisterRoleExtension(ext2)

			extensions := registry.GetRoleExtensions()
			Expect(extensions).To(HaveLen(2))
		})
	})

	Describe("GetRoleGroupExtensions", func() {
		It("should return all role group extensions", func() {
			ext1 := &MockRoleGroupExtension{NameFunc: func() string { return "rg-ext1" }}
			ext2 := &MockRoleGroupExtension{NameFunc: func() string { return "rg-ext2" }}

			registry.RegisterRoleGroupExtension(ext1)
			registry.RegisterRoleGroupExtension(ext2)

			extensions := registry.GetRoleGroupExtensions()
			Expect(extensions).To(HaveLen(2))
		})
	})

	Describe("HasClusterExtensions", func() {
		It("should return false when no cluster extensions registered", func() {
			Expect(registry.HasClusterExtensions()).To(BeFalse())
		})

		It("should return true when cluster extensions registered", func() {
			registry.RegisterClusterExtension(&MockClusterExtension{})
			Expect(registry.HasClusterExtensions()).To(BeTrue())
		})
	})

	Describe("HasRoleExtensions", func() {
		It("should return false when no role extensions registered", func() {
			Expect(registry.HasRoleExtensions()).To(BeFalse())
		})

		It("should return true when role extensions registered", func() {
			registry.RegisterRoleExtension(&MockRoleExtension{})
			Expect(registry.HasRoleExtensions()).To(BeTrue())
		})
	})

	Describe("HasRoleGroupExtensions", func() {
		It("should return false when no role group extensions registered", func() {
			Expect(registry.HasRoleGroupExtensions()).To(BeFalse())
		})

		It("should return true when role group extensions registered", func() {
			registry.RegisterRoleGroupExtension(&MockRoleGroupExtension{})
			Expect(registry.HasRoleGroupExtensions()).To(BeTrue())
		})
	})

	Describe("Clear", func() {
		It("should remove all extensions", func() {
			registry.RegisterClusterExtension(&MockClusterExtension{})
			registry.RegisterRoleExtension(&MockRoleExtension{})
			registry.RegisterRoleGroupExtension(&MockRoleGroupExtension{})

			Expect(registry.Count()).To(Equal(3))

			registry.Clear()

			Expect(registry.Count()).To(Equal(0))
			Expect(registry.HasClusterExtensions()).To(BeFalse())
			Expect(registry.HasRoleExtensions()).To(BeFalse())
			Expect(registry.HasRoleGroupExtensions()).To(BeFalse())
		})
	})

	Describe("Count", func() {
		It("should return total count of all extensions", func() {
			registry.RegisterClusterExtension(&MockClusterExtension{})
			registry.RegisterClusterExtension(&MockClusterExtension{})
			registry.RegisterRoleExtension(&MockRoleExtension{})
			registry.RegisterRoleGroupExtension(&MockRoleGroupExtension{})

			Expect(registry.Count()).To(Equal(4))
		})

		It("should return 0 when no extensions registered", func() {
			Expect(registry.Count()).To(Equal(0))
		})
	})

	Describe("ExecuteClusterPreReconcile", func() {
		It("should execute all PreReconcile hooks", func() {
			executed := false
			ext := &MockClusterExtension{
				PreReconcileFunc: func(ctx context.Context, client client.Client, cr common.ClusterInterface) error {
					executed = true
					return nil
				},
			}
			registry.RegisterClusterExtension(ext)

			err := registry.ExecuteClusterPreReconcile(context.Background(), nil, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(executed).To(BeTrue())
		})

		It("should return error when PreReconcile fails", func() {
			ext := &MockClusterExtension{
				PreReconcileFunc: func(ctx context.Context, client client.Client, cr common.ClusterInterface) error {
					return errors.New("pre-reconcile error")
				},
			}
			registry.RegisterClusterExtension(ext)

			err := registry.ExecuteClusterPreReconcile(context.Background(), nil, nil)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("ExecuteClusterPostReconcile", func() {
		It("should execute all PostReconcile hooks", func() {
			executed := false
			ext := &MockClusterExtension{
				PostReconcileFunc: func(ctx context.Context, client client.Client, cr common.ClusterInterface) error {
					executed = true
					return nil
				},
			}
			registry.RegisterClusterExtension(ext)

			err := registry.ExecuteClusterPostReconcile(context.Background(), nil, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(executed).To(BeTrue())
		})

		It("should return error when PostReconcile fails", func() {
			ext := &MockClusterExtension{
				PostReconcileFunc: func(ctx context.Context, client client.Client, cr common.ClusterInterface) error {
					return errors.New("post-reconcile error")
				},
			}
			registry.RegisterClusterExtension(ext)

			err := registry.ExecuteClusterPostReconcile(context.Background(), nil, nil)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("ExecuteClusterOnError", func() {
		It("should execute all OnReconcileError hooks", func() {
			executed := false
			ext := &MockClusterExtension{
				OnReconcileErrorFunc: func(ctx context.Context, client client.Client, cr common.ClusterInterface, reconcileErr error) error {
					executed = true
					return nil
				},
			}
			registry.RegisterClusterExtension(ext)

			err := registry.ExecuteClusterOnError(context.Background(), nil, nil, errors.New("test error"))
			Expect(err).ToNot(HaveOccurred())
			Expect(executed).To(BeTrue())
		})
	})

	Describe("ExecuteRolePreReconcile", func() {
		It("should execute all role PreReconcile hooks", func() {
			executed := false
			ext := &MockRoleExtension{
				PreReconcileFunc: func(ctx context.Context, client client.Client, cr common.ClusterInterface, roleName string) error {
					executed = true
					Expect(roleName).To(Equal("test-role"))
					return nil
				},
			}
			registry.RegisterRoleExtension(ext)

			err := registry.ExecuteRolePreReconcile(context.Background(), nil, nil, "test-role")
			Expect(err).ToNot(HaveOccurred())
			Expect(executed).To(BeTrue())
		})
	})

	Describe("ExecuteRolePostReconcile", func() {
		It("should execute all role PostReconcile hooks", func() {
			executed := false
			ext := &MockRoleExtension{
				PostReconcileFunc: func(ctx context.Context, client client.Client, cr common.ClusterInterface, roleName string) error {
					executed = true
					return nil
				},
			}
			registry.RegisterRoleExtension(ext)

			err := registry.ExecuteRolePostReconcile(context.Background(), nil, nil, "test-role")
			Expect(err).ToNot(HaveOccurred())
			Expect(executed).To(BeTrue())
		})
	})

	Describe("ExecuteRoleGroupPreReconcile", func() {
		It("should execute all role group PreReconcile hooks", func() {
			executed := false
			ext := &MockRoleGroupExtension{
				PreReconcileFunc: func(ctx context.Context, client client.Client, cr common.ClusterInterface, roleName, roleGroupName string) error {
					executed = true
					Expect(roleName).To(Equal("test-role"))
					Expect(roleGroupName).To(Equal("test-group"))
					return nil
				},
			}
			registry.RegisterRoleGroupExtension(ext)

			err := registry.ExecuteRoleGroupPreReconcile(context.Background(), nil, nil, "test-role", "test-group")
			Expect(err).ToNot(HaveOccurred())
			Expect(executed).To(BeTrue())
		})
	})

	Describe("ExecuteRoleGroupPostReconcile", func() {
		It("should execute all role group PostReconcile hooks", func() {
			executed := false
			ext := &MockRoleGroupExtension{
				PostReconcileFunc: func(ctx context.Context, client client.Client, cr common.ClusterInterface, roleName, roleGroupName string) error {
					executed = true
					return nil
				},
			}
			registry.RegisterRoleGroupExtension(ext)

			err := registry.ExecuteRoleGroupPostReconcile(context.Background(), nil, nil, "test-role", "test-group")
			Expect(err).ToNot(HaveOccurred())
			Expect(executed).To(BeTrue())
		})
	})
})

var _ = Describe("ExtensionPriority constants", func() {
	It("should have correct PriorityHigh value", func() {
		Expect(common.PriorityHigh).To(Equal(common.ExtensionPriority(75)))
	})

	It("should have correct PriorityNormal value", func() {
		Expect(common.PriorityNormal).To(Equal(common.ExtensionPriority(50)))
	})

	It("should have correct PriorityLow value", func() {
		Expect(common.PriorityLow).To(Equal(common.ExtensionPriority(25)))
	})

	It("should have correct PriorityHighest value", func() {
		Expect(common.PriorityHighest).To(Equal(common.ExtensionPriority(100)))
	})

	It("should have correct PriorityLowest value", func() {
		Expect(common.PriorityLowest).To(Equal(common.ExtensionPriority(0)))
	})
})
