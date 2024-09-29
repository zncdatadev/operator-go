package builder

import (
	"maps"
	"slices"

	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var (
	HTTPGetProbHandler2PortNames = []string{"http", "ui", "metrics", "health"}
	TCPProbHandler2PortNames     = []string{"master"}
)

var _ ContainerBuilder = &Container{}

type Container struct {
	Name  string
	Image *util.Image

	obj *corev1.Container
}

// NewContainer returns a new Container
func NewContainer(
	name string,
	image *util.Image,
) *Container {
	return &Container{
		Name:  name,
		Image: image,
	}
}

// NewContainerBuilder returns a new ContainerBuilder
// This method return a ContainerBuilder interface
// Example:
//
//	image := util.Image{Custom: "nginx"}
//	fooContainer := builder.NewContainerBuilder("foo", "nginx").
//		SetImagePullPolicy(corev1.PullAlways).
//		Build()
func NewContainerBuilder(
	name string,
	image *util.Image,
) ContainerBuilder {
	return NewContainer(name, image)
}

func (b *Container) getObject() *corev1.Container {
	if b.obj == nil {
		b.obj = &corev1.Container{
			Name:            b.Name,
			Image:           b.Image.String(),
			ImagePullPolicy: b.Image.GetPullPolicy(),
			Resources:       corev1.ResourceRequirements{},
		}
	}
	return b.obj
}

func (b *Container) Build() *corev1.Container {
	obj := b.getObject()
	return obj
}

func (b *Container) SetImagePullPolicy(policy corev1.PullPolicy) ContainerBuilder {
	if policy == "" {
		logger.V(2).Info("Could not set image pull policy, use default value", "policy", policy, "container", b.Name, "image", b.Image, "default", b.Image.GetPullPolicy())
		return b
	}

	b.getObject().ImagePullPolicy = policy
	return b
}

func (b *Container) AddVolumeMounts(mounts []corev1.VolumeMount) ContainerBuilder {
	v := b.getObject().VolumeMounts
	v = append(v, mounts...)
	b.getObject().VolumeMounts = v
	return b
}

func (b *Container) AddVolumeMount(mount *corev1.VolumeMount) ContainerBuilder {

	v := b.getObject().VolumeMounts
	v = append(v, *mount)
	b.getObject().VolumeMounts = v
	return b
}

func (b *Container) ResetVolumeMounts(mounts []corev1.VolumeMount) {
	b.getObject().VolumeMounts = mounts

}

func (b *Container) GetVolumeMounts() []corev1.VolumeMount {
	return b.getObject().VolumeMounts
}

func (b *Container) AddEnvVars(envVars []corev1.EnvVar) ContainerBuilder {
	envs := b.getObject().Env
	envs = append(envs, envVars...)
	var envNames []string
	for _, env := range envs {
		if slices.Contains(envNames, env.Name) {
			logger.V(2).Info("EnvVar already exists, it may be overwritten", "env", env.Name)
		}
		envNames = append(envNames, env.Name)
	}
	b.getObject().Env = envs

	return b
}

func (b *Container) AddEnvVar(env *corev1.EnvVar) ContainerBuilder {
	b.AddEnvVars([]corev1.EnvVar{*env})
	return b
}

func (b *Container) ResetEnvVars(envVars []corev1.EnvVar) {
	b.getObject().Env = envVars

}

func (b *Container) GetEnvVars() []corev1.EnvVar {
	return b.getObject().Env
}

func (b *Container) AddEnvs(envs map[string]string) ContainerBuilder {
	for _, key := range slices.Sorted(maps.Keys(envs)) {
		b.AddEnv(key, envs[key])
	}
	return b
}

func (b *Container) AddEnv(key, value string) ContainerBuilder {
	return b.AddEnvVar(&corev1.EnvVar{Name: key, Value: value})
}

func (b *Container) AddEnvSource(envs []corev1.EnvFromSource) ContainerBuilder {
	e := b.getObject().EnvFrom
	e = append(e, envs...)
	b.getObject().EnvFrom = e

	return b
}

func (b *Container) AddEnvFromSecret(secretName string) ContainerBuilder {
	b.AddEnvSource([]corev1.EnvFromSource{
		{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secretName,
				},
			},
		},
	})
	return b
}

func (b *Container) AddEnvFromConfigMap(configMapName string) ContainerBuilder {
	b.AddEnvSource([]corev1.EnvFromSource{
		{
			ConfigMapRef: &corev1.ConfigMapEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: configMapName,
				},
			},
		},
	})
	return b
}

func (b *Container) ResetEnvFrom(envs []corev1.EnvFromSource) {
	b.getObject().EnvFrom = envs
}

func (b *Container) GetEnvFrom() []corev1.EnvFromSource {
	return b.getObject().EnvFrom
}

func (b *Container) AddPorts(ports []corev1.ContainerPort) ContainerBuilder {
	p := b.getObject().Ports
	p = append(p, ports...)
	b.getObject().Ports = p

	return b
}

func (b *Container) AddPort(port corev1.ContainerPort) {
	b.AddPorts([]corev1.ContainerPort{port})
}

func (b *Container) ResetPorts(ports []corev1.ContainerPort) {
	b.getObject().Ports = ports

}

func (b *Container) GetPorts() []corev1.ContainerPort {
	return b.getObject().Ports
}

func (b *Container) SetCommand(command []string) ContainerBuilder {
	b.getObject().Command = command
	b.getObject().Args = []string{}

	return b
}

func (b *Container) SetArgs(args []string) ContainerBuilder {
	b.getObject().Args = args

	return b
}

func (b *Container) OverrideEnv(envs map[string]string) {
	b.getObject().Env = []corev1.EnvVar{}
	b.AddEnvs(envs)
}

