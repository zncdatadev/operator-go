package commons_spec

// S3ConnectionSpec defines the desired state of S3Connection
type S3ConnectionSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// +kubebuilder:validation:Required
	S3Credential *S3Credential `json:"credential,omitempty"`
}

type S3Credential struct {
	// +kubebuilder:validation:Optional
	ExistingSecret string `json:"existingSecret,omitempty"`
	// +kubebuilder:validation:Required
	AccessKey string `json:"accessKey,omitempty"`
	// +kubebuilder:validation:Required
	SecretKey string `json:"secretKey,omitempty"`
	// +kubebuilder:validation:Required
	Endpoint string `json:"endpoint,omitempty"`
	Region   string `json:"region,omitempty"`
	SSL      bool   `json:"ssl,omitempty"`
}
