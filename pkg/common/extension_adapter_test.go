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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// typedClusterExtension is written against the concrete product CR, the way a product
// operator writes its extensions, and never has to convert the CR itself.
type typedClusterExtension struct {
	common.BaseExtension
	seen []string
	err  error
}

func newTypedClusterExtension(err error) *typedClusterExtension {
	return &typedClusterExtension{BaseExtension: common.NewBaseExtension("typed-hdfs-extension"), err: err}
}

func (e *typedClusterExtension) PreReconcile(ctx context.Context, c client.Client, cr *HdfsCluster) error {
	e.seen = append(e.seen, cr.Name)
	return e.err
}

func (e *typedClusterExtension) PostReconcile(ctx context.Context, c client.Client, cr *HdfsCluster) error {
	e.seen = append(e.seen, cr.Name)
	return e.err
}

func (e *typedClusterExtension) OnReconcileError(ctx context.Context, c client.Client, cr *HdfsCluster, err error) error {
	e.seen = append(e.seen, cr.Name)
	return e.err
}

// typedRoleExtension is the role-level equivalent of typedClusterExtension.
type typedRoleExtension struct {
	common.BaseExtension
	seen []string
}

func newTypedRoleExtension() *typedRoleExtension {
	return &typedRoleExtension{BaseExtension: common.NewBaseExtension("typed-hdfs-role-extension")}
}

func (e *typedRoleExtension) PreReconcile(ctx context.Context, c client.Client, cr *HdfsCluster, roleName string) error {
	e.seen = append(e.seen, cr.Name+"/"+roleName)
	return nil
}

func (e *typedRoleExtension) PostReconcile(ctx context.Context, c client.Client, cr *HdfsCluster, roleName string) error {
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

func (e *typedRoleGroupExtension) PreReconcile(ctx context.Context, c client.Client, cr *HdfsCluster, roleName, roleGroupName string) error {
	e.seen = append(e.seen, cr.Name+"/"+roleName+"/"+roleGroupName)
	return nil
}

func (e *typedRoleGroupExtension) PostReconcile(ctx context.Context, c client.Client, cr *HdfsCluster, roleName, roleGroupName string) error {
	return nil
}

var _ = Describe("Extension adapters", func() {
	var registry *common.ExtensionRegistry
	var hdfs *HdfsCluster
	var foreign *MockClusterForProductTest

	BeforeEach(func() {
		registry = common.NewExtensionRegistry()
		hdfs = &HdfsCluster{ObjectMeta: metav1.ObjectMeta{Name: "hdfs-adapter", Namespace: "default"}}
		foreign = &MockClusterForProductTest{name: "other-cluster"}
	})

	Describe("AsClusterExtension", func() {
		It("should pass the concrete CR to the typed extension", func() {
			ext := newTypedClusterExtension(nil)
			registry.RegisterClusterExtension(common.AsClusterExtension[*HdfsCluster](ext))

			Expect(registry.ExecuteClusterPreReconcile(context.Background(), nil, hdfs)).To(Succeed())
			Expect(ext.seen).To(Equal([]string{"hdfs-adapter"}))
		})

		It("should preserve the extension name", func() {
			adapted := common.AsClusterExtension[*HdfsCluster](newTypedClusterExtension(nil))
			Expect(adapted.Name()).To(Equal("typed-hdfs-extension"))
		})

		It("should skip CRs of another product type instead of failing the hook", func() {
			ext := newTypedClusterExtension(nil)
			registry.RegisterClusterExtension(common.AsClusterExtension[*HdfsCluster](ext))

			Expect(registry.ExecuteClusterPreReconcile(context.Background(), nil, foreign)).To(Succeed())
			Expect(registry.ExecuteClusterPostReconcile(context.Background(), nil, foreign)).To(Succeed())
			Expect(registry.ExecuteClusterOnError(context.Background(), nil, foreign, errors.New("boom"))).To(Succeed())
			Expect(ext.seen).To(BeEmpty())
		})

		It("should propagate errors from the typed extension", func() {
			ext := newTypedClusterExtension(errors.New("typed failure"))
			registry.RegisterClusterExtension(common.AsClusterExtension[*HdfsCluster](ext))

			err := registry.ExecuteClusterPreReconcile(context.Background(), nil, hdfs)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("typed-hdfs-extension"))
			Expect(err.Error()).To(ContainSubstring("typed failure"))
		})
	})

	Describe("AsRoleExtension", func() {
		It("should pass the concrete CR to the typed extension", func() {
			ext := newTypedRoleExtension()
			registry.RegisterRoleExtension(common.AsRoleExtension[*HdfsCluster](ext))

			Expect(registry.ExecuteRolePreReconcile(context.Background(), nil, hdfs, "nameNodes")).To(Succeed())
			Expect(ext.seen).To(Equal([]string{"hdfs-adapter/nameNodes"}))
		})

		It("should skip CRs of another product type", func() {
			ext := newTypedRoleExtension()
			registry.RegisterRoleExtension(common.AsRoleExtension[*HdfsCluster](ext))

			Expect(registry.ExecuteRolePreReconcile(context.Background(), nil, foreign, "nameNodes")).To(Succeed())
			Expect(ext.seen).To(BeEmpty())
		})
	})

	Describe("AsRoleGroupExtension", func() {
		It("should pass the concrete CR to the typed extension", func() {
			ext := newTypedRoleGroupExtension()
			registry.RegisterRoleGroupExtension(common.AsRoleGroupExtension[*HdfsCluster](ext))

			Expect(registry.ExecuteRoleGroupPreReconcile(context.Background(), nil, hdfs, "nameNodes", "default")).To(Succeed())
			Expect(ext.seen).To(Equal([]string{"hdfs-adapter/nameNodes/default"}))
		})

		It("should skip CRs of another product type", func() {
			ext := newTypedRoleGroupExtension()
			registry.RegisterRoleGroupExtension(common.AsRoleGroupExtension[*HdfsCluster](ext))

			Expect(registry.ExecuteRoleGroupPreReconcile(context.Background(), nil, foreign, "nameNodes", "default")).To(Succeed())
			Expect(ext.seen).To(BeEmpty())
		})
	})
})
