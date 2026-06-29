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

package vector

import "github.com/zncdatadev/operator-go/pkg/constant"

const (
	// VectorSidecarName is the name of the Vector sidecar container.
	VectorSidecarName = "vector"

	// VectorConfigVolumeName is the name of the Vector config volume.
	VectorConfigVolumeName = "vector-config"

	// VectorConfigMountPath is the mount path for Vector config.
	VectorConfigMountPath = "/etc/vector"

	// VectorDataVolumeName is the name of the Vector data volume.
	VectorDataVolumeName = "vector-data"

	// VectorDataMountPath is the mount path for Vector data.
	VectorDataMountPath = "/var/lib/vector"

	// VectorDataVolumeSize is the default size for the Vector data volume.
	VectorDataVolumeSize = "50Mi"

	// VectorLogVolumeName is the canonical name of the shared log volume. The producer side
	// (the role-group base handler) creates this emptyDir and RW-mounts it on each product
	// container; the Vector sidecar (the consumer) RO-mounts the same volume by this name.
	VectorLogVolumeName = "log"

	// VectorLogMountPath is the mount path for the shared log volume on the Vector container.
	// It is the framework-canonical log directory so the consumer reads exactly where the
	// producer (and product file appenders) write. Kept as a package-local alias of
	// constant.KubedoopLogDir to avoid an import cycle-free indirection at call sites.
	VectorLogMountPath = constant.KubedoopLogDir

	// DefaultLogVolumeSize is the default SizeLimit for the shared log emptyDir created by
	// the producer (the role-group base handler). It bounds on-node disk usage for rolling
	// log files that the Vector sidecar consumes. The bound also protects the node from a
	// runaway log producer filling the ephemeral filesystem. It is overridable per role
	// group (see BaseRoleGroupHandler.LogVolumeSize). 33Mi mirrors the rolling-appender
	// budget zk historically used (a few small rolled files) and stays comfortably small.
	DefaultLogVolumeSize = "33Mi"

	// VectorDefaultConfigMapName is the default ConfigMap name for Vector config.
	VectorDefaultConfigMapName = "vector-config"

	// VectorAPIPort is the port for the Vector API.
	VectorAPIPort = 8686

	// VectorHealthEndpoint is the health check endpoint for the Vector API.
	VectorHealthEndpoint = "/health"

	// VectorConfigFileName is the name of the Vector configuration file.
	VectorConfigFileName = "vector.yaml"

	// VectorReadinessInitialDelaySeconds is the initial delay for the readiness probe.
	VectorReadinessInitialDelaySeconds int32 = 5

	// VectorReadinessPeriodSeconds is the period for the readiness probe.
	VectorReadinessPeriodSeconds int32 = 10
)
