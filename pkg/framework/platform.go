package framework

// ClusterConfig is the framework-owned part of the flat clusterConfig object.
// Product-specific S remains separate in Go and shares this object on the wire.
type ClusterConfig struct {
	VectorAgentConfigMap string           `json:"vectorAgentConfigMap"`
	Authentication       []Authentication `json:"authentication,omitempty"`
}

// Authentication references a platform AuthenticationClass; OIDC client identity
// is application-specific and does not belong to the shared provider definition.
type Authentication struct {
	AuthenticationClass string     `json:"authenticationClass"`
	OIDC                OIDCClient `json:"oidc"`
}
type OIDCClient struct {
	ClientCredentialsSecret string   `json:"clientCredentialsSecret"`
	ExtraScopes             []string `json:"extraScopes,omitempty"`
}

// SecretVolume declares a native Secret or a SecretClass CSI directory. Exactly
// one source is set. Secret bytes never enter generated ConfigMaps or status.
type SecretVolume struct {
	SecretName           string
	SecretClass          string
	Format               string
	Scope                []string
	KerberosServiceNames []string
}

// ListenerVolume declares the producer of a listener result. A class creates a
// per-Pod Listener; Name references an existing one. The CSI driver owns its files.
type ListenerVolume struct {
	Class string
	Name  string
}

type ListenerAddress struct {
	Pod       string           `json:"pod"`
	Directory string           `json:"directory"`
	Address   string           `json:"address"`
	Ports     map[string]int32 `json:"ports"`
}

// PlatformObservation is obtained after workload application. Pending does not
// withhold the Pod that produces this result. Addresses are observed, not guessed.
type PlatformObservation struct {
	Phase      string            `json:"phase"`
	Diagnostic FactDiagnostic    `json:"diagnostic"`
	Listeners  []ListenerAddress `json:"listeners,omitempty"`
}

const (
	SecretScopePod  = "pod"
	SecretScopeNode = "node"
)
