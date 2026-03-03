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

package config

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	trinov1alpha1 "github.com/zncdatadev/operator-go/examples/trino-operator/api/v1alpha1"
	"github.com/zncdatadev/operator-go/examples/trino-operator/internal/constants"
	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("TrinoConfigBuilder", func() {
	var (
		builder  *TrinoConfigBuilder
		cluster  *trinov1alpha1.TrinoCluster
		buildCtx *reconciler.RoleGroupBuildContext
	)

	BeforeEach(func() {
		builder = NewTrinoConfigBuilder()
		cluster = &trinov1alpha1.TrinoCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
		}
		buildCtx = &reconciler.RoleGroupBuildContext{
			ClusterName:      "test-cluster",
			ClusterNamespace: "default",
			RoleName:         "coordinators",
			RoleGroupName:    "default",
			ResourceName:     "test-cluster-coordinators-default",
			ClusterLabels:    map[string]string{"app": "trino"},
			RoleGroupSpec:    v1alpha1.RoleGroupSpec{},
		}
	})

	Context("NewTrinoConfigBuilder", func() {
		It("should create a builder with empty properties map", func() {
			builder := NewTrinoConfigBuilder()
			Expect(builder).NotTo(BeNil())
			Expect(builder.properties).NotTo(BeNil())
			Expect(builder.properties).To(BeEmpty())
		})
	})

	Context("ForCoordinator", func() {
		It("should set coordinator to true", func() {
			result := builder.ForCoordinator(cluster, buildCtx, 8080)
			Expect(result).To(Equal(builder))
			Expect(builder.coordinator).To(BeTrue())
		})

		It("should set coordinator-specific properties", func() {
			builder.ForCoordinator(cluster, buildCtx, 8080)
			Expect(builder.properties["coordinator"]).To(Equal("true"))
			Expect(builder.properties["node-scheduler.include-coordinator"]).To(Equal("false"))
			Expect(builder.properties["discovery-server.enabled"]).To(Equal("true"))
		})

		It("should set HTTP port from parameter", func() {
			builder.ForCoordinator(cluster, buildCtx, 9090)
			Expect(builder.properties["http-server.http.port"]).To(Equal("9090"))
		})

		It("should set discovery URI with cluster name and port", func() {
			builder.ForCoordinator(cluster, buildCtx, 8080)
			Expect(builder.properties["discovery.uri"]).To(Equal("http://test-cluster-coordinator:8080"))
		})
	})

	Context("ForWorker", func() {
		BeforeEach(func() {
			buildCtx.RoleName = "workers"
			buildCtx.ResourceName = "test-cluster-workers-default"
		})

		It("should set coordinator to false", func() {
			result := builder.ForWorker(cluster, buildCtx, 8080)
			Expect(result).To(Equal(builder))
			Expect(builder.coordinator).To(BeFalse())
		})

		It("should set worker-specific properties", func() {
			builder.ForWorker(cluster, buildCtx, 8080)
			Expect(builder.properties["coordinator"]).To(Equal("false"))
		})

		It("should not have discovery-server.enabled for worker", func() {
			builder.ForWorker(cluster, buildCtx, 8080)
			_, exists := builder.properties["discovery-server.enabled"]
			Expect(exists).To(BeFalse())
		})

		It("should set discovery URI pointing to coordinator", func() {
			builder.ForWorker(cluster, buildCtx, 8080)
			Expect(builder.properties["discovery.uri"]).To(Equal("http://test-cluster-coordinator:8080"))
		})

		It("should set HTTP port from coordinator port parameter", func() {
			builder.ForWorker(cluster, buildCtx, 9090)
			Expect(builder.properties["http-server.http.port"]).To(Equal("9090"))
		})
	})

	Context("WithProperty", func() {
		It("should add custom property", func() {
			result := builder.WithProperty("custom.key", "custom-value")
			Expect(result).To(Equal(builder))
			Expect(builder.properties["custom.key"]).To(Equal("custom-value"))
		})

		It("should override existing property", func() {
			builder.ForCoordinator(cluster, buildCtx, 8080)
			builder.WithProperty("coordinator", "overridden")
			Expect(builder.properties["coordinator"]).To(Equal("overridden"))
		})

		It("should support method chaining", func() {
			builder.
				WithProperty("key1", "value1").
				WithProperty("key2", "value2")
			Expect(builder.properties["key1"]).To(Equal("value1"))
			Expect(builder.properties["key2"]).To(Equal("value2"))
		})
	})

	Context("Build", func() {
		It("should return empty string for empty properties", func() {
			result := builder.Build()
			Expect(result).To(BeEmpty())
		})

		It("should generate key=value format", func() {
			builder.properties["key1"] = "value1"
			result := builder.Build()
			Expect(result).To(Equal("key1=value1"))
		})

		It("should separate properties with newlines", func() {
			builder.properties["key1"] = "value1"
			builder.properties["key2"] = "value2"
			result := builder.Build()
			Expect(result).To(ContainSubstring("key1=value1"))
			Expect(result).To(ContainSubstring("key2=value2"))
			Expect(strings.Count(result, "\n")).To(Equal(1))
		})

		It("should build coordinator config correctly", func() {
			result := builder.ForCoordinator(cluster, buildCtx, 8080).Build()
			Expect(result).To(ContainSubstring("coordinator=true"))
			Expect(result).To(ContainSubstring("discovery-server.enabled=true"))
			Expect(result).To(ContainSubstring("http-server.http.port=8080"))
		})

		It("should build worker config correctly", func() {
			result := builder.ForWorker(cluster, buildCtx, 8080).Build()
			Expect(result).To(ContainSubstring("coordinator=false"))
			Expect(result).To(ContainSubstring("http-server.http.port=8080"))
			Expect(result).NotTo(ContainSubstring("discovery-server.enabled"))
		})
	})
})

