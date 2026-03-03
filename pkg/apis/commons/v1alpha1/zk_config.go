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

// ZKConfig defines Zookeeper connection configuration.
// Used by products that require Zookeeper for coordination (e.g., HBase, Kafka, Pulsar).
type ZKConfig struct {
	// ConnectionString is the Zookeeper connection string.
	// Format: host1:port1,host2:port2,.../path
	// Example: zk-0.zk:2181,zk-1.zk:2181,zk-2.zk:2181/hbase
	// +kubebuilder:validation:Required
	ConnectionString string `json:"connectionString"`

	// SecretClass references the secret-operator SecretClass for credentials.
	// If specified, the SecretClass will provide authentication credentials.
	// +kubebuilder:validation:Optional
	SecretClass string `json:"secretClass,omitempty"`
}

// GetConnectionString returns the Zookeeper connection string.
func (z *ZKConfig) GetConnectionString() string {
	return z.ConnectionString
}

// GetSecretClass returns the SecretClass name, or empty string if not set.
func (z *ZKConfig) GetSecretClass() string {
	return z.SecretClass
}

// HasSecretClass returns true if a SecretClass is configured.
func (z *ZKConfig) HasSecretClass() bool {
	return z.SecretClass != ""
}
