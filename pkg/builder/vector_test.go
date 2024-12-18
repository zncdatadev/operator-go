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
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/zncdatadev/operator-go/pkg/constants"
	"github.com/zncdatadev/operator-go/pkg/util"
)

func TestNewVector(t *testing.T) {
	image := util.NewImage("vector", "0.1.0", "1.0.0")
	vector := NewVector("config-volume", "log-volume", image)

	assert.Equal(t, "config-volume", vector.VectorConfigVolumeName)
	assert.Equal(t, "log-volume", vector.LogDataVolumeName)
	assert.Equal(t, image, vector.Image)
}

func TestVector_GetContainer(t *testing.T) {
	t.Run("with default port", func(t *testing.T) {
		image := util.NewImage("vector", "0.1.0", "1.0.0")
		vector := NewVector("config-volume", "log-volume", image)

		container := vector.GetContainer()

		assert.Equal(t, VectorContainerName, container.Name)
		assert.Equal(t, image.String(), container.Image)
		assert.Len(t, container.Ports, 1)
		assert.Equal(t, vectorPort, container.Ports[0].ContainerPort)
	})

	t.Run("with custom port", func(t *testing.T) {
		image := util.NewImage("vector", "0.1.0", "1.0.0")
		customPort := int32(9999)
		vector := NewVector("config-volume", "log-volume", image, func(o *VectorOptions) {
			o.Port = customPort
		})

		container := vector.GetContainer()

		assert.Equal(t, VectorContainerName, container.Name)
		assert.Equal(t, image.String(), container.Image)
		assert.Len(t, container.Ports, 1)
		assert.Equal(t, customPort, container.Ports[0].ContainerPort)
	})
}

func TestVector_GetVolumes(t *testing.T) {
	image := util.NewImage("vector", "0.1.0", "1.0.0")
	customSize := resource.NewQuantity(100*1024*1024, resource.BinarySI)
	vector := NewVector("config-volume", "log-volume", image, func(o *VectorOptions) {
		o.VectorDataSize = customSize
	})

	volumes := vector.GetVolumes()

	assert.Len(t, volumes, 1)
	assert.Equal(t, "vector-data", volumes[0].Name)
	assert.NotNil(t, volumes[0].EmptyDir)
	assert.Equal(t, customSize, volumes[0].EmptyDir.SizeLimit)

}

func TestVector_getVolumeMounts(t *testing.T) {
	image := util.NewImage("vector", "0.1.0", "1.0.0")
	vector := NewVector("config-volume", "log-volume", image)

	mounts := vector.getVolumeMounts()

	assert.Len(t, mounts, 3)
	expectedMounts := map[string]string{
		"log-volume":    constants.KubedoopLogDir,
		"config-volume": constants.KubedoopConfigDir,
		"vector-data":   vectorDataDir,
	}

	for _, mount := range mounts {
		expectedPath, exists := expectedMounts[mount.Name]
		assert.True(t, exists)
		assert.Equal(t, expectedPath, mount.MountPath)
	}
}
