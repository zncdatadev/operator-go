package builder

import (
	"slices"

	apiv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var (
	HTTPGetProbHandler2PortNames = []string{"http", "ui", "metrics", "health"}
	TCPProbHandler2PortNames     = []string{"master"}
)

type ContainerBuilder interface {
	Build() *corev1.Container

	AddVolumeMounts(mounts []corev1.VolumeMount)
	AddVolumeMount(mount corev1.VolumeMount)
	ResetVolumeMounts(mounts []corev1.VolumeMount)
	GetVolumeMounts() []corev1.VolumeMount

	AddEnvVars(envVars []corev1.EnvVar)
	ResetEnvVars(envVars []corev1.EnvVar)
	GetEnvVars() []corev1.EnvVar

	AddEnvs(envs map[string]string)
	AddEnv(key, value string)

	AddEnvFrom(envs []corev1.EnvFromSource)
	AddEnvFromSecret(secretName string)
	AddEnvFromConfigMap(configMapName string)
	ResetEnvFrom(envs []corev1.EnvFromSource)
	GetEnvFrom() []corev1.EnvFromSource

	AddPorts(ports []corev1.ContainerPort)
	AddPort(port corev1.ContainerPort)
	ResetPorts(ports []corev1.ContainerPort)
	GetPorts() []corev1.ContainerPort

	SetResources(resources apiv1alpha1.ResourcesSpec)

	SetLiveProbe(probe *corev1.Probe)
	SetReadinessProbe(probe *corev1.Probe)
	SetStartupProbe(probe *corev1.Probe)

	SetSecurityContext(user int64, group int64, nonRoot bool)
	// SetCommand sets the command for the container and clears the args.
	SetCommand(command []string)
	SetArgs(args []string)

	OverrideEnv(envs map[string]string)
	// OverrideCommand sets the command for the container and clears the command.
	OverrideCommand(command []string)

	AutomaticSetProbe()
	SetProbeWithHealth()
}

var _ ContainerBuilder = &GenericContainerBuilder{}

type GenericContainerBuilder struct {
	Name       string
	Image      string
	PullPolicy corev1.PullPolicy

	obj *corev1.Container
}

func NewGenericContainerBuilder(
	name, image string,
	pullPolicy corev1.PullPolicy,
) *GenericContainerBuilder {
	return &GenericContainerBuilder{
		Name:       name,
		Image:      image,
		PullPolicy: pullPolicy,
	}
}

func (b *GenericContainerBuilder) getObject() *corev1.Container {
	if b.obj == nil {
		b.obj = &corev1.Container{
			Name:            b.Name,
			Image:           b.Image,
			ImagePullPolicy: b.PullPolicy,
		}
	}
	return b.obj
}

func (b *GenericContainerBuilder) Build() *corev1.Container {
	obj := b.getObject()
	return obj
}

func (b *GenericContainerBuilder) AddVolumeMounts(mounts []corev1.VolumeMount) {
	v := b.getObject().VolumeMounts
	v = append(v, mounts...)
	b.getObject().VolumeMounts = v

}

func (b *GenericContainerBuilder) AddVolumeMount(mount corev1.VolumeMount) {
	b.AddVolumeMounts([]corev1.VolumeMount{mount})
}

func (b *GenericContainerBuilder) ResetVolumeMounts(mounts []corev1.VolumeMount) {
	b.getObject().VolumeMounts = mounts

}

func (b *GenericContainerBuilder) GetVolumeMounts() []corev1.VolumeMount {
	return b.getObject().VolumeMounts
}

func (b *GenericContainerBuilder) AddEnvVars(envVars []corev1.EnvVar) {
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

}

func (b *GenericContainerBuilder) AddEnvVar(env corev1.EnvVar) {
	b.AddEnvVars([]corev1.EnvVar{env})
}

func (b *GenericContainerBuilder) ResetEnvVars(envVars []corev1.EnvVar) {
	b.getObject().Env = envVars

}

func (b *GenericContainerBuilder) GetEnvVars() []corev1.EnvVar {
	return b.getObject().Env
}

func (b *GenericContainerBuilder) AddEnvs(envs map[string]string) {
	var envVars []corev1.EnvVar
	for name, value := range envs {
		envVars = append(envVars, corev1.EnvVar{
			Name:  name,
			Value: value,
		})
	}
	b.AddEnvVars(envVars)
}

func (b *GenericContainerBuilder) AddEnv(key, value string) {
	b.AddEnvs(map[string]string{key: value})
}

func (b *GenericContainerBuilder) AddEnvFrom(envs []corev1.EnvFromSource) {
	e := b.getObject().EnvFrom
	e = append(e, envs...)
	b.getObject().EnvFrom = e

}

func (b *GenericContainerBuilder) AddEnvFromSecret(secretName string) {
	b.AddEnvFrom([]corev1.EnvFromSource{
		{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secretName,
				},
			},
		},
	})
}

