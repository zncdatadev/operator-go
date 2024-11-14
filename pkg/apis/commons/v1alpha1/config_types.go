package v1alpha1

import k8sruntime "k8s.io/apimachinery/pkg/runtime"

type RoleConfigSpec struct {
	// +kubebuilder:validation:Optional
	PodDisruptionBudget *PodDisruptionBudgetSpec `json:"podDisruptionBudget,omitempty"`
}

type RoleGroupConfigSpec struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:EmbeddedResource
	// +kubebuilder:validation:Type=object
	Affinity *k8sruntime.RawExtension `json:"affinity,omitempty"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="30s"
	GracefulShutdownTimeout string `json:"gracefulShutdownTimeout,omitempty"`
	// +kubebuilder:validation:Optional
	Logging *LoggingSpec `json:"logging,omitempty"`
	// +kubebuilder:validation:Optional
	Resources *ResourcesSpec `json:"resources,omitempty"`
}
