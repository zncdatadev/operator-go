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

package constants

// Image constants
const (
	// DefaultImageRepo is the default container image repository
	DefaultImageRepo = "quay.io/zncdatadev"
	// DefaultImageProductVersion is the default Trino product version
	DefaultImageProductVersion = "476"
	// DefaultImageKubedoopVersion is the default kubedoop stack version
	DefaultImageKubedoopVersion = "0.0.0-dev"
	// ProductName is the product name used for image resolution
	ProductName = "trino"
)

// RoleGroup constants
const (
	// DefaultRoleGroupName is the default role group name
	DefaultRoleGroupName = "default"
)

// Port constants
const (
	// DefaultHTTPPort is the default HTTP API port for Trino
	DefaultHTTPPort int32 = 8080
)

// Replica constants
const (
	// DefaultCoordinatorReplicas is the default number of coordinator replicas
	DefaultCoordinatorReplicas int32 = 1
	// DefaultWorkerReplicas is the default number of worker replicas
	DefaultWorkerReplicas int32 = 2
)

// Resource constants
const (
	// DefaultCoordinatorCPURequest is the default CPU request for coordinators
	DefaultCoordinatorCPURequest = "500m"
	// DefaultCoordinatorCPULimit is the default CPU limit for coordinators
	DefaultCoordinatorCPULimit = "1"
	// DefaultCoordinatorMemoryRequest is the default memory request for coordinators
	DefaultCoordinatorMemoryRequest = "1Gi"
	// DefaultCoordinatorMemoryLimit is the default memory limit for coordinators
	DefaultCoordinatorMemoryLimit = "2Gi"

	// DefaultWorkerCPURequest is the default CPU request for workers
	DefaultWorkerCPURequest = "500m"
	// DefaultWorkerCPULimit is the default CPU limit for workers
	DefaultWorkerCPULimit = "2"
	// DefaultWorkerMemoryRequest is the default memory request for workers
	DefaultWorkerMemoryRequest = "2Gi"
	// DefaultWorkerMemoryLimit is the default memory limit for workers
	DefaultWorkerMemoryLimit = "4Gi"
)

// JVM constants
const (
	// DefaultCoordinatorMaxMemory is the default max heap memory for coordinators
	DefaultCoordinatorMaxMemory = "2G"
	// DefaultWorkerMaxMemory is the default max heap memory for workers
	DefaultWorkerMaxMemory = "4G"
)
