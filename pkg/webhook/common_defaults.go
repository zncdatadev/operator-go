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

package webhook

import (
	corev1 "k8s.io/api/core/v1"

	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
)

// DefaultGenericClusterSpec applies common defaults to a GenericClusterSpec.
// It is intended to be called from a product operator's DefaulterAdapter.Default().
//
//   - If spec.Image is nil and defaultImage is provided, spec.Image is initialized
//     by copying the defaultImage values.
//   - If spec.Image.PullPolicy is empty, it defaults to IfNotPresent.
//
// Example (in product operator webhook):
//
//	func (d *HdfsClusterDefaulter) Default(ctx context.Context, cr *HdfsCluster) error {
//	    webhook.DefaultGenericClusterSpec(&cr.Spec.GenericClusterSpec, &defaultImage)
//	    return nil
//	}
func DefaultGenericClusterSpec(
	spec *commonsv1alpha1.GenericClusterSpec,
	defaultImage *commonsv1alpha1.ImageSpec,
) {
	if spec == nil {
		return
	}

	// Apply default image if none is set
	if spec.Image == nil && defaultImage != nil {
		copied := *defaultImage
		spec.Image = &copied
	}

	// Apply default pull policy
	if spec.Image != nil && spec.Image.PullPolicy == "" {
		spec.Image.PullPolicy = corev1.PullIfNotPresent
	}
}
