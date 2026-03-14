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

package reconciler

import (
	stderrors "errors"
	"fmt"
	"time"
)

// ConfigError represents a configuration-related error.
type ConfigError struct {
	Field   string
	Message string
}

// Error implements error.
func (e *ConfigError) Error() string {
	return fmt.Sprintf("config error in field %q: %s", e.Field, e.Message)
}

// NewConfigError creates a new ConfigError.
func NewConfigError(field, message string) *ConfigError {
	return &ConfigError{
		Field:   field,
		Message: message,
	}
}

// ReconcileError represents a reconciliation-phase error.
type ReconcileError struct {
	Phase   string
	Message string
	Cause   error
}

// Error implements error.
func (e *ReconcileError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("reconcile error in phase %q: %s (cause: %v)", e.Phase, e.Message, e.Cause)
	}
	return fmt.Sprintf("reconcile error in phase %q: %s", e.Phase, e.Message)
}

// Unwrap returns the underlying error.
func (e *ReconcileError) Unwrap() error {
	return e.Cause
}

// NewReconcileError creates a new ReconcileError.
func NewReconcileError(phase, message string, cause error) *ReconcileError {
	return &ReconcileError{
		Phase:   phase,
		Message: message,
		Cause:   cause,
	}
}

// ResourceBuildError represents an error during resource building.
type ResourceBuildError struct {
	ResourceType string
	RoleName     string
	GroupName    string
	Message      string
	Cause        error
}

// Error implements error.
func (e *ResourceBuildError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("failed to build %s for role %q group %q: %s (cause: %v)",
			e.ResourceType, e.RoleName, e.GroupName, e.Message, e.Cause)
	}
	return fmt.Sprintf("failed to build %s for role %q group %q: %s",
		e.ResourceType, e.RoleName, e.GroupName, e.Message)
}

// Unwrap returns the underlying error.
func (e *ResourceBuildError) Unwrap() error {
	return e.Cause
}

// NewResourceBuildError creates a new ResourceBuildError.
func NewResourceBuildError(resourceType, roleName, groupName, message string, cause error) *ResourceBuildError {
	return &ResourceBuildError{
		ResourceType: resourceType,
		RoleName:     roleName,
		GroupName:    groupName,
		Message:      message,
		Cause:        cause,
	}
}

// ResourceApplyError represents an error during resource application.
type ResourceApplyError struct {
	ResourceType string
	ResourceName string
	Namespace    string
	Message      string
	Cause        error
}

// Error implements error.
func (e *ResourceApplyError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("failed to apply %s %s/%s: %s (cause: %v)",
			e.ResourceType, e.Namespace, e.ResourceName, e.Message, e.Cause)
	}
	return fmt.Sprintf("failed to apply %s %s/%s: %s",
		e.ResourceType, e.Namespace, e.ResourceName, e.Message)
}

// Unwrap returns the underlying error.
func (e *ResourceApplyError) Unwrap() error {
	return e.Cause
}

// NewResourceApplyError creates a new ResourceApplyError.
func NewResourceApplyError(resourceType, namespace, resourceName, message string, cause error) *ResourceApplyError {
	return &ResourceApplyError{
		ResourceType: resourceType,
		ResourceName: resourceName,
		Namespace:    namespace,
		Message:      message,
		Cause:        cause,
	}
}

// IsReconcileError checks if an error is a ReconcileError.
func IsReconcileError(err error) bool {
	_, ok := err.(*ReconcileError)
	return ok
}

// IsConfigError checks if an error is a ConfigError.
func IsConfigError(err error) bool {
	_, ok := err.(*ConfigError)
	return ok
}

// IsResourceBuildError checks if an error is a ResourceBuildError.
func IsResourceBuildError(err error) bool {
	_, ok := err.(*ResourceBuildError)
	return ok
}

// IsResourceApplyError checks if an error is a ResourceApplyError.
func IsResourceApplyError(err error) bool {
	_, ok := err.(*ResourceApplyError)
	return ok
}

// RateLimitError represents a 429 Too Many Requests error from the Kubernetes API.
// The caller should back off for RetryAfter before retrying.
type RateLimitError struct {
	RetryAfter time.Duration
	Cause      error
}

// Error implements error.
func (e *RateLimitError) Error() string {
	return fmt.Sprintf("rate limited by Kubernetes API, retry after %s: %v", e.RetryAfter, e.Cause)
}

// Unwrap returns the underlying error.
func (e *RateLimitError) Unwrap() error {
	return e.Cause
}

// NewRateLimitError creates a new RateLimitError.
func NewRateLimitError(retryAfter time.Duration, cause error) *RateLimitError {
	return &RateLimitError{
		RetryAfter: retryAfter,
		Cause:      cause,
	}
}

// IsRateLimitError checks if an error is or wraps a RateLimitError.
func IsRateLimitError(err error) bool {
	var rateLimitErr *RateLimitError
	return stderrors.As(err, &rateLimitErr)
}
