package v1alphav1

// DatabaseProvider defines all types database provider of DatabaseConnection
type DatabaseProvider struct {
	MysqlProvider    *MysqlProvider    `json:"mysql,omitempty"`
	PostgresProvider *PostgresProvider `json:"postgres,omitempty"`
	RedisProvider    *RedisProvider    `json:"redis,omitempty"`
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
