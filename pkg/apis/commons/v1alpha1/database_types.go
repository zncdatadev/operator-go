/*
Copyright 2024 zncdata-labs.

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
)

// DatabaseSpec defines the desired connection info of Database
type DatabaseSpec struct {

	//+kubebuilder:validation:Required
	DatabaseName string `json:"databaseName,omitempty"`

	// Name of DatabaseConnection CR to use for this database.
	//+kubebuilder:validation:Required
	Reference string `json:"reference,omitempty"`

	// Credential is the credential for the database.
	// It contains Username and Password, or ExistSecret.
	//+kubebuilder:validation:Required
	Credential *CredentialSpec `json:"credential,omitempty"`
}

type DatabaseStatus struct {
	// +kubebuilder:validation:Optional
	Conditions []metav1.Condition `json:"condition,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Database is the Schema for the databases API
type Database struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseSpec   `json:"spec,omitempty"`
	Status DatabaseStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DatabaseList contains a list of Database
type DatabaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Database `json:"items"`
}

// DatabaseConnectionSpec defines the desired state of DatabaseConnection
type DatabaseConnectionSpec struct {
	// +kubebuilder:validation:Required
	Provider *DatabaseConnectionProvider `json:"provider,omitempty"`

	// +kubebuilder:validation:Optional
	Default bool `json:"default,omitempty"`
}

// CredentialSpec include: Username and Password or ExistSecret.
type CredentialSpec struct {
	// ExistSecret is a Secret name, created by user.
	// It includes Username and Password, it is encrypted by base64.
	// If ExistSecret is not empty, Username and Password will be ignored.
	// +kubebuilder:validation:Optional
	ExistSecret string `json:"existingSecret,omitempty"`

	// Username is the username for the database.
	// +kubebuilder:validation:Optional
	Username string `json:"username,omitempty"`

	// Password is the password for the database.
	// +kubebuilder:validation:Optional
	Password string `json:"password,omitempty"`
}

type DatabaseConnectionStatus struct {
	// +kubebuilder:validation:Optional
	Conditions []metav1.Condition `json:"condition,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// DatabaseConnection is the Schema for the databaseconnections API
type DatabaseConnection struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseConnectionSpec   `json:"spec,omitempty"`
	Status DatabaseConnectionStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DatabaseConnectionList contains a list of DatabaseConnection
type DatabaseConnectionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatabaseConnection `json:"items"`
}

// DatabaseConnectionProvider defines the enum provider for DataConnection.
// You can choose one of mysql, postgres, redis, and provider is required.
type DatabaseConnectionProvider struct {
	// +kubebuilder:validation:Optional
	Mysq *MysqlProvider `json:"mysql,omitempty"`
	// +kubebuilder:validation:Optional
	Postgres *PostgresProvider `json:"postgres,omitempty"`
	// +kubebuilder:validation:Optional
	Redis *RedisProvider `json:"redis,omitempty"`
}

// MysqlProvider defines the desired connection info of Mysql
type MysqlProvider struct {
	// If you want to use mysql8+ , you should set driver to com.mysql.cj.jdbc.Driver,
	// otherwise you should set driver to com.mysql.jdbc.Driver.
	// +kubebuilder:default=com.mysql.cj.jdbc.Driver
	// +kubebuilder:validation:Required
	Driver string `json:"driver,omitempty"`
	// +kubebuilder:validation:Required
	Host string `json:"host,omitempty"`
	// +kubebuilder:validation:Required
	Port int `json:"port,omitempty"`
	// +kubebuilder:validation:Required
	SSL bool `json:"ssl,omitempty"`
	// +kubebuilder:validation:Required
	Credential *CredentialSpec `json:"credential,omitempty"`
}

// PostgresProvider defines the desired connection info of Postgres
type PostgresProvider struct {
	// +kubebuilder:default=org.postgresql.Driver
	Driver string `json:"driver,omitempty"`
	// +kubebuilder:validation:Required
	Host string `json:"host,omitempty"`
	// +kubebuilder:validation:Required
	Port int `json:"port,omitempty"`
	// +kubebuilder:validation:Required
	SSL bool `json:"ssl,omitempty"`
	// +kubebuilder:validation:Required
	Credential *CredentialSpec `json:"credential,omitempty"`
}

// RedisProvider defines the desired connection info of Redis
type RedisProvider struct {
	// +kubebuilder:validation:Required
	Host string `json:"host,omitempty"`
	// +kubebuilder:validation:Required
	Port string `json:"port,omitempty"`
	// +kubebuilder:validation:Optional
	Credential *CredentialSpec `json:"credential,omitempty"`
}

func init() {
	SchemeBuilder.Register(&Database{}, &DatabaseList{}, &DatabaseConnection{}, &DatabaseConnectionList{})
}
