package builder

import (
	"context"
	"time"

	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/util"
	appv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ContainerBuilder is an interface for building a container
// implementation with build pattern
type ContainerBuilder interface {
	Build() *corev1.Container

	SetImagePullPolicy(policy corev1.PullPolicy) ContainerBuilder

	AddVolumeMounts(mounts []corev1.VolumeMount) ContainerBuilder
	AddVolumeMount(mount *corev1.VolumeMount) ContainerBuilder

	AddEnvVars(envs []corev1.EnvVar) ContainerBuilder
	AddEnvVar(env *corev1.EnvVar) ContainerBuilder
	AddEnvs(envs map[string]string) ContainerBuilder
	AddEnv(name, value string) ContainerBuilder

	AddEnvSource(envs []corev1.EnvFromSource) ContainerBuilder

	AddEnvFromSecret(secretName string) ContainerBuilder
	AddEnvFromConfigMap(configMapName string) ContainerBuilder

	AddPorts(ports []corev1.ContainerPort) ContainerBuilder

	SetCommand(command []string) ContainerBuilder

	SetArgs(args []string) ContainerBuilder

	OverrideCommand(command []string) ContainerBuilder

	SetResources(resources *commonsv1alpha1.ResourcesSpec) ContainerBuilder

	SetLivenessProbe(probe *corev1.Probe) ContainerBuilder
	SetReadinessProbe(probe *corev1.Probe) ContainerBuilder
	SetStartupProbe(probe *corev1.Probe) ContainerBuilder

	SetSecurityContext(user int64, group int64, nonRoot bool) ContainerBuilder
}

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

type WorkloadImage interface {
	SetImage(image *util.Image)
	GetImage() *util.Image
	GetImageWithTag() (string, error)
}

type WorkloadSecurityContext interface {
	SetSecurityContext(user int64, group int64, nonRoot bool)
	GetSecurityContext() *corev1.PodSecurityContext
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
	SetAffinity(affinity *corev1.Affinity)
	GetAffinity() *corev1.Affinity
}

type WorkloadReplicas interface {
	SetReplicas(replicas *int32)
	GetReplicas() *int32
}

type WorkloadTerminationGracePeriodSeconds interface {
	SetTerminationGracePeriod(duration *time.Duration)
	GetTerminationGracePeriod() *time.Duration
}

type WorkloadResource interface {
	SetResources(resources *commonsv1alpha1.ResourcesSpec)
	GetResources() *commonsv1alpha1.ResourcesSpec
}

type StatefulSetBuilder interface {
	ResourceBuilder

	WorkloadReplicas
	WorkloadContainers
	WorkloadInitContainers
	WorkloadVolumes
	WorkloadAffinity
	WorkloadTerminationGracePeriodSeconds
	WorkloadSecurityContext
	WorkloadResource

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
	WorkloadSecurityContext
	WorkloadResource
}

type JobBuilder interface {
	ResourceBuilder
	GetObject() (*batchv1.Job, error)

	WorkloadContainers
	WorkloadInitContainers
	WorkloadVolumes
	WorkloadAffinity
	WorkloadTerminationGracePeriodSeconds
	WorkloadSecurityContext
	WorkloadResource

	SetRestPolicy(policy *corev1.RestartPolicy)
}

type ServiceBuilder interface {
	ResourceBuilder
	GetObject() *corev1.Service
	AddPort(port *corev1.ServicePort)
	GetPorts() []corev1.ServicePort
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

type PodDisruptionBudgetBuilder interface {
	ResourceBuilder
	GetObject() (*policyv1.PodDisruptionBudget, error)
	SetMaxUnavailable(max intstr.IntOrString)
	SetMinAvailable(min intstr.IntOrString)
}
