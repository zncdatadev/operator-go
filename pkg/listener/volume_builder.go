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

package listener

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ListenerClass defines the exposure strategy.
type ListenerClass string

const (
	// ListenerClassClusterInternal creates ClusterIP Service.
	ListenerClassClusterInternal ListenerClass = "cluster-internal"
	// ListenerClassExternalStable creates LoadBalancer with stable IPs.
	ListenerClassExternalStable ListenerClass = "external-stable"
	// ListenerClassExternalUnstable creates LoadBalancer with dynamic IPs.
	ListenerClassExternalUnstable ListenerClass = "external-unstable"
)

// CSI driver constants
const (
	CSIDriverName           = "listeners.stackable.tech"
	ListenerClassAnnotation = "listeners.stackable.tech/class"
	ListenerScopeAnnotation = "listeners.stackable.tech/scope"
)

// ListenerVolumeBuilder builds PVCs for listener-operator CSI integration.
type ListenerVolumeBuilder struct {
	listenerClass ListenerClass
	scope         string
}

// NewListenerVolumeBuilder creates a new ListenerVolumeBuilder.
func NewListenerVolumeBuilder(listenerClass ListenerClass) *ListenerVolumeBuilder {
	return &ListenerVolumeBuilder{
		listenerClass: listenerClass,
	}
}

// WithScope sets the scope (pod, service, node).
func (b *ListenerVolumeBuilder) WithScope(scope string) *ListenerVolumeBuilder {
	b.scope = scope
	return b
}

// BuildPVC creates a PersistentVolumeClaim with listener annotations.
func (b *ListenerVolumeBuilder) BuildPVC(name string) corev1.PersistentVolumeClaim {
	annotations := map[string]string{
		ListenerClassAnnotation: string(b.listenerClass),
	}

	if b.scope != "" {
		annotations[ListenerScopeAnnotation] = b.scope
	}

	return corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Annotations: annotations,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("1Mi"),
				},
			},
		},
	}
}

// BuildVolumeMount creates a VolumeMount for the listener volume.
func (b *ListenerVolumeBuilder) BuildVolumeMount(name, mountPath string) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      name,
		MountPath: mountPath,
		ReadOnly:  true,
	}
}
