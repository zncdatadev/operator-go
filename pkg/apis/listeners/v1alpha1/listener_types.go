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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AddressType string

const (
	AddressTypeHostname AddressType = "Hostname"
	AddressTypeIP       AddressType = "IP"
	// When preferredAddressType is set to HostnameConservative, the controller will
	// attempt to use the ip only `ListenerClassSpec.ServiceType` is NodePort,
	// otherwise it will use the hostname.
	AddressTypeHostnameConservative AddressType = "HostnameConservative"
)

// ListenerSpec defines the desired state of Listener
type ListenerSpec struct {
	// +kubebuilder:validation:Required
	ClassName string `json:"className"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default={}
	ExtraPodSelectorLabels map[string]string `json:"extraPodSelectorLabels,omitempty"`

	// +kubebuilder:validation:Optional
	Ports []PortSpec `json:"ports,omitempty"`

	// Pointer so a client can request false; a plain bool would be dropped by omitempty
	// and the CRD default would restore true.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	PublishNotReadyAddresses *bool `json:"publishNotReadyAddresses,omitempty"`
}

// GetPublishNotReadyAddresses returns whether not-ready addresses are published,
// applying the documented default of true when unset.
func (s *ListenerSpec) GetPublishNotReadyAddresses() bool {
	if s == nil || s.PublishNotReadyAddresses == nil {
		return true
	}
	return *s.PublishNotReadyAddresses
}

type PortSpec struct {
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// L4 protocol, `TCP`` or `UDP`
	// +kubebuilder:validation:Optional
	Protocol corev1.Protocol `json:"protocol,omitempty"`

	// +kubebuilder:validation:Required
	Port int32 `json:"port"`
}

// ListenerStatus defines the observed state of Listener
type ListenerStatus struct {
	IngressAddresses []IngressAddressSpec `json:"ingressAddresses,omitempty"`
	NodePorts        map[string]int32     `json:"nodePorts,omitempty"`
	ServiceName      string               `json:"serviceName,omitempty"`
}

type IngressAddressSpec struct {
	// +kubebuilder:validation:Required
	Address string `json:"address"`

	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=Hostname;IP
	AddressType AddressType `json:"addressType"`

	// +kubebuilder:validation:Required
	Ports map[string]int32 `json:"ports"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Listener is the Schema for the listeners API
type Listener struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ListenerSpec   `json:"spec,omitempty"`
	Status ListenerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ListenerList contains a list of Listener
type ListenerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Listener `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Listener{}, &ListenerList{})
}
