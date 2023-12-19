package commons_spec

// DatabaseConnectionSpec defines the desired state of DatabaseConnection
type DatabaseConnectionSpec struct {
	Provider *ProviderSpec `json:"provider,omitempty"`
	Default  bool          `json:"default,omitempty"`
}

type ProviderSpec struct {
	// +kubebuilder:validation:Enum=mysql;postgres
	// +kubebulider:default=postgres
	Driver string `json:"driver,omitempty"`
	// +kubebuilder:validation:Required
	Host string `json:"host,omitempty"`
	// +kubebuilder:validation:Required
	Port int `json:"port,omitempty"`
	// +kubebuilder:validation:Required
	SSL bool `json:"ssl,omitempty"`
	// +kubebuilder:validation:Required
	Credential *DatabaseConnectionCredentialSpec `json:"credential,omitempty"`
}

type DatabaseConnectionCredentialSpec struct {
	ExistSecret string `json:"existingSecret,omitempty"`
}
