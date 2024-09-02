package v1alpha1

type Credentials struct {

	// SecretClass scope
	// +kubebuilder:validation:Optional
	Scope *CredentialsScope `json:"scope,omitempty"`

	// +kubebuilder:validation:Required
	SecretClass string `json:"secretClass"`
}

type CredentialsScope struct {

	// +kubebuilder:validation:Optional
	Node bool `json:"node,omitempty"`

	// +kubebuilder:validation:Optional
	Pod bool `json:"pod,omitempty"`

	// +kubebuilder:validation:Optional
	Services []string `json:"services,omitempty"`
}
