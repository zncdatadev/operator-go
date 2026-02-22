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

package v1alpha1

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConditionType represents the type of cluster condition.
type ConditionType string

const (
	// ConditionAvailable indicates that at least one replica is ready and serving traffic.
	ConditionAvailable ConditionType = "Available"

	// ConditionProgressing indicates that the cluster is rolling out a new version or scaling replicas.
	ConditionProgressing ConditionType = "Progressing"

	// ConditionDegraded indicates that the cluster is experiencing issues
	// (e.g., missing dependencies, crash loops, health check failures).
	ConditionDegraded ConditionType = "Degraded"

	// ConditionServiceHealthy indicates that the application-level health check passed
	// (e.g., HDFS SafeMode off, RegionServer registered).
	ConditionServiceHealthy ConditionType = "ServiceHealthy"

	// ConditionReconcileComplete indicates that the SDK has finished the latest reconciliation loop successfully.
	ConditionReconcileComplete ConditionType = "ReconcileComplete"
)

// Condition reasons for common scenarios.
const (
	// ReasonCreating indicates the cluster is being created.
	ReasonCreating = "Creating"
	// ReasonUpdating indicates the cluster is being updated.
	ReasonUpdating = "Updating"
	// ReasonDeleting indicates the cluster is being deleted.
	ReasonDeleting = "Deleting"
	// ReasonAvailable indicates the cluster is available.
	ReasonAvailable = "Available"
	// ReasonProgressing indicates the cluster is progressing.
	ReasonProgressing = "Progressing"
	// ReasonDegraded indicates the cluster is degraded.
	ReasonDegraded = "Degraded"
	// ReasonServiceHealthy indicates the service is healthy.
	ReasonServiceHealthy = "ServiceHealthy"
	// ReasonServiceUnhealthy indicates the service is unhealthy.
	ReasonServiceUnhealthy = "ServiceUnhealthy"
	// ReasonReconcileComplete indicates reconciliation is complete.
	ReasonReconcileComplete = "ReconcileComplete"
	// ReasonReconcileError indicates reconciliation encountered an error.
	ReasonReconcileError = "ReconcileError"
	// ReasonDependencyMissing indicates a required dependency is missing.
	ReasonDependencyMissing = "DependencyMissing"
	// ReasonReconciliationPaused indicates reconciliation is paused.
	ReasonReconciliationPaused = "ReconciliationPaused"
	// ReasonStopped indicates the cluster is stopped.
	ReasonStopped = "Stopped"
)

