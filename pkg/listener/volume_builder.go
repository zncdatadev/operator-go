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
	// ListenerClassClusterInternal exposes the workload inside the cluster only: a ClusterIP
	// Service.
	ListenerClassClusterInternal ListenerClass = "cluster-internal"
	// ListenerClassExternalStable exposes the workload at an address that does not change as pods
	// move: a LoadBalancer Service.
	ListenerClassExternalStable ListenerClass = "external-stable"
	// ListenerClassExternalUnstable exposes the workload outside the cluster at an address tied to
	// whichever node the pod lands on: a NodePort Service. "Unstable" is precisely that — the
	// address changes when the pod is rescheduled.
	//
	// This comment previously said "LoadBalancer with dynamic IPs", which is what the name is NOT:
	// a LoadBalancer is the STABLE class. builder.ListenerClassServiceType is the executable form
	// of the mapping, and two downstream operators had already drawn opposite conclusions from the
	// old wording.
	ListenerClassExternalUnstable ListenerClass = "external-unstable"
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
	// AnnotationListenerName identifies the listener. Defaults to pod name if unset.
	AnnotationListenerName = listenerAPIGroupPrefix + "listenerName"
)

// ListenerStorageClassPtr returns a pointer to the ListenerStorageClass constant.
// Useful for Kubernetes PVC spec fields that require *string.
func ListenerStorageClassPtr() *string {
	return ptr.To(ListenerStorageClass)
}
