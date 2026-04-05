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

// This file demonstrates how product operators use the SDK's extension mechanism
// to inject product-specific fields (e.g., jvmArgumentOverrides) into the
// reconciliation flow without modifying the SDK's core types.
//
// Key principle: The SDK's GenericClusterSpec and RoleSpec deliberately do NOT
// contain product-specific fields like jvmArgumentOverrides. Instead, products
// define these fields in their own CR types and use extensions to act on them.

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ---------------------------------------------------------------------------
// Simulated product CR: HdfsCluster
// This represents what a product operator author would define, NOT SDK code.
// ---------------------------------------------------------------------------

// HdfsClusterSpec is the product-specific spec for an HDFS cluster.
// It embeds GenericClusterSpec and adds HDFS-specific fields.
type HdfsClusterSpec struct {
	v1alpha1.GenericClusterSpec `json:",inline"`

	// NameNodes is the HDFS NameNode role configuration.
	// It adds product-specific fields (e.g., JvmArgumentOverrides) on top of
	// the standard RoleSpec via a dedicated product-specific role type.
	NameNodes *HdfsRoleSpec `json:"nameNodes,omitempty"`
}

// HdfsRoleSpec extends the SDK's RoleSpec with HDFS-specific fields.
// This is the pattern for adding product-specific overrides that are not
// part of the SDK's generic configuration.
type HdfsRoleSpec struct {
	v1alpha1.RoleSpec `json:",inline"`

	// JvmArgumentOverrides allows tuning JVM heap and GC settings per role.
	// Example: ["-Xmx4g", "-XX:+UseG1GC"]
	JvmArgumentOverrides []string `json:"jvmArgumentOverrides,omitempty"`
}

// HdfsCluster is the mock HDFS product CR.
type HdfsCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              HdfsClusterSpec               `json:"spec,omitempty"`
	Status            v1alpha1.GenericClusterStatus `json:"status,omitempty"`
}

// Implement common.ClusterInterface so HdfsCluster can be used with GenericReconciler.

func (h *HdfsCluster) GetName() string      { return h.Name }
func (h *HdfsCluster) GetNamespace() string { return h.Namespace }
func (h *HdfsCluster) GetUID() types.UID      { return h.UID }
func (h *HdfsCluster) GetLabels() map[string]string {
	if h.Labels == nil {
		return map[string]string{}
	}
	return h.Labels
}
func (h *HdfsCluster) GetAnnotations() map[string]string {
	if h.Annotations == nil {
		return map[string]string{}
	}
	return h.Annotations
}
func (h *HdfsCluster) GetSpec() *v1alpha1.GenericClusterSpec           { return &h.Spec.GenericClusterSpec }
func (h *HdfsCluster) GetStatus() *v1alpha1.GenericClusterStatus       { return &h.Status }
func (h *HdfsCluster) SetStatus(status *v1alpha1.GenericClusterStatus) { h.Status = *status }
func (h *HdfsCluster) GetObjectMeta() *metav1.ObjectMeta               { return &h.ObjectMeta }
func (h *HdfsCluster) GetScheme() *runtime.Scheme                      { return nil }
func (h *HdfsCluster) GetRuntimeObject() runtime.Object                { return nil }
func (h *HdfsCluster) DeepCopyObject() runtime.Object                  { c := *h; return &c }
func (h *HdfsCluster) DeepCopyCluster() common.ClusterInterface {
	copy := *h
	return &copy
}

// ---------------------------------------------------------------------------
// Product extension: JvmArgumentsExtension
// This is what the HDFS operator would implement to act on JvmArgumentOverrides.
// ---------------------------------------------------------------------------

// JvmArgumentsExtension is an HDFS-specific ClusterExtension that reads
// JvmArgumentOverrides from the product CR and applies them (e.g., appends
// to a ConfigMap or sets an env var).
type JvmArgumentsExtension struct {
	common.BaseExtension
	// appliedArgs captures what was processed, for test verification.
	appliedArgs map[string][]string
}

func NewJvmArgumentsExtension() *JvmArgumentsExtension {
	return &JvmArgumentsExtension{
		BaseExtension: common.NewBaseExtension("hdfs-jvm-arguments"),
		appliedArgs:   make(map[string][]string),
	}
}

// PreReconcile reads JvmArgumentOverrides from the HdfsCluster CR and stores
// them so the RoleGroupHandler can later render them into jvm.properties.
func (e *JvmArgumentsExtension) PreReconcile(ctx context.Context, c client.Client, cr common.ClusterInterface) error {
	hdfs, ok := cr.(*HdfsCluster)
	if !ok {
		return fmt.Errorf("expected *HdfsCluster, got %T", cr)
	}

	if hdfs.Spec.NameNodes != nil && len(hdfs.Spec.NameNodes.JvmArgumentOverrides) > 0 {
		e.appliedArgs["nameNodes"] = hdfs.Spec.NameNodes.JvmArgumentOverrides
	}
	return nil
}

func (e *JvmArgumentsExtension) PostReconcile(ctx context.Context, c client.Client, cr common.ClusterInterface) error {
	return nil
}

