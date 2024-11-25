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

const (
	ListenerAPIGroup     string = "listeners." + KubedoopDomain
	ListenerStorageClass string = ListenerAPIGroup

	listenerAPIGroupPrefix string = ListenerAPIGroup + "/"
)

func ListenerStorageClassPtr() *string {
	listenersStorageClass := ListenerStorageClass
	return &listenersStorageClass
}

// Kubeddoop defined annotations for PVCTemplate.
// Then csi driver can extract annotations from PVC to prepare the listener for pod.
const (
	// Specify which network listening rules to use, it is REQUIRED.
	// It can be one of the following values:
	//	- cluster-internal
	//	- external-unstable
	//	- external-stable
	//	- <other user defined class name>
	AnnotationListenersClass string = listenerAPIGroupPrefix + "class"
	// The listener name is used to identify the listener, it is OPTIONAL.
	// If not set, the listener name will be the same as the pod name.
	AnnotationListenerName string = listenerAPIGroupPrefix + "listenerName"
)

type ListenerClass string

const (
	// ClusterInternal is the default listener class.
	// cluster-internal --> k8s service with ClusterIP
	ClusterInternal ListenerClass = "cluster-internal"
	// external-unstable --> k8s service with NodePort
	ExternalUnstable ListenerClass = "external-unstable"
	// ExternalStable requires a k8s LoadBalancer
	// external-stable --> k8s service with LoadBalancer
	ExternalStable ListenerClass = "external-stable"
)
