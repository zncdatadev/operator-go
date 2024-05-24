package builder

import (
	"context"

	"github.com/zncdatadev/operator-go/pkg/client"
	corev1 "k8s.io/api/core/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type ConfigBuilder interface {
	Builder
	AddData(data map[string]string) ConfigBuilder
	AddDecodeData(data map[string][]byte) ConfigBuilder
	SetData(data map[string]string) ConfigBuilder
	ClearData() ConfigBuilder
	GetData() map[string]string
	GetEncodeData() map[string][]byte
}

var _ ConfigBuilder = &BaseConfigBuilder{}

type BaseConfigBuilder struct {
	BaseResourceBuilder

	data map[string]string
}

func NewBaseConfigBuilder(
	client *client.Client,
	options Options,
) *BaseConfigBuilder {
	return &BaseConfigBuilder{
		BaseResourceBuilder: BaseResourceBuilder{
			Client:  client,
			Options: options,
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

func (b *BaseConfigBuilder) AddDecodeData(data map[string][]byte) ConfigBuilder {
	for k, v := range data {
		b.data[k] = string(v)
	}
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

func (b *BaseConfigBuilder) GetEncodeData() map[string][]byte {
	data := make(map[string][]byte)
	for k, v := range b.data {
		data[k] = []byte(v)
	}
	return data
}

func (b *BaseConfigBuilder) GetData() map[string]string {
	return b.data
}

var _ ConfigBuilder = &ConfigMapBuilder{}

type ConfigMapBuilder struct {
	BaseConfigBuilder
}

func NewConfigMapBuilder(
	client *client.Client,
	options Options,
) *ConfigMapBuilder {
	return &ConfigMapBuilder{
		BaseConfigBuilder: *NewBaseConfigBuilder(client, options),
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
	options Options,
) *SecretBuilder {
	return &SecretBuilder{
		BaseConfigBuilder: *NewBaseConfigBuilder(client, options),
	}
}

func (b *SecretBuilder) GetObject() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: b.GetObjectMeta(),
		Data:       b.GetEncodeData(),
	}
}

func (b *SecretBuilder) Build(_ context.Context) (ctrlclient.Object, error) {
	return b.GetObject(), nil
}
