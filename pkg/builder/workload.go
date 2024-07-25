package builder

import (
	"errors"
	"time"

	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ ResourceBuilder                       = &BaseWorkloadBuilder{}
	_ WorkloadImage                         = &BaseWorkloadBuilder{}
	_ WorkloadContainers                    = &BaseWorkloadBuilder{}
	_ WorkloadInitContainers                = &BaseWorkloadBuilder{}
	_ WorkloadVolumes                       = &BaseWorkloadBuilder{}
	_ WorkloadAffinity                      = &BaseWorkloadBuilder{}
	_ WorkloadTerminationGracePeriodSeconds = &BaseWorkloadBuilder{}
	_ WorkloadSecurityContext               = &BaseWorkloadBuilder{}
	_ WorkloadReplicas                      = &BaseWorkloadReplicasBuilder{}
)

var ErrNoContainers = errors.New("no containers defined")

// WorkloadOptions is a struct to hold the options for a workload
//
// Note: The values of envOverrides and commandOverrides will
// only be overridden on the container with the same name as roleGroupInfo.RoleName,
// if roleGroupInfo exists and roleGroupInfo.RoleName has a value.
type BaseWorkloadBuilder struct {
	BaseResourceBuilder

	image *util.Image

	affinity *corev1.Affinity

	commandOverrides []string
	envOverrides     map[string]string
	podOverrides     *corev1.PodTemplateSpec

	terminationGracePeriod *time.Duration

	resource *commonsv1alpha1.ResourcesSpec

	containers      []corev1.Container         // do not init this field when constructing the struct
	initContainers  []corev1.Container         // do not init this field when constructing the struct
	volumes         []corev1.Volume            // do not init this field when constructing the struct
	securityContext *corev1.PodSecurityContext // do not init this field when constructing the struct

}

func NewBaseWorkloadBuilder(
	client *client.Client,
	name string, // this is resource name when creating
	image *util.Image,
	options WorkloadOptions,
) *BaseWorkloadBuilder {

	return &BaseWorkloadBuilder{
		BaseResourceBuilder: *NewBaseResourceBuilder(
			client,
			name,
			&options.Options,
		),
		image: image,

		commandOverrides: options.CommandOverrides,
		envOverrides:     options.EnvOverrides,
		affinity:         options.Affinity,

		podOverrides: options.PodOverrides,

		terminationGracePeriod: options.TerminationGracePeriod,

		resource: options.Resource,
	}
}

func (b *BaseWorkloadBuilder) SetImage(image *util.Image) {
	b.image = image
}

func (b *BaseWorkloadBuilder) GetImage() string {
	return b.image.String()
}

func (b *BaseWorkloadBuilder) AddContainers(containers []corev1.Container) {
	b.containers = append(b.containers, containers...)
}

func (b *BaseWorkloadBuilder) AddContainer(container *corev1.Container) {
	b.containers = append(b.containers, *container)
}

func (b *BaseWorkloadBuilder) ResetContainers(containers []corev1.Container) {
	b.containers = containers
}

func (b *BaseWorkloadBuilder) GetContainers() []corev1.Container {
	return b.containers
}

func (b *BaseWorkloadBuilder) SetResources(resources *commonsv1alpha1.ResourcesSpec) {
	b.resource = resources
}

func (b *BaseWorkloadBuilder) GetResources() *commonsv1alpha1.ResourcesSpec {
	return b.resource
}

func (b *BaseWorkloadBuilder) SetSecurityContext(user int64, group int64, nonRoot bool) {
	securityContext := &corev1.PodSecurityContext{
		RunAsUser:    &user,
		RunAsGroup:   &group,
		RunAsNonRoot: &nonRoot,
	}

	b.securityContext = securityContext
}

func (b *BaseWorkloadBuilder) GetSecurityContext() *corev1.PodSecurityContext {
	return b.securityContext
}

func (b *BaseWorkloadBuilder) OverrideCommand() {
	containers := b.GetContainers()

	if len(containers) == 0 || b.commandOverrides == nil || len(b.commandOverrides) == 0 || b.roleName == "" {
		containersName := []string{}
		for _, container := range containers {
			containersName = append(containersName, container.Name)
		}
		logger.V(5).Info("Sikpping command override", "containers", containersName, "commandOverrides", b.commandOverrides, "roleName", b.roleName)
		return
	}

	for i := range containers {
		container := &containers[i]
		if container.Name == b.roleName {
			// Override the command, clear the args
			container.Command = b.commandOverrides
			container.Args = []string{}
			logger.V(5).Info("Command override", "container", container.Name, "command", container.Command)
			break
		}
	}

	b.containers = containers
}

func (b *BaseWorkloadBuilder) OverrideEnv() {
	containers := b.GetContainers()

	if len(containers) == 0 || b.envOverrides == nil || len(b.envOverrides) == 0 || b.roleName == "" {
		containersName := []string{}
		for _, container := range containers {
			containersName = append(containersName, container.Name)
		}
		logger.V(5).Info("Sikpping env override", "containers", containersName, "envOverrides", b.envOverrides, "roleName", b.roleName)
		return
	}

	for i := range containers {
		container := &containers[i]
		if container.Name == b.roleName {
			// Override the env
			for key, value := range b.envOverrides {
				container.Env = append(container.Env, corev1.EnvVar{
					Name:  key,
					Value: value,
				})
			}
			logger.V(5).Info("Env override", "container", container.Name, "env", container.Env)
			break
		}
	}

	b.containers = containers
}

