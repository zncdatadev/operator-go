/*
Copyright 2024 zncdatadev.

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

// S3ConnectionSpec defines the desired credential of S3Connection
type S3ConnectionSpec struct {

	// Provides access credentials for S3Connection through SecretClass. SecretClass only needs to include:
	//  - ACCESS_KEY
	//  - SECRET_KEY
	// +kubebuilder:validation:Required
	Credentials *commonsv1alpha1.Credentials `json:"credentials"`

	// +kubebuilder:validation:Required
	Host string `json:"host,omitempty"`

	// +kubebuilder:validation:Optional
	Port int `json:"port,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default:=false
	PathStyle bool `json:"pathStyle,omitempty"`

	// +kubebuilder:validation:Optional
	Tls *Tls `json:"tls,omitempty"`
}

type Tls struct {
	// +kubebuilder:validation:Optional
	Verification *commonsv1alpha1.TLSVerificationSpec `json:"verification,omitempty"`
}

type S3ConnectionStatus struct {
	// +kubebuilder:validation:Optional
	Conditions []metav1.Condition `json:"condition,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// S3Connection is the Schema for the s3connections API
type S3Connection struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   S3ConnectionSpec   `json:"spec,omitempty"`
	Status S3ConnectionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// S3ConnectionList contains a list of S3Connection
type S3ConnectionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []S3Connection `json:"items"`
}

// S3BucketSpec defines the desired fields of S3Bucket
type S3BucketSpec struct {

	// +kubebuilder:validation:Required
	BucketName string `json:"bucketName,omitempty"`

	// +kubebuilder:validation:Optional
	Connection *S3BucketConnectionSpec `json:"connection,omitempty"`
}

type S3BucketConnectionSpec struct {
	// +kubebuilder:validation:Optional
	Reference string `json:"reference,omitempty"`

	// +kubebuilder:validation:Optional
	Inline *S3ConnectionSpec `json:"inline,omitempty"`
}

type S3BucketStatus struct {
	// +kubebuilder:validation:Optional
	Conditions []metav1.Condition `json:"condition,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// S3Bucket is the Schema for the s3buckets API
type S3Bucket struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   S3BucketSpec   `json:"spec,omitempty"`
	Status S3BucketStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// S3BucketList contains a list of S3Bucket
type S3BucketList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []S3Bucket `json:"items"`
}

func init() {
	SchemeBuilder.Register(&S3Bucket{}, &S3BucketList{}, &S3Connection{}, &S3ConnectionList{})
}
