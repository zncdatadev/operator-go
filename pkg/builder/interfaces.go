package builder

import (
	"context"

	"github.com/zncdatadev/operator-go/pkg/client"
	appv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type ResourceBuilder interface {
	Build(ctx context.Context) (ctrlclient.Object, error)
	GetObjectMeta() metav1.ObjectMeta
	GetClient() *client.Client
	SetName(name string)
	GetName() string
	AddLabels(labels map[string]string)
	GetLabels() map[string]string
	AddAnnotations(annotations map[string]string)
	GetAnnotations() map[string]string
}

type WorkloadContainers interface {
	AddContainers(containers []corev1.Container)
	AddContainer(container *corev1.Container)
	ResetContainers(containers []corev1.Container)
	GetContainers() []corev1.Container
}

type WorkloadInitContainers interface {
	AddInitContainers(containers []corev1.Container)
	AddInitContainer(container *corev1.Container)
	ResetInitContainers(containers []corev1.Container)
	GetInitContainers() []corev1.Container
}

type WorkloadVolumes interface {
	AddVolumes(volumes []corev1.Volume)
	AddVolume(volume *corev1.Volume)
	ResetVolumes(volumes []corev1.Volume)
	GetVolumes() []corev1.Volume
}

type WorkloadAffinity interface {
	AddAffinity(affinity *corev1.Affinity)
	GetAffinity() *corev1.Affinity
}

type WorkloadReplicas interface {
	SetReplicas(replicas *int32)
	GetReplicas() *int32
}

type WorkloadTerminationGracePeriodSeconds interface {
	AddTerminationGracePeriodSeconds(seconds *int64)
	GetTerminationGracePeriodSeconds() *int64
}

type StatefulSetBuilder interface {
	ResourceBuilder

	WorkloadReplicas
	WorkloadContainers
	WorkloadInitContainers
	WorkloadVolumes
	WorkloadAffinity
	WorkloadTerminationGracePeriodSeconds

	GetObject() (*appv1.StatefulSet, error)

	AddVolumeClaimTemplates(claims []corev1.PersistentVolumeClaim)
	AddVolumeClaimTemplate(claim *corev1.PersistentVolumeClaim)
	ResetVolumeClaimTemplates(claims []corev1.PersistentVolumeClaim)
	GetVolumeClaimTemplates() []corev1.PersistentVolumeClaim
}

type DeploymentBuilder interface {
	ResourceBuilder

	GetObject() (*appv1.Deployment, error)

	WorkloadReplicas
	WorkloadContainers
	WorkloadInitContainers
	WorkloadVolumes
	WorkloadAffinity
	WorkloadTerminationGracePeriodSeconds
}

type JobBuilder interface {
	ResourceBuilder
	GetObject() (*batchv1.Job, error)

	WorkloadContainers
	WorkloadInitContainers
	WorkloadVolumes
	WorkloadAffinity
	WorkloadTerminationGracePeriodSeconds

	SetRestPolicy(policy *corev1.RestartPolicy)
}

type ServiceBuilder interface {
	ResourceBuilder
	GetObject() *corev1.Service
	AddPort(port *corev1.ServicePort)
	GetPorts() []corev1.ServicePort
	GetServiceType() corev1.ServiceType
}

type ServiceAccountBuilder interface {
	ResourceBuilder
	GetObject() *corev1.ServiceAccount
}

type RoleBuilder interface {
	ResourceBuilder
	GetObject() *rbacv1.Role
}

type RoleBindingBuilder interface {
	ResourceBuilder
	GetObject() *rbacv1.RoleBinding
}

type ClusterRoleBuilder interface {
	ResourceBuilder
	GetObject() *rbacv1.ClusterRole
}

type ClusterRoleBindingBuilder interface {
	ResourceBuilder
	GetObject() *rbacv1.ClusterRoleBinding
}
