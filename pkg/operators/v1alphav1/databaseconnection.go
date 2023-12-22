package v1alphav1

import (
	"github.com/zncdata-labs/operator-go/pkg/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
	Status status.ZncdataStatus   `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DatabaseConnectionList contains a list of DatabaseConnection
type DatabaseConnectionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatabaseConnection `json:"items"`
}
