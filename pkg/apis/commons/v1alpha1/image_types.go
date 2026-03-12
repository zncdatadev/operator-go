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
	corev1 "k8s.io/api/core/v1"
)

// ImageSpec defines the container image configuration for a product workload.
// If Custom is set, it takes precedence and the image is used as-is.
// Otherwise, the image reference is constructed as:
//
//	{Repo}/{productName}:{ProductVersion}-kubedoop{KubedoopVersion}
//
// Product operators are expected to set default values for all fields via their webhook.
type ImageSpec struct {
	// Custom is a fully qualified image reference (e.g. "my-registry.com/ns/product:3.4.1").
	// When set, Repo, ProductVersion and KubedoopVersion are ignored.
	// +kubebuilder:validation:Optional
	Custom string `json:"custom,omitempty"`

	// Repo is the image repository (e.g. "quay.io/kubedoop").
	// Used only when Custom is not set.
	// +kubebuilder:validation:Optional
	Repo string `json:"repo,omitempty"`

	// ProductVersion is the version of the product to deploy (e.g. "3.4.1").
	// Used only when Custom is not set.
	// +kubebuilder:validation:Optional
	ProductVersion string `json:"productVersion,omitempty"`

	// KubedoopVersion is the version of the kubedoop operator stack (e.g. "0.2.0").
	// Used only when Custom is not set.
	// +kubebuilder:validation:Optional
	KubedoopVersion string `json:"kubedoopVersion,omitempty"`

	// PullPolicy defines the image pull policy for the container.
	// Defaults to IfNotPresent.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=IfNotPresent
	// +kubebuilder:validation:Enum=Always;Never;IfNotPresent
	PullPolicy corev1.PullPolicy `json:"pullPolicy,omitempty"`
}

// GetImage returns the resolved container image reference for the given product name.
// If Custom is set it is returned directly; otherwise the image is constructed from
// Repo, productName, ProductVersion and KubedoopVersion.
func (i *ImageSpec) GetImage(productName string) string {
	if i.Custom != "" {
		return i.Custom
	}
	return i.Repo + "/" + productName + ":" + i.ProductVersion + "-kubedoop" + i.KubedoopVersion
}

// GetPullPolicy returns the configured pull policy, defaulting to IfNotPresent.
func (i *ImageSpec) GetPullPolicy() corev1.PullPolicy {
	if i.PullPolicy == "" {
		return corev1.PullIfNotPresent
	}
	return i.PullPolicy
}