func (b *GenericContainerBuilder) AddEnvFromConfigMap(configMapName string) {
	b.AddEnvFrom([]corev1.EnvFromSource{
		{
			ConfigMapRef: &corev1.ConfigMapEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: configMapName,
				},
			},
		},
	})
}

func (b *GenericContainerBuilder) ResetEnvFrom(envs []corev1.EnvFromSource) {
	b.getObject().EnvFrom = envs

}

func (b *GenericContainerBuilder) GetEnvFrom() []corev1.EnvFromSource {
	return b.getObject().EnvFrom
}

func (b *GenericContainerBuilder) AddPorts(ports []corev1.ContainerPort) {
	p := b.getObject().Ports
	p = append(p, ports...)
	b.getObject().Ports = p

}

func (b *GenericContainerBuilder) AddPort(port corev1.ContainerPort) {
	b.AddPorts([]corev1.ContainerPort{port})
}

func (b *GenericContainerBuilder) ResetPorts(ports []corev1.ContainerPort) {
	b.getObject().Ports = ports

}

func (b *GenericContainerBuilder) GetPorts() []corev1.ContainerPort {
	return b.getObject().Ports
}

func (b *GenericContainerBuilder) SetCommand(command []string) {
	b.getObject().Command = command
	b.getObject().Args = []string{}

}

func (b *GenericContainerBuilder) SetArgs(args []string) {
	b.getObject().Args = args

}

func (b *GenericContainerBuilder) OverrideEnv(envs map[string]string) {
	b.getObject().Env = []corev1.EnvVar{}
	b.AddEnvs(envs)
}

func (b *GenericContainerBuilder) OverrideCommand(command []string) {
	b.getObject().Command = []string{}
	b.SetCommand(command)
}

func (b *GenericContainerBuilder) SetResources(resources apiv1alpha1.ResourcesSpec) {
	obj := b.getObject()
	if resources.CPU != nil {
		obj.Resources.Requests[corev1.ResourceCPU] = resources.CPU.Min
		obj.Resources.Limits[corev1.ResourceCPU] = resources.CPU.Max
	}
	if resources.Memory != nil {
		obj.Resources.Requests[corev1.ResourceMemory] = resources.Memory.Limit
	}

}

func (b *GenericContainerBuilder) SetLiveProbe(probe *corev1.Probe) {
	b.getObject().LivenessProbe = probe

}

func (b *GenericContainerBuilder) SetReadinessProbe(probe *corev1.Probe) {
	b.getObject().ReadinessProbe = probe

}

func (b *GenericContainerBuilder) SetStartupProbe(probe *corev1.Probe) {
	b.getObject().StartupProbe = probe

}

func (b *GenericContainerBuilder) SetSecurityContext(user int64, group int64, nonRoot bool) {
	b.getObject().SecurityContext = &corev1.SecurityContext{
		RunAsUser:                &user,
		RunAsGroup:               &group,
		AllowPrivilegeEscalation: &nonRoot,
	}

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
func (b *GenericContainerBuilder) AutomaticSetProbe() {

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
	b.SetLiveProbe(livenessProbe)

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
func (b *GenericContainerBuilder) getProbeHandler() *corev1.ProbeHandler {
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

func (b *GenericContainerBuilder) SetProbeWithHealth() {
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
			b.SetLiveProbe(livenessProbe)

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
