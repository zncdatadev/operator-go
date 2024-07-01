package builder

import (
	"context"

	client "github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/util"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	options *WorkloadReplicasOptions,
) DeploymentBuilder {
	return &deployment{
		BaseWorkloadReplicasBuilder: *NewBaseWorkloadReplicasBuilder(options),
	}
}

func (b *deployment) GetObject() *appv1.Deployment {
	if b.replicas == nil {
		b.replicas = &DefaultReplicas
	}
	obj := &appv1.Deployment{
		ObjectMeta: b.GetObjectMeta(),
		Spec: appv1.DeploymentSpec{
			Replicas: b.replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: b.GetMatchingLabels(),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      b.GetLabels(),
					Annotations: b.GetAnnotations(),
				},
				Spec: corev1.PodSpec{
					InitContainers:                b.initContainers,
					Containers:                    b.containers,
					Volumes:                       b.volumes,
					Affinity:                      b.Affinity,
					TerminationGracePeriodSeconds: b.terminationGracePeriodSeconds,
				},
			},
		},
	}
	return obj
}

func (b *deployment) Build(_ context.Context) (ctrlclient.Object, error) {
	obj := b.GetObject()

	if b.containers == nil {
		obj.Spec.Template.Spec.Containers = []corev1.Container{
			{
				Name:    b.GetName(),
				Image:   b.Image.String(),
				Env:     util.EnvsToEnvVars(b.EnvOverrides),
				Command: b.CommandOverrides,
			},
		}
	}
	return obj, nil
}
