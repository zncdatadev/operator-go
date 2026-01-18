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
	"time"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/client"
	batchv1 "k8s.io/api/batch/v1"
)

var _ ResourceReconciler[builder.JobBuilder] = &Job{}

type Job struct {
	*GenericResourceReconciler[builder.JobBuilder]

	// ReadyRequeueAfter is the duration after which to requeue when checking readiness.
	// Default is 5 seconds.
	ReadyRequeueAfter time.Duration
}

// SetRequeueAfter sets the requeue duration for job reconciliation
func (j *Job) SetRequeueAfter(duration time.Duration) {
	j.RequeueAfter = duration
}

// SetReadyRequeueAfter sets the requeue duration for job readiness checks
func (j *Job) SetReadyRequeueAfter(duration time.Duration) {
	j.ReadyRequeueAfter = duration
}

func (r *Job) Reconcile(ctx context.Context) (ctrl.Result, error) {
	// TODO: Extract a doBuild method to invoke the implementation side's Build method and append some framework logic.
	// Consider abstracting a WorkloadReconciler on top of DeploymentReconciler to extract some of the logic into it.
	resourceBuilder := r.GetBuilder()

	resource, err := resourceBuilder.Build(ctx)

	if err != nil {
		return ctrl.Result{}, err
	}
	return r.ResourceReconcile(ctx, resource)
}

func (r *Job) Ready(ctx context.Context) (ctrl.Result, error) {

	obj := batchv1.Job{
		ObjectMeta: r.GetObjectMeta(),
	}
	logger.V(1).Info("Checking job ready", "namespace", obj.Namespace, "name", obj.Name)
	if err := r.Client.GetWithObject(ctx, &obj); err != nil {
		return ctrl.Result{}, err
	}
	if obj.Status.Succeeded == *obj.Spec.Parallelism {
		logger.Info("Job is ready", "namespace", obj.Namespace, "name", obj.Name, "Parallelism", *obj.Spec.Parallelism, "succeeded", obj.Status.Succeeded)
		return ctrl.Result{}, nil
	}
	logger.Info("Job is not ready", "namespace", obj.Namespace, "name", obj.Name, "Parallelism", *obj.Spec.Parallelism, "succeeded", obj.Status.Succeeded)
	return ctrl.Result{RequeueAfter: r.ReadyRequeueAfter}, nil
}

func NewJob(
	client *client.Client,
	jobBuilder builder.JobBuilder,
	opts ...WorkloadReconcilerOption,
) *Job {
	j := &Job{
		GenericResourceReconciler: NewGenericResourceReconciler[builder.JobBuilder](
			client,
			jobBuilder,
		),
		ReadyRequeueAfter: 5 * time.Second, // Default to 5 seconds
	}
	for _, opt := range opts {
		opt(j)
	}
	return j
}
