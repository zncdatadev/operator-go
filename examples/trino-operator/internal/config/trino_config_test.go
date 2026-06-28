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

	"github.com/zncdatadev/operator-go/examples/trino-operator/internal/constants"
)

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
