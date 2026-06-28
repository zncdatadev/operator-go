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
	"fmt"
	"strings"

	"github.com/zncdatadev/operator-go/examples/trino-operator/internal/constants"
)

// JVMConfigBuilder builds Trino's jvm.config. Unlike config.properties (which is key=value and
// flows through the SDK merge pipeline via product.ComputeConfig), jvm.config is a
// newline-delimited list of JVM flags, so it is generated here and appended to the ConfigMap
// by the handler.
type JVMConfigBuilder struct {
	maxMemory string
	gcOptions []string
	extraOpts []string
}

// NewJVMConfigBuilder creates a new JVMConfigBuilder
func NewJVMConfigBuilder() *JVMConfigBuilder {
	return &JVMConfigBuilder{
		gcOptions: []string{
			"-XX:+UseG1GC",
			"-XX:G1HeapRegionSize=32M",
			"-XX:+ExplicitGCInvokesConcurrent",
			"-XX:+ExitOnOutOfMemoryError",
		},
		extraOpts: []string{
			"-Djdk.attach.allowAttachSelf=true",
		},
	}
}

// ForCoordinator configures the builder for Coordinator role
func (b *JVMConfigBuilder) ForCoordinator() *JVMConfigBuilder {
	b.maxMemory = constants.DefaultCoordinatorMaxMemory
	return b
}

// ForWorker configures the builder for Worker role
func (b *JVMConfigBuilder) ForWorker() *JVMConfigBuilder {
	b.maxMemory = constants.DefaultWorkerMaxMemory
	return b
}

// WithMaxMemory sets the maximum heap memory
func (b *JVMConfigBuilder) WithMaxMemory(memory string) *JVMConfigBuilder {
	b.maxMemory = memory
	return b
}

// Build generates the JVM configuration as a string
func (b *JVMConfigBuilder) Build() string {
	lines := make([]string, 0, 1+len(b.gcOptions)+len(b.extraOpts))

	// Memory settings
	lines = append(lines, fmt.Sprintf("-Xmx%s", b.maxMemory))

	// GC options
	lines = append(lines, b.gcOptions...)

	// Extra options
	lines = append(lines, b.extraOpts...)

	return strings.Join(lines, "\n")
}