func (e *JvmArgumentsExtension) OnReconcileError(ctx context.Context, c client.Client, cr common.ClusterInterface, err error) error {
	return nil
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

var _ = Describe("Extension mechanism: product-specific fields pattern", func() {

	var registry *common.ExtensionRegistry

	BeforeEach(func() {
		registry = common.GetExtensionRegistry()
		registry.Clear()
	})

	AfterEach(func() {
		registry.Clear()
	})

	Describe("JvmArgumentOverrides via ClusterExtension", func() {
		It("allows a product to read custom role fields in PreReconcile", func() {
			hdfs := &HdfsCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "hdfs-test", Namespace: "default"},
				Spec: HdfsClusterSpec{
					NameNodes: &HdfsRoleSpec{
						JvmArgumentOverrides: []string{"-Xmx4g", "-XX:+UseG1GC"},
					},
				},
			}

			ext := NewJvmArgumentsExtension()
			registry.RegisterClusterExtension(ext)

			err := registry.ExecuteClusterPreReconcile(context.Background(), nil, hdfs)
			Expect(err).NotTo(HaveOccurred())
			Expect(ext.appliedArgs["nameNodes"]).To(Equal([]string{"-Xmx4g", "-XX:+UseG1GC"}))
		})

		It("does nothing when JvmArgumentOverrides is empty", func() {
			hdfs := &HdfsCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "hdfs-empty", Namespace: "default"},
				Spec: HdfsClusterSpec{
					NameNodes: &HdfsRoleSpec{
						JvmArgumentOverrides: nil,
					},
				},
			}

			ext := NewJvmArgumentsExtension()
			registry.RegisterClusterExtension(ext)

			err := registry.ExecuteClusterPreReconcile(context.Background(), nil, hdfs)
			Expect(err).NotTo(HaveOccurred())
			Expect(ext.appliedArgs).To(BeEmpty())
		})

		It("returns error when the CR is not of the expected product type", func() {
			// Using a different ClusterInterface implementation (not HdfsCluster)
			// demonstrates that extensions should guard with a type assertion.
			otherCR := &MockClusterForProductTest{name: "other-cluster"}

			ext := NewJvmArgumentsExtension()
			registry.RegisterClusterExtension(ext)

			err := registry.ExecuteClusterPreReconcile(context.Background(), nil, otherCR)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected *HdfsCluster"))
		})
	})

	Describe("RoleGroupExtension pattern: per-role-group JVM tuning", func() {
		It("allows a product RoleGroupExtension to read and apply JVM args per group", func() {
			captured := map[string][]string{}

			// A RoleGroupExtension that captures which JVM args were applied per group.
			ext := &MockRoleGroupExtension{
				PreReconcileFunc: func(ctx context.Context, c client.Client, cr common.ClusterInterface, roleName, groupName string) error {
					hdfs, ok := cr.(*HdfsCluster)
					if !ok {
						return nil
					}
					if hdfs.Spec.NameNodes != nil && roleName == "nameNodes" {
						captured[groupName] = hdfs.Spec.NameNodes.JvmArgumentOverrides
					}
					return nil
				},
			}

			registry.RegisterRoleGroupExtension(ext)

			hdfs := &HdfsCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "hdfs-rg", Namespace: "default"},
				Spec: HdfsClusterSpec{
					NameNodes: &HdfsRoleSpec{
						JvmArgumentOverrides: []string{"-Xmx8g"},
					},
				},
			}

			err := registry.ExecuteRoleGroupPreReconcile(context.Background(), nil, hdfs, "nameNodes", "default")
			Expect(err).NotTo(HaveOccurred())
			Expect(captured["default"]).To(Equal([]string{"-Xmx8g"}))
		})
	})

	Describe("Priority ordering", func() {
		It("executes extensions in highest-priority-first order", func() {
			order := []string{}

			low := &MockClusterExtension{
				NameFunc: func() string { return "low" },
				PreReconcileFunc: func(ctx context.Context, c client.Client, cr common.ClusterInterface) error {
					order = append(order, "low")
					return nil
				},
			}
			high := &MockClusterExtension{
				NameFunc: func() string { return "high" },
				PreReconcileFunc: func(ctx context.Context, c client.Client, cr common.ClusterInterface) error {
					order = append(order, "high")
					return nil
				},
			}

			registry.RegisterClusterExtensionWithPriority(low, common.PriorityLow)
			registry.RegisterClusterExtensionWithPriority(high, common.PriorityHigh)

			err := registry.ExecuteClusterPreReconcile(context.Background(), nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(order).To(Equal([]string{"high", "low"}))
		})
	})
})

// MockClusterForProductTest is a minimal ClusterInterface for testing type-assertion guards.
type MockClusterForProductTest struct {
	name string
}

func (m *MockClusterForProductTest) GetName() string                   { return m.name }
func (m *MockClusterForProductTest) GetNamespace() string              { return "default" }
func (m *MockClusterForProductTest) GetUID() types.UID                   { return "uid-123" }
func (m *MockClusterForProductTest) GetLabels() map[string]string      { return nil }
func (m *MockClusterForProductTest) GetAnnotations() map[string]string { return nil }
func (m *MockClusterForProductTest) GetSpec() *v1alpha1.GenericClusterSpec {
	return &v1alpha1.GenericClusterSpec{}
}
func (m *MockClusterForProductTest) GetStatus() *v1alpha1.GenericClusterStatus {
	return &v1alpha1.GenericClusterStatus{}
}
func (m *MockClusterForProductTest) SetStatus(_ *v1alpha1.GenericClusterStatus) {}
func (m *MockClusterForProductTest) GetObjectMeta() *metav1.ObjectMeta          { return &metav1.ObjectMeta{} }
func (m *MockClusterForProductTest) GetScheme() *runtime.Scheme                 { return nil }
func (m *MockClusterForProductTest) GetRuntimeObject() runtime.Object           { return nil }
func (m *MockClusterForProductTest) DeepCopyCluster() common.ClusterInterface   { c := *m; return &c }
