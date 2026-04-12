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
	"fmt"
	"reflect"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	authv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/authentication/v1alpha1"
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

// isPresent returns true if val is a non-nil interface and, when it holds a
// pointer/slice/map/chan/func value, that value is also non-nil.
// This handles the common Go pitfall where a typed nil pointer stored in an
// interface{}  is not equal to a plain nil interface.
func isPresent(val any) bool {
	if val == nil {
		return false
	}
	v := reflect.ValueOf(val)
	switch v.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Slice, reflect.Map, reflect.Chan, reflect.Func:
		return !v.IsNil()
	default:
		return true
	}
}

// ValidateOneOf validates that exactly one of the provided named values is non-nil.
// Each entry in fields is a (name, value) pair where value is nil when the field is absent.
// If zero or more than one field is set, an error is added to the returned ErrorList.
//
// Example:
//
//	errs := webhook.ValidateOneOf(fldPath, map[string]any{
//	    "oidc":     provider.OIDC,
//	    "tls":      provider.TLS,
//	    "static":   provider.Static,
//	    "ldap":     provider.LDAP,
//	    "kerberos": provider.Kerberos,
//	})
func ValidateOneOf(fldPath *field.Path, fields map[string]any) field.ErrorList {
	var errs field.ErrorList

	var set []string
	for name, val := range fields {
		if isPresent(val) {
			set = append(set, name)
		}
	}

	// Collect all valid field names for the error message (sorted for deterministic output).
	all := make([]string, 0, len(fields))
	for name := range fields {
		all = append(all, name)
	}
	sort.Strings(all)
	sort.Strings(set)

	switch len(set) {
	case 0:
		errs = append(errs, field.Required(fldPath,
			fmt.Sprintf("exactly one of [%s] must be set", strings.Join(all, ", "))))
	case 1:
		// valid — exactly one is set
	default:
		errs = append(errs, field.Invalid(fldPath, strings.Join(set, ", "),
			fmt.Sprintf("exactly one of [%s] must be set, but multiple are configured: [%s]",
				strings.Join(all, ", "), strings.Join(set, ", "))))
	}

	return errs
}

// ValidateAuthenticationProvider validates that exactly one provider is configured
// in the AuthenticationProvider spec (oneOf constraint).
// It follows the same field.ErrorList pattern used by ValidateGenericClusterSpec.
//
// Example:
//
//	func (v *MyValidator) ValidateCreate(ctx context.Context, cr *MyCluster) (admission.Warnings, error) {
//	    fldErrs := webhook.ValidateAuthenticationProvider(
//	        cr.Spec.AuthenticationProvider,
//	        field.NewPath("spec").Child("provider"),
//	    )
//	    if len(fldErrs) > 0 {
//	        return nil, apierrors.NewInvalid(cr.GroupVersionKind().GroupKind(), cr.Name, fldErrs)
//	    }
//	    return nil, nil
//	}
func ValidateAuthenticationProvider(provider *authv1alpha1.AuthenticationProvider, fldPath *field.Path) field.ErrorList {
	if provider == nil {
		return nil
	}

	return ValidateOneOf(fldPath, map[string]any{
		"oidc":     provider.OIDC,
		"tls":      provider.TLS,
		"static":   provider.Static,
		"ldap":     provider.LDAP,
		"kerberos": provider.Kerberos,
	})
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
