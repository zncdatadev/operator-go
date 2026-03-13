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
	"time"

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

const (
	// AnnotationPendingDeletion is set on orphaned resources during the gray-delete grace period.
	// Value is an RFC3339 timestamp indicating when the resource was first marked for deletion.
	AnnotationPendingDeletion = "orphan.zncdata.dev/pending-deletion"

	// AnnotationDeletePVCs when set to "true" on the cluster CR, causes the cleaner to also
	// delete PVCs associated with orphaned StatefulSets.
	AnnotationDeletePVCs = "operator.zncdata.dev/delete-pvcs"
)

// RoleGroupCleaner cleans orphaned role group resources.
type RoleGroupCleaner struct {
	Client                client.Client
	Scheme                *runtime.Scheme
	grayDeleteGracePeriod time.Duration
}

// NewRoleGroupCleaner creates a new RoleGroupCleaner.
func NewRoleGroupCleaner(client client.Client, scheme *runtime.Scheme) *RoleGroupCleaner {
	return &RoleGroupCleaner{
		Client: client,
		Scheme: scheme,
	}
}

// WithGrayDeleteGracePeriod sets the grace period for gray deletion.
// When > 0, orphaned resources are first annotated and only deleted after the grace period.
// When 0 (default), resources are deleted immediately.
func (c *RoleGroupCleaner) WithGrayDeleteGracePeriod(d time.Duration) *RoleGroupCleaner {
	c.grayDeleteGracePeriod = d
	return c
}

