package v1alphav1

import (
	"github.com/zncdata-labs/operator-go/pkg/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// S3ConnectionSpec defines the desired credential of S3Connection
type S3ConnectionSpec struct {
	// +kubebuilder:validation:Required
	S3Credential *S3Credential `json:"credential,omitempty"`
}

// S3Credential include  AccessKey and SecretKey or ExistingSecret. ExistingSecret include AccessKey and SecretKey ,it is encrypted by base64.
type S3Credential struct {
	// +kubebuilder:validation:Optional
	ExistSecret string `json:"existSecret,omitempty"`
	// +kubebuilder:validation:Optional
	AccessKey string `json:"accessKey,omitempty"`
	// +kubebuilder:validation:Optional
	SecretKey string `json:"secretKey,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// S3Connection is the Schema for the s3connections API
type S3Connection struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   S3ConnectionSpec     `json:"spec,omitempty"`
	Status status.ZncdataStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// S3ConnectionList contains a list of S3Connection
type S3ConnectionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []S3Connection `json:"items"`
}
