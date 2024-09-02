package v1alpha1

// ClusterOperationSpec defines the desired state of ClusterOperation
type ClusterOperationSpec struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	ReconciliationPaused bool `json:"reconciliationPaused,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	Stopped bool `json:"stopped,omitempty"`
}
