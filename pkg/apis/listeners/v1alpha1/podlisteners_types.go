/*
Copyright 2024 zncdatadev.

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
)

type PodListenerScope string

const (
	// PodListenerScope is the scope of the pod listener.
	PodlistenerNodeScope PodListenerScope = "Node"

	// PodListenerScope is the scope of the pod listener.
	PodlistenerClusterScope PodListenerScope = "Cluster"
)

// PodListenersSpec defines the desired state of PodListeners.
type PodListenersSpec struct {
	// +kubebuilder:validation:Required
	Listeners map[string]PodListener `json:"listeners,omitempty"`
}

// PodListener defines the listener configuration for a pod.
type PodListener struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=Node;Cluster
	// +kubebuilder:default=Cluster
	Scope PodListenerScope `json:"scope,omitempty"`

	// +kubebuilder:validation:Required
	ListenerIngresses []IngressAddressSpec `json:"listenerIngresses,omitempty"`
}

// PodListenersStatus defines the observed state of PodListeners.
type PodListenersStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// PodListeners is the Schema for the podlisteners API.
type PodListeners struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PodListenersSpec   `json:"spec,omitempty"`
	Status PodListenersStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PodListenersList contains a list of PodListeners.
type PodListenersList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PodListeners `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PodListeners{}, &PodListenersList{})
}
