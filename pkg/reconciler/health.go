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
	"sort"
	"strings"
	"time"

	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/common"
	"github.com/zncdatadev/operator-go/pkg/constant"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
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
		CheckInterval: DefaultHealthCheckInterval,
		Timeout:       DefaultHealthCheckTimeout,
	}
}

// WithServiceHealthCheck sets an optional product-level health check.
// When set, the check is executed after pod-level health is verified.
// If the service is not healthy, the cluster is marked as Degraded.
func (h *HealthManager) WithServiceHealthCheck(check common.ServiceHealthCheck) *HealthManager {
	h.serviceHealthCheck = check
	return h
}

// Check evaluates the cluster's workloads and writes the Available, Progressing, Degraded,
// ServiceHealthy and Paused conditions. It reads only; nothing here mutates a cluster resource.
//
// The three workload conditions answer three different questions, and keeping them apart is the
// whole point of this function:
//
//   - Available — are at least as many replicas ready as the spec asks for? "Can it serve?"
//   - Progressing — is a revision rollout or a replica change in flight? "Is it changing?"
//   - Degraded — is something wrong that the operator cannot fix on its own? "Must a human look?"
//
// Degraded used to be derived from the replica counts, which conflated it with the other two: a
// rolling update, a scale-up and a scale-down all reduce ready replicas on purpose, so every routine
// change reported Degraded=True — a scale-down reported it with Progressing=False, so nothing in the
// status even hinted that the cluster was mid-operation. A signal that fires on every planned change
// is one nobody can alert on. Degraded is therefore computed from *failure states* instead: a pod
// wedged in CrashLoopBackOff or unable to pull its image, a pod nowhere to schedule, a StatefulSet
// that cannot be read, or a failing application health check. Those are state-based, not
// time-based, so a stuck rollout still reports Degraded=True — the pods are visibly failing — while
// a healthy rollout does not.
func (h *HealthManager) Check(ctx context.Context, namespace, clusterName string, spec *v1alpha1.GenericClusterSpec, status *v1alpha1.GenericClusterStatus) error {
	logger := log.FromContext(ctx)

	paused := spec.ClusterOperation != nil && spec.ClusterOperation.ReconciliationPaused

	if spec.ClusterOperation != nil && spec.ClusterOperation.Stopped && !paused {
		status.SetUnavailable(v1alpha1.ReasonStopped, "Cluster is stopped")
		status.SetDegraded(false, v1alpha1.ReasonStopped, "Cluster is intentionally stopped")
		// Every condition this pass can write has to be written, including on the early
		// returns: a condition left untouched keeps whatever the last running cycle put
		// there, so a cluster stopped mid-rollout would advertise Progressing=True (and a
		// healthy service) for as long as it stays stopped.
		status.SetProgressing(false, v1alpha1.ReasonStopped, "Cluster is stopped")
		status.SetPaused(false, v1alpha1.ReasonStopped, "Cluster is stopped, not paused")
		if h.serviceHealthCheck != nil {
			status.SetServiceHealthy(false, v1alpha1.ReasonStopped, "Cluster is stopped")
		}
		return nil
	}

	// Evaluate every role group's workload. The two ways a role group can be unavailable are kept
	// apart, because they need different words: one has replica counts to compare, the other has no
	// StatefulSet to read at all.
	progressing := false
	evaluated := 0
	var shortOfReplicas, unreadable []string

	for roleName, roleSpec := range spec.Roles {
		for groupName, groupSpec := range roleSpec.RoleGroups {
			evaluated++
			resourceName := RoleGroupResourceName(clusterName, roleName, groupName)
			available, isProgressing, err := h.checkRoleGroupHealth(ctx, namespace, resourceName, groupSpec.GetReplicas())
			if err != nil {
				// A role group whose StatefulSet cannot even be read is both not available and a
				// genuine fault: the object the operator applied is gone or unreachable.
				logger.Error(err, "Failed to check role group health", "role", roleName, "group", groupName)
				unreadable = append(unreadable, roleName+"/"+groupName)
				continue
			}

			if !available {
				shortOfReplicas = append(shortOfReplicas, roleName+"/"+groupName)
			}
			if isProgressing {
				progressing = true
			}
		}
	}

	sort.Strings(shortOfReplicas)
	sort.Strings(unreadable)

	switch {
	case evaluated == 0:
		// No role group was evaluated, so nothing runs. Reporting "all replicas are available"
		// for a cluster with zero workloads would make Available useless as a readiness gate.
		status.SetUnavailable(v1alpha1.ReasonCreating, "Cluster declares no role groups")
	case len(shortOfReplicas) == 0 && len(unreadable) == 0:
		status.SetAvailable(v1alpha1.ReasonAvailable, "All replicas are available")
	default:
		// Name the offenders, and say which kind of problem each one is: with several roles the
		// generic message forced an operator to go looking, and calling an unreadable StatefulSet
		// "short of ready replicas" would send them looking for replica counts that do not exist.
		var parts []string
		if len(shortOfReplicas) > 0 {
			parts = append(parts, "fewer ready replicas than desired: "+strings.Join(shortOfReplicas, ", "))
		}
		if len(unreadable) > 0 {
			parts = append(parts, "StatefulSet could not be read: "+strings.Join(unreadable, ", "))
		}
		reason := v1alpha1.ReasonPodsNotReady
		if len(shortOfReplicas) == 0 {
			reason = v1alpha1.ReasonWorkloadUnreadable
		}
		status.SetUnavailable(reason, "Role groups — "+strings.Join(parts, "; "))
	}

	if progressing {
		status.SetProgressing(true, v1alpha1.ReasonProgressing, "Cluster is progressing")
	} else {
		status.SetProgressing(false, v1alpha1.ReasonAvailable, "Cluster is stable")
	}

	// The Degraded verdict is computed here and written ONCE at the end of the pass. Writing it
	// twice — clear after the pod check, set again after the service check — would flip the
	// condition's status within a single pass, and SetCondition re-stamps LastTransitionTime on
	// every flip. The status would then differ on every reconcile, defeating the no-op guard in
	// updateStatus and making the controller reschedule itself indefinitely.
	degraded := false
	degradedReason := v1alpha1.ReasonAvailable
	degradedMessage := "No failing pods"

	if len(unreadable) > 0 {
		degraded = true
		degradedReason = v1alpha1.ReasonWorkloadUnreadable
		degradedMessage = fmt.Sprintf("StatefulSet could not be read for role groups: %s", strings.Join(unreadable, ", "))
	} else if failures, err := h.findFailingPods(ctx, namespace, clusterName); err != nil {
		// Not being able to list pods says nothing about the cluster, so it must not be reported as
		// the cluster's fault. It is logged and the verdict falls back to "no failures observed".
		logger.Error(err, "Failed to list pods while evaluating cluster health")
	} else if len(failures) > 0 {
		degraded = true
		degradedReason = v1alpha1.ReasonPodFailure
		degradedMessage = summarizePodFailures(failures)
	}

	// Run product-level service health check if configured. It calls out to the product (an
	// HDFS SafeMode probe, an HTTP readiness endpoint, ...), so it runs under a deadline: an
	// unbounded probe would pin a reconcile worker for as long as the remote side hangs.
	//
	// Skipped while paused: it is an active probe, and a paused cluster is one an administrator has
	// asked the operator to leave alone. The condition goes Unknown rather than keeping the last
	// verdict, which would otherwise outlive whatever happens during the pause.
	switch {
	case h.serviceHealthCheck == nil:
	case paused:
		status.SetServiceHealthyUnknown(v1alpha1.ReasonReconciliationPaused,
			"Not probed while reconciliation is paused")
	default:
		checkCtx := ctx
		if h.Timeout > 0 {
			var cancel context.CancelFunc
			checkCtx, cancel = context.WithTimeout(ctx, h.Timeout)
			defer cancel()
		}
		healthy, err := h.serviceHealthCheck.CheckHealthy(checkCtx, h.Client, namespace, clusterName)
		switch {
		case err != nil:
			logger.Error(err, "Service health check failed")
			message := fmt.Sprintf("Service health check error: %v", err)
			degraded, degradedReason, degradedMessage = true, v1alpha1.ReasonDegraded, message
			status.SetServiceHealthy(false, v1alpha1.ReasonDegraded, message)
		case !healthy:
			degraded, degradedReason, degradedMessage = true, v1alpha1.ReasonDegraded, "Service health check reported unhealthy"
			status.SetServiceHealthy(false, v1alpha1.ReasonDegraded, "Service is not healthy")
		default:
			status.SetServiceHealthy(true, v1alpha1.ReasonAvailable, "Service is healthy")
		}
	}

	// Pausing is an administrator's decision, so it gets its own condition and clears Degraded.
	// Reporting a maintenance window as a fault pages someone for a planned action — and the
	// framework already treats the sibling operation, `stopped`, exactly this way.
	if paused {
		status.SetPaused(true, v1alpha1.ReasonReconciliationPaused, "Reconciliation is paused")
		status.SetDegraded(false, v1alpha1.ReasonReconciliationPaused,
			"Reconciliation is paused; the cluster is not being reconciled")
		return nil
	}

	status.SetPaused(false, v1alpha1.ReasonReconcileComplete, "Reconciliation is active")
	status.SetDegraded(degraded, degradedReason, degradedMessage)

	return nil
}

