package builder

import (
	"context"

	client "github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/util"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	DefaultReplicas = int32(1)
)

var _ DeploymentBuilder = &deployment{}

type deployment struct {
	BaseWorkloadReplicasBuilder
}

func NewDeployment(
	client *client.Client,
	name string,
	labels map[string]string,
	annotations map[string]string,
	affinity *corev1.Affinity,
	image *util.Image,
	ports []corev1.ContainerPort,
	commandOverrides []string,
	envOverrides map[string]string,
	podOverrides *corev1.PodTemplateSpec,
	terminationGracePeriodSeconds *int64,
	replicas *int32,
) DeploymentBuilder {
	return &deployment{
		BaseWorkloadReplicasBuilder: *NewBaseWorkloadReplicasBuilder(
			client,
			name,
			labels,
			annotations,
			affinity,
			podOverrides,
			terminationGracePeriodSeconds,
			replicas,
		),
	}
}

func (b *deployment) GetObject() (*appv1.Deployment, error) {
	tpl, err := b.getPodTemplate()
	if err != nil {
		return nil, err
	}

	obj := &appv1.Deployment{
		ObjectMeta: b.GetObjectMeta(),
		Spec: appv1.DeploymentSpec{
			Replicas: b.replicas,
			Selector: b.GetSelector(),
			Template: *tpl,
		},
	}
	return obj, nil
}

func (b *deployment) Build(_ context.Context) (ctrlclient.Object, error) {
	return b.GetObject()
}
