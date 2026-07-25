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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

// Everything ClusterInterface needs beyond client.Object, which the embedded TypeMeta and
// ObjectMeta already supply.

func (h *HdfsCluster) GetSpec() *v1alpha1.GenericClusterSpec     { return &h.Spec.GenericClusterSpec }
func (h *HdfsCluster) GetStatus() *v1alpha1.GenericClusterStatus { return &h.Status }
func (h *HdfsCluster) DeepCopy() *HdfsCluster                    { c := *h; return &c }
func (h *HdfsCluster) DeepCopyObject() runtime.Object            { return h.DeepCopy() }

// ---------------------------------------------------------------------------
// Product extension: JvmArgumentsExtension
// This is what the HDFS operator would implement to act on JvmArgumentOverrides.
// ---------------------------------------------------------------------------

// JvmArgumentsExtension is an HDFS-specific ClusterExtension that reads
// JvmArgumentOverrides from the product CR and applies them (e.g., appends
// to a ConfigMap or sets an env var).
//
// It is declared for *HdfsCluster, so the hooks receive the product CR itself and the product
// field is reachable without any conversion or guard.
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
func (e *JvmArgumentsExtension) PreReconcile(ctx context.Context, c client.Client, cr *HdfsCluster) error {
	if cr.Spec.NameNodes != nil && len(cr.Spec.NameNodes.JvmArgumentOverrides) > 0 {
		e.appliedArgs["nameNodes"] = cr.Spec.NameNodes.JvmArgumentOverrides
	}
	return nil
}

func (e *JvmArgumentsExtension) PostReconcile(ctx context.Context, c client.Client, cr *HdfsCluster) error {
	return nil
}

func (e *JvmArgumentsExtension) OnReconcileError(ctx context.Context, c client.Client, cr *HdfsCluster, err error) error {
	return nil
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

var _ = Describe("Extension mechanism: product-specific fields pattern", func() {

	var registry *common.ExtensionRegistry[*HdfsCluster]

	BeforeEach(func() {
		registry = common.NewExtensionRegistry[*HdfsCluster]()
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

	})

	Describe("RoleGroupExtension pattern: per-role-group JVM tuning", func() {
		It("allows a product RoleGroupExtension to read and apply JVM args per group", func() {
			captured := map[string][]string{}

			// A RoleGroupExtension that captures which JVM args were applied per group.
			ext := &hdfsRoleGroupExtension{
				PreReconcileFunc: func(ctx context.Context, c client.Client, cr *HdfsCluster, roleName, groupName string) error {
					if cr.Spec.NameNodes != nil && roleName == "nameNodes" {
						captured[groupName] = cr.Spec.NameNodes.JvmArgumentOverrides
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

			low := &hdfsClusterExtension{
				NameFunc: func() string { return "low" },
				PreReconcileFunc: func(ctx context.Context, c client.Client, cr *HdfsCluster) error {
					order = append(order, "low")
					return nil
				},
			}
			high := &hdfsClusterExtension{
				NameFunc: func() string { return "high" },
				PreReconcileFunc: func(ctx context.Context, c client.Client, cr *HdfsCluster) error {
					order = append(order, "high")
					return nil
				},
			}

			registry.RegisterClusterExtension(low, common.WithPriority(common.PriorityLow))
			registry.RegisterClusterExtension(high, common.WithPriority(common.PriorityHigh))

			err := registry.ExecuteClusterPreReconcile(context.Background(), nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(order).To(Equal([]string{"high", "low"}))
		})
	})
})

// MockClusterForProductTest is a minimal ClusterInterface standing in for a second product's CR,
// so specs can exercise two registries typed for different products.
type MockClusterForProductTest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              v1alpha1.GenericClusterSpec   `json:"spec,omitempty"`
	Status            v1alpha1.GenericClusterStatus `json:"status,omitempty"`
}

func (m *MockClusterForProductTest) GetSpec() *v1alpha1.GenericClusterSpec     { return &m.Spec }
func (m *MockClusterForProductTest) GetStatus() *v1alpha1.GenericClusterStatus { return &m.Status }
func (m *MockClusterForProductTest) DeepCopy() *MockClusterForProductTest      { c := *m; return &c }
func (m *MockClusterForProductTest) DeepCopyObject() runtime.Object            { return m.DeepCopy() }
