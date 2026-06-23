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
	"github.com/zncdatadev/operator-go/pkg/sidecar"
	corev1 "k8s.io/api/core/v1"
)

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
})
