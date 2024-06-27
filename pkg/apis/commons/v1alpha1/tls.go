package v1alpha1

type TLSVerificationSpec struct {

	// +kubebuilder:validation:Optional
	None *NoneVerification `json:"none,omitempty"`

	// +kubebuilder:validation:Optional
	Server *ServerVerification `json:"server,omitempty"`
}

type NoneVerification struct {
}

type ServerVerification struct {
	// +kubebuilder:validation:Required
	CACert *CACert `json:"caCert"`
}

type CACert struct {
	// +kubebuilder:validation:Optional
	SecretClass string `json:"secretClass,omitempty"`

	WebPIK *string `json:"webPIK,omitempty"`
}
