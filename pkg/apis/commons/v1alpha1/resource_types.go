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
	"k8s.io/apimachinery/pkg/api/resource"
)

type ResourcesSpec struct {
	// +kubebuilder:validation:Optional
	CPU *CPUResource `json:"cpu,omitempty"`

	// +kubebuilder:validation:Optional
	Memory *MemoryResource `json:"memory,omitempty"`

	// +kubebuilder:validation:Optional
	Storage *StorageResource `json:"storage,omitempty"`
}

type StorageResourceSpec struct {
	Data *StorageResource `json:"data"`
}

type CPUResource struct {
	// +kubebuilder:validation:Optional
	Max resource.Quantity `json:"max,omitempty"`

	// +kubebuilder:validation:Optional
	Min resource.Quantity `json:"min,omitempty"`
}

type MemoryResource struct {
	// +kubebuilder:validation:Optional
	Limit resource.Quantity `json:"limit,omitempty"`
}

type StorageResource struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="10Gi"
	Capacity resource.Quantity `json:"capacity,omitempty"`

	// +kubebuilder:validation:Optional
	StorageClass string `json:"storageClass,omitempty"`
}
