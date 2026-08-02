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
	"github.com/zncdatadev/operator-go/pkg/config"
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

var _ = Describe("ApplyProductDefaults", func() {
	// The imperative half of the product-config seam: for a product whose configuration depends on
	// an API lookup, which happens inside BuildResources where a ctx and a client already exist.
	// hive-operator and spark-k8s-operator each hand-wrote the same "set only keys the user did not
	// set" helper, byte for byte including its doc comment.

	newCtx := func(user *v1alpha1.OverridesSpec) *reconciler.RoleGroupBuildContext {
		return &reconciler.RoleGroupBuildContext{
			MergedConfig: config.NewConfigMerger().Merge(user),
		}
	}

	It("contributes only what the user left unset", func() {
		buildCtx := newCtx(&v1alpha1.OverridesSpec{
			ConfigOverrides: map[string]map[string]string{
				"hive-site.xml": {"fs.s3a.endpoint": "from-user"},
			},
		})

		buildCtx.ApplyProductDefaults(&v1alpha1.OverridesSpec{
			ConfigOverrides: map[string]map[string]string{
				"hive-site.xml": {
					"fs.s3a.endpoint":          "from-product",
					"fs.s3a.path.style.access": "true",
				},
				"core-site.xml": {"hadoop.tmp.dir": "/tmp"},
			},
		})

		hive := buildCtx.MergedConfig.ConfigFiles["hive-site.xml"]
		Expect(hive).To(HaveKeyWithValue("fs.s3a.endpoint", "from-user"), "the user always wins")
		Expect(hive).To(HaveKeyWithValue("fs.s3a.path.style.access", "true"), "and the rest is contributed")
		Expect(buildCtx.MergedConfig.ConfigFiles).To(HaveKey("core-site.xml"), "a whole new file too")
	})

	It("applies the same rule to env vars, with no ordering dance", func() {
		// Both operators independently discovered that product env must not overwrite envOverrides,
		// and both solved it by PREPENDING to the container's env list. Contributing beneath the
		// merged config is a map operation, so the ordering question never arises.
		buildCtx := newCtx(&v1alpha1.OverridesSpec{
			EnvOverrides: map[string]string{"JAVA_OPTS": "from-user"},
		})

		buildCtx.ApplyProductDefaults(&v1alpha1.OverridesSpec{
			EnvOverrides: map[string]string{"JAVA_OPTS": "from-product", "HIVE_HOME": "/opt/hive"},
		})

		Expect(buildCtx.MergedConfig.EnvVars).To(HaveKeyWithValue("JAVA_OPTS", "from-user"))
		Expect(buildCtx.MergedConfig.EnvVars).To(HaveKeyWithValue("HIVE_HOME", "/opt/hive"))
	})

	It("supplies CLI args only when the user set none", func() {
		// cliOverrides replace rather than accumulate (§15), and an empty override means "unset".
		withUser := newCtx(&v1alpha1.OverridesSpec{CliOverrides: []string{"--from-user"}})
		withUser.ApplyProductDefaults(&v1alpha1.OverridesSpec{CliOverrides: []string{"--from-product"}})
		Expect(withUser.MergedConfig.CliArgs).To(Equal([]string{"--from-user"}))

		withoutUser := newCtx(nil)
		withoutUser.ApplyProductDefaults(&v1alpha1.OverridesSpec{CliOverrides: []string{"--from-product"}})
		Expect(withoutUser.MergedConfig.CliArgs).To(Equal([]string{"--from-product"}))
	})

	It("carries JVM args through, which no overrides layer can contribute", func() {
		// OverridesSpec has no JVM-args field, so the lower layer cannot supply any; a caller that
		// populated them imperatively (MergedConfig.AddJvmArg) must not lose them.
		buildCtx := newCtx(nil)
		buildCtx.MergedConfig.AddJvmArg("-Xmx4g")

		buildCtx.ApplyProductDefaults(&v1alpha1.OverridesSpec{
			EnvOverrides: map[string]string{"HIVE_HOME": "/opt/hive"},
		})

		Expect(buildCtx.MergedConfig.JvmArgs).To(Equal([]string{"-Xmx4g"}))
		Expect(buildCtx.MergedConfig.EnvVars).To(HaveKeyWithValue("HIVE_HOME", "/opt/hive"))
	})

	It("is a no-op for a nil layer or a context with no merged config", func() {
		buildCtx := newCtx(&v1alpha1.OverridesSpec{
			EnvOverrides: map[string]string{"KEEP": "me"},
		})
		buildCtx.ApplyProductDefaults(nil)
		Expect(buildCtx.MergedConfig.EnvVars).To(HaveKeyWithValue("KEEP", "me"))

		bare := &reconciler.RoleGroupBuildContext{}
		Expect(func() {
			bare.ApplyProductDefaults(&v1alpha1.OverridesSpec{EnvOverrides: map[string]string{"X": "1"}})
		}).NotTo(Panic())
		Expect(bare.MergedConfig).To(BeNil())
	})

	It("accumulates across calls, each landing beneath what is already merged", func() {
		// A product resolving two independent things (an S3 endpoint, a metastore URI) contributes
		// each as it learns it, and neither may displace the user or the earlier contribution.
		buildCtx := newCtx(&v1alpha1.OverridesSpec{
			ConfigOverrides: map[string]map[string]string{"hive-site.xml": {"a": "from-user"}},
		})
		buildCtx.ApplyProductDefaults(&v1alpha1.OverridesSpec{
			ConfigOverrides: map[string]map[string]string{"hive-site.xml": {"a": "first", "b": "first"}},
		})
		buildCtx.ApplyProductDefaults(&v1alpha1.OverridesSpec{
			ConfigOverrides: map[string]map[string]string{"hive-site.xml": {"b": "second", "c": "second"}},
		})

		hive := buildCtx.MergedConfig.ConfigFiles["hive-site.xml"]
		Expect(hive).To(HaveKeyWithValue("a", "from-user"))
		Expect(hive).To(HaveKeyWithValue("b", "first"), "the earlier contribution outranks the later one")
		Expect(hive).To(HaveKeyWithValue("c", "second"))
	})
})
