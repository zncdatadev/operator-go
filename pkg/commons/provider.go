package v1alpha1

// DatabaseProvider defines all types database provider of DatabaseConnection
type DatabaseProvider struct {
	// +kubebuilder:validation:Optional
	MysqlProvider *MysqlProvider `json:"mysql,omitempty"`
	// +kubebuilder:validation:Optional
	PostgresProvider *PostgresProvider `json:"postgres,omitempty"`
	// +kubebuilder:validation:Optional
	RedisProvider *RedisProvider `json:"redis,omitempty"`
}

// MysqlProvider defines the desired connection info of Mysql
type MysqlProvider struct {
	// +kubebuilder:default=mysql
	// +kubebuilder:validation:Required
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

// PostgresProvider defines the desired connection info of Postgres
type PostgresProvider struct {
	// +kubebuilder:default=postgres
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

// RedisProvider defines the desired connection info of Redis
type RedisProvider struct {
	// +kubebuilder:validation:Required
	Host string `json:"host,omitempty"`
	// +kubebuilder:validation:Required
	Port string `json:"port,omitempty"`
	// +kubebuilder:validation:Optional
	Password string `json:"password,omitempty"`
}
