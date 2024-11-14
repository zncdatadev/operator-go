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
	Options
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

	return &DefaultPDBBuilder{
		ObjectMeta: *NewObjectMeta(
			client,
			name,
			// func(o *Options) {
			// 	o.Apply(&opt.Options)
			// },
			func(o *Options) {
				o.Labels = opt.Labels
				o.Annotations = opt.Annotations
				o.ClusterName = opt.ClusterName
				o.RoleName = opt.RoleName
				o.RoleGroupName = opt.RoleGroupName
			},
		),
		maxUnavailable: opt.MaxUnavailableAmount,
		minAvailable:   opt.MinAvailableAmount,
	}, nil
}

var _ PodDisruptionBudgetBuilder = &DefaultPDBBuilder{}

type DefaultPDBBuilder struct {
	ObjectMeta

	maxUnavailable *int32
	minAvailable   *int32
}

// Build implements PodDisruptionBudgetBuilder.
// Subtle: this method shadows the method (BaseObjectBuilder).Build of DefaultPodDisruptionBudgetBuilder.BaseObjectBuilder.
func (d *DefaultPDBBuilder) Build(ctx context.Context) (ctrlclient.Object, error) {
	return d.GetObject()
}

// You can specify only one of maxUnavailable and minAvailable in a single PodDisruptionBudget.
// maxUnavailable can only be used to control the eviction of pods that have an associated controller managing them.
func (d *DefaultPDBBuilder) GetObject() (*policyv1.PodDisruptionBudget, error) {
	if err := d.verify(); err != nil {
		return nil, err
	}

	var maxUnavailable, minAvailable *intstr.IntOrString

	if d.maxUnavailable != nil {
		maxUnavailable = ptr.To(intstr.FromInt32(*d.maxUnavailable))
	}

	if d.minAvailable != nil {
		minAvailable = ptr.To(intstr.FromInt32(*d.minAvailable))
	}

	return &policyv1.PodDisruptionBudget{
		ObjectMeta: d.GetObjectMeta(),
		Spec: policyv1.PodDisruptionBudgetSpec{
			MaxUnavailable: maxUnavailable,
			MinAvailable:   minAvailable,
			Selector:       d.GetLabelSelector(),
		},
	}, nil
}

func (b *DefaultPDBBuilder) verify() error {
	// // verify that only one of maxUnavailable and minAvailable is set
	if b.maxUnavailable != nil && b.minAvailable != nil {
		return errors.New("you can specify only one of maxUnavailable and minAvailable in a single PodDisruptionBudget")
	}

	if b.maxUnavailable == nil && b.minAvailable == nil {
		return errors.New("maxUnavailable or minAvailable must be set")
	}
	return nil
}

// SetMaxUnavailable implements PodDisruptionBudgetBuilder.
func (d *DefaultPDBBuilder) SetMaxUnavailable(value *int32) {
	d.maxUnavailable = value
}

// SetMinAvailable implements PodDisruptionBudgetBuilder.
func (d *DefaultPDBBuilder) SetMinAvailable(value *int32) {
	d.minAvailable = value
}
