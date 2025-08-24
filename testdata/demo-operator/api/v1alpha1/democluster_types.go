/*
Copyright 2025 ZNCDataDev.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
)

// DemoClusterSpec defines the desired state of DemoCluster
type DemoClusterSpec struct {
	// +optional
	Image *ImageSpec `json:"image,omitempty"`
	// +optional
	ClusterConfig *ClusterConfigSpec `json:"clusterConfig,omitempty"`
	// +optional
	ClusterOperation *commonsv1alpha1.ClusterOperationSpec `json:"clusterOperation,omitempty"`
	Coordinator      *TrinoCoordinatorSpec                 `json:"Coordinator"`
	Worker           *TrinoWorkerSpec                      `json:"worker"`
}

// DemoClusterStatus defines the observed state of DemoCluster.
type DemoClusterStatus struct {
	// +kubebuilder:validation:Optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type ImageSpec struct {
	// +optional
	Custom string `json:"custom,omitempty"`
	// +optional
	// +kubebuilder:default=quay.io/zncdatadev
	Repository string `json:"repository,omitempty"`
	// +optional
	KubedoopVersion string `json:"kubedoopVersion,omitempty"`
	// +optional
	ProductVersion string `json:"productVersion,omitempty"`
	// +optional
	// +kubebuilder:default=IfNotPresent
	// +kubebuilder:validation:Enum=IfNotPresent;Always;Never
	PullPolicy string `json:"pullPolicy,omitempty"`
	// +optional
	PullSecretName string `json:"pullSecretName,omitempty"`
}

type ClusterConfigSpec struct {
	// +optional
	ListenerClass string `json:"listenerClass,omitempty"`
}

type TrinoCoordinatorSpec struct {
	RoleGroups map[string]TrinoRoleGroupSpec `json:"roleGroups"`
	// +optional
	Config *TrinoConfigSpec `json:"config,omitempty"`
	// +optional
	RoleConfig *commonsv1alpha1.RoleConfigSpec `json:"roleConfig,omitempty"`

	*commonsv1alpha1.OverridesSpec `json:",inline"`
}

type TrinoWorkerSpec struct {
	RoleGroups map[string]TrinoRoleGroupSpec `json:"roleGroups,omitempty"`
	// +optional
	Config *TrinoConfigSpec `json:"config,omitempty"`
	// +optional
	RoleConfig *commonsv1alpha1.RoleConfigSpec `json:"roleConfig,omitempty"`

	*commonsv1alpha1.OverridesSpec `json:",inline"`
}

type TrinoRoleGroupSpec struct {
	*commonsv1alpha1.OverridesSpec `json:",inline"`
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`
	// +optional
	Config *TrinoConfigSpec `json:"config,omitempty"`
}

type TrinoConfigSpec struct {
	commonsv1alpha1.RoleGroupConfigSpec `json:",inline"`
	// +optional
	QueryMaxMemory string `json:"queryMaxMemory,omitempty"`
	// +optional
	QueryMaxMemoryPerNode string `json:"queryMaxMemoryPerNode,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// DemoCluster is the Schema for the democlusters API
type DemoCluster struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of DemoCluster
	// +required
	Spec DemoClusterSpec `json:"spec"`

	// status defines the observed state of DemoCluster
	// +optional
	Status DemoClusterStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// DemoClusterList contains a list of DemoCluster
type DemoClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DemoCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DemoCluster{}, &DemoClusterList{})
}
