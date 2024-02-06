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

const S3BucketFinalizer = "s3bucket.finalizers.stack.zncdata.net"

// S3ConnectionSpec defines the desired credential of S3Connection
type S3ConnectionSpec struct {

	// +kubebuilder:validation:Required
	Credential *S3Credential `json:"credential,omitempty"`

	// +kubebuilder:validation:Required
	Endpoint string `json:"endpoint,omitempty"`

	// +kubebuilder:validation:Optional
	Region string `json:"region,omitempty"`

	// +kubebuilder:validation:Optional
	SSL bool `json:"ssl,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default:=false
	PathStyle bool `json:"pathStyle,omitempty"`
}

// S3Credential include `ACCESS_KEY` and `SECRET_KEY` or ExistingSecret.
type S3Credential struct {

	// ExistingSecret include `ACCESS_KEY` and `SECRET_KEY` ,it is encrypted by base64.
	// If ExistingSecret is not empty, `ACCESS_KEY` and `SECRET_KEY` will be ignored.
	// +kubebuilder:validation:Optional
	ExistSecret string `json:"existSecret,omitempty"`

	// +kubebuilder:validation:Optional
	AccessKey string `json:"accessKey,omitempty"`

	// +kubebuilder:validation:Optional
	SecretKey string `json:"secretKey,omitempty"`
}

type S3ConnectionStatus struct {

	// +kubebuilder:validation:Optional
	Conditions []metav1.Condition `json:"condition,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// S3Connection is the Schema for the s3connections API
type S3Connection struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   S3ConnectionSpec   `json:"spec,omitempty"`
	Status S3ConnectionStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

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

	// +kubebuilder:validation:Required
	Reference string `json:"reference,omitempty"`

	// +kubebuilder:validation:Optional
	Credential *S3Credential `json:"credential,omitempty"`
}

type S3BucketStatus struct {

	// +kubebuilder:validation:Optional
	Conditions []metav1.Condition `json:"condition,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// S3Bucket is the Schema for the s3buckets API
type S3Bucket struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   S3BucketSpec   `json:"spec,omitempty"`
	Status S3BucketStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// S3BucketList contains a list of S3Bucket
type S3BucketList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []S3Bucket `json:"items"`
}

func init() {
	SchemeBuilder.Register(&S3Bucket{}, &S3BucketList{}, &S3Connection{}, &S3ConnectionList{})
}
