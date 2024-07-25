package builder

import (
	"context"

	client "github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/util"
	appv1 "k8s.io/api/apps/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	DefaultReplicas = int32(1)
)

var _ DeploymentBuilder = &Deployment{}

type Deployment struct {
	BaseWorkloadReplicasBuilder
}

func NewDeployment(
	client *client.Client,
	name string,
	replicas *int32,
	image *util.Image,
	options WorkloadOptions,
) *Deployment {
	return &Deployment{
		BaseWorkloadReplicasBuilder: *NewBaseWorkloadReplicasBuilder(
			client,
			name,
			replicas,
			image,
			options,
		),
	}
}

func (b *Deployment) GetObject() (*appv1.Deployment, error) {
	tpl, err := b.getPodTemplate()
	if err != nil {
		return nil, err
	}

	obj := &appv1.Deployment{
		ObjectMeta: b.GetObjectMeta(),
		Spec: appv1.DeploymentSpec{
			Replicas: b.GetReplicas(),
			Selector: b.GetLabelSelector(),
			Template: *tpl,
		},
	}

	return obj, nil
}

func (b *Deployment) Build(_ context.Context) (ctrlclient.Object, error) {
	return b.GetObject()
}
