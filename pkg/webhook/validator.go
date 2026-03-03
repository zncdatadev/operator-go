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
)

// ValidateFieldLength validates string length
func ValidateFieldLength(value, fieldName string, minLength, maxLength int) error {
	if len(value) < minLength || len(value) > maxLength {
		return common.ConfigValidationError(fieldName, fmt.Errorf("%s: length must be between %d and %d characters", fieldName, minLength, maxLength))
	}
	return nil
}

// ValidateNonEmptyMap validates map is not empty
func ValidateNonEmptyMap(m map[string]string, fieldName string) error {
	if len(m) == 0 {
		return common.ConfigValidationError(fieldName, fmt.Errorf("%s: map cannot be empty", fieldName))
	}
	return nil
}

// ProductValidator defines the interface for validating CRs.
// Products implement this interface to provide custom validation logic
// that runs during the ValidatingWebhook phase.
//
// Usage:
//
//	type HdfsClusterValidator struct{}
//
//	func (v *HdfsClusterValidator) Validate(ctx context.Context, cr *HdfsCluster) error {
//	    if cr.Spec.Replicas < 1 {
//	        return webhook.NewValidationError("spec.replicas", "must be at least 1")
//	    }
//	    return nil
//	}
type ProductValidator[CR any] interface {
	// Validate validates the CR and returns an error if invalid.
	// This is called by the ValidatingWebhook before persistence.
	// Return a ValidationError or ValidationErrors for field-specific errors.
	Validate(ctx context.Context, cr CR) error
}

// NoOpValidator is a validator that does nothing.
// Useful for testing or when no validation is needed.
type NoOpValidator[CR any] struct{}

// Validate does nothing and returns nil.
func (v *NoOpValidator[CR]) Validate(_ context.Context, _ CR) error {
	return nil
}

// NewNoOpValidator creates a new NoOpValidator.
func NewNoOpValidator[CR any]() *NoOpValidator[CR] {
	return &NoOpValidator[CR]{}
}

// FuncValidator wraps a function as a ProductValidator.
type FuncValidator[CR any] struct {
	fn func(ctx context.Context, cr CR) error
}

// NewFuncValidator creates a new FuncValidator.
func NewFuncValidator[CR any](fn func(ctx context.Context, cr CR) error) *FuncValidator[CR] {
	return &FuncValidator[CR]{fn: fn}
}

// Validate calls the wrapped function.
func (v *FuncValidator[CR]) Validate(ctx context.Context, cr CR) error {
	if v.fn == nil {
		return nil
	}
	return v.fn(ctx, cr)
}
