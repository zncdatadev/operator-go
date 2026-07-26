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

package sidecar

import (
	corev1 "k8s.io/api/core/v1"
)

// SidecarProbes overrides the probes a provider sets by default. Its zero value keeps every
// provider default, so a SidecarConfig that says nothing about probes gets the framework policy.
//
// # Why the framework has a probe policy at all
//
// Every provider in this package injects its container as a NATIVE SIDECAR — an init container
// with restartPolicy Always (see SidecarRestartPolicy). That makes the choice of probe TYPE a
// correctness question rather than a matter of taste, because Kubernetes gives the three probes
// three different blast radii on such a container:
//
//   - readinessProbe: "If a readinessProbe is specified for this init container, its result will
//     be used to determine the ready state of the Pod." A failing readiness probe on an
//     observability sidecar therefore removes the pod from EVERY Service it belongs to — the log
//     shipper or the metrics exporter takes the product offline. No provider here sets one.
//   - livenessProbe: the kubelet restarts the container, subject to its restartPolicy (Always for
//     a sidecar, so it comes back). Pod readiness and Service membership are untouched. This is
//     the probe that answers "is this sidecar still doing its job", and it is what the providers
//     set by default.
//   - startupProbe: the kubelet starts the next init container only once this one has "started",
//     which for a container with a startupProbe means once that probe has succeeded. It is
//     therefore the only way to make a sidecar's readiness a PRECONDITION of the main container
//     starting — used by the oauth2-proxy provider, which terminates client traffic.
//
// # Overriding
//
// A non-nil probe replaces the provider's default wholesale (never merged — a half-merged probe
// is how a container ends up with a handler nobody chose and timings from two different
// intentions). The matching Disable flag removes the probe instead, and wins if both are set.
//
// Setting Readiness is legal and occasionally right — a product whose sidecar really is in the
// request path may want the pod gated on it — but re-read the readinessProbe bullet above first:
// on a native sidecar that field is not "this container is unready", it is "this POD is unready".
type SidecarProbes struct {
	// Startup replaces the provider's default startup probe.
	Startup *corev1.Probe

	// Liveness replaces the provider's default liveness probe.
	Liveness *corev1.Probe

	// Readiness sets a readiness probe. No provider sets one by default; on a native sidecar this
	// gates the whole Pod's ready state, and with it the pod's membership of every Service.
	Readiness *corev1.Probe

	// DisableStartup removes the provider's default startup probe.
	DisableStartup bool

	// DisableLiveness removes the provider's default liveness probe, leaving the sidecar with no
	// automatic recovery from a wedged-but-running process.
	DisableLiveness bool

	// DisableReadiness removes a readiness probe. Only meaningful for a provider that sets one.
	DisableReadiness bool
}

// ApplyProbes folds a product's overrides onto the probes a provider has already set on the
// container. Probes are deep-copied in, so the returned pod spec shares no state with the
// SidecarConfig the caller keeps — the same rule the resource builders follow.
func ApplyProbes(container *corev1.Container, probes SidecarProbes) {
	if container == nil {
		return
	}
	container.StartupProbe = resolveProbe(container.StartupProbe, probes.Startup, probes.DisableStartup)
	container.LivenessProbe = resolveProbe(container.LivenessProbe, probes.Liveness, probes.DisableLiveness)
	container.ReadinessProbe = resolveProbe(container.ReadinessProbe, probes.Readiness, probes.DisableReadiness)
}

// resolveProbe picks between the provider default and the override. Disable wins over a stated
// probe: the two together are contradictory, and dropping the probe is the outcome that cannot
// silently attach a handler the caller also asked to remove.
func resolveProbe(providerDefault, override *corev1.Probe, disable bool) *corev1.Probe {
	if disable {
		return nil
	}
	if override != nil {
		return override.DeepCopy()
	}
	return providerDefault
}
