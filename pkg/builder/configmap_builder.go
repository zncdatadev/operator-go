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
	"fmt"

	"github.com/zncdatadev/operator-go/pkg/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// ConfigMapBuilder constructs ConfigMap resources.
type ConfigMapBuilder struct {
	Name        string
	Namespace   string
	Labels      map[string]string
	Annotations map[string]string
	Data        map[string]string
	BinaryData  map[string][]byte
}

// NewConfigMapBuilder creates a new ConfigMapBuilder.
func NewConfigMapBuilder(name, namespace string) *ConfigMapBuilder {
	return &ConfigMapBuilder{
		Name:        name,
		Namespace:   namespace,
		Labels:      make(map[string]string),
		Annotations: make(map[string]string),
		Data:        make(map[string]string),
		BinaryData:  make(map[string][]byte),
	}
}

// WithLabels sets the labels.
func (b *ConfigMapBuilder) WithLabels(labels map[string]string) *ConfigMapBuilder {
	for k, v := range labels {
		b.Labels[k] = v
	}
	return b
}

// WithAnnotations sets the annotations.
func (b *ConfigMapBuilder) WithAnnotations(annotations map[string]string) *ConfigMapBuilder {
	for k, v := range annotations {
		b.Annotations[k] = v
	}
	return b
}

// AddData adds a key-value pair to the data section.
func (b *ConfigMapBuilder) AddData(key, value string) *ConfigMapBuilder {
	b.Data[key] = value
	return b
}

// AddBinaryData adds a key-value pair to the binary data section.
func (b *ConfigMapBuilder) AddBinaryData(key string, value []byte) *ConfigMapBuilder {
	b.BinaryData[key] = value
	return b
}

// WithConfigFiles sets the data from a config file map.
func (b *ConfigMapBuilder) WithConfigFiles(files map[string]string) *ConfigMapBuilder {
	for filename, content := range files {
		b.Data[filename] = content
	}
	return b
}

// WithMergedConfig sets the data from a MergedConfig using the provided generator.
// Returns an error if config generation fails to prevent creating ConfigMaps with incomplete data.
func (b *ConfigMapBuilder) WithMergedConfig(cfg *config.MergedConfig, generator *config.MultiFormatConfigGenerator) (*ConfigMapBuilder, error) {
	if cfg == nil || generator == nil {
		return b, nil
	}

	files, err := generator.GenerateFiles(cfg.ConfigFiles)
	if err != nil {
		return b, fmt.Errorf("failed to generate config files for %s: %w", b.Name, err)
	}

	for filename, content := range files {
		b.Data[filename] = content
	}

	return b, nil
}

// Build creates the ConfigMap.
func (b *ConfigMapBuilder) Build() *corev1.ConfigMap {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        b.Name,
			Namespace:   b.Namespace,
			Labels:      b.Labels,
			Annotations: b.Annotations,
		},
	}

	if len(b.Data) > 0 {
		cm.Data = b.Data
	}

	if len(b.BinaryData) > 0 {
		cm.BinaryData = b.BinaryData
	}

	return cm
}

// NamespacedName returns the NamespacedName for the ConfigMap.
func (b *ConfigMapBuilder) NamespacedName() types.NamespacedName {
	return types.NamespacedName{
		Name:      b.Name,
		Namespace: b.Namespace,
	}
}
