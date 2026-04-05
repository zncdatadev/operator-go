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

package common

import (
	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// ClusterInterface defines cluster-level operations that all product CRs must implement.
// This interface enables the SDK to work with any product-specific cluster type.
type ClusterInterface interface {
	// GetName returns the cluster name (from ObjectMeta.Name).
	GetName() string

	// GetNamespace returns the cluster namespace (from ObjectMeta.Namespace).
	GetNamespace() string

	// GetUID returns the cluster UID (from ObjectMeta.UID).
	GetUID() types.UID

	// GetLabels returns the cluster labels (from ObjectMeta.Labels).
	GetLabels() map[string]string

	// GetAnnotations returns the cluster annotations (from ObjectMeta.Annotations).
	GetAnnotations() map[string]string

	// GetSpec returns the cluster spec as GenericClusterSpec.
	GetSpec() *v1alpha1.GenericClusterSpec

	// GetStatus returns the cluster status as GenericClusterStatus.
	GetStatus() *v1alpha1.GenericClusterStatus

	// SetStatus updates the cluster status.
	SetStatus(status *v1alpha1.GenericClusterStatus)

	// GetObjectMeta returns the object metadata.
	GetObjectMeta() *metav1.ObjectMeta

	// GetScheme returns the runtime scheme.
	GetScheme() *runtime.Scheme

	// DeepCopy creates a deep copy of the cluster.
	DeepCopyCluster() ClusterInterface

	// GetRuntimeObject returns the underlying runtime.Object.
	GetRuntimeObject() runtime.Object
}

// ClusterObject is a helper struct that can be embedded in product-specific CRs
// to provide default implementations of ClusterInterface methods.
// Note: Product CRs must still implement GetSpec() and GetStatus() to return
// the embedded GenericClusterSpec and GenericClusterStatus.
type ClusterObject struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
}

// GetName returns the cluster name.
func (c *ClusterObject) GetName() string {
	return c.Name
}

// GetNamespace returns the cluster namespace.
func (c *ClusterObject) GetNamespace() string {
	return c.Namespace
}

// GetUID returns the cluster UID.
func (c *ClusterObject) GetUID() types.UID {
	return c.UID
}

// GetLabels returns the cluster labels.
func (c *ClusterObject) GetLabels() map[string]string {
	if c.Labels == nil {
		return make(map[string]string)
	}
	return c.Labels
}

// GetAnnotations returns the cluster annotations.
func (c *ClusterObject) GetAnnotations() map[string]string {
	if c.Annotations == nil {
		return make(map[string]string)
	}
	return c.Annotations
}

// GetObjectMeta returns the object metadata.
func (c *ClusterObject) GetObjectMeta() *metav1.ObjectMeta {
	return &c.ObjectMeta
}
