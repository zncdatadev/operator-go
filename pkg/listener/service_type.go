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
)

// ServiceTypeFor maps a listener class to the Service type that realises it.
//
//	cluster-internal   -> ClusterIP      (reachable inside the cluster only)
//	external-unstable  -> NodePort       (outside, at an address tied to the node the pod is on)
//	external-stable    -> LoadBalancer   (outside, at an address that survives rescheduling)
//
// An unrecognised or empty class maps to ClusterIP: the exposure a user did not ask for should be
// the narrowest one, not an accidental public address.
//
// This restores the mapping v0.12.6 shipped as builder.ListenerClass2ServiceType, which v0.13
// dropped while keeping the class constants. The concept survived and the code that applied it did
// not, so every product exposing a UI re-derived the same three-way switch AFTER the framework had
// built the Service — and the two operators that had already migrated disagreed about
// external-stable. A shared mapping is exactly what prevents that.
//
// It lives here rather than in pkg/builder, where the issue proposed it: pkg/listener already
// imports pkg/builder, so the reverse direction does not compile. Pair it with the Service
// builder's existing WithServiceType rather than adding a second entry point:
//
//	builder.NewServiceBuilder(name, ns).
//	    WithServiceType(builder.ServiceType(listener.ServiceTypeFor(class)))
//
// The conversion is there because pkg/builder declares its own ServiceType enum parallel to
// corev1's. This returns the Kubernetes type, which is the one a product ends up comparing against
// on a live Service.
func ServiceTypeFor(class ListenerClass) corev1.ServiceType {
	switch class {
	case ListenerClassExternalUnstable:
		return corev1.ServiceTypeNodePort
	case ListenerClassExternalStable:
		return corev1.ServiceTypeLoadBalancer
	case ListenerClassClusterInternal:
		return corev1.ServiceTypeClusterIP
	default:
		return corev1.ServiceTypeClusterIP
	}
}
