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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// The mocks are generic over the CR type so a single definition can be instantiated for either
// simulated product below, mirroring how a registry is instantiated per product CR.

// MockClusterExtension is a mock implementation of ClusterExtension for testing
type MockClusterExtension[CR common.ClusterInterface] struct {
	NameFunc             func() string
	PreReconcileFunc     func(ctx context.Context, client client.Client, cr CR) error
	PostReconcileFunc    func(ctx context.Context, client client.Client, cr CR) error
	OnReconcileErrorFunc func(ctx context.Context, client client.Client, cr CR, reconcileErr error) error
}

func (m *MockClusterExtension[CR]) Name() string {
	if m.NameFunc != nil {
		return m.NameFunc()
	}
	return "mock-cluster-extension"
}

func (m *MockClusterExtension[CR]) PreReconcile(ctx context.Context, client client.Client, cr CR) error {
	if m.PreReconcileFunc != nil {
		return m.PreReconcileFunc(ctx, client, cr)
	}
	return nil
}

func (m *MockClusterExtension[CR]) PostReconcile(ctx context.Context, client client.Client, cr CR) error {
	if m.PostReconcileFunc != nil {
		return m.PostReconcileFunc(ctx, client, cr)
	}
	return nil
}

func (m *MockClusterExtension[CR]) OnReconcileError(ctx context.Context, client client.Client, cr CR, reconcileErr error) error {
	if m.OnReconcileErrorFunc != nil {
		return m.OnReconcileErrorFunc(ctx, client, cr, reconcileErr)
	}
	return nil
}

// MockRoleExtension is a mock implementation of RoleExtension for testing
type MockRoleExtension[CR common.ClusterInterface] struct {
	NameFunc          func() string
	PreReconcileFunc  func(ctx context.Context, client client.Client, cr CR, roleName string) error
	PostReconcileFunc func(ctx context.Context, client client.Client, cr CR, roleName string) error
}

func (m *MockRoleExtension[CR]) Name() string {
	if m.NameFunc != nil {
		return m.NameFunc()
	}
	return "mock-role-extension"
}

func (m *MockRoleExtension[CR]) PreReconcile(ctx context.Context, client client.Client, cr CR, roleName string) error {
	if m.PreReconcileFunc != nil {
		return m.PreReconcileFunc(ctx, client, cr, roleName)
	}
	return nil
}

func (m *MockRoleExtension[CR]) PostReconcile(ctx context.Context, client client.Client, cr CR, roleName string) error {
	if m.PostReconcileFunc != nil {
		return m.PostReconcileFunc(ctx, client, cr, roleName)
	}
	return nil
}

// MockRoleGroupExtension is a mock implementation of RoleGroupExtension for testing
type MockRoleGroupExtension[CR common.ClusterInterface] struct {
	NameFunc          func() string
	PreReconcileFunc  func(ctx context.Context, client client.Client, cr CR, roleName, roleGroupName string) error
	PostReconcileFunc func(ctx context.Context, client client.Client, cr CR, roleName, roleGroupName string) error
}

func (m *MockRoleGroupExtension[CR]) Name() string {
	if m.NameFunc != nil {
		return m.NameFunc()
	}
	return "mock-rolegroup-extension"
}

func (m *MockRoleGroupExtension[CR]) PreReconcile(ctx context.Context, client client.Client, cr CR, roleName, roleGroupName string) error {
	if m.PreReconcileFunc != nil {
		return m.PreReconcileFunc(ctx, client, cr, roleName, roleGroupName)
	}
	return nil
}

func (m *MockRoleGroupExtension[CR]) PostReconcile(ctx context.Context, client client.Client, cr CR, roleName, roleGroupName string) error {
	if m.PostReconcileFunc != nil {
		return m.PostReconcileFunc(ctx, client, cr, roleName, roleGroupName)
	}
	return nil
}

