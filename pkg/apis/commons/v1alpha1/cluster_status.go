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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConditionType represents the type of cluster condition.
type ConditionType string

const (
	// ConditionAvailable indicates that at least one replica is ready and serving traffic.
	ConditionAvailable ConditionType = "Available"

	// ConditionProgressing indicates that the cluster is rolling out a new version or scaling replicas.
	ConditionProgressing ConditionType = "Progressing"

	// ConditionDegraded indicates that something is wrong that the operator cannot resolve on its
	// own — a pod wedged in CrashLoopBackOff or ImagePullBackOff, a pod that cannot be scheduled, a
	// StatefulSet that cannot be read, a failing application health check.
	//
	// It deliberately does NOT mean "not all replicas are ready". A rolling update, a scale-up and
	// a scale-down all pass through that state on purpose, and reporting Degraded for them makes
	// the one condition an operator would alert on fire during every routine change — after which
	// it stops being alerted on at all. "Not serving" is ConditionAvailable and "changing" is
	// ConditionProgressing; this condition is for "a human needs to look".
	ConditionDegraded ConditionType = "Degraded"

	// ConditionPaused indicates that spec.clusterOperation.reconciliationPaused is set, so the
	// framework is deliberately not reconciling this cluster.
	//
	// It exists so that a paused cluster does not have to be reported as Degraded. Pausing is an
	// administrator's decision — a maintenance window, an investigation — and folding it into the
	// fault signal pages someone for a planned action. The framework already draws that distinction
	// for the sibling operation, `stopped`, which reports Degraded=False and "intentionally
	// stopped"; the two are the same class of state and now read the same way.
	ConditionPaused ConditionType = "Paused"

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
	// ReasonPodsNotReady indicates fewer replicas are ready than the spec asks for. It is a reason
	// for Available=False, never for Degraded: converging is not a fault.
	ReasonPodsNotReady = "PodsNotReady"
	// ReasonPodFailure indicates at least one pod is wedged in a state the operator cannot resolve
	// (CrashLoopBackOff, an image that cannot be pulled, nowhere to schedule it).
	ReasonPodFailure = "PodFailure"
	// ReasonWorkloadUnreadable indicates a role group's StatefulSet could not be read at all.
	ReasonWorkloadUnreadable = "WorkloadUnreadable"
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
// If a condition of that type already exists, Reason, Message and ObservedGeneration are
// updated in place, and LastTransitionTime is preserved unless Status actually changed.
// Rewriting LastTransitionTime on every call would make each reconcile produce a different
// status object, which triggers a new watch event and hence an endless reconcile loop.
//
// A zero LastTransitionTime is stamped with the current time, and a zero ObservedGeneration
// inherits GenericClusterStatus.ObservedGeneration, so callers only need to supply
// Type, Status, Reason and Message.
func (s *GenericClusterStatus) SetCondition(condition metav1.Condition) {
	if condition.LastTransitionTime.IsZero() {
		condition.LastTransitionTime = metav1.Now()
	}
	if condition.ObservedGeneration == 0 {
		condition.ObservedGeneration = s.ObservedGeneration
	}

	existing := s.GetCondition(ConditionType(condition.Type))
	if existing == nil {
		s.Conditions = append(s.Conditions, condition)
		return
	}

	if existing.Status != condition.Status {
		existing.Status = condition.Status
		existing.LastTransitionTime = condition.LastTransitionTime
	}
	existing.Reason = condition.Reason
	existing.Message = condition.Message
	existing.ObservedGeneration = condition.ObservedGeneration
}

// SetObservedGeneration records the CR generation this status was computed from. Conditions set
// afterwards inherit it, so call it before the Set* condition helpers. It means "observed", not
// "successfully reconciled": whether the cycle actually succeeded is carried by the
// ReconcileComplete and Degraded conditions, which are the pair consumers should gate on.
func (s *GenericClusterStatus) SetObservedGeneration(generation int64) {
	s.ObservedGeneration = generation
}

// SetAvailable sets the Available condition to True.
func (s *GenericClusterStatus) SetAvailable(reason, message string) {
	s.SetCondition(metav1.Condition{
		Type:    string(ConditionAvailable),
		Status:  metav1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})
}

// SetUnavailable sets the Available condition to False.
func (s *GenericClusterStatus) SetUnavailable(reason, message string) {
	s.SetCondition(metav1.Condition{
		Type:    string(ConditionAvailable),
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	})
}

// SetProgressing sets the Progressing condition.
func (s *GenericClusterStatus) SetProgressing(isProgressing bool, reason, message string) {
	status := metav1.ConditionFalse
	if isProgressing {
		status = metav1.ConditionTrue
	}
	s.SetCondition(metav1.Condition{
		Type:    string(ConditionProgressing),
		Status:  status,
		Reason:  reason,
		Message: message,
	})
}

// SetDegraded sets the Degraded condition.
func (s *GenericClusterStatus) SetDegraded(isDegraded bool, reason, message string) {
	status := metav1.ConditionFalse
	if isDegraded {
		status = metav1.ConditionTrue
	}
	s.SetCondition(metav1.Condition{
		Type:    string(ConditionDegraded),
		Status:  status,
		Reason:  reason,
		Message: message,
	})
}

// SetPaused sets the Paused condition. It must be written on every pass — including to False — or a
// cluster that was un-paused keeps advertising the pause forever.
func (s *GenericClusterStatus) SetPaused(isPaused bool, reason, message string) {
	status := metav1.ConditionFalse
	if isPaused {
		status = metav1.ConditionTrue
	}
	s.SetCondition(metav1.Condition{
		Type:    string(ConditionPaused),
		Status:  status,
		Reason:  reason,
		Message: message,
	})
}

// SetServiceHealthyUnknown records that the application-level health check was deliberately not
// run, so the condition neither claims health nor reports a fault. Used while reconciliation is
// paused: a ServiceHealthCheck is an active probe against the product, and leaving the previous
// verdict in place would let a stale True outlive whatever the cluster is doing during the pause.
func (s *GenericClusterStatus) SetServiceHealthyUnknown(reason, message string) {
	s.SetCondition(metav1.Condition{
		Type:    string(ConditionServiceHealthy),
		Status:  metav1.ConditionUnknown,
		Reason:  reason,
		Message: message,
	})
}

// SetServiceHealthy sets the ServiceHealthy condition.
func (s *GenericClusterStatus) SetServiceHealthy(isHealthy bool, reason, message string) {
	status := metav1.ConditionFalse
	if isHealthy {
		status = metav1.ConditionTrue
	}
	s.SetCondition(metav1.Condition{
		Type:    string(ConditionServiceHealthy),
		Status:  status,
		Reason:  reason,
		Message: message,
	})
}

// SetReconcileComplete sets the ReconcileComplete condition.
func (s *GenericClusterStatus) SetReconcileComplete(isComplete bool, reason, message string) {
	status := metav1.ConditionFalse
	if isComplete {
		status = metav1.ConditionTrue
	}
	s.SetCondition(metav1.Condition{
		Type:    string(ConditionReconcileComplete),
		Status:  status,
		Reason:  reason,
		Message: message,
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

// IsPaused returns true if the Paused condition is True.
func (s *GenericClusterStatus) IsPaused() bool {
	cond := s.GetCondition(ConditionPaused)
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
