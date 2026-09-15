// Package product contains the existing Trino and presence fixture data shapes.
// It deliberately has no dependency on the prototype or generated input API.
package product

type TrinoConfig struct {
	HTTPPort             int32  `json:"httpPort"`
	CatalogConfigMapName string `json:"catalogConfigMapName"`
}

type TrinoClusterConfig struct {
	NodeEnvironment string `json:"nodeEnvironment"`
}

type PresenceConfig struct {
	Label    string             `json:"label"`
	Enabled  bool               `json:"enabled"`
	Args     []string           `json:"args"`
	Backends map[string]Backend `json:"backends"`
}

type PresenceClusterConfig struct {
	Enabled bool              `json:"enabled"`
	Count   int32             `json:"count"`
	Labels  map[string]string `json:"labels"`
	Args    []string          `json:"args"`
}

type Backend struct {
	Host    string `json:"host"`
	Enabled bool   `json:"enabled"`
}
