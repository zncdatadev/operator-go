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
	"context"

	"github.com/zncdatadev/operator-go/pkg/client"
	corev1 "k8s.io/api/core/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type ConfigBuilder interface {
	ObjectBuilder
	AddData(data map[string]string) ConfigBuilder
	AddItem(key, value string) ConfigBuilder
	SetData(data map[string]string) ConfigBuilder
	ClearData() ConfigBuilder
	GetData() map[string]string
}

var _ ConfigBuilder = &BaseConfigBuilder{}

type BaseConfigBuilder struct {
	ObjectMeta

	data map[string]string
}

func NewBaseConfigBuilder(
	client *client.Client,
	name string,
	options ...Option,
) *BaseConfigBuilder {

	opts := &Options{}
	for _, o := range options {
		o(opts)
	}

	return &BaseConfigBuilder{
		ObjectMeta: *NewObjectMeta(client, name, options...),
		data:       make(map[string]string),
	}
}

func (b *BaseConfigBuilder) AddData(data map[string]string) ConfigBuilder {
	for k, v := range data {
		b.data[k] = v
	}
	return b
}

func (b *BaseConfigBuilder) AddItem(key, value string) ConfigBuilder {
	b.data[key] = value
	return b
}

func (b *BaseConfigBuilder) SetData(data map[string]string) ConfigBuilder {
	b.data = data
	return b
}

func (b *BaseConfigBuilder) ClearData() ConfigBuilder {
	b.data = make(map[string]string)
	return b
}

func (b *BaseConfigBuilder) GetData() map[string]string {
	return b.data
}

func (b *BaseConfigBuilder) Build(_ context.Context) (ctrlclient.Object, error) {
	panic("implement me")
}

var _ ConfigBuilder = &ConfigMapBuilder{}

type ConfigMapBuilder struct {
	BaseConfigBuilder
}

func NewConfigMapBuilder(
	client *client.Client,
	name string,
	options ...Option,
) *ConfigMapBuilder {
	return &ConfigMapBuilder{
		BaseConfigBuilder: *NewBaseConfigBuilder(
			client,
			name,
			options...,
		),
	}
}

func (b *ConfigMapBuilder) GetObject() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: b.GetObjectMeta(),
		Data:       b.GetData(),
	}
}

func (b *ConfigMapBuilder) Build(_ context.Context) (ctrlclient.Object, error) {
	return b.GetObject(), nil
}

type SecretBuilder struct {
	BaseConfigBuilder
}

var _ ConfigBuilder = &SecretBuilder{}

func NewSecretBuilder(
	client *client.Client,
	name string,
	options ...Option,
) *SecretBuilder {
	return &SecretBuilder{
		BaseConfigBuilder: *NewBaseConfigBuilder(
			client,
			name,
			options...,
		),
	}
}

func (b *SecretBuilder) GetObject() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: b.GetObjectMeta(),
		StringData: b.GetData(),
	}
}

func (b *SecretBuilder) Build(_ context.Context) (ctrlclient.Object, error) {
	return b.GetObject(), nil
}
