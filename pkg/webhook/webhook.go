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

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// WebhookManager manages defaulting and validation webhooks for a product CR.
// It provides a unified interface for applying defaults and running validation
// during the admission webhook phase.
//
// # Recommended Usage with controller-runtime
//
// Use DefaulterAdapter and ValidatorAdapter to bridge SDK types to controller-runtime:
//
//	func SetupWebhookWithManager(mgr ctrl.Manager) error {
//	   return ctrl.NewWebhookManagedBy(mgr, &MyCluster{}).
//	       WithDefaulter(webhook.NewDefaulterAdapter(&MyClusterDefaulter{})).
//	       WithValidator(webhook.NewValidatorAdapter[*MyCluster](&MyClusterValidator{})).
//	       Complete()
//	}
//
// # Alternative: WebhookManager (for testing or custom handlers)
//
// manager := webhook.NewWebhookManager[*HdfsCluster]()
// manager.WithDefaulter(&HdfsClusterDefaulter{})
// manager.WithValidator(&HdfsClusterValidator{})
// if err := manager.ApplyDefaults(ctx, cr); err != nil { ... }
type WebhookManager[CR any] struct {
	defaulter ProductDefaulter[CR]
	validator ProductValidator[CR]
}

// NewWebhookManager creates a new WebhookManager.
func NewWebhookManager[CR any]() *WebhookManager[CR] {
	return &WebhookManager[CR]{
		defaulter: NewNoOpDefaulter[CR](),
		validator: NewNoOpValidator[CR](),
	}
}

// WithDefaulter sets the defaulter for the webhook manager.
func (m *WebhookManager[CR]) WithDefaulter(d ProductDefaulter[CR]) *WebhookManager[CR] {
	if d != nil {
		m.defaulter = d
	}
	return m
}

// WithValidator sets the validator for the webhook manager.
func (m *WebhookManager[CR]) WithValidator(v ProductValidator[CR]) *WebhookManager[CR] {
	if v != nil {
		m.validator = v
	}
	return m
}

// ApplyDefaults applies all defaults to the CR.
// This should be called in the MutatingWebhook handler.
func (m *WebhookManager[CR]) ApplyDefaults(ctx context.Context, cr CR) error {
	if m.defaulter == nil {
		return nil
	}
	if err := m.defaulter.Default(ctx, cr); err != nil {
		return fmt.Errorf("failed to apply defaults: %w", err)
	}
	return nil
}

// Validate validates the CR on creation (calls ValidateCreate internally).
// This should be called in the ValidatingWebhook handler.
func (m *WebhookManager[CR]) Validate(ctx context.Context, cr CR) error {
	if m.validator == nil {
		return nil
	}
	_, err := m.validator.ValidateCreate(ctx, cr)
	if err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}
	return nil
}

// HasDefaulter returns true if a custom defaulter is set.
func (m *WebhookManager[CR]) HasDefaulter() bool {
	_, ok := m.defaulter.(*NoOpDefaulter[CR])
	return !ok
}

// HasValidator returns true if a custom validator is set.
func (m *WebhookManager[CR]) HasValidator() bool {
	_, ok := m.validator.(*NoOpValidator[CR])
	return !ok
}

// DefaulterAdapter adapts a typed ProductDefaulter[CR] to the untyped
// admission.CustomDefaulter (i.e. admission.Defaulter[runtime.Object]).
// Use this to pass a typed SDK defaulter to ctrl.NewWebhookManagedBy(...).WithDefaulter(...).
//
// Example:
//
// ctrl.NewWebhookManagedBy(mgr, &MyCluster{}).
//
//	WithDefaulter(webhook.NewDefaulterAdapter(&MyClusterDefaulter{})).
//	Complete()
type DefaulterAdapter[CR runtime.Object] struct {
	inner ProductDefaulter[CR]
}

// NewDefaulterAdapter creates a DefaulterAdapter wrapping the given ProductDefaulter.
func NewDefaulterAdapter[CR runtime.Object](inner ProductDefaulter[CR]) *DefaulterAdapter[CR] {
	return &DefaulterAdapter[CR]{inner: inner}
}

// Default implements admission.CustomDefaulter.
// It casts obj to CR and delegates to the inner ProductDefaulter.Default.
func (a *DefaulterAdapter[CR]) Default(ctx context.Context, obj runtime.Object) error {
	cr, ok := obj.(CR)
	if !ok {
		return fmt.Errorf("expected %T, got %T", *new(CR), obj)
	}
	return a.inner.Default(ctx, cr)
}

// ValidatorAdapter adapts a typed ProductValidator[CR] to the untyped
// admission.CustomValidator (i.e. admission.Validator[runtime.Object]).
// Use this to pass a typed SDK validator to ctrl.NewWebhookManagedBy(...).WithValidator(...).
//
// Example:
//
// ctrl.NewWebhookManagedBy(mgr, &MyCluster{}).
//
//	WithValidator(webhook.NewValidatorAdapter[*MyCluster](&MyClusterValidator{})).
//	Complete()
type ValidatorAdapter[CR runtime.Object] struct {
	inner ProductValidator[CR]
}

// NewValidatorAdapter creates a ValidatorAdapter wrapping the given ProductValidator.
func NewValidatorAdapter[CR runtime.Object](inner ProductValidator[CR]) *ValidatorAdapter[CR] {
	return &ValidatorAdapter[CR]{inner: inner}
}

// ValidateCreate implements admission.CustomValidator.
func (a *ValidatorAdapter[CR]) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	cr, ok := obj.(CR)
	if !ok {
		return nil, fmt.Errorf("expected %T, got %T", *new(CR), obj)
	}
	return a.inner.ValidateCreate(ctx, cr)
}

// ValidateUpdate implements admission.CustomValidator.
func (a *ValidatorAdapter[CR]) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldCR, ok := oldObj.(CR)
	if !ok {
		return nil, fmt.Errorf("expected %T for oldObj, got %T", *new(CR), oldObj)
	}
	newCR, ok := newObj.(CR)
	if !ok {
		return nil, fmt.Errorf("expected %T for newObj, got %T", *new(CR), newObj)
	}
	return a.inner.ValidateUpdate(ctx, oldCR, newCR)
}

// ValidateDelete implements admission.CustomValidator.
func (a *ValidatorAdapter[CR]) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	cr, ok := obj.(CR)
	if !ok {
		return nil, fmt.Errorf("expected %T, got %T", *new(CR), obj)
	}
	return a.inner.ValidateDelete(ctx, cr)
}