// checkRoleGroupHealth reports whether a role group has at least as many ready replicas as the spec
// asks for, and whether the StatefulSet controller is mid-change.
//
// available uses >= rather than ==. A role group scaled DOWN briefly reports more ready replicas
// than desired while the extra pods terminate, and that is not a problem — the previous `==` test
// made a plain scale-down look unhealthy, with no rollout in flight to explain it. A role group
// scaled to 0 on purpose is available at 0 ready replicas for the same reason.
func (h *HealthManager) checkRoleGroupHealth(ctx context.Context, namespace, name string, expectedReplicas int32) (available, progressing bool, err error) {
	sts := &appsv1.StatefulSet{}
	key := types.NamespacedName{Namespace: namespace, Name: name}

	if err = h.Client.Get(ctx, key, sts); err != nil {
		return false, false, err
	}

	available = sts.Status.ReadyReplicas >= expectedReplicas

	// A revision rollout, or a replica count the controller has not finished applying.
	progressing = sts.Status.CurrentRevision != sts.Status.UpdateRevision ||
		sts.Status.CurrentReplicas != sts.Status.Replicas

	return available, progressing, nil
}

// stuckContainerReasons are the container `waiting.reason` values that mean the kubelet has given
// up on its own: retrying will not help, so a human has to change something. Transient startup
// reasons (ContainerCreating, PodInitializing) are deliberately absent — they are what a healthy
// pod looks like for its first seconds, and treating them as faults would put every rollout back
// into Degraded, which is the bug this list exists to avoid.
var stuckContainerReasons = map[string]struct{}{
	"CrashLoopBackOff":           {},
	"ImagePullBackOff":           {},
	"ErrImagePull":               {},
	"InvalidImageName":           {},
	"CreateContainerConfigError": {},
	"CreateContainerError":       {},
	"RunContainerError":          {},
}