// The registry is instantiated per CR type, so the specs below register extensions bound to the
// simulated HDFS product CR (defined in extension_product_pattern_test.go).
type hdfsClusterExtension = MockClusterExtension[*HdfsCluster]
type hdfsRoleExtension = MockRoleExtension[*HdfsCluster]
type hdfsRoleGroupExtension = MockRoleGroupExtension[*HdfsCluster]

// typedClusterExtension is written the way a product operator writes its extensions: against the
// concrete CR, reading product-only spec fields without converting anything.
type typedClusterExtension struct {
	common.BaseExtension
	seen     []string
	seenArgs []string
	err      error
}

func newTypedClusterExtension(err error) *typedClusterExtension {
	return &typedClusterExtension{BaseExtension: common.NewBaseExtension("typed-hdfs-extension"), err: err}
}

func (e *typedClusterExtension) record(cr *HdfsCluster) error {
	e.seen = append(e.seen, cr.Name)
	if cr.Spec.NameNodes != nil {
		e.seenArgs = cr.Spec.NameNodes.JvmArgumentOverrides
	}
	return e.err
}

func (e *typedClusterExtension) PreReconcile(_ context.Context, _ client.Client, cr *HdfsCluster) error {
	return e.record(cr)
}

func (e *typedClusterExtension) PostReconcile(_ context.Context, _ client.Client, cr *HdfsCluster) error {
	return e.record(cr)
}

func (e *typedClusterExtension) OnReconcileError(_ context.Context, _ client.Client, cr *HdfsCluster, _ error) error {
	return e.record(cr)
}

// typedRoleExtension is the role-level equivalent of typedClusterExtension.
type typedRoleExtension struct {
	common.BaseExtension
	seen []string
}

func newTypedRoleExtension() *typedRoleExtension {
	return &typedRoleExtension{BaseExtension: common.NewBaseExtension("typed-hdfs-role-extension")}
}

func (e *typedRoleExtension) PreReconcile(_ context.Context, _ client.Client, cr *HdfsCluster, roleName string) error {
	e.seen = append(e.seen, cr.Name+"/"+roleName)
	return nil
}

func (e *typedRoleExtension) PostReconcile(_ context.Context, _ client.Client, _ *HdfsCluster, _ string) error {
	return nil
}

// typedRoleGroupExtension is the role-group-level equivalent of typedClusterExtension.
type typedRoleGroupExtension struct {
	common.BaseExtension
	seen []string
}

func newTypedRoleGroupExtension() *typedRoleGroupExtension {
	return &typedRoleGroupExtension{BaseExtension: common.NewBaseExtension("typed-hdfs-rolegroup-extension")}
}

func (e *typedRoleGroupExtension) PreReconcile(_ context.Context, _ client.Client, cr *HdfsCluster, roleName, roleGroupName string) error {
	e.seen = append(e.seen, cr.Name+"/"+roleName+"/"+roleGroupName)
	return nil
}

func (e *typedRoleGroupExtension) PostReconcile(_ context.Context, _ client.Client, _ *HdfsCluster, _, _ string) error {
	return nil
}

