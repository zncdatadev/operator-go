package v1alpha1

type AuthenticationSpec struct {
	// +kubebuilder:validation:Required
	AuthenticationClass string    `json:"authenticationClass"`
	Oidc                *OidcSpec `json:"oidc,omitempty"`
}

// OidcSpec defines the OIDC spec.
type OidcSpec struct {
	// OIDC client credentials secret. It must contain the following keys:
	//   - `CLIENT_ID`: The client ID of the OIDC client.
	//   - `CLIENT_SECRET`: The client secret of the OIDC client.
	// credentials will omit to pod environment variables.
	// +kubebuilder:validation:Required
	ClientCredentialsSecret string `json:"clientCredentialsSecret"`

	// Extra scopes to request during the OIDC flow. e.g. `["email", "profile"]`
	// +kubebuilder:validation:Optional
	ExtraScopes []string `json:"extraScopes,omitempty"`
}
