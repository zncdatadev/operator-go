package controller

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/zncdatadev/operator-go/pkg/framework"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	kubernetesjson "sigs.k8s.io/json"
)

const coordinationAnnotation = "framework.kubedoop.dev/workload-coordination"
const coordinationProgressAnnotation = "framework.kubedoop.dev/workload-progress"

type workloadProgress struct {
	Version     int         `json:"version"`
	UID         types.UID   `json:"uid"`
	Target      string      `json:"target"`
	Observation string      `json:"observation"`
	Since       metav1.Time `json:"since"`
}

func workloadPolicy(set *appsv1.StatefulSet) (*framework.WorkloadCoordination, error) {
	raw := set.Annotations[coordinationAnnotation]
	if raw == "" {
		return nil, nil
	}
	var value framework.WorkloadCoordination
	strict, err := kubernetesjson.UnmarshalStrict([]byte(raw), &value)
	if err != nil || len(strict) != 0 || value.ProgressDeadline.Duration < time.Second ||
		value.ProgressDeadline.Duration > time.Hour {
		return nil, fmt.Errorf("StatefulSet %s has an invalid workload coordination receipt", set.Name)
	}
	return &value, nil
}

func stampCoordination(desired client.Object, policies []*framework.WorkloadCoordination) client.Object {
	set, ok := desired.(*appsv1.StatefulSet)
	if !ok || len(policies) == 0 || policies[0] == nil {
		return desired
	}
	out := set.DeepCopy()
	if out.Annotations == nil {
		out.Annotations = map[string]string{}
	}
	raw, _ := json.Marshal(policies[0])
	out.Annotations[coordinationAnnotation] = string(raw)
	return out
}

// This query authenticates every canonical Pod against the live StatefulSet UID.
// A same-name foreign Pod blocks a transition rather than becoming deletion authority.
func coordinationPods(ctx context.Context, c client.Client, set *appsv1.StatefulSet) ([]corev1.Pod, error) {
	var list corev1.PodList
	if err := c.List(ctx, &list, client.InNamespace(set.Namespace)); err != nil {
		return nil, err
	}
	var pods []corev1.Pod
	for _, pod := range list.Items {
		suffix, ok := strings.CutPrefix(pod.Name, set.Name+"-")
		if !ok {
			continue
		}
		ordinal, err := strconv.ParseUint(suffix, 10, 32)
		if err != nil || strconv.FormatUint(ordinal, 10) != suffix {
			continue
		}
		owner := metav1.GetControllerOf(&pod)
		if owner == nil || owner.UID != set.UID || owner.Name != set.Name ||
			owner.Kind != statefulSetKind || owner.APIVersion != appsv1.SchemeGroupVersion.String() {
			return nil, fmt.Errorf("workload coordination: Pod %s lacks the current StatefulSet ownership identity", pod.Name)
		}
		pods = append(pods, pod)
	}
	slices.SortFunc(pods, func(a, b corev1.Pod) int { return strings.Compare(a.Name, b.Name) })
	return pods, nil
}

func replicas(set *appsv1.StatefulSet) int32 {
	if set.Spec.Replicas == nil {
		return 1
	}
	return *set.Spec.Replicas
}

// nextScaleDown is shared by ordinary changes, stop and retirement. A new step
// requires direct Pod absence for the previous ordinal, not status counts alone.
func nextScaleDown(ctx context.Context, c client.Client, live *appsv1.StatefulSet, want int32) (int32, error) {
	policy, err := workloadPolicy(live)
	if err != nil || policy == nil || replicas(live) <= want {
		return want, err
	}
	pods, err := coordinationPods(ctx, c, live)
	if err != nil {
		return replicas(live), err
	}
	for _, pod := range pods {
		ordinal, _ := strconv.ParseInt(strings.TrimPrefix(pod.Name, live.Name+"-"), 10, 32)
		if !pod.DeletionTimestamp.IsZero() || ordinal >= int64(replicas(live)) {
			return replicas(live), nil
		}
	}
	if live.Status.ObservedGeneration < live.Generation || live.Status.Replicas > replicas(live) {
		return replicas(live), nil
	}
	return replicas(live) - 1, nil
}

