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
	labels map[string]string,
	annotations map[string]string,
) *BaseConfigBuilder {
	return &BaseConfigBuilder{
		ObjectMeta: ObjectMeta{
			Client:      client,
			Name:        name,
			labels:      labels,
			annotations: annotations,
		},
		data: make(map[string]string),
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
	labels map[string]string,
	annotations map[string]string,
) *ConfigMapBuilder {
	return &ConfigMapBuilder{
		BaseConfigBuilder: *NewBaseConfigBuilder(
			client,
			name,
			labels,
			annotations,
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
	labels map[string]string,
	annotations map[string]string,
) *SecretBuilder {
	return &SecretBuilder{
		BaseConfigBuilder: *NewBaseConfigBuilder(
			client,
			name,
			labels,
			annotations,
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
