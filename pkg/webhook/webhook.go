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
)

// WebhookManager manages defaulting and validation webhooks for a product CR.
// It provides a unified interface for applying defaults and running validation
// during the admission webhook phase.
//
// Usage:
//
//	manager := webhook.NewWebhookManager[*HdfsCluster]()
//	manager.WithDefaulter(&HdfsClusterDefaulter{})
//	manager.WithValidator(&HdfsClusterValidator{})
//
//	// In MutatingWebhook handler:
//	if err := manager.ApplyDefaults(ctx, cr); err != nil {
//	    return err
//	}
//
//	// In ValidatingWebhook handler:
//	if err := manager.Validate(ctx, cr); err != nil {
//	    return err
//	}
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
	if err := m.defaulter.SetDefaults(ctx, cr); err != nil {
		return fmt.Errorf("failed to apply defaults: %w", err)
	}
	return nil
}

// Validate validates the CR.
// This should be called in the ValidatingWebhook handler.
func (m *WebhookManager[CR]) Validate(ctx context.Context, cr CR) error {
	if m.validator == nil {
		return nil
	}
	if err := m.validator.Validate(ctx, cr); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}
	return nil
}

// HasDefaulter returns true if a defaulter is set.
func (m *WebhookManager[CR]) HasDefaulter() bool {
	_, ok := m.defaulter.(*NoOpDefaulter[CR])
	return !ok
}

// HasValidator returns true if a validator is set.
func (m *WebhookManager[CR]) HasValidator() bool {
	_, ok := m.validator.(*NoOpValidator[CR])
	return !ok
}
