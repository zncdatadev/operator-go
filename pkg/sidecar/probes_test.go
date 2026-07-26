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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/zncdatadev/operator-go/pkg/sidecar"
)

// providerDefaults stands in for the probes a provider has already set on the container by the
// time ApplyProbes runs.
func providerDefaults() *corev1.Container {
	handler := corev1.ProbeHandler{
		HTTPGet: &corev1.HTTPGetAction{Path: "/metrics", Port: intstr.FromInt(9598)},
	}
	return &corev1.Container{
		Name:          "sidecar",
		StartupProbe:  &corev1.Probe{ProbeHandler: handler, PeriodSeconds: 2},
		LivenessProbe: &corev1.Probe{ProbeHandler: handler, PeriodSeconds: 20},
	}
}

var _ = Describe("ApplyProbes", func() {
	It("keeps every provider default for a zero SidecarProbes", func() {
		// The common case: a SidecarConfig that says nothing about probes must get the framework
		// policy, not an unprobed container.
		container := providerDefaults()
		sidecar.ApplyProbes(container, sidecar.SidecarProbes{})

		Expect(container.StartupProbe).NotTo(BeNil())
		Expect(container.StartupProbe.PeriodSeconds).To(BeEquivalentTo(2))
		Expect(container.LivenessProbe).NotTo(BeNil())
		Expect(container.LivenessProbe.PeriodSeconds).To(BeEquivalentTo(20))
		Expect(container.ReadinessProbe).To(BeNil())
	})

	It("replaces a stated probe wholesale instead of merging it", func() {
		// Probe handlers are a Kubernetes one-of. Merging an exec override onto an httpGet default
		// would produce a probe carrying two handlers, which the API server rejects — and would
		// reject the whole workload, not just the probe.
		container := providerDefaults()
		sidecar.ApplyProbes(container, sidecar.SidecarProbes{
			Liveness: &corev1.Probe{
				ProbeHandler:  corev1.ProbeHandler{Exec: &corev1.ExecAction{Command: []string{"true"}}},
				PeriodSeconds: 7,
			},
		})

		Expect(container.LivenessProbe.Exec).NotTo(BeNil())
		Expect(container.LivenessProbe.HTTPGet).To(BeNil())
		Expect(container.LivenessProbe.PeriodSeconds).To(BeEquivalentTo(7))
		// Untouched probes keep their defaults.
		Expect(container.StartupProbe.HTTPGet).NotTo(BeNil())
	})

	It("deep-copies the override so the pod spec does not alias the caller's config", func() {
		// Same rule the resource builders follow: a built object shares no state with what built
		// it, otherwise mutating a reused SidecarConfig silently rewrites an already-built pod.
		override := &corev1.Probe{
			ProbeHandler:  corev1.ProbeHandler{Exec: &corev1.ExecAction{Command: []string{"true"}}},
			PeriodSeconds: 7,
		}
		container := providerDefaults()
		sidecar.ApplyProbes(container, sidecar.SidecarProbes{Liveness: override})

		override.PeriodSeconds = 99
		override.Exec.Command = []string{"false"}

		Expect(container.LivenessProbe.PeriodSeconds).To(BeEquivalentTo(7))
		Expect(container.LivenessProbe.Exec.Command).To(Equal([]string{"true"}))
	})

	It("removes a probe when disabled", func() {
		container := providerDefaults()
		sidecar.ApplyProbes(container, sidecar.SidecarProbes{
			DisableStartup:  true,
			DisableLiveness: true,
		})

		Expect(container.StartupProbe).To(BeNil())
		Expect(container.LivenessProbe).To(BeNil())
	})

	It("lets Disable win over a stated probe", func() {
		// The two together are contradictory. Dropping is the outcome that cannot silently attach
		// a probe the caller also asked to remove.
		container := providerDefaults()
		sidecar.ApplyProbes(container, sidecar.SidecarProbes{
			Liveness:        &corev1.Probe{ProbeHandler: corev1.ProbeHandler{Exec: &corev1.ExecAction{Command: []string{"true"}}}},
			DisableLiveness: true,
		})

		Expect(container.LivenessProbe).To(BeNil())
	})

	It("adds a readiness probe only when the caller asks for one", func() {
		container := providerDefaults()
		sidecar.ApplyProbes(container, sidecar.SidecarProbes{
			Readiness: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{Exec: &corev1.ExecAction{Command: []string{"true"}}}},
		})

		Expect(container.ReadinessProbe).NotTo(BeNil())
	})

	It("tolerates a nil container", func() {
		Expect(func() { sidecar.ApplyProbes(nil, sidecar.SidecarProbes{DisableLiveness: true}) }).NotTo(Panic())
	})
})
