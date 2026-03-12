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
	"context"
	"fmt"

	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// RoleGroupCleaner cleans orphaned role group resources.
type RoleGroupCleaner struct {
	Client   client.Client
	Scheme   *runtime.Scheme
	ownerUID types.UID
}

// NewRoleGroupCleaner creates a new RoleGroupCleaner.
func NewRoleGroupCleaner(client client.Client, scheme *runtime.Scheme) *RoleGroupCleaner {
	return &RoleGroupCleaner{
		Client: client,
		Scheme: scheme,
	}
}

// WithOwner sets the owner UID used for ownerReference validation.
// Only resources owned by this UID will be deleted.
func (c *RoleGroupCleaner) WithOwner(owner metav1.Object) *RoleGroupCleaner {
	c.ownerUID = owner.GetUID()
	return c
}

// Cleanup removes orphaned resources for a cluster.
// Resources are deleted in order: PDB → StatefulSet → ConfigMap → Service
// PVCs are intentionally preserved to protect data.
// Only resources owned by the cluster CR (via ownerReference) are deleted.
func (c *RoleGroupCleaner) Cleanup(ctx context.Context, namespace, clusterName string, spec *v1alpha1.GenericClusterSpec, status *v1alpha1.GenericClusterStatus) error {
	logger := log.FromContext(ctx)

	// Get orphaned role groups
	orphanedGroups := status.GetOrphanedRoleGroups(spec.Roles)
	if len(orphanedGroups) == 0 {
		return nil
	}

	logger.Info("Cleaning up orphaned role groups", "count", countOrphanedGroups(orphanedGroups))

	for roleName, groups := range orphanedGroups {
		for _, groupName := range groups {
			resourceName := fmt.Sprintf("%s-%s", clusterName, groupName)

			if err := c.cleanupRoleGroup(ctx, namespace, resourceName); err != nil {
				return fmt.Errorf("failed to cleanup role group %s/%s: %w", roleName, groupName, err)
			}

			logger.Info("Cleaned up orphaned role group", "role", roleName, "group", groupName)
		}
	}

	return nil
}

// countOrphanedGroups counts total orphaned groups.
func countOrphanedGroups(orphaned map[string][]string) int {
	count := 0
	for _, groups := range orphaned {
		count += len(groups)
	}
	return count
}

// cleanupRoleGroup cleans up all resources for a single role group.
func (c *RoleGroupCleaner) cleanupRoleGroup(ctx context.Context, namespace, resourceName string) error {
	// Delete in order: PDB → StatefulSet → ConfigMap → Service

	// 1. Delete PDB
	if err := c.deletePDB(ctx, namespace, resourceName); err != nil {
		return err
	}

	// 2. Delete StatefulSet
	if err := c.deleteStatefulSet(ctx, namespace, resourceName); err != nil {
		return err
	}

	// 3. Delete ConfigMap
	if err := c.deleteConfigMap(ctx, namespace, resourceName); err != nil {
		return err
	}

	// 4. Delete Services (headless and regular)
	if err := c.deleteService(ctx, namespace, resourceName); err != nil {
		return err
	}

	if err := c.deleteService(ctx, namespace, resourceName+"-headless"); err != nil {
		return err
	}

	return nil
}

// isOwnedByCluster returns true if the object's ownerReferences contain the cluster owner UID.
// If ownerUID is not set on the cleaner, all resources are considered owned (backward compatible).
func (c *RoleGroupCleaner) isOwnedByCluster(obj metav1.Object) bool {
	if c.ownerUID == "" {
		return true
	}
	for _, ref := range obj.GetOwnerReferences() {
		if ref.UID == c.ownerUID && ref.Controller != nil && *ref.Controller {
			return true
		}
	}
	return false
}

// deletePDB deletes a PodDisruptionBudget if it exists and is owned by the cluster.
func (c *RoleGroupCleaner) deletePDB(ctx context.Context, namespace, name string) error {
	pdb := &policyv1.PodDisruptionBudget{}
	key := types.NamespacedName{Namespace: namespace, Name: name}

	err := c.Client.Get(ctx, key, pdb)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return c.Client.Delete(ctx, pdb)
}

// deleteStatefulSet deletes a StatefulSet if it exists and is owned by the cluster.
// PVCs managed by the StatefulSet's VolumeClaimTemplates are intentionally preserved.
func (c *RoleGroupCleaner) deleteStatefulSet(ctx context.Context, namespace, name string) error {
	sts := &appsv1.StatefulSet{}
	key := types.NamespacedName{Namespace: namespace, Name: name}

	err := c.Client.Get(ctx, key, sts)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if !c.isOwnedByCluster(sts) {
		log.FromContext(ctx).Info("Skipping StatefulSet deletion: not owned by this cluster", "name", name)
		return nil
	}

	// Scale to 0 first for graceful shutdown
	if sts.Spec.Replicas != nil && *sts.Spec.Replicas > 0 {
		zero := int32(0)
		sts.Spec.Replicas = &zero
		if err := c.Client.Update(ctx, sts); err != nil {
			log.FromContext(ctx).V(1).Info("Failed to scale StatefulSet to zero, continuing with deletion", "error", err)
		}
	}

	return c.Client.Delete(ctx, sts)
}

// deleteConfigMap deletes a ConfigMap if it exists and is owned by the cluster.
func (c *RoleGroupCleaner) deleteConfigMap(ctx context.Context, namespace, name string) error {
	cm := &corev1.ConfigMap{}
	key := types.NamespacedName{Namespace: namespace, Name: name}

	err := c.Client.Get(ctx, key, cm)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if !c.isOwnedByCluster(cm) {
		log.FromContext(ctx).Info("Skipping ConfigMap deletion: not owned by this cluster", "name", name)
		return nil
	}

	return c.Client.Delete(ctx, cm)
}

// deleteService deletes a Service if it exists and is owned by the cluster.
func (c *RoleGroupCleaner) deleteService(ctx context.Context, namespace, name string) error {
	svc := &corev1.Service{}
	key := types.NamespacedName{Namespace: namespace, Name: name}

	err := c.Client.Get(ctx, key, svc)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if !c.isOwnedByCluster(svc) {
		log.FromContext(ctx).Info("Skipping Service deletion: not owned by this cluster", "name", name)
		return nil
	}

	return c.Client.Delete(ctx, svc)
}
