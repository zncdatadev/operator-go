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

package builder

import corev1 "k8s.io/api/core/v1"

// VolumeProvisioner is implemented by components that contribute a set of pod Volumes together
// with the matching container VolumeMounts — for example the security SecretProvisioner and the
// listener ListenerProvisioner. It is the single abstraction the StatefulSet builder (and, by
// extension, the reconciler's role group handler) uses to inject any such provisioner uniformly,
// without depending on its concrete type. New provisioners satisfy it simply by exposing the two
// methods below.
type VolumeProvisioner interface {
	// Volumes returns the volumes to add to the pod spec.
	Volumes() []corev1.Volume
	// VolumeMounts returns the volume mounts to add to the workload's containers.
	VolumeMounts() []corev1.VolumeMount
}

// AddVolumeProvisioner injects a VolumeProvisioner's volumes and volume mounts into the builder.
// It is the shared implementation behind the provisioners' AutoInject helpers, so secret, listener
// and any future volume provisioner are wired in exactly the same way.
func (b *StatefulSetBuilder) AddVolumeProvisioner(p VolumeProvisioner) *StatefulSetBuilder {
	for _, v := range p.Volumes() {
		b.AddVolume(v)
	}
	for _, m := range p.VolumeMounts() {
		b.AddVolumeMount(m)
	}
	return b
}
