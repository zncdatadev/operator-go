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

	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/common"
)

// ConfigError represents a configuration-related error.
type ConfigError struct {
	Field   string
	Message string
	// Cause is the underlying failure, when there is one. It was the only error type in this
	// package without one, so a ConfigError built from another error had to flatten it to a
	// string — and callers lost errors.Is/errors.As, which is exactly how a product tells an
	// apierrors.IsNotFound lookup failure from a real one.
	Cause error
}

// Error implements error.
func (e *ConfigError) Error() string {
	if e.Cause != nil && e.Message == "" {
		return fmt.Sprintf("config error in field %q: %s", e.Field, e.Cause.Error())
	}
	return fmt.Sprintf("config error in field %q: %s", e.Field, e.Message)
}

// Unwrap exposes the underlying failure to errors.Is and errors.As.
func (e *ConfigError) Unwrap() error { return e.Cause }

// NewConfigError creates a new ConfigError.
func NewConfigError(field, message string) *ConfigError {
	return &ConfigError{
		Field:   field,
		Message: message,
	}
}

// WrapConfigError creates a ConfigError that carries cause, so errors.Is and errors.As still see
// through it. Prefer this over NewConfigError(field, err.Error()), which flattens the chain.
func WrapConfigError(field string, cause error) *ConfigError {
	return &ConfigError{
		Field: field,
		Cause: cause,
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

// ValidationError represents a failed pre-apply validation of a role group's declared
// dependencies (e.g. a sidecar provider referencing a ConfigMap that does not exist). It is
// distinct from a build or apply failure: nothing was written, the desired state is simply not
// satisfiable yet.
type ValidationError struct {
	Subject   string
	RoleName  string
	GroupName string
	Cause     error
}

// Error implements error.
func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s validation failed for role %q group %q: %v",
		e.Subject, e.RoleName, e.GroupName, e.Cause)
}

// Unwrap returns the underlying error.
func (e *ValidationError) Unwrap() error {
	return e.Cause
}

// NewValidationError creates a new ValidationError.
func NewValidationError(subject, roleName, groupName string, cause error) *ValidationError {
	return &ValidationError{
		Subject:   subject,
		RoleName:  roleName,
		GroupName: groupName,
		Cause:     cause,
	}
}

// IsValidationError checks if an error is or wraps a ValidationError.
func IsValidationError(err error) bool {
	var validationErr *ValidationError
	return stderrors.As(err, &validationErr)
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

// ConditionWaiting reports that the framework is waiting on something outside its control before
// the desired state can be reached — a database migration Job, a peer cluster's discovery
// ConfigMap, a secret an external controller has not issued yet.
//
// It is a condition of its own rather than an overload of Progressing for two reasons. Progressing
// has exactly one owner, HealthManager.Check, which writes it on every non-paused pass; a second
// writer in the same pass flips it and re-stamps LastTransitionTime, so the CR would be rewritten
// forever. And a wait that spans reconciles needs its OWN LastTransitionTime to be alertable at all
// — "waiting for more than 30 minutes" is the alert an operator actually wants, and it cannot be
// expressed on a field another writer resets. ConditionOrphanCleanupPending exists for the same
// reason and follows the same "only clear what this framework raised" rule.
const ConditionWaiting v1alpha1.ConditionType = "Waiting"

const (
	// ReasonWaiting is the ReconcileComplete=False reason while something is waiting.
	ReasonWaiting = "Waiting"
	// ReasonWaitSatisfied means nothing is waiting any more.
	ReasonWaitSatisfied = "WaitSatisfied"
)

// waitState is the wait a pass has accumulated. Several sites can raise one — the cluster
// PreReconcile hook, dependency validation, the role aggregate — and the pass reports a single
// condition, so they are merged rather than the last one winning.
type waitState struct {
	After   time.Duration
	Reason  string
	Message string
}

// newWaitState renders a wait from the error that carried it. The reason and message come from the
// FIRST RequeueAfterError found, which is the one whose delay the tree walk already reported.
func newWaitState(after time.Duration, err error) *waitState {
	state := &waitState{After: common.ClampRequeueAfter(after), Reason: ReasonWaiting}
	var requeueErr *common.RequeueAfterError
	if stderrors.As(err, &requeueErr) {
		if requeueErr.Reason != "" {
			state.Reason = requeueErr.Reason
		}
		state.Message = requeueErr.Message
	}
	if state.Message == "" {
		state.Message = err.Error()
	}
	return state
}

// mergeWait keeps the SHORTEST delay of the waits a pass raised, so a ten-minute wait at one site
// cannot make the cluster sit idle through a five-second wait at another. The reason and message of
// the earliest-deadline wait are the ones reported, since that is the one that will resolve first.
func mergeWait(existing, next *waitState) *waitState {
	switch {
	case existing == nil:
		return next
	case next == nil:
		return existing
	case next.After < existing.After:
		return next
	default:
		return existing
	}
}
