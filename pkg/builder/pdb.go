package builder

import (
	"context"

	"github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/errors"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type PDBBuilderOptions struct {
	Option
	MaxUnavailableAmount *int32
	MinAvailableAmount   *int32
}

type PDBBuilderOption func(*PDBBuilderOptions)

func NewDefaultPDBBuilder(
	client *client.Client,
	name string,
	options ...PDBBuilderOption,
) (*DefaultPDBBuilder, error) {
	opt := &PDBBuilderOptions{}
	for _, o := range options {
		o(opt)
	}
	maxUnavailableAmount := opt.MaxUnavailableAmount
	minAvailableAmount := opt.MinAvailableAmount
	// verify that only one of maxUnavailable and minAvailable is set
	if maxUnavailableAmount != nil && minAvailableAmount != nil {
		return nil, errors.New("you can specify only one of maxUnavailable and minAvailable in a single PodDisruptionBudget")
	}

	var maxUnavailable, minAvailable *intstr.IntOrString
	if maxUnavailableAmount != nil {
		maxUnavailable = ptr.To(intstr.FromInt(int(*maxUnavailableAmount)))
	}
	if minAvailableAmount != nil {
		minAvailable = ptr.To(intstr.FromInt(int(*minAvailableAmount)))
	}

	return &DefaultPDBBuilder{
		BaseResourceBuilder: BaseResourceBuilder{
			Client:      client,
			Name:        name,
			labels:      opt.Labels,
			annotations: opt.Annotations,
		},
		maxUnavailable: maxUnavailable,
		minAvailable:   minAvailable,
	}, nil
}

var _ PodDisruptionBudgetBuilder = &DefaultPDBBuilder{}

type DefaultPDBBuilder struct {
	BaseResourceBuilder

	maxUnavailable *intstr.IntOrString
	minAvailable   *intstr.IntOrString
}

// Build implements PodDisruptionBudgetBuilder.
// Subtle: this method shadows the method (BaseResourceBuilder).Build of DefaultPodDisruptionBudgetBuilder.BaseResourceBuilder.
func (d *DefaultPDBBuilder) Build(ctx context.Context) (ctrlclient.Object, error) {
	return d.GetObject()
}

// GetObject implements PodDisruptionBudgetBuilder.
// Subtle: this method shadows the method (BaseResourceBuilder).GetObject of DefaultPodDisruptionBudgetBuilder.BaseResourceBuilder.

// You can specify only one of maxUnavailable and minAvailable in a single PodDisruptionBudget.
// maxUnavailable can only be used to control the eviction of pods that have an associated controller managing them.
func (d *DefaultPDBBuilder) GetObject() (*policyv1.PodDisruptionBudget, error) {
	// verify Either minUnavailable or maxUnavailable must be set at this point!
	if d.maxUnavailable == nil && d.minAvailable == nil {
		return nil, errors.Errorf("maxUnavailable or minUnavailable must be set,but both are nil, role: %s, roleGroup: %s, namespace: %s",
			d.RoleName, d.RoleGroupName, d.ClusterName)
	}
	//verify only one of minUnavailable or maxUnavailable must be set at this point!
	if d.maxUnavailable != nil && d.minAvailable != nil {
		return nil, errors.Errorf("either minUnavailable or maxUnavailable must be set,but both are set, role: %s, roleGroup: %s, namespace: %s",
			d.RoleName, d.RoleGroupName, d.ClusterName)
	}
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: d.GetObjectMeta(),
		Spec: policyv1.PodDisruptionBudgetSpec{
			MaxUnavailable: d.maxUnavailable,
			MinAvailable:   d.minAvailable,
			Selector:       d.GetLabelSelector(),
		},
	}, nil
}

// SetMaxUnavailable implements PodDisruptionBudgetBuilder.
func (d *DefaultPDBBuilder) SetMaxUnavailable(max intstr.IntOrString) {
	d.maxUnavailable = &max
}

// SetMinAvailable implements PodDisruptionBudgetBuilder.
func (d *DefaultPDBBuilder) SetMinAvailable(min intstr.IntOrString) {
	d.minAvailable = &min
}
