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

package util

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// Default retry backoff for conflict resolution.
var defaultBackoff = wait.Backoff{
	Steps:    5,
	Duration: 100 * time.Millisecond,
	Factor:   1.0,
	Jitter:   0.1,
}

// K8sUtil provides common Kubernetes resource operations.
type K8sUtil struct {
	Client client.Client
	Scheme *runtime.Scheme
}

// NewK8sUtil creates a new K8sUtil.
func NewK8sUtil(client client.Client, scheme *runtime.Scheme) *K8sUtil {
	return &K8sUtil{
		Client: client,
		Scheme: scheme,
	}
}

// CreateOrUpdate creates or updates a resource (idempotent).
// The mutateFn is called to update the object if it already exists.
func (k *K8sUtil) CreateOrUpdate(ctx context.Context, obj client.Object, mutateFn func() error) (controllerutil.OperationResult, error) {
	result, err := controllerutil.CreateOrUpdate(ctx, k.Client, obj, mutateFn)
	if err != nil {
		return controllerutil.OperationResultNone, fmt.Errorf("failed to create or update %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}
	return result, nil
}

// DeleteIfExists deletes a resource if it exists (idempotent).
// Returns nil if the resource doesn't exist or was successfully deleted.
func (k *K8sUtil) DeleteIfExists(ctx context.Context, obj client.Object) error {
	err := k.Client.Delete(ctx, obj)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil // Resource doesn't exist, nothing to do
		}
		return fmt.Errorf("failed to delete %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}
	return nil
}

// Get retrieves a resource by name and namespace.
func (k *K8sUtil) Get(ctx context.Context, key types.NamespacedName, obj client.Object) error {
	if err := k.Client.Get(ctx, key, obj); err != nil {
		return fmt.Errorf("failed to get %s/%s: %w", key.Namespace, key.Name, err)
	}
	return nil
}

// List retrieves a list of resources.
func (k *K8sUtil) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if err := k.Client.List(ctx, list, opts...); err != nil {
		return fmt.Errorf("failed to list resources: %w", err)
	}
	return nil
}

// Update updates a resource.
func (k *K8sUtil) Update(ctx context.Context, obj client.Object) error {
	if err := k.Client.Update(ctx, obj); err != nil {
		return fmt.Errorf("failed to update %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}
	return nil
}

// UpdateStatus updates resource status with retry on conflict.
func (k *K8sUtil) UpdateStatus(ctx context.Context, obj client.Object) error {
	if err := k.Client.Status().Update(ctx, obj); err != nil {
		return fmt.Errorf("failed to update status for %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}
	return nil
}

// Patch applies a patch to a resource.
func (k *K8sUtil) Patch(ctx context.Context, obj client.Object, patch client.Patch) error {
	if err := k.Client.Patch(ctx, obj, patch); err != nil {
		return fmt.Errorf("failed to patch %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}
	return nil
}

// ApplyOwnership sets owner references on an object.
// This ensures the owned object is garbage collected when the owner is deleted.
func (k *K8sUtil) ApplyOwnership(owner, owned client.Object) error {
	if err := controllerutil.SetControllerReference(owner, owned, k.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}
	return nil
}

// SetOwnerReference sets an owner reference (non-controller) on an object.
func (k *K8sUtil) SetOwnerReference(owner, owned client.Object) error {
	gvk := owner.GetObjectKind().GroupVersionKind()
	ref := metav1.OwnerReference{
		APIVersion: gvk.GroupVersion().String(),
		Kind:       gvk.Kind,
		Name:       owner.GetName(),
		UID:        owner.GetUID(),
	}

	refs := owned.GetOwnerReferences()
	for _, r := range refs {
		if r.UID == ref.UID {
			return nil // Already set
		}
	}

	owned.SetOwnerReferences(append(refs, ref))
	return nil
}

// ResourceExists checks if a resource exists.
func (k *K8sUtil) ResourceExists(ctx context.Context, key types.NamespacedName, obj client.Object) (bool, error) {
	err := k.Client.Get(ctx, key, obj)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Create creates a new resource.
// Returns an error if the resource already exists.
func (k *K8sUtil) Create(ctx context.Context, obj client.Object) error {
	if err := k.Client.Create(ctx, obj); err != nil {
		return fmt.Errorf("failed to create %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}
	return nil
}

// UpdateWithRetry updates a resource with retry on conflict.
func (k *K8sUtil) UpdateWithRetry(ctx context.Context, obj client.Object, updateFn func() error) error {
	return retry.RetryOnConflict(defaultBackoff, func() error {
		if err := k.Client.Get(ctx, types.NamespacedName{
			Namespace: obj.GetNamespace(),
			Name:      obj.GetName(),
		}, obj); err != nil {
			return err
		}

		if err := updateFn(); err != nil {
			return err
		}

		return k.Client.Update(ctx, obj)
	})
}

// UpdateStatusWithRetry updates status with retry on conflict.
func (k *K8sUtil) UpdateStatusWithRetry(ctx context.Context, obj client.Object, updateFn func() error) error {
	return retry.RetryOnConflict(defaultBackoff, func() error {
		if err := k.Client.Get(ctx, types.NamespacedName{
			Namespace: obj.GetNamespace(),
			Name:      obj.GetName(),
		}, obj); err != nil {
			return err
		}

		if err := updateFn(); err != nil {
			return err
		}

		return k.Client.Status().Update(ctx, obj)
	})
}