func coordinateDesired(ctx context.Context, c client.Client, next, previous client.Object) error {
	set, ok := next.(*appsv1.StatefulSet)
	if !ok {
		return nil
	}
	live := previous.(*appsv1.StatefulSet)
	policy, err := workloadPolicy(set)
	if err != nil || policy == nil {
		return err
	}
	// Enabling coordination takes one observed pass before a scale step; the live
	// receipt is the execution authority used equally by stop and retirement.
	if live.Annotations[coordinationAnnotation] == "" && replicas(live) > replicas(set) {
		set.Spec.Replicas = live.Spec.Replicas
		return nil
	}
	count, err := nextScaleDown(ctx, c, live, replicas(set))
	if err != nil {
		return err
	}
	set.Spec.Replicas = &count
	return nil
}

// Persist an observation deadline on the live StatefulSet. Neither a controller
// restart nor repeating a failed initializer resets it. A real progress milestone
// or new target starts a new budget. Timeout never issues force-deletion.
func (r *Reconciler[CR, C, S, F]) observeCoordination(
	ctx context.Context, cr CR, set *appsv1.StatefulSet, want int32, ready bool,
) error {
	policy, err := workloadPolicy(set)
	if err != nil || policy == nil {
		return err
	}
	raw := set.Annotations[coordinationProgressAnnotation]
	if ready {
		if raw == "" {
			return nil
		}
		patch := client.MergeFrom(set.DeepCopy())
		delete(set.Annotations, coordinationProgressAnnotation)
		return r.Client.Patch(ctx, set, patch)
	}
	pods, err := coordinationPods(ctx, r.Client, set)
	if err != nil {
		return err
	}
	// Do not include resourceVersion, failure/restart counters or wall clock in
	// progress: those change without advancing initialization or shutdown.
	observation := fmt.Sprintf("%d/%d/%d/%d/%s/%s", replicas(set), set.Status.Replicas, set.Status.ReadyReplicas,
		set.Status.UpdatedReplicas, set.Status.CurrentRevision, set.Status.UpdateRevision)
	for _, pod := range pods {
		observation += fmt.Sprintf(";%s/%s/%s/%t", pod.Name, pod.UID, pod.Status.Phase, !pod.DeletionTimestamp.IsZero())
		for _, status := range pod.Status.InitContainerStatuses {
			if status.State.Terminated != nil && status.State.Terminated.ExitCode == 0 {
				observation += "/initialized:" + status.Name
			}
		}
	}
	digest := sha256.Sum256([]byte(observation))
	template, _ := json.Marshal(set.Spec.Template)
	target := fmt.Sprintf("%d/%x", want, sha256.Sum256(template))
	var progress workloadProgress
	if raw != "" {
		strict, decodeErr := kubernetesjson.UnmarshalStrict([]byte(raw), &progress)
		if decodeErr != nil || len(strict) != 0 || progress.Version != 1 ||
			progress.UID != set.UID || progress.Since.IsZero() {
			return fmt.Errorf("StatefulSet %s has an invalid workload progress receipt", set.Name)
		}
	}
	token := fmt.Sprintf("%x", digest)
	if raw == "" || progress.Target != target || progress.Observation != token {
		progress = workloadProgress{Version: 1, UID: set.UID, Target: target, Observation: token, Since: metav1.Now()}
		data, _ := json.Marshal(progress)
		patch := client.MergeFrom(set.DeepCopy())
		set.Annotations = maps.Clone(set.Annotations)
		if set.Annotations == nil {
			set.Annotations = map[string]string{}
		}
		set.Annotations[coordinationProgressAnnotation] = string(data)
		if err := r.currentInput(ctx, cr); err != nil {
			return err
		}
		return r.Client.Patch(ctx, set, patch)
	}
	if time.Since(progress.Since.Time) >= policy.ProgressDeadline.Duration {
		return fmt.Errorf("workload coordination deadline exceeded for %s; target replicas=%d; "+
			"initialization, exit or rollout has not progressed; no forced deletion was issued", set.Name, want)
	}
	return nil
}

// Priority comes from authenticated live slots, so removal or invalid product
// configuration cannot discard a workload's previously declared shutdown order.
func (r *Reconciler[CR, C, S, F]) shutdownPriorities(
	ctx context.Context, cr CR, groups map[string]groupSlot,
) (map[string]int32, error) {
	priorities := map[string]int32{}
	for key, group := range groups {
		group.Slot = slotStatefulset
		object, err := r.readSlot(ctx, cr, group)
		if err != nil {
			return nil, err
		}
		if object == nil {
			continue
		}
		policy, err := workloadPolicy(object.(*appsv1.StatefulSet))
		if err != nil {
			return nil, err
		}
		if policy != nil {
			priorities[key] = policy.ShutdownPriority
		}
	}
	return priorities, nil
}
