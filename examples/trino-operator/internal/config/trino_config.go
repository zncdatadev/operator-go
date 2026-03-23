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

	trinov1alpha1 "github.com/zncdatadev/operator-go/examples/trino-operator/api/v1alpha1"
	"github.com/zncdatadev/operator-go/examples/trino-operator/internal/constants"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
)

// TrinoConfigBuilder builds Trino configuration
type TrinoConfigBuilder struct {
	coordinator bool
	cluster     *trinov1alpha1.TrinoCluster
	buildCtx    *reconciler.RoleGroupBuildContext
	port        int32
	properties  map[string]string
}

// NewTrinoConfigBuilder creates a new TrinoConfigBuilder
func NewTrinoConfigBuilder() *TrinoConfigBuilder {
	return &TrinoConfigBuilder{
		properties: make(map[string]string),
	}
}

// ForCoordinator configures the builder for Coordinator role
func (b *TrinoConfigBuilder) ForCoordinator(cluster *trinov1alpha1.TrinoCluster, buildCtx *reconciler.RoleGroupBuildContext, port int32) *TrinoConfigBuilder {
	b.coordinator = true
	b.cluster = cluster
	b.buildCtx = buildCtx
	b.port = port

	// Coordinator-specific configuration
	b.properties["coordinator"] = "true"
	b.properties["node-scheduler.include-coordinator"] = "false"
	b.properties["http-server.http.port"] = fmt.Sprintf("%d", port)
	b.properties["discovery-server.enabled"] = "true"
	b.properties["discovery.uri"] = fmt.Sprintf("http://%s:%d", coordinatorServiceName(cluster), port)

	return b
}

// ForWorker configures the builder for Worker role
func (b *TrinoConfigBuilder) ForWorker(cluster *trinov1alpha1.TrinoCluster, buildCtx *reconciler.RoleGroupBuildContext, coordinatorPort int32) *TrinoConfigBuilder {
	b.coordinator = false
	b.cluster = cluster
	b.buildCtx = buildCtx
	b.port = coordinatorPort // Used for discovery URI

	// Worker-specific configuration
	b.properties["coordinator"] = "false"
	b.properties["http-server.http.port"] = fmt.Sprintf("%d", coordinatorPort)
	b.properties["discovery.uri"] = fmt.Sprintf("http://%s:%d", coordinatorServiceName(cluster), coordinatorPort)

	return b
}

// coordinatorServiceName returns the client-facing Service name for the coordinator.
// The SDK names resources as {clusterName}-{groupName}.
func coordinatorServiceName(cr *trinov1alpha1.TrinoCluster) string {
	if cr.Spec.Coordinators != nil {
		for groupName := range cr.Spec.Coordinators.RoleGroups {
			return fmt.Sprintf("%s-%s", cr.Name, groupName)
		}
	}
	return fmt.Sprintf("%s-coordinator", cr.Name)
}

// WithProperty adds a custom property
func (b *TrinoConfigBuilder) WithProperty(key, value string) *TrinoConfigBuilder {
	b.properties[key] = value
	return b
}

// Build generates the Trino configuration as a string
func (b *TrinoConfigBuilder) Build() string {
	lines := make([]string, 0, len(b.properties))
	for key, value := range b.properties {
		lines = append(lines, fmt.Sprintf("%s=%s", key, value))
	}
	return strings.Join(lines, "\n")
}

// JVMConfigBuilder builds JVM configuration
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
