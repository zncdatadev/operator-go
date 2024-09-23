package reconciler

import (
	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/client"
	netv1 "k8s.io/api/networking/v1"
)

var _ ResourceReconciler[builder.IngressBuilder] = &Ingress{}

type Ingress struct {
	GenericResourceReconciler[builder.IngressBuilder]
	rules []netv1.IngressRule
}

func NewIngressReconciler(
	client *client.Client,
	name string,
	labels map[string]string,
	annotations map[string]string,
	rules []netv1.IngressRule,
) *Ingress {
	svcBuilder := builder.NewIngressBuilder(
		client,
		name,
		labels,
		annotations,
		rules,
	)

	return &Ingress{
		GenericResourceReconciler: *NewGenericResourceReconciler[builder.IngressBuilder](
			client,
			name,
			svcBuilder,
		),
		rules: rules,
	}
}
