package v1alpha1

// TLSPrivider defines the TLS provider for authentication.
// You can specify the none or server or mutual verification.
type TLSVerificationSpec struct {

	// +kubebuilder:validation:Optional
	None *NoneVerification `json:"none,omitempty"`

	// +kubebuilder:validation:Optional
	Server *ServerVerification `json:"server,omitempty"`

	// +kubebuilder:validation:Optional
	Mutual *MutualVerification `json:"mutual,omitempty"`
}

type MutualVerification struct {
	// +kubebuilder:validation:Required
	CertSecretClass string `json:"certSecretClass"`
}

type NoneVerification struct {
}

type ServerVerification struct {
	// +kubebuilder:validation:Required
	CACert *CACert `json:"caCert"`
}

// CACert is the CA certificate for server verification.
// You can specify the secret class or the webPki.
type CACert struct {
	// +kubebuilder:validation:Optional
	SecretClass string `json:"secretClass,omitempty"`

	// +kubebuilder:validation:Optional
	WebPki *WebPki `json:"webPki,omitempty"`
}

type WebPki struct {
}
