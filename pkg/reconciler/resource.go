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
	// todo: remove this, as it can be get from the builder
	Name string
}

func NewGenericResourceReconciler[B builder.ResourceBuilder](
	client *client.Client,
	name string,
	builder B,
) *GenericResourceReconciler[B] {
	return &GenericResourceReconciler[B]{
		BaseReconciler: BaseReconciler[AnySpec]{
			Client: client,
			Spec:   nil,
		},
		Builder: builder,
		Name:    name,
	}
}

func (r *GenericResourceReconciler[B]) GetName() string {
	return r.Name
}

func (r *GenericResourceReconciler[b]) GetObjectMeta() metav1.ObjectMeta {
	return r.Builder.GetObjectMeta()
}

func (r *GenericResourceReconciler[B]) GetBuilder() B {
	return r.Builder
}

// ResourceReconcile creates or updates a resource.
// If the resource is created or updated, it returns a Result with a requeue time of 1 second.
//
// Most of the time you should not call this method directly, but call the r.Reconcile() method instead.
func (r *GenericResourceReconciler[B]) ResourceReconcile(ctx context.Context, resource ctrlclient.Object) *Result {
	logExtraValues := []interface{}{
		"name", resource.GetName(),
		"namespace", resource.GetNamespace(),
		"cluster", r.GetName(),
	}

	if mutation, err := r.Client.CreateOrUpdate(ctx, resource); err != nil {
		resourceLogger.Error(err, "Failed to create or update resource", logExtraValues...)
		return NewResult(true, 0, err)
	} else if mutation {
		resourceLogger.Info("Resource created or updated", logExtraValues...)
		// TODO: Different resources may have different retry times based on their characteristics,
		// for example: the creation time of a Deployment may be longer, so a longer retry time can be set,
		// while the creation time of a Service may be shorter, so a shorter retry time can be set.
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

// GenericResourceReconciler[B] does not check anythins, so it is always ready.
func (r *GenericResourceReconciler[B]) Ready(ctx context.Context) *Result {
	return NewResult(false, 0, nil)
}

type SimpleResourceReconciler[B builder.ResourceBuilder] struct {
	GenericResourceReconciler[B]
}

// NewSimpleResourceReconciler creates a new resource reconciler with a simple builder
// that does not require a spec, and can not use the spec.
func NewSimpleResourceReconciler[B builder.ResourceBuilder](
	client *client.Client,
	name string,
	builder B,
) *SimpleResourceReconciler[B] {
	return &SimpleResourceReconciler[B]{
		GenericResourceReconciler: *NewGenericResourceReconciler[B](
			client,
			name,
			builder,
		),
	}
}
