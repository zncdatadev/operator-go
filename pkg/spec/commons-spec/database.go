package commons_spec

// DatabaseSpec defines the desired state of Database
type DatabaseSpec struct {
	//+kubebuilder:validation:Required
	Name string `json:"name,omitempty"`
	//+kubebuilder:validation:Required
	Reference string `json:"reference,omitempty"`
	//+kubebuilder:validation:Required
	Credential *DatabaseCredentialSpec `json:"credential,omitempty"`
}

type DatabaseCredentialSpec struct {
	ExistSecret string `json:"existingSecret,omitempty"`
	Username    string `json:"username,omitempty"`
	Password    string `json:"password,omitempty"`
}
