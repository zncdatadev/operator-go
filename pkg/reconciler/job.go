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
	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

func NewJob(
	client *client.Client,
	jobBuilder builder.JobBuilder,
) *Job {
	return &Job{
		GenericResourceReconciler: NewGenericResourceReconciler[builder.JobBuilder](
			client,
			jobBuilder,
		),
	}
}