var _ = Describe("ExtensionRegistry", func() {
	var registry *common.ExtensionRegistry[*HdfsCluster]

	BeforeEach(func() {
		registry = common.NewExtensionRegistry[*HdfsCluster]()
	})

	Describe("NewExtensionRegistry", func() {
		It("should create an empty registry", func() {
			Expect(registry.Count()).To(Equal(0))
		})

		It("should create registries that are isolated from one another", func() {
			other := common.NewExtensionRegistry[*HdfsCluster]()
			registry.RegisterClusterExtension(&hdfsClusterExtension{})

			Expect(registry.Count()).To(Equal(1))
			Expect(other.Count()).To(Equal(0))
		})
	})

	Describe("Typed registration", func() {
		var hdfs *HdfsCluster

		BeforeEach(func() {
			hdfs = &HdfsCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "hdfs-typed", Namespace: "default"},
				Spec: HdfsClusterSpec{
					NameNodes: &HdfsRoleSpec{JvmArgumentOverrides: []string{"-Xmx4g"}},
				},
			}
		})

		It("should hand the concrete CR to a cluster extension written for it", func() {
			ext := newTypedClusterExtension(nil)
			registry.RegisterClusterExtension(ext)

			Expect(registry.ExecuteClusterPreReconcile(context.Background(), nil, hdfs)).To(Succeed())

			// The hook read a field that only exists on the product CR, so it received the
			// concrete type rather than the ClusterInterface view of it.
			Expect(ext.seen).To(Equal([]string{"hdfs-typed"}))
			Expect(ext.seenArgs).To(Equal([]string{"-Xmx4g"}))
		})

		It("should hand the concrete CR to a role extension written for it", func() {
			ext := newTypedRoleExtension()
			registry.RegisterRoleExtension(ext)

			Expect(registry.ExecuteRolePreReconcile(context.Background(), nil, hdfs, "nameNodes")).To(Succeed())
			Expect(ext.seen).To(Equal([]string{"hdfs-typed/nameNodes"}))
		})

		It("should hand the concrete CR to a role group extension written for it", func() {
			ext := newTypedRoleGroupExtension()
			registry.RegisterRoleGroupExtension(ext)

			Expect(registry.ExecuteRoleGroupPreReconcile(context.Background(), nil, hdfs, "nameNodes", "default")).To(Succeed())
			Expect(ext.seen).To(Equal([]string{"hdfs-typed/nameNodes/default"}))
		})

		It("should report a typed hook failure under the extension name", func() {
			ext := newTypedClusterExtension(errors.New("typed failure"))
			registry.RegisterClusterExtension(ext)

			err := registry.ExecuteClusterPreReconcile(context.Background(), nil, hdfs)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("typed-hdfs-extension"))
			Expect(err.Error()).To(ContainSubstring("typed failure"))
		})
	})

	// A registry only ever holds extensions of its own CR type — a foreign extension does not
	// typecheck — so a process hosting several products cannot run one product's hooks against
	// another product's clusters. Before the registry was typed, all of them shared one instance
	// and every hook ran for every CR type.
	Describe("Per-CR-type isolation", func() {
		It("should execute only the extensions of the registry's own CR type", func() {
			var ran []string

			hdfsRegistry := common.NewExtensionRegistry[*HdfsCluster]()
			hdfsRegistry.RegisterClusterExtension(&hdfsClusterExtension{
				PreReconcileFunc: func(context.Context, client.Client, *HdfsCluster) error {
					ran = append(ran, "hdfs")
					return nil
				},
			})

			otherRegistry := common.NewExtensionRegistry[*MockClusterForProductTest]()
			otherRegistry.RegisterClusterExtension(&MockClusterExtension[*MockClusterForProductTest]{
				PreReconcileFunc: func(context.Context, client.Client, *MockClusterForProductTest) error {
					ran = append(ran, "other")
					return nil
				},
			})

			hdfs := &HdfsCluster{ObjectMeta: metav1.ObjectMeta{Name: "hdfs-isolated"}}
			Expect(hdfsRegistry.ExecuteClusterPreReconcile(context.Background(), nil, hdfs)).To(Succeed())
			Expect(ran).To(Equal([]string{"hdfs"}))

			other := &MockClusterForProductTest{name: "other-isolated"}
			Expect(otherRegistry.ExecuteClusterPreReconcile(context.Background(), nil, other)).To(Succeed())
			Expect(ran).To(Equal([]string{"hdfs", "other"}))

			Expect(hdfsRegistry.Count()).To(Equal(1))
			Expect(otherRegistry.Count()).To(Equal(1))
		})
	})

	Describe("RegisterClusterExtension", func() {
		It("should register a cluster extension", func() {
			registry.RegisterClusterExtension(&hdfsClusterExtension{})
			Expect(registry.HasClusterExtensions()).To(BeTrue())
			Expect(registry.Count()).To(Equal(1))
		})

		It("should register a cluster extension with priority", func() {
			registry.RegisterClusterExtension(&hdfsClusterExtension{}, common.WithPriority(common.PriorityHigh))
			Expect(registry.HasClusterExtensions()).To(BeTrue())
		})

		It("should sort extensions by priority", func() {
			lowExt := &hdfsClusterExtension{NameFunc: func() string { return "low" }}
			highExt := &hdfsClusterExtension{NameFunc: func() string { return "high" }}

			registry.RegisterClusterExtension(lowExt, common.WithPriority(common.PriorityLow))
			registry.RegisterClusterExtension(highExt, common.WithPriority(common.PriorityHigh))

			extensions := registry.GetClusterExtensions()
			Expect(extensions[0].Name()).To(Equal("high"))
			Expect(extensions[1].Name()).To(Equal("low"))
		})
	})

	Describe("RegisterRoleExtension", func() {
		It("should register a role extension", func() {
			registry.RegisterRoleExtension(&hdfsRoleExtension{})
			Expect(registry.HasRoleExtensions()).To(BeTrue())
			Expect(registry.Count()).To(Equal(1))
		})

		It("should register a role extension with priority", func() {
			registry.RegisterRoleExtension(&hdfsRoleExtension{}, common.WithPriority(common.PriorityHigh))
			Expect(registry.HasRoleExtensions()).To(BeTrue())
		})
	})

	Describe("RegisterRoleGroupExtension", func() {
		It("should register a role group extension", func() {
			registry.RegisterRoleGroupExtension(&hdfsRoleGroupExtension{})
			Expect(registry.HasRoleGroupExtensions()).To(BeTrue())
			Expect(registry.Count()).To(Equal(1))
		})

		It("should register a role group extension with priority", func() {
			registry.RegisterRoleGroupExtension(&hdfsRoleGroupExtension{}, common.WithPriority(common.PriorityHigh))
			Expect(registry.HasRoleGroupExtensions()).To(BeTrue())
		})
	})

	Describe("GetClusterExtensions", func() {
		It("should return all cluster extensions", func() {
			ext1 := &hdfsClusterExtension{NameFunc: func() string { return "ext1" }}
			ext2 := &hdfsClusterExtension{NameFunc: func() string { return "ext2" }}

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
			ext1 := &hdfsRoleExtension{NameFunc: func() string { return "role-ext1" }}
			ext2 := &hdfsRoleExtension{NameFunc: func() string { return "role-ext2" }}

			registry.RegisterRoleExtension(ext1)
			registry.RegisterRoleExtension(ext2)

			extensions := registry.GetRoleExtensions()
			Expect(extensions).To(HaveLen(2))
		})
	})

	Describe("GetRoleGroupExtensions", func() {
		It("should return all role group extensions", func() {
			ext1 := &hdfsRoleGroupExtension{NameFunc: func() string { return "rg-ext1" }}
			ext2 := &hdfsRoleGroupExtension{NameFunc: func() string { return "rg-ext2" }}

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
			registry.RegisterClusterExtension(&hdfsClusterExtension{})
			Expect(registry.HasClusterExtensions()).To(BeTrue())
		})
	})

	Describe("HasRoleExtensions", func() {
		It("should return false when no role extensions registered", func() {
			Expect(registry.HasRoleExtensions()).To(BeFalse())
		})

		It("should return true when role extensions registered", func() {
			registry.RegisterRoleExtension(&hdfsRoleExtension{})
			Expect(registry.HasRoleExtensions()).To(BeTrue())
		})
	})

	Describe("HasRoleGroupExtensions", func() {
		It("should return false when no role group extensions registered", func() {
			Expect(registry.HasRoleGroupExtensions()).To(BeFalse())
		})

		It("should return true when role group extensions registered", func() {
			registry.RegisterRoleGroupExtension(&hdfsRoleGroupExtension{})
			Expect(registry.HasRoleGroupExtensions()).To(BeTrue())
		})
	})

	Describe("Clear", func() {
		It("should remove all extensions", func() {
			registry.RegisterClusterExtension(&hdfsClusterExtension{})
			registry.RegisterRoleExtension(&hdfsRoleExtension{})
			registry.RegisterRoleGroupExtension(&hdfsRoleGroupExtension{})

			Expect(registry.Count()).To(Equal(3))

			registry.Clear()

			Expect(registry.Count()).To(Equal(0))
			Expect(registry.HasClusterExtensions()).To(BeFalse())
			Expect(registry.HasRoleExtensions()).To(BeFalse())
			Expect(registry.HasRoleGroupExtensions()).To(BeFalse())
		})

		It("should empty the registry in place so captured references stay valid", func() {
			// Reconcilers capture the registry at construction time; emptying has to be visible
			// through the reference they already hold.
			captured := registry
			captured.RegisterClusterExtension(&hdfsClusterExtension{})

			registry.Clear()

			Expect(captured.Count()).To(Equal(0))
			Expect(captured).To(BeIdenticalTo(registry))
		})
	})

	Describe("Count", func() {
		It("should return total count of all extensions", func() {
			registry.RegisterClusterExtension(&hdfsClusterExtension{})
			registry.RegisterClusterExtension(&hdfsClusterExtension{})
			registry.RegisterRoleExtension(&hdfsRoleExtension{})
			registry.RegisterRoleGroupExtension(&hdfsRoleGroupExtension{})

			Expect(registry.Count()).To(Equal(4))
		})

		It("should return 0 when no extensions registered", func() {
			Expect(registry.Count()).To(Equal(0))
		})
	})

	Describe("ExecuteClusterPreReconcile", func() {
		It("should execute all PreReconcile hooks", func() {
			executed := false
			ext := &hdfsClusterExtension{
				PreReconcileFunc: func(ctx context.Context, client client.Client, cr *HdfsCluster) error {
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
			ext := &hdfsClusterExtension{
				PreReconcileFunc: func(ctx context.Context, client client.Client, cr *HdfsCluster) error {
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
			ext := &hdfsClusterExtension{
				PostReconcileFunc: func(ctx context.Context, client client.Client, cr *HdfsCluster) error {
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
			ext := &hdfsClusterExtension{
				PostReconcileFunc: func(ctx context.Context, client client.Client, cr *HdfsCluster) error {
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
			ext := &hdfsClusterExtension{
				OnReconcileErrorFunc: func(ctx context.Context, client client.Client, cr *HdfsCluster, reconcileErr error) error {
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
			ext := &hdfsRoleExtension{
				PreReconcileFunc: func(ctx context.Context, client client.Client, cr *HdfsCluster, roleName string) error {
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
			ext := &hdfsRoleExtension{
				PostReconcileFunc: func(ctx context.Context, client client.Client, cr *HdfsCluster, roleName string) error {
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
			ext := &hdfsRoleGroupExtension{
				PreReconcileFunc: func(ctx context.Context, client client.Client, cr *HdfsCluster, roleName, roleGroupName string) error {
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
			ext := &hdfsRoleGroupExtension{
				PostReconcileFunc: func(ctx context.Context, client client.Client, cr *HdfsCluster, roleName, roleGroupName string) error {
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

	Describe("Execution order", func() {
		// Enough registrations, spread over enough priorities, that an unstable sort visibly
		// reshuffles the same-priority extensions.
		const extensionCount = 20

		priorities := []common.ExtensionPriority{
			common.PriorityHighest,
			common.PriorityHigh,
			common.PriorityNormal,
			common.PriorityLow,
			common.PriorityLowest,
		}

		extensionName := func(i int) string { return fmt.Sprintf("ext%02d", i) }

		// expectedOrder lists the extensions by descending priority, and within one priority
		// in registration order.
		expectedOrder := func() []string {
			expected := make([]string, 0, extensionCount)
			for bucket := range priorities {
				for i := 0; i < extensionCount; i++ {
					if i%len(priorities) == bucket {
						expected = append(expected, extensionName(i))
					}
				}
			}
			return expected
		}

		It("should order cluster extensions by priority, then by registration order", func() {
			for i := 0; i < extensionCount; i++ {
				name := extensionName(i)
				registry.RegisterClusterExtension(
					&hdfsClusterExtension{NameFunc: func() string { return name }},
					common.WithPriority(priorities[i%len(priorities)]),
				)
			}

			names := make([]string, 0, extensionCount)
			for _, ext := range registry.GetClusterExtensions() {
				names = append(names, ext.Name())
			}
			Expect(names).To(Equal(expectedOrder()))
		})

		It("should order role extensions by priority, then by registration order", func() {
			for i := 0; i < extensionCount; i++ {
				name := extensionName(i)
				registry.RegisterRoleExtension(
					&hdfsRoleExtension{NameFunc: func() string { return name }},
					common.WithPriority(priorities[i%len(priorities)]),
				)
			}

			names := make([]string, 0, extensionCount)
			for _, ext := range registry.GetRoleExtensions() {
				names = append(names, ext.Name())
			}
			Expect(names).To(Equal(expectedOrder()))
		})

		It("should order role group extensions by priority, then by registration order", func() {
			for i := 0; i < extensionCount; i++ {
				name := extensionName(i)
				registry.RegisterRoleGroupExtension(
					&hdfsRoleGroupExtension{NameFunc: func() string { return name }},
					common.WithPriority(priorities[i%len(priorities)]),
				)
			}

			names := make([]string, 0, extensionCount)
			for _, ext := range registry.GetRoleGroupExtensions() {
				names = append(names, ext.Name())
			}
			Expect(names).To(Equal(expectedOrder()))
		})

		It("should execute cluster hooks in registration order at equal priority", func() {
			executed := []string{}
			for i := 0; i < extensionCount; i++ {
				name := extensionName(i)
				registry.RegisterClusterExtension(&hdfsClusterExtension{
					NameFunc: func() string { return name },
					PreReconcileFunc: func(ctx context.Context, client client.Client, cr *HdfsCluster) error {
						executed = append(executed, name)
						return nil
					},
				})
			}

			Expect(registry.ExecuteClusterPreReconcile(context.Background(), nil, nil)).To(Succeed())

			registrationOrder := make([]string, 0, extensionCount)
			for i := 0; i < extensionCount; i++ {
				registrationOrder = append(registrationOrder, extensionName(i))
			}
			Expect(executed).To(Equal(registrationOrder))
		})
	})

	Describe("StopOnError", func() {
		// failing/following record their invocation so the tests can assert which hooks ran.
		var executed []string

		failingClusterExtension := func() *hdfsClusterExtension {
			return &hdfsClusterExtension{
				NameFunc: func() string { return "failing" },
				PreReconcileFunc: func(ctx context.Context, client client.Client, cr *HdfsCluster) error {
					executed = append(executed, "failing")
					return errors.New("hook failed")
				},
				OnReconcileErrorFunc: func(ctx context.Context, client client.Client, cr *HdfsCluster, reconcileErr error) error {
					executed = append(executed, "failing")
					return errors.New("handler failed")
				},
			}
		}

		followingClusterExtension := func() *hdfsClusterExtension {
			return &hdfsClusterExtension{
				NameFunc: func() string { return "following" },
				PreReconcileFunc: func(ctx context.Context, client client.Client, cr *HdfsCluster) error {
					executed = append(executed, "following")
					return nil
				},
				OnReconcileErrorFunc: func(ctx context.Context, client client.Client, cr *HdfsCluster, reconcileErr error) error {
					executed = append(executed, "following")
					return nil
				},
			}
		}

		BeforeEach(func() {
			executed = nil
		})

		It("should skip the remaining PreReconcile hooks by default", func() {
			registry.RegisterClusterExtension(failingClusterExtension(), common.WithPriority(common.PriorityHigh))
			registry.RegisterClusterExtension(followingClusterExtension(), common.WithPriority(common.PriorityLow))

			err := registry.ExecuteClusterPreReconcile(context.Background(), nil, nil)
			Expect(err).To(HaveOccurred())
			Expect(executed).To(Equal([]string{"failing"}))
		})

		It("should run the remaining PreReconcile hooks when the extension opts out", func() {
			registry.RegisterClusterExtension(failingClusterExtension(),
				common.WithPriority(common.PriorityHigh), common.WithStopOnError(false))
			registry.RegisterClusterExtension(followingClusterExtension(), common.WithPriority(common.PriorityLow))

			err := registry.ExecuteClusterPreReconcile(context.Background(), nil, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("hook failed"))
			Expect(executed).To(Equal([]string{"failing", "following"}))
		})

		It("should run every OnReconcileError handler and swallow their failures", func() {
			registry.RegisterClusterExtension(failingClusterExtension(), common.WithPriority(common.PriorityHigh))
			registry.RegisterClusterExtension(followingClusterExtension(), common.WithPriority(common.PriorityLow))

			err := registry.ExecuteClusterOnError(context.Background(), nil, nil, errors.New("reconcile error"))
			Expect(err).ToNot(HaveOccurred())
			Expect(executed).To(Equal([]string{"failing", "following"}))
		})

		It("should skip the remaining OnReconcileError handlers when the handler opts in", func() {
			registry.RegisterClusterExtension(failingClusterExtension(),
				common.WithPriority(common.PriorityHigh), common.WithStopOnError(true))
			registry.RegisterClusterExtension(followingClusterExtension(), common.WithPriority(common.PriorityLow))

			err := registry.ExecuteClusterOnError(context.Background(), nil, nil, errors.New("reconcile error"))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("handler failed"))
			Expect(executed).To(Equal([]string{"failing"}))
		})

		It("should run the remaining role hooks when the extension opts out", func() {
			registry.RegisterRoleExtension(&hdfsRoleExtension{
				NameFunc: func() string { return "failing" },
				PreReconcileFunc: func(ctx context.Context, client client.Client, cr *HdfsCluster, roleName string) error {
					executed = append(executed, "failing")
					return errors.New("hook failed")
				},
			}, common.WithPriority(common.PriorityHigh), common.WithStopOnError(false))

			registry.RegisterRoleExtension(&hdfsRoleExtension{
				NameFunc: func() string { return "following" },
				PreReconcileFunc: func(ctx context.Context, client client.Client, cr *HdfsCluster, roleName string) error {
					executed = append(executed, "following")
					return nil
				},
			}, common.WithPriority(common.PriorityLow))

			err := registry.ExecuteRolePreReconcile(context.Background(), nil, nil, "test-role")
			Expect(err).To(HaveOccurred())
			Expect(executed).To(Equal([]string{"failing", "following"}))
		})

		It("should run the remaining role group hooks when the extension opts out", func() {
			registry.RegisterRoleGroupExtension(&hdfsRoleGroupExtension{
				NameFunc: func() string { return "failing" },
				PostReconcileFunc: func(ctx context.Context, client client.Client, cr *HdfsCluster, roleName, roleGroupName string) error {
					executed = append(executed, "failing")
					return errors.New("hook failed")
				},
			}, common.WithPriority(common.PriorityHigh), common.WithStopOnError(false))

			registry.RegisterRoleGroupExtension(&hdfsRoleGroupExtension{
				NameFunc: func() string { return "following" },
				PostReconcileFunc: func(ctx context.Context, client client.Client, cr *HdfsCluster, roleName, roleGroupName string) error {
					executed = append(executed, "following")
					return nil
				},
			}, common.WithPriority(common.PriorityLow))

			err := registry.ExecuteRoleGroupPostReconcile(context.Background(), nil, nil, "test-role", "test-group")
			Expect(err).To(HaveOccurred())
			Expect(executed).To(Equal([]string{"failing", "following"}))
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
