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

// GenericResourceReconcilerOption is a functional option for configuring a GenericResourceReconciler
type GenericResourceReconcilerOption[T builder.ObjectBuilder] func(*GenericResourceReconciler[T])

// WithRequeueAfter sets the requeue duration for the reconciler
func WithRequeueAfter[T builder.ObjectBuilder](duration time.Duration) GenericResourceReconcilerOption[T] {
	return func(r *GenericResourceReconciler[T]) {
		r.RequeueAfter = duration
	}
}

type GenericResourceReconciler[T builder.ObjectBuilder] struct {
	// Do not use ptr, to avoid other packages to modify the client
	Client *client.Client

	Builder T

	// RequeueAfter is the duration after which to requeue the reconcile request
	// when a resource is created or updated. Default is 1 second.
	RequeueAfter time.Duration
}

func NewGenericResourceReconciler[T builder.ObjectBuilder](
	client *client.Client,
	builder T,
	opts ...GenericResourceReconcilerOption[T],
) *GenericResourceReconciler[T] {
	r := &GenericResourceReconciler[T]{
		Client:       client,
		Builder:      builder,
		RequeueAfter: time.Second, // Default to 1 second
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
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
// If the resource is created or updated, it returns a Result with a requeue time configured via RequeueAfter.
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
		return ctrl.Result{RequeueAfter: r.RequeueAfter}, nil
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

// WorkloadReconciler is an interface for workload reconcilers that have ReadyRequeueAfter
// This is a marker interface that allows common option functions
type WorkloadReconciler interface {
	SetRequeueAfter(duration time.Duration)
	SetReadyRequeueAfter(duration time.Duration)
}

// WorkloadReconcilerOption is a functional option for configuring workload reconcilers
type WorkloadReconcilerOption func(WorkloadReconciler)

// RequeueAfter sets the requeue duration for workload reconciliation
func RequeueAfter(duration time.Duration) WorkloadReconcilerOption {
	return func(r WorkloadReconciler) {
		r.SetRequeueAfter(duration)
	}
}

// ReadyRequeueAfter sets the requeue duration for workload readiness checks
func ReadyRequeueAfter(duration time.Duration) WorkloadReconcilerOption {
	return func(r WorkloadReconciler) {
		r.SetReadyRequeueAfter(duration)
	}
}

type SimpleResourceReconciler[T builder.ObjectBuilder] struct {
	GenericResourceReconciler[T]
}

// NewSimpleResourceReconciler creates a new resource reconciler with a simple builder
// that does not require a spec, and can not use the spec.
func NewSimpleResourceReconciler[T builder.ObjectBuilder](
	client *client.Client,
	builder T,
	opts ...GenericResourceReconcilerOption[T],
) *SimpleResourceReconciler[T] {
	return &SimpleResourceReconciler[T]{
		GenericResourceReconciler: *NewGenericResourceReconciler[T](
			client,
			builder,
			opts...,
		),
	}
}
