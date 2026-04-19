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

// ServiceRegistration declares a Kubernetes Service need mapped from a ListenerClass.
type ServiceRegistration struct {
	name          string
	listenerClass ListenerClass
	ports         []corev1.ServicePort
	headless      bool
}

// NewService creates a ServiceRegistration with the given name and listener class.
func NewService(name string, class ListenerClass) *ServiceRegistration {
	return &ServiceRegistration{
		name:          name,
		listenerClass: class,
	}
}

// WithPorts sets the service ports.
func (r *ServiceRegistration) WithPorts(ports ...corev1.ServicePort) *ServiceRegistration {
	r.ports = ports
	return r
}

// WithHeadless enables generation of an additional headless service (ClusterIP: None)
// alongside the regular service. The headless service name has a "-headless" suffix.
func (r *ServiceRegistration) WithHeadless(headless bool) *ServiceRegistration {
	r.headless = headless
	return r
}

// buildService creates the Service from this registration.
func (r *ServiceRegistration) buildService(namespace string) *corev1.Service {
	serviceType := corev1.ServiceTypeClusterIP
	switch r.listenerClass {
	case ListenerClassExternalStable, ListenerClassExternalUnstable:
		serviceType = corev1.ServiceTypeLoadBalancer
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.name,
			Namespace: namespace,
			Annotations: map[string]string{
				ListenerClassAnnotation: string(r.listenerClass),
			},
		},
		Spec: corev1.ServiceSpec{
			Type:  serviceType,
			Ports: r.ports,
		},
	}
}

// buildHeadlessService creates a headless variant (ClusterIP: None) without ListenerClassAnnotation.
func (r *ServiceRegistration) buildHeadlessService(namespace string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.name + "-headless",
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: corev1.ClusterIPNone,
			Ports:     r.ports,
		},
	}
}

// VolumeRegistration declares a CSI listener volume need.
type VolumeRegistration struct {
	volumeName    string
	listenerClass ListenerClass
	scope         *ListenerScope
	listenerName  string
}

// NewVolume creates a VolumeRegistration with the given volume name and listener class.
func NewVolume(name string, class ListenerClass) *VolumeRegistration {
	return &VolumeRegistration{
		volumeName:    name,
		listenerClass: class,
	}
}

// WithScope sets the listener scope annotation on the PVC template.
func (r *VolumeRegistration) WithScope(scope ListenerScope) *VolumeRegistration {
	r.scope = &scope
	return r
}

// WithListenerName sets the listener name annotation on the PVC template.
func (r *VolumeRegistration) WithListenerName(name string) *VolumeRegistration {
	r.listenerName = name
	return r
}

// buildAnnotations constructs the PVC template annotations for this registration.
func (r *VolumeRegistration) buildAnnotations() map[string]string {
	annotations := map[string]string{
		ListenerClassAnnotation: string(r.listenerClass),
	}

	if r.scope != nil {
		annotations[ListenerScopeAnnotation] = string(*r.scope)
	}

	if r.listenerName != "" {
		annotations[AnnotationListenerName] = r.listenerName
	}

	return annotations
}

// buildVolume constructs the EphemeralVolumeSource for this registration.
func (r *VolumeRegistration) buildVolume() corev1.Volume {
	return corev1.Volume{
		Name: r.volumeName,
		VolumeSource: corev1.VolumeSource{
			Ephemeral: &corev1.EphemeralVolumeSource{
				VolumeClaimTemplate: &corev1.PersistentVolumeClaimTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: r.buildAnnotations(),
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						StorageClassName: func() *string {
							v := ListenerStorageClass
							return &v
						}(),
						VolumeMode: func() *corev1.PersistentVolumeMode {
							v := corev1.PersistentVolumeFilesystem
							return &v
						}(),
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse("1Mi"),
							},
						},
					},
				},
			},
		},
	}
}