// Cleanup removes orphaned resources for a cluster.
// Resources are deleted in order: PDB → StatefulSet → ConfigMap → Service
// PVCs are intentionally preserved to protect data unless AnnotationDeletePVCs is set in crAnnotations.
// Only resources with an ownerReference pointing to ownerUID (with controller=true) are deleted.
// If GrayDeleteGracePeriod > 0, resources are annotated on first detection and only deleted
// after the grace period has elapsed. Resources that are no longer orphaned have the annotation cleared.
func (c *RoleGroupCleaner) Cleanup(
	ctx context.Context,
	namespace, clusterName string,
	spec *v1alpha1.GenericClusterSpec,
	status *v1alpha1.GenericClusterStatus,
	ownerUID types.UID,
	crAnnotations map[string]string,
) error {
	logger := log.FromContext(ctx)

	deletePVCs := crAnnotations[AnnotationDeletePVCs] == "true"

	// Get orphaned role groups
	orphanedGroups := status.GetOrphanedRoleGroups(spec.Roles)

	// LOW-3: If gray-delete is enabled, clear AnnotationPendingDeletion from resources
	// that are no longer orphaned (re-added to spec). This ensures the grace period is
	// respected correctly on any future re-orphaning.
	if c.grayDeleteGracePeriod > 0 {
		for _, roleSpec := range spec.Roles {
			for groupName := range roleSpec.RoleGroups {
				resourceName := fmt.Sprintf("%s-%s", clusterName, groupName)
				if err := c.clearGrayDeleteAnnotation(ctx, namespace, resourceName, ownerUID); err != nil {
					logger.V(1).Info("Failed to clear gray-delete annotation from active resource",
						"resource", resourceName, "error", err)
				}
			}
		}
	}

	if len(orphanedGroups) == 0 {
		return nil
	}

	logger.Info("Cleaning up orphaned role groups", "count", countOrphanedGroups(orphanedGroups))

	for roleName, groups := range orphanedGroups {
		for _, groupName := range groups {
			resourceName := fmt.Sprintf("%s-%s", clusterName, groupName)

			if err := c.cleanupRoleGroup(ctx, namespace, resourceName, ownerUID, deletePVCs); err != nil {
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
// When GrayDeleteGracePeriod is set, resources are first annotated and deleted only after
// the grace period has elapsed.
func (c *RoleGroupCleaner) cleanupRoleGroup(ctx context.Context, namespace, resourceName string, ownerUID types.UID, deletePVCs bool) error {
	if c.grayDeleteGracePeriod > 0 {
		// Gray delete: check if the primary resource (StatefulSet or ConfigMap) is already
		// annotated. If not, annotate and defer; if yes and grace period elapsed, proceed.
		ready, err := c.checkOrMarkGrayDelete(ctx, namespace, resourceName, ownerUID)
		if err != nil {
			return err
		}
		if !ready {
			// Grace period not yet elapsed; skip deletion this cycle
			return nil
		}
	}

	// Delete in order: PDB → StatefulSet → ConfigMap → Service

	// 1. Delete PDB
	if err := c.deletePDB(ctx, namespace, resourceName, ownerUID); err != nil {
		return err
	}

	// 2. Delete StatefulSet (and optionally PVCs before scaling down)
	if err := c.deleteStatefulSet(ctx, namespace, resourceName, ownerUID, deletePVCs); err != nil {
		return err
	}

	// 3. Delete ConfigMap
	if err := c.deleteConfigMap(ctx, namespace, resourceName, ownerUID); err != nil {
		return err
	}

	// 4. Delete Services (headless and regular)
	if err := c.deleteService(ctx, namespace, resourceName, ownerUID); err != nil {
		return err
	}

	if err := c.deleteService(ctx, namespace, resourceName+"-headless", ownerUID); err != nil {
		return err
	}

	return nil
}

// checkOrMarkGrayDelete checks whether the grace period for a gray-deleted resource has elapsed.
// Uses the StatefulSet (falling back to ConfigMap) as the primary resource to annotate.
// Only resources owned by ownerUID are annotated; foreign resources are skipped.
// Returns true if the resource should be deleted now, false if still within grace period.
func (c *RoleGroupCleaner) checkOrMarkGrayDelete(ctx context.Context, namespace, name string, ownerUID types.UID) (bool, error) {
	logger := log.FromContext(ctx)

	// Try StatefulSet first, then ConfigMap as fallback
	var primaryObj client.Object
	sts := &appsv1.StatefulSet{}
	if err := c.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, sts); err == nil {
		primaryObj = sts
	} else if !errors.IsNotFound(err) {
		return false, err
	} else {
		cm := &corev1.ConfigMap{}
		if err := c.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, cm); err == nil {
			primaryObj = cm
		} else if errors.IsNotFound(err) {
			// Resource already gone — allow deletion pass-through
			return true, nil
		} else {
			return false, err
		}
	}

	// Skip foreign resources to avoid mutating unrelated objects on name collision
	if !isOwnedByCluster(primaryObj.(metav1.Object), ownerUID) {
		logger.V(1).Info("Skipping gray-delete annotation: resource not owned by this cluster", "name", name)
		return false, nil
	}

	annotations := primaryObj.(metav1.Object).GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	markedAt, exists := annotations[AnnotationPendingDeletion]
	if !exists {
		// First detection: annotate and defer
		annotations[AnnotationPendingDeletion] = time.Now().UTC().Format(time.RFC3339)
		primaryObj.(metav1.Object).SetAnnotations(annotations)
		if err := c.Client.Update(ctx, primaryObj); err != nil {
			return false, fmt.Errorf("failed to mark resource for gray deletion: %w", err)
		}
		logger.Info("Marked orphaned resource for gray deletion", "name", name, "gracePeriod", c.grayDeleteGracePeriod)
		return false, nil
	}

	// Check if grace period has elapsed
	markedTime, err := time.Parse(time.RFC3339, markedAt)
	if err != nil {
		// Invalid timestamp — proceed with deletion
		return true, nil
	}

	if time.Since(markedTime) >= c.grayDeleteGracePeriod {
		logger.Info("Gray deletion grace period elapsed, proceeding with deletion", "name", name)
		return true, nil
	}

	logger.Info("Gray deletion grace period not yet elapsed", "name", name,
		"markedAt", markedAt, "gracePeriod", c.grayDeleteGracePeriod)
	return false, nil
}

// isOwnedByCluster returns true if the object's ownerReferences contain an entry
// matching ownerUID with controller=true.
// If ownerUID is empty, all resources are considered owned (backward compatible).
func isOwnedByCluster(obj metav1.Object, ownerUID types.UID) bool {
	if ownerUID == "" {
		return true
	}
	for _, ref := range obj.GetOwnerReferences() {
		if ref.UID == ownerUID && ref.Controller != nil && *ref.Controller {
			return true
		}
	}
	return false
}

// deletePDB deletes a PodDisruptionBudget if it exists and is owned by the cluster.
func (c *RoleGroupCleaner) deletePDB(ctx context.Context, namespace, name string, ownerUID types.UID) error {
	pdb := &policyv1.PodDisruptionBudget{}
	key := types.NamespacedName{Namespace: namespace, Name: name}

	err := c.Client.Get(ctx, key, pdb)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if !isOwnedByCluster(pdb, ownerUID) {
		log.FromContext(ctx).Info("Skipping PDB deletion: not owned by this cluster", "name", name)
		return nil
	}

	return c.Client.Delete(ctx, pdb)
}

// deleteStatefulSet deletes a StatefulSet if it exists and is owned by the cluster.
// If deletePVCs is true, PVCs associated with the StatefulSet's VolumeClaimTemplates are
// deleted BEFORE scaling to zero (while the replica count is still valid).
// Otherwise PVCs are preserved.
func (c *RoleGroupCleaner) deleteStatefulSet(ctx context.Context, namespace, name string, ownerUID types.UID, deletePVCs bool) error {
	sts := &appsv1.StatefulSet{}
	key := types.NamespacedName{Namespace: namespace, Name: name}

	err := c.Client.Get(ctx, key, sts)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if !isOwnedByCluster(sts, ownerUID) {
		log.FromContext(ctx).Info("Skipping StatefulSet deletion: not owned by this cluster", "name", name)
		return nil
	}

	// Delete PVCs BEFORE scaling to 0 (replica count is still valid at this point)
	if deletePVCs {
		if err := c.deletePVCsForStatefulSet(ctx, sts); err != nil {
			return err
		}
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

// deletePVCsForStatefulSet deletes PVCs associated with a StatefulSet by listing existing PVCs
// using the StatefulSet's pod selector labels. This is more reliable than deriving names from
// replica count, as it handles scaled-down StatefulSets and catches all existing PVCs regardless
// of current replica count.
func (c *RoleGroupCleaner) deletePVCsForStatefulSet(ctx context.Context, sts *appsv1.StatefulSet) error {
	if len(sts.Spec.VolumeClaimTemplates) == 0 {
		return nil
	}

	logger := log.FromContext(ctx)
	namespace := sts.Namespace

	// List PVCs matching the StatefulSet's pod selector
	pvcList := &corev1.PersistentVolumeClaimList{}
	if err := c.Client.List(ctx, pvcList,
		client.InNamespace(namespace),
		client.MatchingLabels(sts.Spec.Selector.MatchLabels),
	); err != nil {
		return fmt.Errorf("failed to list PVCs for StatefulSet %s/%s: %w", namespace, sts.Name, err)
	}

	for i := range pvcList.Items {
		pvc := &pvcList.Items[i]
		if err := c.Client.Delete(ctx, pvc); err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete PVC %s/%s: %w", namespace, pvc.Name, err)
		}
		logger.Info("Deleted PVC", "name", pvc.Name, "namespace", namespace)
	}
	return nil
}

// deleteConfigMap deletes a ConfigMap if it exists and is owned by the cluster.
func (c *RoleGroupCleaner) deleteConfigMap(ctx context.Context, namespace, name string, ownerUID types.UID) error {
	cm := &corev1.ConfigMap{}
	key := types.NamespacedName{Namespace: namespace, Name: name}

	err := c.Client.Get(ctx, key, cm)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if !isOwnedByCluster(cm, ownerUID) {
		log.FromContext(ctx).Info("Skipping ConfigMap deletion: not owned by this cluster", "name", name)
		return nil
	}

	return c.Client.Delete(ctx, cm)
}

// deleteService deletes a Service if it exists and is owned by the cluster.
func (c *RoleGroupCleaner) deleteService(ctx context.Context, namespace, name string, ownerUID types.UID) error {
	svc := &corev1.Service{}
	key := types.NamespacedName{Namespace: namespace, Name: name}

	err := c.Client.Get(ctx, key, svc)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if !isOwnedByCluster(svc, ownerUID) {
		log.FromContext(ctx).Info("Skipping Service deletion: not owned by this cluster", "name", name)
		return nil
	}

	return c.Client.Delete(ctx, svc)
}

// clearGrayDeleteAnnotation removes the AnnotationPendingDeletion annotation from a resource
// (StatefulSet or ConfigMap) if it is present. Only modifies resources owned by ownerUID
// to avoid mutating unrelated objects on name collision.
func (c *RoleGroupCleaner) clearGrayDeleteAnnotation(ctx context.Context, namespace, name string, ownerUID types.UID) error {
	// Try StatefulSet first
	sts := &appsv1.StatefulSet{}
	if err := c.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, sts); err == nil {
		if !isOwnedByCluster(sts, ownerUID) {
			return nil
		}
		if _, ok := sts.GetAnnotations()[AnnotationPendingDeletion]; ok {
			annotations := sts.GetAnnotations()
			delete(annotations, AnnotationPendingDeletion)
			sts.SetAnnotations(annotations)
			return c.Client.Update(ctx, sts)
		}
		return nil
	} else if !errors.IsNotFound(err) {
		return err
	}

	// Try ConfigMap as fallback
	cm := &corev1.ConfigMap{}
	if err := c.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, cm); err == nil {
		if !isOwnedByCluster(cm, ownerUID) {
			return nil
		}
		if _, ok := cm.GetAnnotations()[AnnotationPendingDeletion]; ok {
			annotations := cm.GetAnnotations()
			delete(annotations, AnnotationPendingDeletion)
			cm.SetAnnotations(annotations)
			return c.Client.Update(ctx, cm)
		}
	}
	return nil
}
