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

package sidecar_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/constant"
	"github.com/zncdatadev/operator-go/pkg/sidecar"
	corev1 "k8s.io/api/core/v1"
)

// findVolumeMount returns the named VolumeMount from the container, or nil.
func findVolumeMount(c *corev1.Container, name string) *corev1.VolumeMount {
	for i := range c.VolumeMounts {
		if c.VolumeMounts[i].Name == name {
			return &c.VolumeMounts[i]
		}
	}
	return nil
}

// findVolume returns the named Volume from the pod spec, or nil.
func findVolume(podSpec *corev1.PodSpec, name string) *corev1.Volume {
	for i := range podSpec.Volumes {
		if podSpec.Volumes[i].Name == name {
			return &podSpec.Volumes[i]
		}
	}
	return nil
}

var _ = Describe("VectorSidecarProvider", func() {
	It("always injects Vector as a native sidecar (init container, restartPolicy Always)", func() {
		podSpec := &corev1.PodSpec{Containers: []corev1.Container{{Name: "main"}}}
		provider := sidecar.NewVectorSidecarProvider()

		Expect(provider.Inject(podSpec, &sidecar.SidecarConfig{Enabled: true, Image: "vector:1"})).To(Succeed())

		idx := sidecar.FindInitContainerIndex(podSpec, "vector")
		Expect(idx).To(BeNumerically(">=", 0))
		Expect(podSpec.InitContainers[idx].RestartPolicy).NotTo(BeNil())
		Expect(*podSpec.InitContainers[idx].RestartPolicy).To(Equal(corev1.ContainerRestartPolicyAlways))
		// Never placed as a regular container.
		Expect(sidecar.FindContainer(podSpec, "vector")).To(BeNil())
	})

	It("mounts the shared log volume read-only into the Vector container at KubedoopLogDir", func() {
		podSpec := &corev1.PodSpec{Containers: []corev1.Container{{Name: "main"}}}
		provider := sidecar.NewVectorSidecarProvider()

		Expect(provider.Inject(podSpec, &sidecar.SidecarConfig{Enabled: true, Image: "vector:1"})).To(Succeed())

		vectorContainer := sidecar.FindInitContainer(podSpec, "vector")
		Expect(vectorContainer).NotTo(BeNil())

		logMount := findVolumeMount(vectorContainer, sidecar.VectorLogVolumeName)
		Expect(logMount).NotTo(BeNil(), "Vector container must mount the shared log volume")
		Expect(logMount.MountPath).To(Equal(constant.KubedoopLogDir))
		Expect(logMount.ReadOnly).To(BeTrue(), "Vector must read logs read-only")

		// The default framework-managed log volume is created as an emptyDir.
		logVolume := findVolume(podSpec, sidecar.VectorLogVolumeName)
		Expect(logVolume).NotTo(BeNil())
		Expect(logVolume.EmptyDir).NotTo(BeNil())

		// The main container also mounts the same volume (read-write) at KubedoopLogDir.
		mainMount := findVolumeMount(&podSpec.Containers[0], sidecar.VectorLogVolumeName)
		Expect(mainMount).NotTo(BeNil())
		Expect(mainMount.MountPath).To(Equal(constant.KubedoopLogDir))
		Expect(mainMount.ReadOnly).To(BeFalse())
	})

	It("reuses a product-supplied log volume via WithLogVolume without creating an emptyDir", func() {
		const productLogVolume = "zk-log"
		podSpec := &corev1.PodSpec{
			Containers: []corev1.Container{{
				Name: "main",
				VolumeMounts: []corev1.VolumeMount{
					{Name: productLogVolume, MountPath: constant.KubedoopLogDir},
				},
			}},
			Volumes: []corev1.Volume{{
				Name:         productLogVolume,
				VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
			}},
		}
		provider := sidecar.NewVectorSidecarProvider().WithLogVolume(productLogVolume)

		Expect(provider.Inject(podSpec, &sidecar.SidecarConfig{Enabled: true, Image: "vector:1"})).To(Succeed())

		vectorContainer := sidecar.FindInitContainer(podSpec, "vector")
		Expect(vectorContainer).NotTo(BeNil())

		// Vector reads the product's log volume read-only at KubedoopLogDir.
		logMount := findVolumeMount(vectorContainer, productLogVolume)
		Expect(logMount).NotTo(BeNil(), "Vector must mount the product log volume")
		Expect(logMount.MountPath).To(Equal(constant.KubedoopLogDir))
		Expect(logMount.ReadOnly).To(BeTrue())

		// The framework must NOT create its own default emptyDir log volume.
		Expect(findVolume(podSpec, sidecar.VectorLogVolumeName)).To(BeNil())
		// Exactly one volume with the product name (no duplicate created).
		count := 0
		for _, v := range podSpec.Volumes {
			if v.Name == productLogVolume {
				count++
			}
		}
		Expect(count).To(Equal(1))
	})
})
