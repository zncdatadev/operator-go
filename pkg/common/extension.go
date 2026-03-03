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
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Extension is the base interface for all extensions.
// Extensions allow injecting custom logic into the reconciliation process.
type Extension interface {
	// Name returns the extension name for identification and logging.
	Name() string
}

// ClusterExtension defines cluster-level extension points.
// Extensions run at specific phases of the reconciliation loop.
//
// Extension Lifecycle:
// 1. PreReconcile: Called before reconciliation starts
// 2. PostReconcile: Called after reconciliation completes successfully
// 3. OnReconcileError: Called when reconciliation encounters an error
type ClusterExtension[CR ClusterInterface] interface {
	Extension

	// PreReconcile is called before reconciliation starts.
	// Use this for setup, validation, or state initialization.
	// Return an error to abort reconciliation.
	PreReconcile(ctx context.Context, client client.Client, cr CR) error

	// PostReconcile is called after reconciliation completes successfully.
	// Use this for cleanup, notifications, or status updates.
	PostReconcile(ctx context.Context, client client.Client, cr CR) error

	// OnReconcileError is called when reconciliation encounters an error.
	// Use this for cleanup, logging, or error recovery.
	OnReconcileError(ctx context.Context, client client.Client, cr CR, err error) error
}

// RoleExtension defines role-level extension points.
// These extensions are called for each role during reconciliation.
type RoleExtension[CR ClusterInterface] interface {
	Extension

	// PreReconcile is called before role reconciliation starts.
	PreReconcile(ctx context.Context, client client.Client, cr CR, roleName string) error

	// PostReconcile is called after role reconciliation completes.
	PostReconcile(ctx context.Context, client client.Client, cr CR, roleName string) error
}

// RoleGroupExtension defines role group-level extension points.
// These extensions are called for each role group during reconciliation.
type RoleGroupExtension[CR ClusterInterface] interface {
	Extension

	// PreReconcile is called before role group reconciliation starts.
	PreReconcile(ctx context.Context, client client.Client, cr CR, roleName, roleGroupName string) error

	// PostReconcile is called after role group reconciliation completes.
	PostReconcile(ctx context.Context, client client.Client, cr CR, roleName, roleGroupName string) error
}

// ExtensionPriority defines the execution order of extensions.
type ExtensionPriority int

const (
	// PriorityLowest extensions run last.
	PriorityLowest ExtensionPriority = 0
	// PriorityLow extensions run after normal priority.
	PriorityLow ExtensionPriority = 25
	// PriorityNormal is the default priority.
	PriorityNormal ExtensionPriority = 50
	// PriorityHigh extensions run before normal priority.
	PriorityHigh ExtensionPriority = 75
	// PriorityHighest extensions run first.
	PriorityHighest ExtensionPriority = 100
)

// PrioritizedExtension wraps an extension with a priority for ordering.
type PrioritizedExtension interface {
	Extension
	// Priority returns the extension priority.
	Priority() ExtensionPriority
}

// BaseExtension provides a base implementation for extensions.
type BaseExtension struct {
	name string
}

// NewBaseExtension creates a new BaseExtension.
func NewBaseExtension(name string) BaseExtension {
	return BaseExtension{name: name}
}

// Name returns the extension name.
func (e BaseExtension) Name() string {
	return e.name
}

// NoOpExtension is an extension that does nothing.
// Useful for testing or as a placeholder.
type NoOpExtension struct {
	BaseExtension
}

// NewNoOpExtension creates a new NoOpExtension.
func NewNoOpExtension(name string) *NoOpExtension {
	return &NoOpExtension{BaseExtension: NewBaseExtension(name)}
}

// PreReconcile does nothing.
func (e *NoOpExtension) PreReconcile(ctx context.Context, client client.Client, cr ClusterInterface) error {
	return nil
}

// PostReconcile does nothing.
func (e *NoOpExtension) PostReconcile(ctx context.Context, client client.Client, cr ClusterInterface) error {
	return nil
}

// OnReconcileError does nothing.
func (e *NoOpExtension) OnReconcileError(ctx context.Context, client client.Client, cr ClusterInterface, err error) error {
	return nil
}

// ExtensionError wraps an error with extension context.
type ExtensionError struct {
	ExtensionName string
	Err           error
}

// Error implements error.
func (e *ExtensionError) Error() string {
	return "extension " + e.ExtensionName + ": " + e.Err.Error()
}

// Unwrap returns the underlying error.
func (e *ExtensionError) Unwrap() error {
	return e.Err
}

// NewExtensionError creates a new ExtensionError.
func NewExtensionError(extensionName string, err error) *ExtensionError {
	return &ExtensionError{
		ExtensionName: extensionName,
		Err:           err,
	}
}
