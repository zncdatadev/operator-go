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
	"sort"
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// clusterExtensionEntry wraps a ClusterExtension with metadata.
type clusterExtensionEntry struct {
	extension ClusterExtension[ClusterInterface]
	priority  ExtensionPriority
}

// roleExtensionEntry wraps a RoleExtension with metadata.
type roleExtensionEntry struct {
	extension RoleExtension[ClusterInterface]
	priority  ExtensionPriority
}

// roleGroupExtensionEntry wraps a RoleGroupExtension with metadata.
type roleGroupExtensionEntry struct {
	extension RoleGroupExtension[ClusterInterface]
	priority  ExtensionPriority
}

// ExtensionRegistry manages all registered extensions.
// Extensions are executed in priority order (highest first).
type ExtensionRegistry struct {
	clusterExtensions   []clusterExtensionEntry
	roleExtensions      []roleExtensionEntry
	roleGroupExtensions []roleGroupExtensionEntry
	mu                  sync.RWMutex
}

// globalRegistry is the singleton instance.
// Thread-safety: All access to globalRegistry is protected by sync.RWMutex.
// For testing scenarios, use ResetExtensionRegistry() to reset state between tests,
// or consider creating isolated instances with NewExtensionRegistry() for parallel tests.
var globalRegistry = &ExtensionRegistry{
	clusterExtensions:   make([]clusterExtensionEntry, 0),
	roleExtensions:      make([]roleExtensionEntry, 0),
	roleGroupExtensions: make([]roleGroupExtensionEntry, 0),
}

// GetExtensionRegistry returns the global registry singleton.
func GetExtensionRegistry() *ExtensionRegistry {
	return globalRegistry
}

// ResetExtensionRegistry resets the global registry (for testing).
func ResetExtensionRegistry() {
	globalRegistry = &ExtensionRegistry{
		clusterExtensions:   make([]clusterExtensionEntry, 0),
		roleExtensions:      make([]roleExtensionEntry, 0),
		roleGroupExtensions: make([]roleGroupExtensionEntry, 0),
	}
}

// RegisterClusterExtension registers a cluster-level extension with default priority.
func (r *ExtensionRegistry) RegisterClusterExtension(extension ClusterExtension[ClusterInterface]) {
	r.RegisterClusterExtensionWithPriority(extension, PriorityNormal)
}

// RegisterClusterExtensionWithPriority registers a cluster-level extension with specific priority.
func (r *ExtensionRegistry) RegisterClusterExtensionWithPriority(extension ClusterExtension[ClusterInterface], priority ExtensionPriority) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.clusterExtensions = append(r.clusterExtensions, clusterExtensionEntry{
		extension: extension,
		priority:  priority,
	})

	// Sort by priority (highest first)
	sort.Slice(r.clusterExtensions, func(i, j int) bool {
		return r.clusterExtensions[i].priority > r.clusterExtensions[j].priority
	})
}

// RegisterRoleExtension registers a role-level extension with default priority.
func (r *ExtensionRegistry) RegisterRoleExtension(extension RoleExtension[ClusterInterface]) {
	r.RegisterRoleExtensionWithPriority(extension, PriorityNormal)
}

// RegisterRoleExtensionWithPriority registers a role-level extension with specific priority.
func (r *ExtensionRegistry) RegisterRoleExtensionWithPriority(extension RoleExtension[ClusterInterface], priority ExtensionPriority) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.roleExtensions = append(r.roleExtensions, roleExtensionEntry{
		extension: extension,
		priority:  priority,
	})

	// Sort by priority (highest first)
	sort.Slice(r.roleExtensions, func(i, j int) bool {
		return r.roleExtensions[i].priority > r.roleExtensions[j].priority
	})
}

// RegisterRoleGroupExtension registers a role group-level extension with default priority.
func (r *ExtensionRegistry) RegisterRoleGroupExtension(extension RoleGroupExtension[ClusterInterface]) {
	r.RegisterRoleGroupExtensionWithPriority(extension, PriorityNormal)
}

// RegisterRoleGroupExtensionWithPriority registers a role group-level extension with specific priority.
func (r *ExtensionRegistry) RegisterRoleGroupExtensionWithPriority(extension RoleGroupExtension[ClusterInterface], priority ExtensionPriority) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.roleGroupExtensions = append(r.roleGroupExtensions, roleGroupExtensionEntry{
		extension: extension,
		priority:  priority,
	})

	// Sort by priority (highest first)
	sort.Slice(r.roleGroupExtensions, func(i, j int) bool {
		return r.roleGroupExtensions[i].priority > r.roleGroupExtensions[j].priority
	})
}

// GetClusterExtensions returns all registered cluster extensions.
func (r *ExtensionRegistry) GetClusterExtensions() []ClusterExtension[ClusterInterface] {
	r.mu.RLock()
	defer r.mu.RUnlock()

	extensions := make([]ClusterExtension[ClusterInterface], len(r.clusterExtensions))
	for i, entry := range r.clusterExtensions {
		extensions[i] = entry.extension
	}
	return extensions
}

