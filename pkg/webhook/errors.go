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
	"strings"
)

// ValidationError represents a single field validation failure.
type ValidationError struct {
	// Field is the field path that failed validation (e.g., "spec.replicas").
	Field string `json:"field"`
	// Message describes the validation failure.
	Message string `json:"message"`
	// Value is the invalid value (optional).
	Value interface{} `json:"value,omitempty"`
}

// Error implements error.
func (e *ValidationError) Error() string {
	if e.Value != nil {
		return fmt.Sprintf("%s: %s (got %v)", e.Field, e.Message, e.Value)
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// NewValidationError creates a new ValidationError.
func NewValidationError(field, message string) *ValidationError {
	return &ValidationError{
		Field:   field,
		Message: message,
	}
}

// NewValidationErrorWithValue creates a new ValidationError with a value.
func NewValidationErrorWithValue(field, message string, value interface{}) *ValidationError {
	return &ValidationError{
		Field:   field,
		Message: message,
		Value:   value,
	}
}

// ValidationErrors collects multiple validation errors.
type ValidationErrors []*ValidationError

// Error implements error.
func (errs ValidationErrors) Error() string {
	if len(errs) == 0 {
		return "no validation errors"
	}

	msgs := make([]string, len(errs))
	for i, err := range errs {
		msgs[i] = err.Error()
	}
	return strings.Join(msgs, "; ")
}

// Add adds a new validation error to the collection.
func (errs *ValidationErrors) Add(field, message string) {
	*errs = append(*errs, NewValidationError(field, message))
}

// AddWithValue adds a new validation error with a value to the collection.
func (errs *ValidationErrors) AddWithValue(field, message string, value interface{}) {
	*errs = append(*errs, NewValidationErrorWithValue(field, message, value))
}

// HasErrors returns true if there are any validation errors.
func (errs ValidationErrors) HasErrors() bool {
	return len(errs) > 0
}

// ToError returns nil if no errors, or the ValidationErrors if any exist.
func (errs ValidationErrors) ToError() error {
	if len(errs) == 0 {
		return nil
	}
	return errs
}

// Merge combines multiple ValidationErrors into one.
func Merge(errsList ...ValidationErrors) ValidationErrors {
	totalLen := 0
	for _, errs := range errsList {
		totalLen += len(errs)
	}
	result := make(ValidationErrors, 0, totalLen)
	for _, errs := range errsList {
		result = append(result, errs...)
	}
	return result
}
