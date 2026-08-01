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

// DefaultStorageCapacity is the capacity the framework uses for a data PVC whose role and role
// group both leave it unset. It is applied at CONSUMPTION time (StorageResource.GetCapacity), not
// as a `+kubebuilder:default`: a CRD default is stamped in by the API server as soon as the
// enclosing object exists, which would make "the group did not set a capacity" indistinguishable
// from "the group asked for 10Gi" and silently override the role's value. See the package
// AGENTS.md rule — defaulted fields are pointers, not bare values.
const DefaultStorageCapacity = "10Gi"

type ResourcesSpec struct {
	// +kubebuilder:validation:Optional
	CPU *CPUResource `json:"cpu,omitempty"`

	// +kubebuilder:validation:Optional
	Memory *MemoryResource `json:"memory,omitempty"`

	// +kubebuilder:validation:Optional
	Storage *StorageResource `json:"storage,omitempty"`
}

// CPUResource bounds the container's CPU. Both fields are pointers so that "unset" is
// representable: a bare resource.Quantity is a struct, which `omitempty` cannot omit and whose
// MarshalJSON renders the zero value as "0", so a Go-constructed spec would transmit an explicit
// zero and a role group would silently erase the role's value.
type CPUResource struct {
	// +kubebuilder:validation:Optional
	Max *resource.Quantity `json:"max,omitempty"`

	// +kubebuilder:validation:Optional
	Min *resource.Quantity `json:"min,omitempty"`
}

// MemoryResource bounds the container's memory. Limit is a pointer for the same reason as
// CPUResource's fields.
type MemoryResource struct {
	// +kubebuilder:validation:Optional
	Limit *resource.Quantity `json:"limit,omitempty"`
}

// StorageResource describes the role group's data PVC.
type StorageResource struct {
	// Capacity of the data volume. Unset means DefaultStorageCapacity.
	//
	// It carries no `+kubebuilder:default`: the enclosing storage block exists as soon as a user
	// overrides ANY leaf of it (a storageClass, say), and a CRD default would then be stamped into
	// that block and win the role/roleGroup merge — turning a one-line storageClass override into
	// a silent downgrade of the role's capacity, baked into a StatefulSet volumeClaimTemplate that
	// Kubernetes will not let the operator change afterwards.
	//
	// +kubebuilder:validation:Optional
	Capacity *resource.Quantity `json:"capacity,omitempty"`

	// StorageClass names the StorageClass for the PVC. Unset means "use the cluster default", which
	// is what an absent `storageClassName` asks Kubernetes for.
	//
	// It is a POINTER because the empty string is not a synonym for unset here — Kubernetes reads
	// `storageClassName: ""` as "no class at all", i.e. bind a pre-provisioned PV and do no dynamic
	// provisioning. With a plain string a role group could never express that over a role that names
	// a class, because "" was how the merge spelled "inherit". This is the same reason
	// StorageResource.Capacity, CPUResource.Min/Max, MemoryResource.Limit and
	// RoleGroupConfigSpec.GracefulShutdownTimeout are pointers, and it carries the same rule: no
	// `+kubebuilder:default`, or structural defaulting would fill it as soon as the enclosing
	// object exists and the role's value could never win.
	//
	// +kubebuilder:validation:Optional
	StorageClass *string `json:"storageClass,omitempty"`
}

// GetCapacity returns the configured capacity, or DefaultStorageCapacity when unset.
func (s *StorageResource) GetCapacity() resource.Quantity {
	if s == nil || s.Capacity == nil {
		return resource.MustParse(DefaultStorageCapacity)
	}
	return *s.Capacity
}
