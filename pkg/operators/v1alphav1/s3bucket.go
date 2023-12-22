package v1alphav1

import (
	"github.com/zncdata-labs/operator-go/pkg/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const S3BucketFinalizer = "s3bucket.finalizers.stack.zncdata.net"

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// S3BucketSpec defines the desired fields of S3Bucket
type S3BucketSpec struct {

	// +kubebuilder:validation:Required
	Reference string `json:"reference,omitempty"`

	// +kubebuilder:validation:Optional
	Credential *S3BucketCredential `json:"credential,omitempty"`

	// +kubebuilder:validation:Required
	Name string `json:"name,omitempty"`
}

// S3BucketCredential defines the desired secret of S3Bucket
type S3BucketCredential struct {
	// +kubebuilder:validation:Optional
	ExistSecret string `json:"existSecret,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// S3Bucket is the Schema for the s3buckets API
type S3Bucket struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   S3BucketSpec         `json:"spec,omitempty"`
	Status status.ZncdataStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// S3BucketList contains a list of S3Bucket
type S3BucketList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []S3Bucket `json:"items"`
}
