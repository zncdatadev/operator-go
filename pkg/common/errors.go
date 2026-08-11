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
	"errors"
	"fmt"
	"time"
)

// ConfigValidationError creates an error for configuration validation failures.
func ConfigValidationError(field string, err error) error {
	return fmt.Errorf("configuration validation failed for %s: %w", field, err)
}

// ResourceNotFoundError creates an error indicating a resource was not found.
func ResourceNotFoundError(resourceType, namespace, name string, err error) error {
	return fmt.Errorf("%s %s/%s not found: %w", resourceType, namespace, name, err)
}

// CreateResourceError creates an error for resource creation failures.
func CreateResourceError(resourceType, namespace, name string, err error) error {
	return fmt.Errorf("failed to create %s %s/%s: %w", resourceType, namespace, name, err)
}

// ConfigMergeError creates an error for configuration merge failures.
func ConfigMergeError(context string, err error) error {
	return fmt.Errorf("configuration merge failed for %s: %w", context, err)
}

// ConfigParseError creates an error for configuration parsing failures.
func ConfigParseError(format string, err error) error {
	return fmt.Errorf("failed to parse %s configuration: %w", format, err)
}

// MinRequeueAfter and DefaultRequeueAfter bound the delay a RequeueAfterError can ask for.
//
// Both exist because controller-runtime treats the delay as a switch, not a hint: its
// `case result.RequeueAfter > 0` falls through to a default branch that only Forgets the item, so a
// NON-POSITIVE delay does not requeue at all. A product computing `deadline.Sub(time.Now())` therefore
// wedges the cluster permanently the moment the deadline passes — the wait is reported, no error is
// returned, and nothing ever wakes the controller again.
const (
	MinRequeueAfter     = 1 * time.Second
	DefaultRequeueAfter = 10 * time.Second
)

// RequeueAfterError reports that the desired state is not satisfiable YET — an external condition
// the operator does not control has not happened. It is not a failure: the framework requeues
// without setting Degraded and without emitting a Warning event.
//
// It is an ERROR TYPE rather than a change to the hook signatures because the same "not yet" arises
// in places that are not hooks — sidecar validation, ProductConfig's live lookups, a handler's
// BuildResources — and an error is the only channel all of them already have. RateLimitError is the
// same shape for the same reason.
//
// Reason and Message must be STABLE across passes while the wait lasts. The framework's status
// write is guarded by a whole-object DeepEqual, so a message that counts ("3/5 brokers registered")
// rewrites the CR every pass; and because this path returns no error the workqueue Forgets the item,
// so nothing backs that ping-pong off. Put the varying detail in a log line, not in the condition.
type RequeueAfterError struct {
	// After is how long to wait before the next reconcile. Clamped: see NewRequeueAfterError.
	After time.Duration
	// Reason is a CamelCase condition reason, e.g. "WaitingForDatabase".
	Reason string
	// Message is a human-readable explanation surfaced on the Waiting condition.
	Message string
}

func (e *RequeueAfterError) Error() string {
	return fmt.Sprintf("waiting (%s): %s", e.Reason, e.Message)
}

// NewRequeueAfterError builds a wait, clamping the delay: a non-positive value becomes
// DefaultRequeueAfter (a zero delay would never requeue at all), and anything under MinRequeueAfter
// becomes MinRequeueAfter (a nanosecond delay is a hot loop against the API server).
func NewRequeueAfterError(after time.Duration, reason, message string) *RequeueAfterError {
	return &RequeueAfterError{After: ClampRequeueAfter(after), Reason: reason, Message: message}
}

// ClampRequeueAfter applies the bounds above. It is exported and applied again at the framework's
// own branch, because RequeueAfterError is a struct literal a caller can build without the
// constructor.
func ClampRequeueAfter(after time.Duration) time.Duration {
	switch {
	case after <= 0:
		return DefaultRequeueAfter
	case after < MinRequeueAfter:
		return MinRequeueAfter
	default:
		return after
	}
}

// IsRequeueAfterError reports whether err is or wraps a RequeueAfterError.
func IsRequeueAfterError(err error) bool {
	var requeueErr *RequeueAfterError
	return errors.As(err, &requeueErr)
}

// WaitingErrors walks an error tree and reports whether EVERY leaf is a RequeueAfterError,
// returning the one with the SHORTEST delay.
//
// Both halves matter. The framework aggregates per-role and per-extension failures with
// errors.Join, so a tree can hold a wait next to a genuine failure — and `errors.As` would find the
// wait and suppress the failure's report. It also returns the FIRST match depth-first, so a 10-minute
// wait on one branch would hide a 5-second wait on another and the cluster would sit idle for the
// difference.
//
// Returning the winning error rather than only its duration keeps the reported Reason and Message
// attached to the delay they belong to: with several waits joined, deriving the reason separately
// (with errors.As, which matches depth-first) would describe one wait while waiting for another.
//
// A nil error is not a wait. A wrapper whose Unwrap returns nothing is opaque and counts as a
// failure, because its cause cannot be inspected.
func WaitingErrors(err error) (*RequeueAfterError, bool) {
	if err == nil {
		return nil, false
	}
	// A direct type assertion, NOT errors.As: As searches the whole tree and would report "waiting"
	// for a wrapper around a join that also holds a genuine failure, laundering it.
	if requeueErr, ok := err.(*RequeueAfterError); ok {
		return requeueErr, true
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		members := joined.Unwrap()
		if len(members) == 0 {
			return nil, false
		}
		var shortest *RequeueAfterError
		for _, member := range members {
			memberErr, waiting := WaitingErrors(member)
			if !waiting {
				return nil, false
			}
			if shortest == nil || ClampRequeueAfter(memberErr.After) < ClampRequeueAfter(shortest.After) {
				shortest = memberErr
			}
		}
		return shortest, true
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		if inner := wrapped.Unwrap(); inner != nil {
			return WaitingErrors(inner)
		}
	}
	// Opaque: its cause cannot be inspected, so it counts as a failure rather than a wait.
	return nil, false
}
