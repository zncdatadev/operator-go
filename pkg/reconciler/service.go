package reconciler

import (
	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/client"
)

var _ ResourceReconciler[builder.ServiceBuilder] = &GenericServiceReconciler{}

type GenericServiceReconciler struct {
	GenericResourceReconciler[builder.ServiceBuilder]
}

func NewServiceReconciler(
	client *client.Client,
	options builder.Options,
) *GenericServiceReconciler {
	svcBuilder := builder.NewServiceBuilder(
		client,
		options,
	)
	return &GenericServiceReconciler{
		GenericResourceReconciler: *NewGenericResourceReconciler[builder.ServiceBuilder](
			client,
			options,
			svcBuilder,
		),
	}
}