func (b *Container) OverrideCommand(command []string) ContainerBuilder {
	b.getObject().Command = []string{}
	b.SetCommand(command)

	return b
}

func (b *Container) SetResources(resources *commonsv1alpha1.ResourcesSpec) ContainerBuilder {
	obj := b.getObject()
	if resources == nil {
		return b
	}

	if obj.Resources.Requests == nil {
		obj.Resources.Requests = corev1.ResourceList{}
	}

	if obj.Resources.Limits == nil {
		obj.Resources.Limits = corev1.ResourceList{}
	}

	if resources.CPU != nil {
		obj.Resources.Requests[corev1.ResourceCPU] = resources.CPU.Min
		obj.Resources.Limits[corev1.ResourceCPU] = resources.CPU.Max
	}

	if resources.Memory != nil {
		obj.Resources.Requests[corev1.ResourceMemory] = resources.Memory.Limit
		obj.Resources.Limits[corev1.ResourceMemory] = resources.Memory.Limit
	}

	return b
}

func (b *Container) SetLivenessProbe(probe *corev1.Probe) ContainerBuilder {
	b.getObject().LivenessProbe = probe
	return b

}

func (b *Container) SetReadinessProbe(probe *corev1.Probe) ContainerBuilder {
	b.getObject().ReadinessProbe = probe
	return b

}

func (b *Container) SetStartupProbe(probe *corev1.Probe) ContainerBuilder {
	b.getObject().StartupProbe = probe
	return b
}

func (b *Container) SetSecurityContext(user int64, group int64, nonRoot bool) ContainerBuilder {
	b.getObject().SecurityContext = &corev1.SecurityContext{
		RunAsUser:                &user,
		RunAsGroup:               &group,
		AllowPrivilegeEscalation: &nonRoot,
	}
	return b

}

// AutomaticSetProbe sets the liveness, readiness and startup probes
// policy:
// - handle policy:
//   - if name of ports contains "http", "ui", "metrics" or "health", use httpGet
//   - if name of ports contains "master", use tcpSocket
//   - todo: add more rules
//
// - startupProbe:
//   - failureThreshold: 30
//   - initialDelaySeconds: 4
//   - periodSeconds: 6
//   - successThreshold: 1
//   - timeoutSeconds: 3
//
// - livenessProbe:
//   - failureThreshold: 3
//   - periodSeconds: 10
//   - successThreshold: 1
//   - timeoutSeconds: 3
//
// - readinessProbe:
//   - failureThreshold: 3
//   - periodSeconds: 10
//   - successThreshold: 1
//   - timeoutSeconds: 3
func (b *Container) AutomaticSetProbe() {

	probeHandler := b.getProbeHandler()

	if probeHandler == nil {
		logger.V(2).Info("No probe handler found, skip setting probes")
		return
	}

	// Set startup probe
	startupProbe := &corev1.Probe{
		FailureThreshold:    30,
		InitialDelaySeconds: 4,
		PeriodSeconds:       6,
		SuccessThreshold:    1,
		TimeoutSeconds:      3,
		ProbeHandler:        *probeHandler,
	}
	b.SetStartupProbe(startupProbe)

	// Set liveness probe
	livenessProbe := &corev1.Probe{
		FailureThreshold: 3,
		PeriodSeconds:    10,
		SuccessThreshold: 1,
		TimeoutSeconds:   3,
		ProbeHandler:     *probeHandler,
	}
	b.SetLivenessProbe(livenessProbe)

	// Set readiness probe
	readinessProbe := &corev1.Probe{
		FailureThreshold: 3,
		PeriodSeconds:    10,
		SuccessThreshold: 1,
		TimeoutSeconds:   3,
		ProbeHandler:     *probeHandler,
	}
	b.SetReadinessProbe(readinessProbe)

}

// getProbeHandler returns the handler for the probe
// policy:
// - handle policy:
//   - if name of ports contains "http", "ui", "metrics" or "health", use httpGet
//   - if name of ports contains "master", use tcpSocket
//   - todo: add more rules
func (b *Container) getProbeHandler() *corev1.ProbeHandler {
	for _, port := range b.getObject().Ports {
		if slices.Contains(HTTPGetProbHandler2PortNames, port.Name) {
			return &corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/",
					Port: intstr.FromString(port.Name),
				},
			}
		}
		if slices.Contains(TCPProbHandler2PortNames, port.Name) {
			return &corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromString(port.Name),
				},
			}
		}
	}
	return nil
}

func (b *Container) SetProbeWithHealth() {
	ok := false
	for _, port := range b.getObject().Ports {
		if port.Name == "http" {
			ok = true
			probeHandler := &corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/health",
					Port: intstr.FromString("http"),
				},
			}
			// Set startup probe
			startupProbe := &corev1.Probe{
				FailureThreshold:    30,
				InitialDelaySeconds: 4,
				PeriodSeconds:       6,
				SuccessThreshold:    1,
				TimeoutSeconds:      3,
				ProbeHandler:        *probeHandler,
			}
			b.SetStartupProbe(startupProbe)

			// Set liveness probe
			livenessProbe := &corev1.Probe{
				FailureThreshold: 3,
				PeriodSeconds:    10,
				SuccessThreshold: 1,
				TimeoutSeconds:   3,
				ProbeHandler:     *probeHandler,
			}
			b.SetLivenessProbe(livenessProbe)

			// Set readiness probe
			readinessProbe := &corev1.Probe{
				FailureThreshold: 3,
				PeriodSeconds:    10,
				SuccessThreshold: 1,
				TimeoutSeconds:   3,
				ProbeHandler:     *probeHandler,
			}
			b.SetReadinessProbe(readinessProbe)
			break
		}
	}

	if !ok {
		logger.V(2).Info("No http port found, skip setting probes")
	}

}
