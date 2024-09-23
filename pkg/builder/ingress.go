package builder

import (
	"context"

	"github.com/zncdatadev/operator-go/pkg/client"
	netv1 "k8s.io/api/networking/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func NewIngressBuilder(
	client *client.Client,
	name string,
	labels map[string]string,
	annotations map[string]string,
	rules []netv1.IngressRule,
) *BaseIngressBuilder {
	return &BaseIngressBuilder{
		BaseResourceBuilder: *NewBaseResourceBuilder(client, name, &Options{Labels: labels, Annotations: annotations}),
		rules:               rules,
	}
}

var _ IngressBuilder = &BaseIngressBuilder{}

type BaseIngressBuilder struct {
	BaseResourceBuilder
	rules []netv1.IngressRule
}

// AddRules implements IngressBuilder.
func (b *BaseIngressBuilder) AddRules(rules []netv1.IngressRule) {
	b.rules = append(b.rules, rules...)
}

// ResetRules implements IngressBuilder.
func (b *BaseIngressBuilder) ResetRules(rules []netv1.IngressRule) {
	b.rules = rules
}

func (b *BaseIngressBuilder) AddRule(rule netv1.IngressRule) {
	b.rules = append(b.rules, rule)
}

func (b *BaseIngressBuilder) ResetRule(rule netv1.IngressRule) {
	b.rules = []netv1.IngressRule{rule}
}

func (b *BaseIngressBuilder) GetRules() []netv1.IngressRule {
	return b.rules
}

// Build implements IngressBuilder.
// Subtle: this method shadows the method (BaseResourceBuilder).Build of BaseIngressBuilder.BaseResourceBuilder.
func (b *BaseIngressBuilder) Build(ctx context.Context) (ctrlclient.Object, error) {
	return b.GetObject(), nil
}

// GetObject implements IngressBuilder.
func (b *BaseIngressBuilder) GetObject() *netv1.Ingress {
	return &netv1.Ingress{
		ObjectMeta: b.GetObjectMeta(),
		Spec:       netv1.IngressSpec{Rules: b.rules},
	}
}
