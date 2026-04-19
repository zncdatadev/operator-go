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
	"fmt"
	"path"

	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/constant"
	corev1 "k8s.io/api/core/v1"
)

// ListenerProvisioner manages listener service and volume declarations.
// It provides a unified interface for registering service and CSI volume needs,
// then materializing them as Kubernetes resources.
//
// Primary integration methods:
//   - Services(), Volumes(), VolumeMounts() for direct resource construction
//   - Path() and MustPath() for config generation (mount path lookup)
//
// Convenience method:
//   - AutoInject() for operators using StatefulSetBuilder
type ListenerProvisioner struct {
	serviceRegs   []*ServiceRegistration
	volumeRegs    []*VolumeRegistration
	serviceNames  map[string]struct{}
	volumeNames   map[string]struct{}
	mountBasePath string
}

// NewProvisioner creates a provisioner with the default mount base path.
func NewProvisioner() *ListenerProvisioner {
	return &ListenerProvisioner{
		mountBasePath: constant.KubedoopListenerDir,
		serviceNames:  make(map[string]struct{}),
		volumeNames:   make(map[string]struct{}),
	}
}

// WithMountBasePath overrides the default mount base path.
func (p *ListenerProvisioner) WithMountBasePath(basePath string) *ListenerProvisioner {
	if basePath != "" {
		p.mountBasePath = path.Clean(basePath)
	}
	return p
}

// RegisterService adds service declarations to the provisioner.
// Panics if a service with the same name is already registered, or if a service
// name would collide with an auto-generated headless name (name + "-headless").
func (p *ListenerProvisioner) RegisterService(registrations ...*ServiceRegistration) *ListenerProvisioner {
	for _, reg := range registrations {
		headlessName := reg.name + "-headless"
		if _, exists := p.serviceNames[reg.name]; exists {
			panic(fmt.Sprintf("listener service %q is already registered", reg.name))
		}
		if _, exists := p.serviceNames[headlessName]; exists {
			panic(fmt.Sprintf("listener service %q collides with existing headless service name", reg.name))
		}
		p.serviceNames[reg.name] = struct{}{}
		if reg.headless {
			p.serviceNames[headlessName] = struct{}{}
		}
		p.serviceRegs = append(p.serviceRegs, reg)
	}
	return p
}

// RegisterVolume adds CSI volume declarations to the provisioner.
// Panics if a volume with the same name is already registered.
func (p *ListenerProvisioner) RegisterVolume(registrations ...*VolumeRegistration) *ListenerProvisioner {
	for _, reg := range registrations {
		if _, exists := p.volumeNames[reg.volumeName]; exists {
			panic(fmt.Sprintf("listener volume %q is already registered", reg.volumeName))
		}
		p.volumeNames[reg.volumeName] = struct{}{}
		p.volumeRegs = append(p.volumeRegs, reg)
	}
	return p
}

// Services returns all registered services for the given namespace.
// Panics if namespace is empty.
// For each ServiceRegistration with headless enabled, returns both the regular
// service and a headless variant (ClusterIP: None, name suffixed with "-headless").
func (p *ListenerProvisioner) Services(namespace string) []*corev1.Service {
	if namespace == "" {
		panic("namespace is required for Services()")
	}

	var services []*corev1.Service
	for _, reg := range p.serviceRegs {
		services = append(services, reg.buildService(namespace))
		if reg.headless {
			services = append(services, reg.buildHeadlessService(namespace))
		}
	}
	return services
}

// Volumes returns all registered CSI volumes for manual StatefulSet injection.
func (p *ListenerProvisioner) Volumes() []corev1.Volume {
	volumes := make([]corev1.Volume, 0, len(p.volumeRegs))
	for _, reg := range p.volumeRegs {
		volumes = append(volumes, reg.buildVolume())
	}
	return volumes
}

// VolumeMounts returns all registered volume mounts for manual container injection.
// All mounts have ReadOnly set to true.
func (p *ListenerProvisioner) VolumeMounts() []corev1.VolumeMount {
	mounts := make([]corev1.VolumeMount, 0, len(p.volumeRegs))
	for _, reg := range p.volumeRegs {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      reg.volumeName,
			MountPath: p.mountPath(reg.volumeName),
			ReadOnly:  true,
		})
	}
	return mounts
}

// AutoInject adds all registered volumes and mounts to a StatefulSetBuilder.
// This is a convenience method for operators that use StatefulSetBuilder.
// Services are NOT injected -- they are separate K8s resources applied by the reconciler.
func (p *ListenerProvisioner) AutoInject(stsBuilder *builder.StatefulSetBuilder) {
	for _, vol := range p.Volumes() {
		stsBuilder.AddVolume(vol)
	}
	for _, mount := range p.VolumeMounts() {
		stsBuilder.AddVolumeMount(mount)
	}
}

// Path returns the mount path for a registered volume (no trailing slash).
// Returns an error if the volume name is not registered.
func (p *ListenerProvisioner) Path(volumeName string) (string, error) {
	if _, exists := p.volumeNames[volumeName]; !exists {
		return "", fmt.Errorf("listener volume %q not registered", volumeName)
	}
	return p.mountPath(volumeName), nil
}

// MustPath returns the mount path for a registered volume (no trailing slash).
// Panics if the volume name is not registered.
func (p *ListenerProvisioner) MustPath(volumeName string) string {
	result, err := p.Path(volumeName)
	if err != nil {
		panic(err)
	}
	return result
}

// mountPath returns the full mount path for a volume name (no trailing slash).
func (p *ListenerProvisioner) mountPath(volumeName string) string {
	return path.Join(p.mountBasePath, volumeName)
}
