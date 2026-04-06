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

	// VectorLogVolumeName is the name of the shared log volume.
	VectorLogVolumeName = "log-volume"

	// VectorLogMountPath is the mount path for logs.
	VectorLogMountPath = "/var/log/app"

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