func (b *BaseWorkloadBuilder) AddInitContainers(containers []corev1.Container) {
	b.initContainers = append(b.initContainers, containers...)
}

func (b *BaseWorkloadBuilder) AddInitContainer(container *corev1.Container) {
	b.initContainers = append(b.initContainers, *container)
}

func (b *BaseWorkloadBuilder) ResetInitContainers(containers []corev1.Container) {
	b.initContainers = containers
}

func (b *BaseWorkloadBuilder) GetInitContainers() []corev1.Container {
	return b.initContainers
}

func (b *BaseWorkloadBuilder) AddVolumes(volumes []corev1.Volume) {
	b.volumes = append(b.volumes, volumes...)
}

func (b *BaseWorkloadBuilder) AddVolume(volume *corev1.Volume) {
	b.volumes = append(b.volumes, *volume)
}

func (b *BaseWorkloadBuilder) ResetVolumes(volumes []corev1.Volume) {
	b.volumes = volumes
}

func (b *BaseWorkloadBuilder) GetVolumes() []corev1.Volume {
	return b.volumes
}

func (b *BaseWorkloadBuilder) SetAffinity(affinity *corev1.Affinity) {
	b.affinity = affinity
}

func (b *BaseWorkloadBuilder) GetAffinity() *corev1.Affinity {
	return b.affinity
}

func (b *BaseWorkloadBuilder) SetTerminationGracePeriod(duration *time.Duration) {
	b.terminationGracePeriod = duration
}

func (b *BaseWorkloadBuilder) GetTerminationGracePeriod() *time.Duration {
	return b.terminationGracePeriod
}

func (b *BaseWorkloadBuilder) GetTerminationGracePeriodSeconds() *int64 {
	if b.terminationGracePeriod != nil {
		seconds := int64(b.terminationGracePeriod.Seconds())
		return &seconds
	}
	return nil
}

func (b *BaseWorkloadBuilder) GetImagePullSecrets() []corev1.LocalObjectReference {
	if b.image.PullSecretName != "" {
		return []corev1.LocalObjectReference{{Name: b.image.PullSecretName}}
	}
	return nil
}

func (b *BaseWorkloadBuilder) getDefaultPodTemplate() (*corev1.PodTemplateSpec, error) {
	containers := b.GetContainers()

	if len(containers) == 0 {
		return &corev1.PodTemplateSpec{}, ErrNoContainers
	}

	pod := &corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      b.GetLabels(),
			Annotations: b.GetAnnotations(),
		},
		Spec: corev1.PodSpec{
			InitContainers:                b.GetInitContainers(),
			Containers:                    b.GetContainers(),
			Volumes:                       b.GetVolumes(),
			Affinity:                      b.GetAffinity(),
			TerminationGracePeriodSeconds: b.GetTerminationGracePeriodSeconds(),
			ImagePullSecrets:              b.GetImagePullSecrets(),
			SecurityContext:               b.GetSecurityContext(),
		},
	}
	return pod, nil
}

func (b *BaseWorkloadBuilder) getOverridedPodTemplate() (*corev1.PodTemplateSpec, error) {
	podTemplate := b.podOverrides.DeepCopy()

	meta := &podTemplate.ObjectMeta
	meta.Labels = util.MergeStringMaps(meta.Labels, b.GetLabels())
	meta.Annotations = util.MergeStringMaps(meta.Annotations, b.GetAnnotations())

	pod := &podTemplate.Spec

	pod.Volumes = append(pod.Volumes, b.GetVolumes()...)
	pod.InitContainers = append(pod.InitContainers, b.GetInitContainers()...)
	pod.Containers = append(pod.Containers, b.GetContainers()...)
	pod.ImagePullSecrets = append(pod.ImagePullSecrets, b.GetImagePullSecrets()...)

	if b.affinity != nil {
		pod.Affinity = b.affinity
	}

	if b.terminationGracePeriod != nil {
		pod.TerminationGracePeriodSeconds = b.GetTerminationGracePeriodSeconds()
	}

	if b.securityContext != nil {
		pod.SecurityContext = b.securityContext
	}

	return podTemplate, nil
}

func (b *BaseWorkloadBuilder) getPodTemplate() (*corev1.PodTemplateSpec, error) {

	if b.commandOverrides != nil {
		b.OverrideCommand()
	}

	if b.envOverrides != nil {
		b.OverrideEnv()
	}

	if b.podOverrides != nil {
		return b.getOverridedPodTemplate()
	}
	return b.getDefaultPodTemplate()
}

var _ WorkloadReplicas = &BaseWorkloadReplicasBuilder{}

type BaseWorkloadReplicasBuilder struct {
	BaseWorkloadBuilder
	replicas *int32
}

func NewBaseWorkloadReplicasBuilder(
	client *client.Client,
	name string,
	replicas *int32,
	image *util.Image,
	options WorkloadOptions,
) *BaseWorkloadReplicasBuilder {
	return &BaseWorkloadReplicasBuilder{
		BaseWorkloadBuilder: *NewBaseWorkloadBuilder(
			client,
			name,
			image,
			options,
		),
		replicas: replicas,
	}
}

func (b *BaseWorkloadReplicasBuilder) SetReplicas(replicas *int32) {
	b.replicas = replicas
}

func (b *BaseWorkloadReplicasBuilder) GetReplicas() *int32 {

	if b.replicas == nil {
		replicas := int32(1)
		logger.Info("Replicas not set, defaulting to 1")
		return &replicas
	}
	return b.replicas
}
