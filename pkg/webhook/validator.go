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
	"context"
	"fmt"

	"github.com/zncdatadev/operator-go/pkg/common"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// ProductValidator defines the interface for validating a product CR on admission.
// It mirrors controller-runtime's admission.Validator[T] interface, so implementations
// can be passed directly to ctrl.NewWebhookManagedBy(...).WithValidator(...).
//
// The recommended pattern to share logic between Create and Update is to extract a
// private validate method:
//
// type HdfsClusterValidator struct{}
//
//	func (v *HdfsClusterValidator) ValidateCreate(ctx context.Context, cr *HdfsCluster) (admission.Warnings, error) {
//	   return nil, v.validate(ctx, cr)
//	}
//
//	func (v *HdfsClusterValidator) ValidateUpdate(ctx context.Context, _, cr *HdfsCluster) (admission.Warnings, error) {
//	   return nil, v.validate(ctx, cr)
//	}
//
//	func (v *HdfsClusterValidator) ValidateDelete(ctx context.Context, cr *HdfsCluster) (admission.Warnings, error) {
//	   return nil, nil
//	}
//
//	func (v *HdfsClusterValidator) validate(ctx context.Context, cr *HdfsCluster) error {
//	   fieldErrs := webhook.ValidateGenericClusterSpec(&cr.Spec.GenericClusterSpec, field.NewPath("spec"))
//	   if len(fieldErrs) > 0 {
//	       return apierrors.NewInvalid(cr.GroupVersionKind().GroupKind(), cr.Name, fieldErrs)
//	   }
//	   return nil
//	}
type ProductValidator[CR any] interface {
	// ValidateCreate validates the CR upon creation.
	ValidateCreate(ctx context.Context, cr CR) (admission.Warnings, error)

	// ValidateUpdate validates the CR upon update.
	// oldCR is the previous version; use it to enforce immutable field constraints.
	ValidateUpdate(ctx context.Context, oldCR, newCR CR) (admission.Warnings, error)

	// ValidateDelete validates the CR upon deletion.
	ValidateDelete(ctx context.Context, cr CR) (admission.Warnings, error)
}

// NoOpValidator is a validator that does nothing.
// Useful for testing or when no validation is needed.
type NoOpValidator[CR any] struct{}

// ValidateCreate does nothing and returns nil.
func (v *NoOpValidator[CR]) ValidateCreate(_ context.Context, _ CR) (admission.Warnings, error) {
	return nil, nil
}

// ValidateUpdate does nothing and returns nil.
func (v *NoOpValidator[CR]) ValidateUpdate(_ context.Context, _, _ CR) (admission.Warnings, error) {
	return nil, nil
}

// ValidateDelete does nothing and returns nil.
func (v *NoOpValidator[CR]) ValidateDelete(_ context.Context, _ CR) (admission.Warnings, error) {
	return nil, nil
}

// NewNoOpValidator creates a new NoOpValidator.
func NewNoOpValidator[CR any]() *NoOpValidator[CR] {
	return &NoOpValidator[CR]{}
}

// FuncValidator wraps a validate function as a ProductValidator.
// ValidateCreate and ValidateUpdate both call fn; ValidateDelete is a no-op.
type FuncValidator[CR any] struct {
	fn func(ctx context.Context, cr CR) error
}

// NewFuncValidator creates a new FuncValidator.
func NewFuncValidator[CR any](fn func(ctx context.Context, cr CR) error) *FuncValidator[CR] {
	return &FuncValidator[CR]{fn: fn}
}

// ValidateCreate calls the wrapped function.
func (v *FuncValidator[CR]) ValidateCreate(ctx context.Context, cr CR) (admission.Warnings, error) {
	if v.fn == nil {
		return nil, nil
	}
	return nil, v.fn(ctx, cr)
}

// ValidateUpdate calls the wrapped function on the new CR.
func (v *FuncValidator[CR]) ValidateUpdate(ctx context.Context, _, newCR CR) (admission.Warnings, error) {
	if v.fn == nil {
		return nil, nil
	}
	return nil, v.fn(ctx, newCR)
}

// ValidateDelete is a no-op.
func (v *FuncValidator[CR]) ValidateDelete(_ context.Context, _ CR) (admission.Warnings, error) {
	return nil, nil
}

// ValidateFieldLength validates that a string field length is within [minLength, maxLength].
// Returns an error using the common.ConfigValidationError format.
func ValidateFieldLength(value, fieldName string, minLength, maxLength int) error {
	if len(value) < minLength || len(value) > maxLength {
		return common.ConfigValidationError(fieldName, fmt.Errorf("%s: length must be between %d and %d characters", fieldName, minLength, maxLength))
	}
	return nil
}

// ValidateNonEmptyMap validates that a map is not empty.
func ValidateNonEmptyMap(m map[string]string, fieldName string) error {
	if len(m) == 0 {
		return common.ConfigValidationError(fieldName, fmt.Errorf("%s: map cannot be empty", fieldName))
	}
	return nil
}
