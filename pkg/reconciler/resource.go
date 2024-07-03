package reconciler

import (
	"context"
	"time"

	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	resourceLogger = ctrl.Log.WithName("reconciler").WithName("resource")
)

type ResourceReconciler[B builder.ResourceBuilder] interface {
	Reconciler
	GetObjectMeta() metav1.ObjectMeta
	GetBuilder() B
	ResourceReconcile(ctx context.Context, resource ctrlclient.Object) *Result
}

var _ ResourceReconciler[builder.ResourceBuilder] = &GenericResourceReconciler[builder.ResourceBuilder]{}

type GenericResourceReconciler[B builder.ResourceBuilder] struct {
	BaseReconciler[AnySpec]
	Builder B
}

type ResourceReconcilerOptions struct {
	Name string
}

func NewGenericResourceReconciler[B builder.ResourceBuilder](
	client *client.Client,
	options *ResourceReconcilerOptions,
	builder B,
) *GenericResourceReconciler[B] {
	return &GenericResourceReconciler[B]{
		BaseReconciler: BaseReconciler[AnySpec]{
			Client: client,
			Name:   options.Name,
			Spec:   nil,
		},
		Builder: builder,
	}
}

func (r *GenericResourceReconciler[b]) GetObjectMeta() metav1.ObjectMeta {
	return r.Builder.GetObjectMeta()
}

func (r *GenericResourceReconciler[B]) GetBuilder() B {
	return r.Builder
}

func (r *GenericResourceReconciler[B]) ResourceReconcile(ctx context.Context, resource ctrlclient.Object) *Result {

	if mutation, err := r.Client.CreateOrUpdate(ctx, resource); err != nil {
		resourceLogger.Error(err, "Failed to create or update resource", "name", resource.GetName(), "namespace", resource.GetNamespace(), "cluster", r.GetName())
		return NewResult(true, 0, err)
	} else if mutation {
		resourceLogger.Info("Resource created or updated", "name", resource.GetName(), "namespace", resource.GetNamespace(), "cluster", r.GetName())
		return NewResult(true, time.Second, nil)
	}
	return NewResult(false, 0, nil)
}

func (r *GenericResourceReconciler[B]) Reconcile(ctx context.Context) *Result {
	resource, err := r.GetBuilder().Build(ctx)

	if err != nil {
		return NewResult(true, 0, err)
	}
	return r.ResourceReconcile(ctx, resource)
}

func (r *GenericResourceReconciler[B]) Ready(ctx context.Context) *Result {
	return NewResult(false, 0, nil)
}

type SimpleResourceReconciler[B builder.ResourceBuilder] struct {
	GenericResourceReconciler[B]
}

type SimpleResourceReconcilerOptions struct {
	ResourceReconcilerOptions
}

// NewSimpleResourceReconciler creates a new resource reconciler with a simple builder
// that does not require a spec, and can not use the spec.
func NewSimpleResourceReconciler[B builder.ResourceBuilder](
	client *client.Client,
	options *SimpleResourceReconcilerOptions,
	builder B,
) *SimpleResourceReconciler[B] {
	return &SimpleResourceReconciler[B]{
		GenericResourceReconciler: *NewGenericResourceReconciler[B](
			client,
			&options.ResourceReconcilerOptions,
			builder,
		),
	}
}
