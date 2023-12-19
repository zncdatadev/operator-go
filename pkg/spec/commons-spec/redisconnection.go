package commons_spec

// RedisConnectionSpec defines the desired state of RedisConnection
type RedisConnectionSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// +kubebuilder:validation:Required
	Host string `json:"host,omitempty"`
	// +kubeBuilder:validation:Required
	Port     string `json:"port,omitempty"`
	Password string `json:"password,omitempty"`
}
