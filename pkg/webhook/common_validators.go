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
	"k8s.io/apimachinery/pkg/util/validation/field"

	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
)

// ValidateGenericClusterSpec validates the common fields of a GenericClusterSpec.
// It returns a field.ErrorList; append the result to your own list for composite validation.
//
// Validated fields:
//   - spec.image.pullPolicy — must be one of Always, IfNotPresent, Never
//   - spec.image.productVersion — must not be empty when Custom is not set
//   - spec.image.kubedoopVersion — must not be empty when Custom is not set
//
// Example:
//
//	func (v *MyValidator) ValidateCreate(ctx, cr *MyCluster) (Warnings, error) {
//	    fldErrs := webhook.ValidateGenericClusterSpec(&cr.Spec.GenericClusterSpec, field.NewPath("spec"))
//	    if len(fldErrs) > 0 {
//	        gvk := cr.GroupVersionKind()
//	        return nil, apierrors.NewInvalid(gvk.GroupKind(), cr.Name, fldErrs)
//	    }
//	    return nil, nil
//	}
func ValidateGenericClusterSpec(spec *commonsv1alpha1.GenericClusterSpec, fldPath *field.Path) field.ErrorList {
	var errs field.ErrorList

	if spec == nil {
		return errs
	}

	if spec.Image != nil {
		errs = append(errs, validateImageSpec(spec.Image, fldPath.Child("image"))...)
	}

	return errs
}

// validateImageSpec validates an ImageSpec field.
func validateImageSpec(image *commonsv1alpha1.ImageSpec, fldPath *field.Path) field.ErrorList {
	var errs field.ErrorList

	// If Custom is set, no further validation needed
	if image.Custom != "" {
		return errs
	}

	// Validate Repo
	if image.Repo == "" {
		errs = append(errs, field.Required(fldPath.Child("repo"),
			"repo is required when custom image is not set"))
	}

	// Validate ProductVersion
	if image.ProductVersion == "" {
		errs = append(errs, field.Required(fldPath.Child("productVersion"),
			"productVersion is required when custom image is not set"))
	}

	// Validate KubedoopVersion
	if image.KubedoopVersion == "" {
		errs = append(errs, field.Required(fldPath.Child("kubedoopVersion"),
			"kubedoopVersion is required when custom image is not set"))
	}

	// Validate PullPolicy
	if image.PullPolicy != "" {
		switch image.PullPolicy {
		case corev1.PullAlways, corev1.PullNever, corev1.PullIfNotPresent:
			// valid
		default:
			errs = append(errs, field.NotSupported(fldPath.Child("pullPolicy"), image.PullPolicy,
				[]string{string(corev1.PullAlways), string(corev1.PullNever), string(corev1.PullIfNotPresent)}))
		}
	}

	return errs
}
