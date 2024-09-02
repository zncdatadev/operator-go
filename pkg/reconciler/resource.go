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

type ResourceReconciler[B builder.ResourceBuilder] interface {
	Reconciler

	GetObjectMeta() metav1.ObjectMeta
	GetBuilder() B
	ResourceReconcile(ctx context.Context, resource ctrlclient.Object) (ctrl.Result, error)
}

var _ ResourceReconciler[builder.ResourceBuilder] = &GenericResourceReconciler[builder.ResourceBuilder]{}

type GenericResourceReconciler[B builder.ResourceBuilder] struct {
	BaseReconciler[AnySpec]
	Builder B

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
func (r *GenericResourceReconciler[B]) ResourceReconcile(ctx context.Context, resource ctrlclient.Object) (ctrl.Result, error) {
	logExtraValues := []interface{}{
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

func (r *GenericResourceReconciler[B]) Reconcile(ctx context.Context) (ctrl.Result, error) {
	resource, err := r.GetBuilder().Build(ctx)

	if err != nil {
		return ctrl.Result{}, err
	}
	return r.ResourceReconcile(ctx, resource)
}

// GenericResourceReconciler[B] does not check anythins, so it is always ready.
func (r *GenericResourceReconciler[B]) Ready(ctx context.Context) (ctrl.Result, error) {
	return ctrl.Result{}, nil
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