// GenericClusterStatus defines the observed state of a cluster.
// Product-specific statuses should embed this struct to inherit common functionality.
type GenericClusterStatus struct {
	// Conditions represent the latest available observations of the cluster state.
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:Optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// RoleGroups tracks the actual deployed role groups.
	// Map key is the role name, value is the list of role group names.
	// This is used for orphaned resource cleanup.
	// +kubebuilder:validation:Optional
	RoleGroups map[string][]string `json:"roleGroups,omitempty"`

	// ObservedGeneration is the most recent generation observed for this cluster.
	// It corresponds to the metadata generation of the CR.
	// +kubebuilder:validation:Optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// GetCondition returns the condition with the given type, or nil if not found.
func (s *GenericClusterStatus) GetCondition(conditionType ConditionType) *metav1.Condition {
	for i := range s.Conditions {
		if s.Conditions[i].Type == string(conditionType) {
			return &s.Conditions[i]
		}
	}
	return nil
}

// SetCondition sets the condition with the given type.
// If a condition of that type already exists, it is updated.
func (s *GenericClusterStatus) SetCondition(condition metav1.Condition) {
	// Ensure the conditions slice is initialized
	if s.Conditions == nil {
		s.Conditions = make([]metav1.Condition, 0)
	}

	// Find existing condition
	for i := range s.Conditions {
		if s.Conditions[i].Type == condition.Type {
			s.Conditions[i] = condition
			return
		}
	}

	// Add new condition
	s.Conditions = append(s.Conditions, condition)
}

// SetAvailable sets the Available condition to True.
func (s *GenericClusterStatus) SetAvailable(reason, message string) {
	s.SetCondition(metav1.Condition{
		Type:               string(ConditionAvailable),
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.NewTime(time.Now()),
	})
}

// SetUnavailable sets the Available condition to False.
func (s *GenericClusterStatus) SetUnavailable(reason, message string) {
	s.SetCondition(metav1.Condition{
		Type:               string(ConditionAvailable),
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.NewTime(time.Now()),
	})
}

// SetProgressing sets the Progressing condition.
func (s *GenericClusterStatus) SetProgressing(isProgressing bool, reason, message string) {
	status := metav1.ConditionFalse
	if isProgressing {
		status = metav1.ConditionTrue
	}
	s.SetCondition(metav1.Condition{
		Type:               string(ConditionProgressing),
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.NewTime(time.Now()),
	})
}

// SetDegraded sets the Degraded condition.
func (s *GenericClusterStatus) SetDegraded(isDegraded bool, reason, message string) {
	status := metav1.ConditionFalse
	if isDegraded {
		status = metav1.ConditionTrue
	}
	s.SetCondition(metav1.Condition{
		Type:               string(ConditionDegraded),
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.NewTime(time.Now()),
	})
}

// SetServiceHealthy sets the ServiceHealthy condition.
func (s *GenericClusterStatus) SetServiceHealthy(isHealthy bool, reason, message string) {
	status := metav1.ConditionFalse
	if isHealthy {
		status = metav1.ConditionTrue
	}
	s.SetCondition(metav1.Condition{
		Type:               string(ConditionServiceHealthy),
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.NewTime(time.Now()),
	})
}

// SetReconcileComplete sets the ReconcileComplete condition.
func (s *GenericClusterStatus) SetReconcileComplete(isComplete bool, reason, message string) {
	status := metav1.ConditionFalse
	if isComplete {
		status = metav1.ConditionTrue
	}
	s.SetCondition(metav1.Condition{
		Type:               string(ConditionReconcileComplete),
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.NewTime(time.Now()),
	})
}

// IsAvailable returns true if the Available condition is True.
func (s *GenericClusterStatus) IsAvailable() bool {
	cond := s.GetCondition(ConditionAvailable)
	return cond != nil && cond.Status == metav1.ConditionTrue
}

// IsDegraded returns true if the Degraded condition is True.
func (s *GenericClusterStatus) IsDegraded() bool {
	cond := s.GetCondition(ConditionDegraded)
	return cond != nil && cond.Status == metav1.ConditionTrue
}

// IsProgressing returns true if the Progressing condition is True.
func (s *GenericClusterStatus) IsProgressing() bool {
	cond := s.GetCondition(ConditionProgressing)
	return cond != nil && cond.Status == metav1.ConditionTrue
}

// IsServiceHealthy returns true if the ServiceHealthy condition is True.
func (s *GenericClusterStatus) IsServiceHealthy() bool {
	cond := s.GetCondition(ConditionServiceHealthy)
	return cond != nil && cond.Status == metav1.ConditionTrue
}

// IsReconcileComplete returns true if the ReconcileComplete condition is True.
func (s *GenericClusterStatus) IsReconcileComplete() bool {
	cond := s.GetCondition(ConditionReconcileComplete)
	return cond != nil && cond.Status == metav1.ConditionTrue
}

// GetRoleGroups returns the map of role names to role group names.
func (s *GenericClusterStatus) GetRoleGroups() map[string][]string {
	if s.RoleGroups == nil {
		return make(map[string][]string)
	}
	return s.RoleGroups
}

// SetRoleGroup sets a role group in the status.
func (s *GenericClusterStatus) SetRoleGroup(roleName, roleGroupName string) {
	if s.RoleGroups == nil {
		s.RoleGroups = make(map[string][]string)
	}

	groups, exists := s.RoleGroups[roleName]
	if !exists {
		groups = make([]string, 0)
	}

	// Check if already exists
	for _, g := range groups {
		if g == roleGroupName {
			return
		}
	}

	s.RoleGroups[roleName] = append(groups, roleGroupName)
}

// RemoveRoleGroup removes a role group from the status.
func (s *GenericClusterStatus) RemoveRoleGroup(roleName, roleGroupName string) {
	if s.RoleGroups == nil {
		return
	}

	groups, exists := s.RoleGroups[roleName]
	if !exists {
		return
	}

	newGroups := make([]string, 0, len(groups))
	for _, g := range groups {
		if g != roleGroupName {
			newGroups = append(newGroups, g)
		}
	}

	if len(newGroups) == 0 {
		delete(s.RoleGroups, roleName)
	} else {
		s.RoleGroups[roleName] = newGroups
	}
}

// GetOrphanedRoleGroups returns role groups that exist in status but not in the desired spec.
func (s *GenericClusterStatus) GetOrphanedRoleGroups(desiredRoles map[string]RoleSpec) map[string][]string {
	orphaned := make(map[string][]string)

	for roleName, actualGroups := range s.GetRoleGroups() {
		desiredRole, exists := desiredRoles[roleName]
		if !exists {
			// Entire role is orphaned
			orphaned[roleName] = actualGroups
			continue
		}

		// Check for orphaned role groups
		desiredGroups := desiredRole.GetRoleGroups()
		var orphanedGroups []string
		for _, groupName := range actualGroups {
			if _, exists := desiredGroups[groupName]; !exists {
				orphanedGroups = append(orphanedGroups, groupName)
			}
		}
		if len(orphanedGroups) > 0 {
			orphaned[roleName] = orphanedGroups
		}
	}

	return orphaned
}
