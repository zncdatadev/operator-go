package builder

import (
	"context"

	resourceClient "github.com/zncdatadev/operator-go/pkg/client"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type ServiceAccountBuilder interface {
	Builder
	GetObject() *corev1.ServiceAccount
}

type RoleBuilder interface {
	Builder
	GetObject() *rbacv1.Role
}

type RoleBindingBuilder interface {
	Builder
	GetObject() *rbacv1.RoleBinding
}

type ClusterRoleBuilder interface {
	Builder
	GetObject() *rbacv1.ClusterRole
}

type ClusterRoleBindingBuilder interface {
	Builder
	GetObject() *rbacv1.ClusterRoleBinding
}

var _ ServiceAccountBuilder = &GenericServiceAccountBuilder{}

type GenericServiceAccountBuilder struct {
	BaseResourceBuilder

	obj *corev1.ServiceAccount
}

func NewGenericServiceAccountBuilder(
	client *resourceClient.Client,
	options Options,
) *GenericServiceAccountBuilder {
	return &GenericServiceAccountBuilder{
		BaseResourceBuilder: BaseResourceBuilder{
			Client:  client,
			Options: options,
		},
	}
}

func (b *GenericServiceAccountBuilder) GetObject() *corev1.ServiceAccount {
	if b.obj == nil {
		b.obj = &corev1.ServiceAccount{
			ObjectMeta: b.GetObjectMeta(),
		}
	}
	return b.obj
}

func (b *GenericServiceAccountBuilder) Build(ctx context.Context) (ctrlclient.Object, error) {
	return b.GetObject(), nil
}

var _ RoleBuilder = &GenericRoleBuilder{}

type GenericRoleBuilder struct {
	BaseResourceBuilder

	obj *rbacv1.Role
}

func NewGenericRoleBuilder(
	client *resourceClient.Client,
	options Options,
) *GenericRoleBuilder {
	return &GenericRoleBuilder{
		BaseResourceBuilder: BaseResourceBuilder{
			Client:  client,
			Options: options,
		},
	}
}

func (b *GenericRoleBuilder) GetObject() *rbacv1.Role {
	if b.obj == nil {
		b.obj = &rbacv1.Role{
			ObjectMeta: b.GetObjectMeta(),
		}
	}
	return b.obj
}

func (b *GenericRoleBuilder) Build(ctx context.Context) (ctrlclient.Object, error) {
	return b.GetObject(), nil
}

var _ RoleBindingBuilder = &GenericRoleBindingBuilder{}

type GenericRoleBindingBuilder struct {
	BaseResourceBuilder

	obj *rbacv1.RoleBinding
}

func NewGenericRoleBindingBuilder(
	client *resourceClient.Client,
	options Options,
) *GenericRoleBindingBuilder {
	return &GenericRoleBindingBuilder{
		BaseResourceBuilder: BaseResourceBuilder{
			Client:  client,
			Options: options,
		},
	}
}

func (b *GenericRoleBindingBuilder) GetObject() *rbacv1.RoleBinding {
	if b.obj == nil {
		b.obj = &rbacv1.RoleBinding{
			ObjectMeta: b.GetObjectMeta(),
		}
	}
	return b.obj
}

func (b *GenericRoleBindingBuilder) Build(ctx context.Context) (ctrlclient.Object, error) {
	return b.GetObject(), nil
}

var _ ClusterRoleBuilder = &GenericClusterRoleBuilder{}

type GenericClusterRoleBuilder struct {
	BaseResourceBuilder

	obj *rbacv1.ClusterRole
}

func NewGenericClusterRoleBuilder(
	client *resourceClient.Client,
	options Options,
) *GenericClusterRoleBuilder {
	return &GenericClusterRoleBuilder{
		BaseResourceBuilder: BaseResourceBuilder{
			Client:  client,
			Options: options,
		},
	}
}

func (b *GenericClusterRoleBuilder) GetObject() *rbacv1.ClusterRole {
	if b.obj == nil {
		b.obj = &rbacv1.ClusterRole{
			ObjectMeta: b.GetObjectMeta(),
		}
	}
	return b.obj
}

func (b *GenericClusterRoleBuilder) Build(ctx context.Context) (ctrlclient.Object, error) {
	return b.GetObject(), nil
}

var _ ClusterRoleBindingBuilder = &GenericClusterRoleBindingBuilder{}

type GenericClusterRoleBindingBuilder struct {
	BaseResourceBuilder

	obj *rbacv1.ClusterRoleBinding

	roleRef rbacv1.RoleRef
}

func NewGenericClusterRoleBindingBuilder(
	client *resourceClient.Client,
	options Options,
) *GenericClusterRoleBindingBuilder {
	return &GenericClusterRoleBindingBuilder{
		BaseResourceBuilder: BaseResourceBuilder{
			Client:  client,
			Options: options,
		},
	}
}

func (b *GenericClusterRoleBindingBuilder) GetObject() *rbacv1.ClusterRoleBinding {
	if b.obj == nil {
		b.obj = &rbacv1.ClusterRoleBinding{
			ObjectMeta: b.GetObjectMeta(),
			RoleRef:    b.roleRef,
		}
	}
	return b.obj
}

func (b *GenericClusterRoleBindingBuilder) Build(ctx context.Context) (ctrlclient.Object, error) {
	return b.GetObject(), nil
}
