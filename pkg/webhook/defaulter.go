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
)

// ProductDefaulter defines the interface for setting default values on CRs.
// Products implement this interface to provide custom defaulting logic
// that runs during the MutatingWebhook phase.
//
// Usage:
//
//	type HdfsClusterDefaulter struct{}
//
//	func (d *HdfsClusterDefaulter) SetDefaults(ctx context.Context, cr *HdfsCluster) error {
//	    if cr.Spec.SomeField == "" {
//	        cr.Spec.SomeField = "default-value"
//	    }
//	    return nil
//	}
type ProductDefaulter[CR any] interface {
	// SetDefaults populates default values for the CR.
	// This is called by the MutatingWebhook before persistence.
	SetDefaults(ctx context.Context, cr CR) error
}

// NoOpDefaulter is a defaulter that does nothing.
// Useful for testing or when no defaulting is needed.
type NoOpDefaulter[CR any] struct{}

// SetDefaults does nothing and returns nil.
func (d *NoOpDefaulter[CR]) SetDefaults(_ context.Context, _ CR) error {
	return nil
}

// NewNoOpDefaulter creates a new NoOpDefaulter.
func NewNoOpDefaulter[CR any]() *NoOpDefaulter[CR] {
	return &NoOpDefaulter[CR]{}
}

// FuncDefaulter wraps a function as a ProductDefaulter.
type FuncDefaulter[CR any] struct {
	fn func(ctx context.Context, cr CR) error
}

// NewFuncDefaulter creates a new FuncDefaulter.
func NewFuncDefaulter[CR any](fn func(ctx context.Context, cr CR) error) *FuncDefaulter[CR] {
	return &FuncDefaulter[CR]{fn: fn}
}

// SetDefaults calls the wrapped function.
func (d *FuncDefaulter[CR]) SetDefaults(ctx context.Context, cr CR) error {
	if d.fn == nil {
		return nil
	}
	return d.fn(ctx, cr)
}
