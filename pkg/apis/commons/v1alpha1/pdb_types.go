package v1alpha1

type RoleConfigSpec struct {
	// +kubebuilder:validation:Optional
	PodDisruptionBudget *PodDisruptionBudgetSpec `json:"podDisruptionBudget,omitempty"`
}

// This struct is used to configure:
//
// 1. If PodDisruptionBudgets are created by the operator
// 2. The allowed number of Pods to be unavailable (`maxUnavailable`)
type PodDisruptionBudgetSpec struct {
	// +kubebuilder:validation:Optional
	// MinAvailable *int32 `json:"minAvailable,omitempty"`

	// Whether a PodDisruptionBudget should be written out for this role.
	// Disabling this enables you to specify your own - custom - one.
	// Defaults to true.

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled"`

	// The number of Pods that are allowed to be down because of voluntary disruptions.
	// If you don't explicitly set this, the operator will use a sane default based
	/// upon knowledge about the individual product.

	// +kubebuilder:validation:Optional
	MaxUnavailable *int32 `json:"maxUnavailable,omitempty"`
}
