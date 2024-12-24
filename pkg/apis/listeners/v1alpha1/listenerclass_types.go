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

// ListenerClassSpec defines the desired state of ListenerClass
type ListenerClassSpec struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=LoadBalancer;NodePort;ClusterIP
	ServiceType *corev1.ServiceType `json:"serviceType,omitempty"`

	// +kubebuilder:validation:Optional
	ServiceAnnotations map[string]string `json:"serviceAnnotations,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=Local
	// +kubebuilder:validation:Enum=Local;Cluster
	ServiceExternalTrafficPolicy corev1.ServiceExternalTrafficPolicyType `json:"serviceExternalTrafficPolicy,omitempty"`

	// When preferredAddressType is set to HostnameConservative, the controller will
	// attempt to use the ip only `ListenerClassSpec.ServiceType` is NodePort,
	// otherwise it will use the hostname.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=HostnameConservative
	// +kubebuilder:validation:Enum=HostnameConservative;Hostname;IP
	PreferredAddressType AddressType `json:"preferredAddressType,omitempty"`
}

// ListenerClassStatus defines the observed state of ListenerClass
type ListenerClassStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=listenerclasses,scope=Cluster
// +kubebuilder:subresource:status

// ListenerClass is the Schema for the listenerclasses API
type ListenerClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ListenerClassSpec   `json:"spec,omitempty"`
	Status ListenerClassStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ListenerClassList contains a list of ListenerClass
type ListenerClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ListenerClass `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ListenerClass{}, &ListenerClassList{})
}