var _ = Describe("JVMConfigBuilder", func() {
	var builder *JVMConfigBuilder

	BeforeEach(func() {
		builder = NewJVMConfigBuilder()
	})

	Context("NewJVMConfigBuilder", func() {
		It("should create builder with default GC options", func() {
			Expect(builder).NotTo(BeNil())
			// Verify GC options are included in build output
			result := builder.WithMaxMemory("4G").Build()
			Expect(result).To(ContainSubstring("-XX:+UseG1GC"))
			Expect(result).To(ContainSubstring("-XX:G1HeapRegionSize=32M"))
			Expect(result).To(ContainSubstring("-XX:+ExplicitGCInvokesConcurrent"))
			Expect(result).To(ContainSubstring("-XX:+ExitOnOutOfMemoryError"))
		})

		It("should create builder with default extra options", func() {
			Expect(builder).NotTo(BeNil())
			// Verify extra options are included in build output
			result := builder.WithMaxMemory("4G").Build()
			Expect(result).To(ContainSubstring("-Djdk.attach.allowAttachSelf=true"))
		})
	})

	Context("ForCoordinator", func() {
		It("should set coordinator max memory", func() {
			result := builder.ForCoordinator()
			Expect(result).To(Equal(builder))
			Expect(builder.maxMemory).To(Equal(constants.DefaultCoordinatorMaxMemory))
		})
	})

	Context("ForWorker", func() {
		It("should set worker max memory", func() {
			result := builder.ForWorker()
			Expect(result).To(Equal(builder))
			Expect(builder.maxMemory).To(Equal(constants.DefaultWorkerMaxMemory))
		})
	})

	Context("WithMaxMemory", func() {
		It("should set custom max memory", func() {
			result := builder.WithMaxMemory("8G")
			Expect(result).To(Equal(builder))
			Expect(builder.maxMemory).To(Equal("8G"))
		})

		It("should override previous memory setting", func() {
			builder.ForCoordinator()
			builder.WithMaxMemory("16G")
			Expect(builder.maxMemory).To(Equal("16G"))
		})
	})

	Context("Build", func() {
		It("should start with -Xmx memory setting", func() {
			builder.WithMaxMemory("4G")
			result := builder.Build()
			Expect(result).To(HavePrefix("-Xmx4G"))
		})

		It("should include all GC options", func() {
			builder.WithMaxMemory("4G")
			result := builder.Build()
			Expect(result).To(ContainSubstring("-XX:+UseG1GC"))
			Expect(result).To(ContainSubstring("-XX:G1HeapRegionSize=32M"))
			Expect(result).To(ContainSubstring("-XX:+ExplicitGCInvokesConcurrent"))
			Expect(result).To(ContainSubstring("-XX:+ExitOnOutOfMemoryError"))
		})

		It("should include extra options", func() {
			builder.WithMaxMemory("4G")
			result := builder.Build()
			Expect(result).To(ContainSubstring("-Djdk.attach.allowAttachSelf=true"))
		})

		It("should separate options with newlines", func() {
			builder.WithMaxMemory("4G")
			result := builder.Build()
			lines := strings.Split(result, "\n")
			Expect(len(lines)).To(BeNumerically(">", 5))
		})

		It("should build coordinator JVM config correctly", func() {
			result := builder.ForCoordinator().Build()
			Expect(result).To(ContainSubstring("-Xmx" + constants.DefaultCoordinatorMaxMemory))
		})

		It("should build worker JVM config correctly", func() {
			result := builder.ForWorker().Build()
			Expect(result).To(ContainSubstring("-Xmx" + constants.DefaultWorkerMaxMemory))
		})
	})
})
