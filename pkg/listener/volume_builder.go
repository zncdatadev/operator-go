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

package listener

import (
	"github.com/zncdatadev/operator-go/pkg/constant"
	"k8s.io/utils/ptr"
)

// ListenerClass defines the exposure strategy.
type ListenerClass string

const (
	// ListenerClassClusterInternal creates ClusterIP Service.
	ListenerClassClusterInternal ListenerClass = "cluster-internal"
	// ListenerClassExternalStable creates LoadBalancer with stable IPs.
	ListenerClassExternalStable ListenerClass = "external-stable"
	// ListenerClassExternalUnstable creates LoadBalancer with dynamic IPs.
	ListenerClassExternalUnstable ListenerClass = "external-unstable"
)

// ListenerScope defines the scope of listener volumes provisioned by listener-operator.
type ListenerScope string

const (
	// ListenerScopeNode limits listener discovery to the node level.
	ListenerScopeNode ListenerScope = "Node"
	// ListenerScopeCluster enables listener discovery across the cluster.
	ListenerScopeCluster ListenerScope = "Cluster"
)

// Listener constants for listener-operator CSI integration.
// All annotations and labels derive from KubedoopDomain for single source of truth.
const (
	ListenerAPIGroup       = "listeners." + constant.KubedoopDomain
	ListenerStorageClass   = ListenerAPIGroup
	listenerAPIGroupPrefix = ListenerAPIGroup + "/"

	// CSIDriverName is the CSI driver name for listener-operator.
	CSIDriverName = ListenerAPIGroup
	// ListenerClassAnnotation specifies the listener class for PVC templates.
	ListenerClassAnnotation = listenerAPIGroupPrefix + "class"
	// ListenerScopeAnnotation specifies the listener scope for PVC templates.
	ListenerScopeAnnotation = listenerAPIGroupPrefix + "scope"
	// AnnotationListenerName identifies the listener. Defaults to pod name if unset.
	AnnotationListenerName = listenerAPIGroupPrefix + "listenerName"
)

// ListenerStorageClassPtr returns a pointer to the ListenerStorageClass constant.
// Useful for Kubernetes PVC spec fields that require *string.
func ListenerStorageClassPtr() *string {
	return ptr.To(ListenerStorageClass)
}
