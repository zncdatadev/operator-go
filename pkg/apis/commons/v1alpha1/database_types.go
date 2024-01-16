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
	"github.com/zncdata-labs/operator-go/pkg/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DatabaseSpec defines the desired connection info of Database
type DatabaseSpec struct {
	//+kubebuilder:validation:Required
	Name string `json:"name,omitempty"`
	//+kubebuilder:validation:Required
	Reference string `json:"reference,omitempty"`
	//+kubebuilder:validation:Required
	Credential *DatabaseCredentialSpec `json:"credential,omitempty"`
}

// DatabaseCredentialSpec include:
// Username and Password or ExistSecret.
// ExistSecret include Username and Password ,it is encrypted by base64.
type DatabaseCredentialSpec struct {
	ExistSecret string `json:"existingSecret,omitempty"`
	Username    string `json:"username,omitempty"`
	Password    string `json:"password,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Database is the Schema for the databases API
type Database struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseSpec  `json:"spec,omitempty"`
	Status status.Status `json:"status,omitempty"`
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
	Provider *DatabaseProvider `json:"provider,omitempty"`
	Default  bool              `json:"default,omitempty"`
}

// DatabaseConnectionCredentialSpec include ExistSecret, it is encrypted by base64.
type DatabaseConnectionCredentialSpec struct {
	ExistSecret string `json:"existingSecret,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// DatabaseConnection is the Schema for the databaseconnections API
type DatabaseConnection struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseConnectionSpec `json:"spec,omitempty"`
	Status status.Status          `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DatabaseConnectionList contains a list of DatabaseConnection
type DatabaseConnectionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatabaseConnection `json:"items"`
}

// DatabaseProvider defines all types database provider of DatabaseConnection
type DatabaseProvider struct {
	// +kubebuilder:validation:Optional
	Mysq *MysqlProvider `json:"mysql,omitempty"`
	// +kubebuilder:validation:Optional
	Postgres *PostgresProvider `json:"postgres,omitempty"`
	// +kubebuilder:validation:Optional
	Redis *RedisProvider `json:"redis,omitempty"`
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
	Credential *DatabaseConnectionCredentialSpec `json:"credential,omitempty"`
}

func init() {
	SchemeBuilder.Register(&Database{}, &DatabaseList{}, &DatabaseConnection{}, &DatabaseConnectionList{})
}
