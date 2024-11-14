package builder

import (
	"context"
	"fmt"

	resourceClient "github.com/zncdatadev/operator-go/pkg/client"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// TODO: remove Generic prefix in next release

var _ ServiceAccountBuilder = &GenericServiceAccountBuilder{}

type GenericServiceAccountBuilder struct {
	ObjectMeta

	obj *corev1.ServiceAccount
}

func NewGenericServiceAccountBuilder(
	client *resourceClient.Client,
	name string,
	labels map[string]string,
	annotations map[string]string,
) *GenericServiceAccountBuilder {
	return &GenericServiceAccountBuilder{
		ObjectMeta: ObjectMeta{
			Client:      client,
			Name:        name,
			labels:      labels,
			annotations: annotations,
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
	ObjectMeta

	obj   *rbacv1.Role
	rules []rbacv1.PolicyRule
}

func NewGenericRoleBuilder(
	client *resourceClient.Client,
	name string,
	labels map[string]string,
	annotations map[string]string,
) *GenericRoleBuilder {
	return &GenericRoleBuilder{
		ObjectMeta: ObjectMeta{
			Client:      client,
			Name:        name,
			labels:      labels,
			annotations: annotations,
		},
	}
}

func (b *GenericRoleBuilder) AddPolicyRule(rule rbacv1.PolicyRule) {
	b.rules = append(b.rules, rule)
}

func (b *GenericRoleBuilder) AddPolicyRules(rules []rbacv1.PolicyRule) {
	b.rules = append(b.rules, rules...)
}

func (b *GenericRoleBuilder) ResetPolicyRules(rules []rbacv1.PolicyRule) {
	b.rules = rules
}

// set obj
func (b *GenericRoleBuilder) SetObject(obj *rbacv1.Role) {
	b.obj = obj
}

func (b *GenericRoleBuilder) GetObject() *rbacv1.Role {
	if b.obj == nil {
		b.obj = &rbacv1.Role{
			ObjectMeta: b.GetObjectMeta(),
			Rules:      b.rules,
		}
	}
	return b.obj
}

func (b *GenericRoleBuilder) Build(ctx context.Context) (ctrlclient.Object, error) {
	return b.GetObject(), nil
}

var _ RoleBindingBuilder = &GenericRoleBindingBuilder{}

type GenericRoleBindingBuilder struct {
	ObjectMeta

	obj      *rbacv1.RoleBinding
	subjects []rbacv1.Subject
	roleRef  rbacv1.RoleRef
}

func NewGenericRoleBindingBuilder(
	client *resourceClient.Client,
	name string,
	labels map[string]string,
	annotations map[string]string,
) *GenericRoleBindingBuilder {
	return &GenericRoleBindingBuilder{
		ObjectMeta: ObjectMeta{
			Client:      client,
			Name:        name,
			labels:      labels,
			annotations: annotations,
		},
	}
}

// add subect
func (b *GenericRoleBindingBuilder) AddSubject(saName string) RoleBindingBuilder {
	subject := rbacv1.Subject{
		Kind:      "ServiceAccount",
		Name:      saName,
		Namespace: b.Client.GetOwnerNamespace(),
	}
	b.subjects = append(b.subjects, subject)
	return b
}

// set subjects
// after the  resource is applied, the subjects can be set continuously
func (b *GenericRoleBindingBuilder) SetSubjects(saNames []string) RoleBindingBuilder {
	var subjects []rbacv1.Subject
	for _, saName := range saNames {
		subject := rbacv1.Subject{
			Kind:      "ServiceAccount",
			Name:      saName,
			Namespace: b.Client.GetOwnerNamespace(),
		}
		subjects = append(subjects, subject)
	}
	b.subjects = subjects
	return b
}

// set roleref
// when obj not provided, need to set it, after the resource is applied, the roleRef is Immutable
func (b *GenericRoleBindingBuilder) SetRoleRef(roleRefName string, isCluster bool) RoleBindingBuilder {
	kind := "Role"
	if isCluster {
		kind = "ClusterRole"
	}
	b.roleRef = rbacv1.RoleRef{
		APIGroup: "rbac.authorization.k8s.io",
		Kind:     kind,
		Name:     roleRefName,
	}
	return b
}
func (b *GenericRoleBindingBuilder) GetObject() *rbacv1.RoleBinding {
	if b.obj == nil {
		b.obj = &rbacv1.RoleBinding{
			ObjectMeta: b.GetObjectMeta(),
			Subjects:   b.subjects,
			RoleRef:    b.roleRef,
		}
	}
	return b.obj
}

func (b *GenericRoleBindingBuilder) Build(ctx context.Context) (ctrlclient.Object, error) {
	return b.GetObject(), nil
}

var _ ClusterRoleBuilder = &GenericClusterRoleBuilder{}

type GenericClusterRoleBuilder struct {
	ObjectMeta

	obj *rbacv1.ClusterRole
}

func NewGenericClusterRoleBuilder(
	client *resourceClient.Client,
	name string,
	labels map[string]string,
	annotations map[string]string,
) *GenericClusterRoleBuilder {
	return &GenericClusterRoleBuilder{
		ObjectMeta: ObjectMeta{
			Client:      client,
			Name:        name,
			labels:      labels,
			annotations: annotations,
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
	ObjectMeta

	obj *rbacv1.ClusterRoleBinding

	roleRef  rbacv1.RoleRef
	subjects []rbacv1.Subject
}

func NewGenericClusterRoleBindingBuilder(
	client *resourceClient.Client,
	name string,
	labels map[string]string,
	annotations map[string]string,
) *GenericClusterRoleBindingBuilder {
	return &GenericClusterRoleBindingBuilder{
		ObjectMeta: ObjectMeta{
			Client:      client,
			Name:        name,
			labels:      labels,
			annotations: annotations,
		},
	}
}

// set clusterRoleBinding
func (b *GenericClusterRoleBindingBuilder) SetClusterRoleBinding(obj *rbacv1.ClusterRoleBinding) ClusterRoleBindingBuilder {
	b.obj = obj
	return b
}

// add subect
func (b *GenericClusterRoleBindingBuilder) AddSubject(saName string) ClusterRoleBindingBuilder {
	subject := rbacv1.Subject{
		Kind:      "ServiceAccount",
		Name:      saName,
		Namespace: b.Client.GetOwnerNamespace(),
	}
	b.subjects = append(b.subjects, subject)
	return b
}

// set subjects
// after the  resource is applied, the subjects can be set continuously
func (b *GenericClusterRoleBindingBuilder) SetSubjects(saNames []string) ClusterRoleBindingBuilder {
	var subjects []rbacv1.Subject
	for _, saName := range saNames {
		subject := rbacv1.Subject{
			Kind:      "ServiceAccount",
			Name:      saName,
			Namespace: b.Client.GetOwnerNamespace(),
		}
		subjects = append(subjects, subject)
	}
	b.subjects = subjects
	return b
}

// set roleref
// when obj not provided, need to set it, after the resource is applied, the roleRef is Immutable
func (b *GenericClusterRoleBindingBuilder) SetRoleRef(roleRefName string) ClusterRoleBindingBuilder {
	b.roleRef = rbacv1.RoleRef{
		APIGroup: "rbac.authorization.k8s.io",
		Kind:     "ClusterRole",
		Name:     roleRefName,
	}
	return b
}

func (b *GenericClusterRoleBindingBuilder) GetObject() *rbacv1.ClusterRoleBinding {
	if b.obj == nil {
		b.obj = &rbacv1.ClusterRoleBinding{
			ObjectMeta: b.GetObjectMeta(),
			RoleRef:    b.roleRef,
		}
		b.obj.Subjects = b.subjects
		b.obj.RoleRef = b.roleRef
	}
	return b.obj
}

func (b *GenericClusterRoleBindingBuilder) Build(ctx context.Context) (ctrlclient.Object, error) {
	return b.GetObject(), nil
}

// Deprecated: will be removed in next release
func ServiceAccountName(rbacPrefix string) string {
	return fmt.Sprintf("%s-serviceaccount", rbacPrefix)
}

// Deprecated: will be removed in next release
func RoleBindingName(rbacPrefix string) string {
	return fmt.Sprintf("%s-rolebinding", rbacPrefix)
}

// Deprecated: will be removed in next release
func ClusterRoleBindingName(rbacPrefix string) string {
	return fmt.Sprintf("%s-clusterrolebinding", rbacPrefix)
}

// Deprecated: will be removed in next release
func RoleName(rbacPrefix string) string {
	return fmt.Sprintf("%s-role", rbacPrefix)
}

// Deprecated: will be removed in next release
func ClusterRoleName(rbacPrefix string) string {
	return fmt.Sprintf("%s-clusterrole", rbacPrefix)
}