// GetRoleExtensions returns all registered role extensions.
func (r *ExtensionRegistry) GetRoleExtensions() []RoleExtension[ClusterInterface] {
	r.mu.RLock()
	defer r.mu.RUnlock()

	extensions := make([]RoleExtension[ClusterInterface], len(r.roleExtensions))
	for i, entry := range r.roleExtensions {
		extensions[i] = entry.extension
	}
	return extensions
}

// GetRoleGroupExtensions returns all registered role group extensions.
func (r *ExtensionRegistry) GetRoleGroupExtensions() []RoleGroupExtension[ClusterInterface] {
	r.mu.RLock()
	defer r.mu.RUnlock()

	extensions := make([]RoleGroupExtension[ClusterInterface], len(r.roleGroupExtensions))
	for i, entry := range r.roleGroupExtensions {
		extensions[i] = entry.extension
	}
	return extensions
}

// ExecuteClusterPreReconcile executes all cluster PreReconcile hooks.
func (r *ExtensionRegistry) ExecuteClusterPreReconcile(ctx context.Context, client client.Client, cr ClusterInterface) error {
	for _, ext := range r.GetClusterExtensions() {
		if err := ext.PreReconcile(ctx, client, cr); err != nil {
			return NewExtensionError(ext.Name(), err)
		}
	}
	return nil
}

// ExecuteClusterPostReconcile executes all cluster PostReconcile hooks.
func (r *ExtensionRegistry) ExecuteClusterPostReconcile(ctx context.Context, client client.Client, cr ClusterInterface) error {
	for _, ext := range r.GetClusterExtensions() {
		if err := ext.PostReconcile(ctx, client, cr); err != nil {
			return NewExtensionError(ext.Name(), err)
		}
	}
	return nil
}

// ExecuteClusterOnError executes all cluster OnReconcileError hooks.
func (r *ExtensionRegistry) ExecuteClusterOnError(ctx context.Context, client client.Client, cr ClusterInterface, reconcileErr error) error {
	logger := log.FromContext(ctx)
	for _, ext := range r.GetClusterExtensions() {
		if err := ext.OnReconcileError(ctx, client, cr, reconcileErr); err != nil {
			// Log but don't return - we want to run all error handlers
			logger.Error(err, "Extension error handler failed", "extension", ext.Name())
		}
	}
	return nil
}

// ExecuteRolePreReconcile executes all role PreReconcile hooks.
func (r *ExtensionRegistry) ExecuteRolePreReconcile(ctx context.Context, client client.Client, cr ClusterInterface, roleName string) error {
	for _, ext := range r.GetRoleExtensions() {
		if err := ext.PreReconcile(ctx, client, cr, roleName); err != nil {
			return NewExtensionError(ext.Name(), err)
		}
	}
	return nil
}

// ExecuteRolePostReconcile executes all role PostReconcile hooks.
func (r *ExtensionRegistry) ExecuteRolePostReconcile(ctx context.Context, client client.Client, cr ClusterInterface, roleName string) error {
	for _, ext := range r.GetRoleExtensions() {
		if err := ext.PostReconcile(ctx, client, cr, roleName); err != nil {
			return NewExtensionError(ext.Name(), err)
		}
	}
	return nil
}

// ExecuteRoleGroupPreReconcile executes all role group PreReconcile hooks.
func (r *ExtensionRegistry) ExecuteRoleGroupPreReconcile(ctx context.Context, client client.Client, cr ClusterInterface, roleName, roleGroupName string) error {
	for _, ext := range r.GetRoleGroupExtensions() {
		if err := ext.PreReconcile(ctx, client, cr, roleName, roleGroupName); err != nil {
			return NewExtensionError(ext.Name(), err)
		}
	}
	return nil
}

// ExecuteRoleGroupPostReconcile executes all role group PostReconcile hooks.
func (r *ExtensionRegistry) ExecuteRoleGroupPostReconcile(ctx context.Context, client client.Client, cr ClusterInterface, roleName, roleGroupName string) error {
	for _, ext := range r.GetRoleGroupExtensions() {
		if err := ext.PostReconcile(ctx, client, cr, roleName, roleGroupName); err != nil {
			return NewExtensionError(ext.Name(), err)
		}
	}
	return nil
}

// HasClusterExtensions returns true if any cluster extensions are registered.
func (r *ExtensionRegistry) HasClusterExtensions() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.clusterExtensions) > 0
}

// HasRoleExtensions returns true if any role extensions are registered.
func (r *ExtensionRegistry) HasRoleExtensions() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.roleExtensions) > 0
}

// HasRoleGroupExtensions returns true if any role group extensions are registered.
func (r *ExtensionRegistry) HasRoleGroupExtensions() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.roleGroupExtensions) > 0
}

// Clear removes all registered extensions (for testing).
func (r *ExtensionRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.clusterExtensions = make([]clusterExtensionEntry, 0)
	r.roleExtensions = make([]roleExtensionEntry, 0)
	r.roleGroupExtensions = make([]roleGroupExtensionEntry, 0)
}

// Count returns the total number of registered extensions.
func (r *ExtensionRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.clusterExtensions) + len(r.roleExtensions) + len(r.roleGroupExtensions)
}
