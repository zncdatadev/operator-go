package v1alpha1

import (
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
)

type OverridesSpec struct {
	// +kubebuilder:validation:Optional
	CliOverrides []string `json:"cliOverrides,omitempty"`
	// +kubebuilder:validation:Optional
	EnvOverrides map[string]string `json:"envOverrides,omitempty"`
	// +kubebuilder:validation:Optional
	ConfigOverrides map[string]map[string]string `json:"configOverrides,omitempty"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:EmbeddedResource
	// +kubebuilder:validation:Type=object
	PodOverrides *k8sruntime.RawExtension `json:"podOverrides,omitempty"`
}
