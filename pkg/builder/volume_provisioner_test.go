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

package builder_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/listener"
	"github.com/zncdatadev/operator-go/pkg/security"
)

// The two first-party provisioners must satisfy the unified VolumeProvisioner interface, so a
// single handler hook (WithVolumeProvisioners) can inject either. These compile-time assertions
// guard that contract.
var (
	_ builder.VolumeProvisioner = (*security.SecretProvisioner)(nil)
	_ builder.VolumeProvisioner = (*listener.ListenerProvisioner)(nil)
)

// fakeVolumeProvisioner is a minimal VolumeProvisioner for testing the injection helper.
type fakeVolumeProvisioner struct {
	volumes []corev1.Volume
	mounts  []corev1.VolumeMount
}

func (f *fakeVolumeProvisioner) Volumes() []corev1.Volume           { return f.volumes }
func (f *fakeVolumeProvisioner) VolumeMounts() []corev1.VolumeMount { return f.mounts }

var _ = Describe("AddVolumeProvisioner", func() {
	It("injects the provisioner's volumes and mounts into the builder", func() {
		stsBuilder := builder.NewStatefulSetBuilder("test-sts", "test-ns")
		p := &fakeVolumeProvisioner{
			volumes: []corev1.Volume{{Name: "listener"}, {Name: "tls"}},
			mounts: []corev1.VolumeMount{
				{Name: "listener", MountPath: "/kubedoop/listener"},
				{Name: "tls", MountPath: "/kubedoop/tls"},
			},
		}

		result := stsBuilder.AddVolumeProvisioner(p)

		Expect(result).To(BeIdenticalTo(stsBuilder), "should return the builder for chaining")
		Expect(stsBuilder.Volumes).To(HaveLen(2))
		Expect(stsBuilder.Volumes[0].Name).To(Equal("listener"))
		Expect(stsBuilder.Volumes[1].Name).To(Equal("tls"))
		Expect(stsBuilder.VolumeMounts).To(HaveLen(2))
		Expect(stsBuilder.VolumeMounts[0].MountPath).To(Equal("/kubedoop/listener"))
		Expect(stsBuilder.VolumeMounts[1].MountPath).To(Equal("/kubedoop/tls"))
	})

	It("appends across multiple provisioners in order", func() {
		stsBuilder := builder.NewStatefulSetBuilder("test-sts", "test-ns")
		stsBuilder.
			AddVolumeProvisioner(&fakeVolumeProvisioner{volumes: []corev1.Volume{{Name: "a"}}}).
			AddVolumeProvisioner(&fakeVolumeProvisioner{volumes: []corev1.Volume{{Name: "b"}}})

		Expect(stsBuilder.Volumes).To(HaveLen(2))
		Expect(stsBuilder.Volumes[0].Name).To(Equal("a"))
		Expect(stsBuilder.Volumes[1].Name).To(Equal("b"))
	})
})
