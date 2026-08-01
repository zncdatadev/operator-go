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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
)

var _ = Describe("MergeRoleGroupConfig leaf granularity", func() {
	// The defect: merging `resources` struct-by-struct meant a group that set ONE leaf discarded
	// every sibling the role had configured, because the group's enclosing struct was non-nil and
	// the role's was therefore never consulted. Overriding one knob is the normal way to use this
	// API, so this silently dropped configuration on the most ordinary edit there is.
	It("keeps the role's storage capacity when the group overrides only storageClass", func() {
		merged := reconciler.MergeRoleGroupConfig(
			&v1alpha1.RoleGroupConfigSpec{Resources: &v1alpha1.ResourcesSpec{
				Storage: &v1alpha1.StorageResource{Capacity: ptr.To(resource.MustParse("500Gi"))},
			}},
			&v1alpha1.RoleGroupConfigSpec{Resources: &v1alpha1.ResourcesSpec{
				Storage: &v1alpha1.StorageResource{StorageClass: ptr.To("fast-ssd")},
			}},
		)

		Expect(merged.Resources.Storage.StorageClass).To(HaveValue(Equal("fast-ssd")), "the group's leaf wins")
		Expect(merged.Resources.Storage.Capacity).NotTo(BeNil(),
			"the role's capacity must survive: it is baked into an immutable volumeClaimTemplate, "+
				"so losing it here is unrecoverable without deleting the StatefulSet")
		Expect(merged.Resources.Storage.Capacity.String()).To(Equal("500Gi"))
	})

	It("copies an inherited storageClass instead of aliasing the role spec", func() {
		// Every other branch of this merge deep-copies. The role spec passed in is a pointer into
		// the CR the informer cache holds, so a merged config that aliases it turns any later write
		// through the pointer into a mutation of the cached object — an object the reconciler
		// compares against to decide whether to write the status at all.
		roleStorage := &v1alpha1.StorageResource{StorageClass: ptr.To("fast-ssd")}
		merged := reconciler.MergeRoleGroupConfig(
			&v1alpha1.RoleGroupConfigSpec{Resources: &v1alpha1.ResourcesSpec{Storage: roleStorage}},
			&v1alpha1.RoleGroupConfigSpec{Resources: &v1alpha1.ResourcesSpec{
				Storage: &v1alpha1.StorageResource{Capacity: ptr.To(resource.MustParse("100Gi"))},
			}},
		)

		Expect(merged.Resources.Storage.StorageClass).To(HaveValue(Equal("fast-ssd")))
		Expect(merged.Resources.Storage.StorageClass).NotTo(BeIdenticalTo(roleStorage.StorageClass),
			"the merged config must not share the role's pointer")

		*merged.Resources.Storage.StorageClass = "slow-hdd"
		Expect(roleStorage.StorageClass).To(HaveValue(Equal("fast-ssd")),
			"writing through the merged config must not reach the role spec")
	})

	It("keeps the role's cpu.max when the group overrides only cpu.min", func() {
		merged := reconciler.MergeRoleGroupConfig(
			&v1alpha1.RoleGroupConfigSpec{Resources: &v1alpha1.ResourcesSpec{
				CPU: &v1alpha1.CPUResource{
					Min: ptr.To(resource.MustParse("100m")),
					Max: ptr.To(resource.MustParse("2")),
				},
			}},
			&v1alpha1.RoleGroupConfigSpec{Resources: &v1alpha1.ResourcesSpec{
				CPU: &v1alpha1.CPUResource{Min: ptr.To(resource.MustParse("500m"))},
			}},
		)

		Expect(merged.Resources.CPU.Min.String()).To(Equal("500m"), "the group's leaf wins")
		Expect(merged.Resources.CPU.Max).NotTo(BeNil(), "the role's limit must survive")
		Expect(merged.Resources.CPU.Max.String()).To(Equal("2"))
	})

	It("keeps the role's other resource blocks when the group sets only one", func() {
		merged := reconciler.MergeRoleGroupConfig(
			&v1alpha1.RoleGroupConfigSpec{Resources: &v1alpha1.ResourcesSpec{
				CPU:     &v1alpha1.CPUResource{Max: ptr.To(resource.MustParse("2"))},
				Memory:  &v1alpha1.MemoryResource{Limit: ptr.To(resource.MustParse("2Gi"))},
				Storage: &v1alpha1.StorageResource{Capacity: ptr.To(resource.MustParse("100Gi"))},
			}},
			&v1alpha1.RoleGroupConfigSpec{Resources: &v1alpha1.ResourcesSpec{
				Memory: &v1alpha1.MemoryResource{Limit: ptr.To(resource.MustParse("8Gi"))},
			}},
		)

		Expect(merged.Resources.Memory.Limit.String()).To(Equal("8Gi"))
		Expect(merged.Resources.CPU).NotTo(BeNil())
		Expect(merged.Resources.CPU.Max.String()).To(Equal("2"))
		Expect(merged.Resources.Storage).NotTo(BeNil())
		Expect(merged.Resources.Storage.Capacity.String()).To(Equal("100Gi"))
	})

	It("lets a role-level gracefulShutdownTimeout reach a group that declares an unrelated config", func() {
		merged := reconciler.MergeRoleGroupConfig(
			&v1alpha1.RoleGroupConfigSpec{GracefulShutdownTimeout: ptr.To("5m")},
			&v1alpha1.RoleGroupConfigSpec{Resources: &v1alpha1.ResourcesSpec{}},
		)

		// This is the case the old CRD default made unreachable: the group declared a config block
		// for another reason, the API server stamped "30s" into it, and the role's 5m lost.
		Expect(merged.GetGracefulShutdownTimeout()).To(Equal("5m"))
	})

	It("does not mutate its inputs", func() {
		role := &v1alpha1.RoleGroupConfigSpec{Resources: &v1alpha1.ResourcesSpec{
			CPU: &v1alpha1.CPUResource{Max: ptr.To(resource.MustParse("2"))},
		}}
		group := &v1alpha1.RoleGroupConfigSpec{Resources: &v1alpha1.ResourcesSpec{
			CPU: &v1alpha1.CPUResource{Min: ptr.To(resource.MustParse("1"))},
		}}

		merged := reconciler.MergeRoleGroupConfig(role, group)
		merged.Resources.CPU.Max = ptr.To(resource.MustParse("99"))

		// The merged config is handed to product handlers; aliasing the live CR spec would let a
		// handler edit corrupt the object the reconciler later compares against.
		Expect(role.Resources.CPU.Max.String()).To(Equal("2"))
		Expect(group.Resources.CPU.Min).NotTo(BeNil())
	})
})
