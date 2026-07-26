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
	"k8s.io/utils/ptr"
)

// DefaultSecurityContext returns the container security context every framework-injected sidecar
// gets unless the caller supplies its own through SidecarConfig.SecurityContext.
//
// Without it the injected containers ship with a nil security context while the product's own
// container is hardened, so a namespace enforcing the restricted Pod Security Standard rejects
// the whole workload — the sidecars are the reason the pod cannot be admitted.
//
// The four fields here are exactly the container-level requirements of the restricted profile:
//
//   - AllowPrivilegeEscalation: false
//   - Capabilities: drop ALL
//   - RunAsNonRoot: true
//   - SeccompProfile: RuntimeDefault
//
// It deliberately does NOT set RunAsUser, and so differs from
// security.PodSecurityBuilder.BuildDefaultSecurityContext, which pins the kubedoop uid 1001.
// Identity is a pod-level property here: BuildDefaultPodSecurityContext already sets RunAsUser,
// RunAsGroup and FSGroup on the pod, and a container inherits them. Pinning a uid at the
// container level would additionally impose it on sidecars running a THIRD-PARTY image with its
// own USER (oauth2-proxy ships as uid 2000), which the framework has no basis to override.
// RunAsNonRoot and SeccompProfile are restated rather than left to inheritance so the container
// stays compliant even when a product replaces the pod security context wholesale through
// podOverrides.
//
// ReadOnlyRootFilesystem is not set either: it is not part of the restricted profile, and a
// container that writes anywhere outside its mounts — a JVM dropping hsperfdata into /tmp, for
// one — would fail to start. A provider that has established its container never writes sets it
// on the returned value:
//
//	sc := sidecar.DefaultSecurityContext()
//	sc.ReadOnlyRootFilesystem = ptr.To(true)
//
// The returned value is freshly allocated on every call, so mutating it cannot affect any other
// container.
func DefaultSecurityContext() *corev1.SecurityContext {
	return &corev1.SecurityContext{
		RunAsNonRoot:             ptr.To(true),
		AllowPrivilegeEscalation: ptr.To(false),
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
}
