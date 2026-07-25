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

package common

import (
	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ClusterInterface is the cluster-level contract a product CR must satisfy to be driven by the
// SDK.
//
// The embedded client.Object carries everything the framework needs from the object itself —
// name, namespace, UID, labels, annotations, generation, resourceVersion, GVK — and, more
// importantly, makes the CR usable directly wherever controller-runtime expects an object
// (client.Get, Status().Update, SetControllerReference, event recording). A CR that embeds
// metav1.TypeMeta and metav1.ObjectMeta and is registered with a scheme already satisfies it,
// so none of those accessors are product-written code.
//
// The two remaining methods are the whole SDK-specific surface: they project the product's own
// spec and status onto the generic shapes the framework reconciles against. GetStatus returns a
// pointer into the CR — the framework mutates the generic status through it, which is why no
// setter exists and why a product's own status fields survive a reconcile cycle untouched.
type ClusterInterface interface {
	client.Object

	// GetSpec returns the cluster spec projected onto the framework's generic shape. A CR that
	// keeps its roles in typed fields (spec.coordinators, spec.workers) assembles the Roles map
	// here rather than carrying a redundant spec.roles in its CRD.
	GetSpec() *v1alpha1.GenericClusterSpec

	// GetStatus returns a pointer to the generic status embedded in the CR. Writes through the
	// returned pointer must be visible on the CR, because that is how the framework records
	// conditions, observedGeneration and role group state.
	GetStatus() *v1alpha1.GenericClusterStatus
}

// ClusterResource binds a product CR to its own concrete type. It exists because a type
// parameter cannot be allocated with new(T) when T is a pointer type, so the reconciler
// materialises the empty object it reads into by copying a prototype; going through
// runtime.Object (client.Object's DeepCopyObject) would hand back an interface and force the
// runtime assertion this SDK does not allow.
//
// DeepCopy is the method controller-gen already generates for every root API type, so a product
// satisfies this constraint without writing anything.
//
// This is a constraint, not a value type: hold a CR as ClusterInterface, parameterise over one
// as ClusterResource[CR].
type ClusterResource[T ClusterInterface] interface {
	ClusterInterface

	// DeepCopy returns a deep copy of the receiver, typed as the concrete CR.
	DeepCopy() T
}
