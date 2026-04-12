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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/common"
)

// TrinoClusterSpec defines the desired state of TrinoCluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].status"
// +kubebuilder:printcolumn:name="Workers",type="integer",JSONPath=".status.registeredWorkers"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type TrinoClusterSpec struct {
	// ClusterOperation controls operator behavior at runtime.
	// Allows pausing reconciliation or stopping the cluster gracefully.
	// +kubebuilder:validation:Optional
	ClusterOperation *commonsv1alpha1.ClusterOperationSpec `json:"clusterOperation,omitempty"`

	// Image specifies the Trino container image configuration.
	// If not set, the webhook defaulter will provide product defaults.
	// +kubebuilder:validation:Optional
	Image *commonsv1alpha1.ImageSpec `json:"image,omitempty"`

	// Coordinators defines the Coordinators role configuration (plural naming)
	// Coordinator is responsible for query coordination, metadata management, and client request handling
	Coordinators *CoordinatorsSpec `json:"coordinators,omitempty"`

	// Workers defines the Workers role configuration (plural naming)
	// Worker is responsible for executing query tasks and can be horizontally scaled
	Workers *WorkersSpec `json:"workers,omitempty"`

	// Catalogs defines the data source Catalog configuration list
	// Supports Hive, Iceberg, Kafka, MySQL, PostgreSQL, Delta, etc.
	Catalogs []CatalogSpec `json:"catalogs,omitempty"`
}

// CoordinatorsSpec defines the Coordinators role configuration
type CoordinatorsSpec struct {
	// Embed generic role spec, including RoleGroups, Overrides, etc.
	commonsv1alpha1.RoleSpec `json:",inline"`

	// DiscoveryEnabled indicates whether to enable Discovery service (for Worker discovery)
	// +kubebuilder:default=true
	DiscoveryEnabled bool `json:"discoveryEnabled,omitempty"`

	// HTTPPort is the HTTP API port
	// +kubebuilder:default=8080
	HTTPPort int32 `json:"httpPort,omitempty"`
}

// WorkersSpec defines the Workers role configuration
type WorkersSpec struct {
	// Embed generic role spec, including RoleGroups, Overrides, etc.
	commonsv1alpha1.RoleSpec `json:",inline"`

	// HTTPPort is the HTTP API port
	// +kubebuilder:default=8080
	HTTPPort int32 `json:"httpPort,omitempty"`
}

// CatalogSpec defines the data source Catalog configuration
type CatalogSpec struct {
	// Name is the Catalog name (e.g., hive, iceberg, kafka)
	Name string `json:"name"`

	// Type is the Catalog type
	// +kubebuilder:validation:Enum=hive;iceberg;kafka;mysql;postgresql;delta;tpch;tpcds
	Type string `json:"type"`

	// Properties are the Catalog configuration properties (key-value form)
	Properties map[string]string `json:"properties,omitempty"`
}

// TrinoClusterStatus defines the observed state of TrinoCluster
type TrinoClusterStatus struct {
	// Embed generic cluster status, including Conditions, RoleGroups, etc.
	commonsv1alpha1.GenericClusterStatus `json:",inline"`

	// ==================== Trino Specific Status ====================

	// RegisteredWorkers is the number of registered Workers
	RegisteredWorkers int32 `json:"registeredWorkers,omitempty"`

	// CatalogsReady is the list of ready Catalogs
	CatalogsReady []string `json:"catalogsReady,omitempty"`
}

// +kubebuilder:object:root=true

// TrinoCluster is the CRD for Trino cluster
// TrinoClusterList contains a list of TrinoCluster
type TrinoCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TrinoClusterSpec   `json:"spec,omitempty"`
	Status TrinoClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TrinoClusterList contains a list of TrinoCluster
type TrinoClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TrinoCluster `json:"items"`
}

// ==================== ClusterInterface Implementation ====================
// This is the key to using operator-go SDK: implement ClusterInterface

// GetSpec builds and returns a GenericClusterSpec from the typed role fields.
// This bridges the type-safe coordinators/workers fields to the SDK framework's
// generic Roles map, without exposing a redundant spec.roles field in the CRD.
func (t *TrinoCluster) GetSpec() *commonsv1alpha1.GenericClusterSpec {
	roles := make(map[string]commonsv1alpha1.RoleSpec)
	if t.Spec.Coordinators != nil {
		roles["coordinators"] = t.Spec.Coordinators.RoleSpec
	}
	if t.Spec.Workers != nil {
		roles["workers"] = t.Spec.Workers.RoleSpec
	}
	return &commonsv1alpha1.GenericClusterSpec{
		Image:            t.Spec.Image,
		ClusterOperation: t.Spec.ClusterOperation,
		Roles:            roles,
	}
}

// GetStatus returns the generic cluster status
func (t *TrinoCluster) GetStatus() *commonsv1alpha1.GenericClusterStatus {
	return &t.Status.GenericClusterStatus
}

// SetStatus sets the generic cluster status
func (t *TrinoCluster) SetStatus(status *commonsv1alpha1.GenericClusterStatus) {
	t.Status.GenericClusterStatus = *status
}

// DeepCopyCluster creates a deep copy of the cluster
func (t *TrinoCluster) DeepCopyCluster() common.ClusterInterface {
	return t.DeepCopy()
}

// GetRuntimeObject returns the runtime object
func (t *TrinoCluster) GetRuntimeObject() runtime.Object {
	return t
}

// GetObjectMeta returns the object metadata
func (t *TrinoCluster) GetObjectMeta() *metav1.ObjectMeta {
	return &t.ObjectMeta
}

// GetScheme returns the runtime scheme
func (t *TrinoCluster) GetScheme() *runtime.Scheme {
	return nil // Scheme is set by the manager
}

// GetUID returns the cluster UID
func (t *TrinoCluster) GetUID() types.UID {
	return t.UID
}

func init() {
	SchemeBuilder.Register(&TrinoCluster{}, &TrinoClusterList{})
}
