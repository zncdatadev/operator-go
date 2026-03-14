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
	"github.com/zncdatadev/operator-go/pkg/common"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Default health check configuration.
const (
	DefaultCheckInterval = 120 * time.Second
	DefaultTimeout       = 300 * time.Second
)

// HealthManager manages health checks and status updates.
type HealthManager struct {
	Client             client.Client
	CheckInterval      time.Duration
	Timeout            time.Duration
	serviceHealthCheck common.ServiceHealthCheck
}

// NewHealthManager creates a new HealthManager.
func NewHealthManager(client client.Client) *HealthManager {
	return &HealthManager{
		Client:        client,
		CheckInterval: DefaultCheckInterval,
		Timeout:       DefaultTimeout,
	}
}

// WithServiceHealthCheck sets an optional product-level health check.
// When set, the check is executed after pod-level health is verified.
// If the service is not healthy, the cluster is marked as Degraded.
func (h *HealthManager) WithServiceHealthCheck(check common.ServiceHealthCheck) *HealthManager {
	h.serviceHealthCheck = check
	return h
}

// Check performs health checks and updates status.
func (h *HealthManager) Check(ctx context.Context, namespace, clusterName string, spec *v1alpha1.GenericClusterSpec, status *v1alpha1.GenericClusterStatus) error {
	logger := log.FromContext(ctx)

	// Check cluster operation
	if spec.ClusterOperation != nil {
		if spec.ClusterOperation.ReconciliationPaused {
			status.SetDegraded(true, v1alpha1.ReasonReconciliationPaused, "Reconciliation is paused")
			return nil
		}
		if spec.ClusterOperation.Stopped {
			status.SetUnavailable(v1alpha1.ReasonStopped, "Cluster is stopped")
			status.SetDegraded(false, v1alpha1.ReasonStopped, "Cluster is intentionally stopped")
			return nil
		}
	}

	// Check pod health for each role group
	allHealthy := true
	allAvailable := true
	progressing := false

	for roleName, roleSpec := range spec.Roles {
		for groupName, groupSpec := range roleSpec.RoleGroups {
			resourceName := fmt.Sprintf("%s-%s", clusterName, groupName)
			healthy, available, isProgressing, err := h.checkRoleGroupHealth(ctx, namespace, resourceName, groupSpec.GetReplicas())
			if err != nil {
				logger.Error(err, "Failed to check role group health", "role", roleName, "group", groupName)
				allHealthy = false
				continue
			}

			if !healthy {
				allHealthy = false
			}
			if !available {
				allAvailable = false
			}
			if isProgressing {
				progressing = true
			}
		}
	}

	// Update status conditions
	if allAvailable {
		status.SetAvailable(v1alpha1.ReasonAvailable, "All replicas are available")
	} else {
		status.SetUnavailable(v1alpha1.ReasonCreating, "Not all replicas are available")
	}

	if progressing {
		status.SetProgressing(true, v1alpha1.ReasonProgressing, "Cluster is progressing")
	} else {
		status.SetProgressing(false, v1alpha1.ReasonAvailable, "Cluster is stable")
	}

	if !allHealthy {
		status.SetDegraded(true, v1alpha1.ReasonDegraded, "Some replicas are unhealthy")
	} else {
		status.SetDegraded(false, v1alpha1.ReasonAvailable, "All replicas are healthy")
	}

	// Run product-level service health check if configured.
	if h.serviceHealthCheck != nil {
		healthy, err := h.serviceHealthCheck.CheckHealthy(ctx, h.Client, namespace, clusterName)
		if err != nil {
			logger.Error(err, "Service health check failed")
			status.SetDegraded(true, v1alpha1.ReasonDegraded, fmt.Sprintf("Service health check error: %v", err))
			status.SetServiceHealthy(false, v1alpha1.ReasonDegraded, fmt.Sprintf("Service health check error: %v", err))
			return nil
		}
		if !healthy {
			status.SetDegraded(true, v1alpha1.ReasonDegraded, "Service health check reported unhealthy")
			status.SetServiceHealthy(false, v1alpha1.ReasonDegraded, "Service is not healthy")
			return nil
		}
		status.SetServiceHealthy(true, v1alpha1.ReasonAvailable, "Service is healthy")
	}

	return nil
}

// checkRoleGroupHealth checks the health of a role group.
func (h *HealthManager) checkRoleGroupHealth(ctx context.Context, namespace, name string, expectedReplicas int32) (healthy, available, progressing bool, err error) {
	// Get StatefulSet
	sts := &appsv1.StatefulSet{}
	key := types.NamespacedName{Namespace: namespace, Name: name}

	if err = h.Client.Get(ctx, key, sts); err != nil {
		available = false
		healthy = false
		return
	}

	// Check availability
	readyReplicas := sts.Status.ReadyReplicas
	replicas := sts.Status.Replicas

	available = readyReplicas >= expectedReplicas

	// Check if progressing (update in progress)
	progressing = sts.Status.CurrentRevision != sts.Status.UpdateRevision ||
		sts.Status.CurrentReplicas != replicas

	// Check health (all pods ready)
	healthy = readyReplicas == expectedReplicas && expectedReplicas > 0

	return
}

// CheckPodHealth checks the health of individual pods.
func (h *HealthManager) CheckPodHealth(ctx context.Context, namespace string, labels map[string]string) (int, int, error) {
	podList := &corev1.PodList{}
	if err := h.Client.List(ctx, podList,
		client.InNamespace(namespace),
		client.MatchingLabels(labels),
	); err != nil {
		return 0, 0, err
	}

	total := len(podList.Items) // nolint:prealloc
	ready := 0

	for _, pod := range podList.Items {
		if h.isPodReady(&pod) {
			ready++
		}
	}

	return total, ready, nil
}

// isPodReady checks if a pod is ready.
func (h *HealthManager) isPodReady(pod *corev1.Pod) bool {
	if pod.Status.Phase != corev1.PodRunning {
		return false
	}

	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady {
			return cond.Status == corev1.ConditionTrue
		}
	}

	return false
}

// UpdateStatusCondition updates a specific status condition.
func (h *HealthManager) UpdateStatusCondition(status *v1alpha1.GenericClusterStatus, conditionType v1alpha1.ConditionType, statusValue metav1.ConditionStatus, reason, message string) {
	status.SetCondition(metav1.Condition{
		Type:               string(conditionType),
		Status:             statusValue,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.NewTime(time.Now()),
	})
}
