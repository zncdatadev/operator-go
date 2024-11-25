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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/client"
)

var (
	resourceLogger = ctrl.Log.WithName("reconciler").WithName("resource")
)

type ResourceReconciler[B builder.ObjectBuilder] interface {
	Reconciler

	// Deprecated: Use GetObjectKey instead.
	// this method is marked deprecated in `v0.12.0` and will be removed in next release.
	GetObjectMeta() metav1.ObjectMeta
	GetObjectKey() ctrlclient.ObjectKey
	GetBuilder() B
	ResourceReconcile(ctx context.Context, resource ctrlclient.Object) (ctrl.Result, error)
}

var _ ResourceReconciler[builder.ObjectBuilder] = &GenericResourceReconciler[builder.ObjectBuilder]{}

type GenericResourceReconciler[T builder.ObjectBuilder] struct {
	// Do not use ptr, to avoid other packages to modify the client
	Client *client.Client

	Builder T
}

func NewGenericResourceReconciler[T builder.ObjectBuilder](
	client *client.Client,
	builder T,
) *GenericResourceReconciler[T] {
	return &GenericResourceReconciler[T]{
		Client:  client,
		Builder: builder,
	}
}

func (r *GenericResourceReconciler[T]) GetName() string {
	return r.Builder.GetName()
}

func (r *GenericResourceReconciler[T]) GetNamespace() string {
	return r.Client.GetOwnerNamespace()
}

func (r *GenericResourceReconciler[T]) GetClient() *client.Client {
	return r.Client
}

// Deprecated: Use r.GetObjectKey instead.
// This method is marked deprecated in `v0.12.0` and will be removed in next release.
func (r *GenericResourceReconciler[T]) GetObjectMeta() metav1.ObjectMeta {
	return r.Builder.GetObjectMeta()
}

func (r *GenericResourceReconciler[T]) GetObjectKey() ctrlclient.ObjectKey {
	return ctrlclient.ObjectKey{
		Namespace: r.GetNamespace(),
		Name:      r.GetName(),
	}
}

func (r *GenericResourceReconciler[T]) GetBuilder() T {
	return r.Builder
}

// ResourceReconcile creates or updates a resource.
// If the resource is created or updated, it returns a Result with a requeue time of 1 second.
//
// Most of the time you should not call this method directly, but call the r.Reconcile() method instead.
func (r *GenericResourceReconciler[T]) ResourceReconcile(ctx context.Context, resource ctrlclient.Object) (ctrl.Result, error) {
	logger.V(5).Info("Reconciling resource", "namespace", r.GetNamespace(), "cluster", r.GetName(), "name", resource.GetName())
	logExtraValues := []any{
		"name", resource.GetName(),
		"namespace", resource.GetNamespace(),
		"cluster", r.GetName(),
	}

	if mutation, err := r.Client.CreateOrUpdate(ctx, resource); err != nil {
		resourceLogger.Error(err, "Failed to create or update resource", logExtraValues...)
		return ctrl.Result{}, err
	} else if mutation {
		resourceLogger.Info("Resource created or updated", logExtraValues...)
		return ctrl.Result{Requeue: true}, nil
	}
	return ctrl.Result{}, nil
}

func (r *GenericResourceReconciler[T]) Reconcile(ctx context.Context) (ctrl.Result, error) {
	logger.V(5).Info("Building resource", "namespace", r.GetNamespace(), "cluster", r.GetName(), "name", r.GetName())
	resource, err := r.GetBuilder().Build(ctx)

	if err != nil {
		return ctrl.Result{}, err
	}
	return r.ResourceReconcile(ctx, resource)
}

// GenericResourceReconciler[T] does not check anythins, so it is always ready.
func (r *GenericResourceReconciler[T]) Ready(ctx context.Context) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

type SimpleResourceReconciler[T builder.ObjectBuilder] struct {
	GenericResourceReconciler[T]
}

// NewSimpleResourceReconciler creates a new resource reconciler with a simple builder
// that does not require a spec, and can not use the spec.
func NewSimpleResourceReconciler[T builder.ObjectBuilder](
	client *client.Client,
	builder T,
) *SimpleResourceReconciler[T] {
	return &SimpleResourceReconciler[T]{
		GenericResourceReconciler: *NewGenericResourceReconciler[T](
			client,
			builder,
		),
	}
}