// podFailure is one pod the operator cannot help.
type podFailure struct {
	pod    string
	reason string
}

// findFailingPods lists the cluster's pods and returns those wedged in a state the operator cannot
// resolve. It is one List for the whole cluster, matched on the framework's own identity labels.
//
// This is what makes Degraded independent of whether a rollout is in flight: a bad image reports
// Degraded=True from the first pod that cannot pull it, even though the StatefulSet is still
// Progressing, while a healthy rollout produces no failures at all.
func (h *HealthManager) findFailingPods(ctx context.Context, namespace, clusterName string) ([]podFailure, error) {
	podList := &corev1.PodList{}
	if err := h.Client.List(ctx, podList,
		client.InNamespace(namespace),
		client.MatchingLabels{
			constant.LabelKubernetesInstance:  clusterName,
			constant.LabelKubernetesManagedBy: managedByValue,
		},
	); err != nil {
		return nil, err
	}

	var failures []podFailure
	for i := range podList.Items {
		pod := &podList.Items[i]
		// A pod on its way out is not a fault, whoever asked for it to go.
		if !pod.DeletionTimestamp.IsZero() {
			continue
		}
		if reason := podFailureReason(pod); reason != "" {
			failures = append(failures, podFailure{pod: pod.Name, reason: reason})
		}
	}

	sort.Slice(failures, func(i, j int) bool { return failures[i].pod < failures[j].pod })
	return failures, nil
}

// podFailureReason returns why a pod is stuck, or "" when it is not.
func podFailureReason(pod *corev1.Pod) string {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodScheduled && cond.Status == corev1.ConditionFalse &&
			cond.Reason == corev1.PodReasonUnschedulable {
			return corev1.PodReasonUnschedulable
		}
	}
	// Init and sidecar containers count: a pod whose init container cannot pull its image never
	// reaches its main container at all.
	for _, statuses := range [][]corev1.ContainerStatus{
		pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses,
	} {
		for i := range statuses {
			waiting := statuses[i].State.Waiting
			if waiting == nil {
				continue
			}
			if _, stuck := stuckContainerReasons[waiting.Reason]; stuck {
				return waiting.Reason
			}
		}
	}
	return ""
}

// maxReportedPodFailures bounds the Degraded message. A whole role group failing the same way is
// one problem, and a condition message listing 200 pods is unreadable in `kubectl describe`.
const maxReportedPodFailures = 3

// summarizePodFailures renders the Degraded message: the offending pods with their reasons, capped,
// and never silently truncated — the remainder is counted.
func summarizePodFailures(failures []podFailure) string {
	shown := failures
	if len(shown) > maxReportedPodFailures {
		shown = shown[:maxReportedPodFailures]
	}
	parts := make([]string, 0, len(shown))
	for _, f := range shown {
		parts = append(parts, fmt.Sprintf("%s (%s)", f.pod, f.reason))
	}
	message := "Pods requiring attention: " + strings.Join(parts, ", ")
	if remaining := len(failures) - len(shown); remaining > 0 {
		message += fmt.Sprintf(", and %d more", remaining)
	}
	return message
}
