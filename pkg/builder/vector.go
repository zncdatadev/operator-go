/*
Copyright 2024 ZNCDataDev.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package builder

import (
	"path"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/zncdatadev/operator-go/pkg/constants"
	"github.com/zncdatadev/operator-go/pkg/util"
)

const (
	VectorContainerName = "vector"

	VectorConfigVolumeName = "vector-config"
	vectorDataVolumeName   = "vector-data"
	LogDataVolumeName      = "log"

	VectorConfigFileName = "vector.yaml"

	vectorPort = 8686
)

var (
	vectorDataDir = path.Join(constants.KubedoopRoot, "vector", "var")

	VectorConfigFile   = path.Join(constants.KubedoopConfigDir, VectorConfigFileName)
	VectorWatcherDir   = path.Join(constants.KubedoopLogDir, "_vector")
	VectorShutdownFile = path.Join(VectorWatcherDir, "shutdown")

	// Vector data size default is 50Mi
	vectorDataSize = resource.NewQuantity(50*1024*1024, resource.BinarySI)
)

type Vector struct {
	VectorConfigVolumeName string
	LogDataVolumeName      string

	Image *util.Image

	Port int32

	vectorDataSize *resource.Quantity
}

type VectorOptions struct {
	Port           int32
	VectorDataSize *resource.Quantity
}

type VectorOption func(*VectorOptions)

// NewVector creates a new Vector instance.
// When use vector, a log volume is required to store the log data.
// To mount the log data to vector container, and vector agent will collect the log data.
// The config volume should container vector configuration file named "vector.yaml".
func NewVector(
	configVolumeName string,
	logVolumeName string, // Mount log data to vector container
	image *util.Image,
	options ...VectorOption,
) *Vector {

	opts := &VectorOptions{}

	for _, opt := range options {
		opt(opts)
	}

	return &Vector{
		VectorConfigVolumeName: configVolumeName,
		LogDataVolumeName:      logVolumeName,
		Image:                  image,
		Port:                   opts.Port,
		vectorDataSize:         opts.VectorDataSize,
	}
}

func (v *Vector) GetContainer() *corev1.Container {
	container := NewContainer(VectorContainerName, v.Image)
	container.AddVolumeMounts(v.getVolumeMounts())
	container.AddPort(corev1.ContainerPort{ContainerPort: v.Port, Name: "vector", Protocol: corev1.ProtocolTCP})
	container.SetCommand(v.getCommand())
	container.SetArgs(v.getArgs())
	container.SetReadinessProbe(&corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Port: intstr.FromInt(int(v.Port)),
				Path: "/health",
			},
		},
		InitialDelaySeconds: 5,
		PeriodSeconds:       10,
		SuccessThreshold:    1,
		FailureThreshold:    3,
		TimeoutSeconds:      1,
	})
	return container.Build()
}

func (v *Vector) getCommand() []string {
	return []string{"/bin/bash", "-x", "-euo", "pipefail", "-c"}
}

func (v *Vector) getArgs() []string {
	arg := `
# Vector will ignore SIGTERM (as PID != 1) and must be shut down by writing a shutdown trigger file
vector --config ` + VectorConfigFile + ` & vector_pid=$!
if [ ! -f ` + VectorShutdownFile + ` ]; then
    mkdir -p ` + VectorWatcherDir + `
    inotifywait -qq --event create ` + VectorWatcherDir + `
fi

sleep 1

kill $vector_pid
`
	return []string{util.IndentTab4Spaces(arg)}

}

func (v *Vector) GetVolumes() []corev1.Volume {
	if v.vectorDataSize == nil {
		v.vectorDataSize = vectorDataSize
	}

	objs := []corev1.Volume{
		{
			Name: vectorDataDir,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{SizeLimit: v.vectorDataSize},
			},
		},
	}
	return objs
}

func (v *Vector) getVolumeMounts() []corev1.VolumeMount {
	objs := []corev1.VolumeMount{
		{
			Name:      v.LogDataVolumeName,
			MountPath: constants.KubedoopLogDir,
		},
		{
			Name:      VectorConfigVolumeName,
			MountPath: constants.KubedoopConfigDir,
		},
		{
			Name:      vectorDataVolumeName,
			MountPath: vectorDataDir,
		},
	}

	return objs
}
