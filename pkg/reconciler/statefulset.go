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

	appv1 "k8s.io/api/apps/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/client"
)

var _ ResourceReconciler[builder.StatefulSetBuilder] = &StatefulSet{}

type StatefulSet struct {
	GenericResourceReconciler[builder.StatefulSetBuilder]

	// When the cluster is stopped, the statefulset will be scaled to 0
	// and the reconcile will be not executed until the cluster is started
	Stopped bool
}

func NewStatefulSet(
	client *client.Client,
	statefulset builder.StatefulSetBuilder,
	stopped bool,
) *StatefulSet {
	return &StatefulSet{
		GenericResourceReconciler: *NewGenericResourceReconciler[builder.StatefulSetBuilder](
			client,
			statefulset,
		),
		Stopped: stopped,
	}
}

func (r *StatefulSet) Reconcile(ctx context.Context) (ctrl.Result, error) {
	resourceBuilder := r.GetBuilder()

	if r.Stopped {
		resourceBuilder.SetReplicas(ptr.To[int32](0))
	}

	resource, err := resourceBuilder.Build(ctx)

	if err != nil {
		return ctrl.Result{}, err
	}
	return r.ResourceReconcile(ctx, resource)
}

func (r *StatefulSet) Ready(ctx context.Context) (ctrl.Result, error) {
	obj := &appv1.StatefulSet{
		ObjectMeta: r.GetObjectMeta(),
	}
	logger.V(1).Info("Checking statefulset ready", "namespace", obj.Namespace, "name", obj.Name)
	if err := r.Client.GetWithObject(ctx, obj); err != nil {
		return ctrl.Result{}, err
	}
	if obj.Status.ReadyReplicas == *obj.Spec.Replicas {
		logger.Info("StatefulSet is ready", "namespace", obj.Namespace, "name", obj.Name, "replicas", *obj.Spec.Replicas, "readyReplicas", obj.Status.ReadyReplicas)
		return ctrl.Result{}, nil
	}
	logger.Info("StatefulSet is not ready", "namespace", obj.Namespace, "name", obj.Name, "replicas", *obj.Spec.Replicas, "readyReplicas", obj.Status.ReadyReplicas)
	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}
