/*
Copyright 2024 ZNCDataDev.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
)

// DatabaseDriver represents the type of database.
// +kubebuilder:validation:Enum=mysql;postgres;mariadb
type DatabaseDriver string

const (
	// DatabaseDriverMySQL represents MySQL database driver.
	DatabaseDriverMySQL DatabaseDriver = "mysql"
	// DatabaseDriverPostgres represents PostgreSQL database driver.
	DatabaseDriverPostgres DatabaseDriver = "postgres"
	// DatabaseDriverMariaDB represents MariaDB database driver.
	DatabaseDriverMariaDB DatabaseDriver = "mariadb"
)

// DatabaseConnectionSpec defines the database connection configuration.
type DatabaseConnectionSpec struct {
	// Host is the database server hostname.
	// +kubebuilder:validation:Required
	Host string `json:"host,omitempty"`

	// Port is the database server port.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int `json:"port,omitempty"`

	// Driver is the database driver type (mysql, postgres, mariadb).
	// +kubebuilder:validation:Required
	Driver DatabaseDriver `json:"driver,omitempty"`

	// Database is the database name.
	// +kubebuilder:validation:Optional
	Database string `json:"database,omitempty"`

	// Credentials references the secret containing authentication credentials.
	// +kubebuilder:validation:Required
	Credentials *commonsv1alpha1.Credentials `json:"credentials,omitempty"`

	// TLS configuration for secure connections.
	// +kubebuilder:validation:Optional
	TLS *TLS `json:"tls,omitempty"`
}

// TLS defines the TLS configuration for database connections.
type TLS struct {
	// +kubebuilder:validation:Optional
	Verification *commonsv1alpha1.TLSVerificationSpec `json:"verification,omitempty"`
}

// DatabaseConnectionStatus defines the observed state of DatabaseConnection.
type DatabaseConnectionStatus struct {
	// +kubebuilder:validation:Optional
	Conditions []metav1.Condition `json:"condition,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// DatabaseConnection is the Schema for the databaseconnections API
type DatabaseConnection struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseConnectionSpec   `json:"spec,omitempty"`
	Status DatabaseConnectionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// DatabaseConnectionList contains a list of DatabaseConnection.
type DatabaseConnectionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatabaseConnection `json:"items"`
}

// GetHost returns the database host.
func (d *DatabaseConnectionSpec) GetHost() string {
	return d.Host
}

// GetPort returns the database port.
func (d *DatabaseConnectionSpec) GetPort() int {
	return d.Port
}

// GetDriver returns the database driver.
func (d *DatabaseConnectionSpec) GetDriver() DatabaseDriver {
	return d.Driver
}

// GetDatabase returns the database name.
func (d *DatabaseConnectionSpec) GetDatabase() string {
	return d.Database
}

// GetCredentials returns the credentials specification.
func (d *DatabaseConnectionSpec) GetCredentials() *commonsv1alpha1.Credentials {
	return d.Credentials
}

// GetTLS returns the TLS configuration.
func (d *DatabaseConnectionSpec) GetTLS() *TLS {
	return d.TLS
}

// HasTLS returns true if TLS is configured.
func (d *DatabaseConnectionSpec) HasTLS() bool {
	return d.TLS != nil
}

// IsMySQL returns true if the driver is MySQL.
func (d *DatabaseConnectionSpec) IsMySQL() bool {
	return d.Driver == DatabaseDriverMySQL
}

// IsPostgres returns true if the driver is PostgreSQL.
func (d *DatabaseConnectionSpec) IsPostgres() bool {
	return d.Driver == DatabaseDriverPostgres
}

// IsMariaDB returns true if the driver is MariaDB.
func (d *DatabaseConnectionSpec) IsMariaDB() bool {
	return d.Driver == DatabaseDriverMariaDB
}

// Validate checks if the DatabaseConnectionSpec is valid.
func (d *DatabaseConnectionSpec) Validate() error {
	// Validation logic can be extended as needed
	return nil
}

func init() {
	SchemeBuilder.Register(&DatabaseConnection{}, &DatabaseConnectionList{})
}
