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
	"maps"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// RoleBuilder builds a Kubernetes Role.
type RoleBuilder struct {
	Name      string
	Namespace string
	Labels    map[string]string
	Rules     []rbacv1.PolicyRule
}

// NewRoleBuilder creates a new RoleBuilder.
func NewRoleBuilder(name, namespace string) *RoleBuilder {
	return &RoleBuilder{
		Name:      name,
		Namespace: namespace,
		Labels:    make(map[string]string),
	}
}

// WithLabels merges the given labels into the builder-owned label map, like every other builder's
// WithLabels: repeated calls accumulate and the caller's map is never aliased.
func (b *RoleBuilder) WithLabels(labels map[string]string) *RoleBuilder {
	for k, v := range labels {
		b.Labels[k] = v
	}
	return b
}

// AddPolicyRule adds a policy rule to the Role.
func (b *RoleBuilder) AddPolicyRule(rule rbacv1.PolicyRule) *RoleBuilder {
	b.Rules = append(b.Rules, rule)
	return b
}

// Build constructs and returns the Role.
func (b *RoleBuilder) Build() *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.Name,
			Namespace: b.Namespace,
			Labels:    maps.Clone(b.Labels),
		},
		Rules: cloneSlice(b.Rules),
	}
}

// NamespacedName returns the NamespacedName for the Role.
func (b *RoleBuilder) NamespacedName() types.NamespacedName {
	return types.NamespacedName{
		Name:      b.Name,
		Namespace: b.Namespace,
	}
}

// RoleBindingBuilder builds a Kubernetes RoleBinding.
type RoleBindingBuilder struct {
	Name      string
	Namespace string
	Labels    map[string]string
	RoleName  string
	Subjects  []rbacv1.Subject
}

// NewRoleBindingBuilder creates a new RoleBindingBuilder.
func NewRoleBindingBuilder(name, namespace string) *RoleBindingBuilder {
	return &RoleBindingBuilder{
		Name:      name,
		Namespace: namespace,
		Labels:    make(map[string]string),
	}
}

// WithLabels merges the given labels into the builder-owned label map.
func (b *RoleBindingBuilder) WithLabels(labels map[string]string) *RoleBindingBuilder {
	for k, v := range labels {
		b.Labels[k] = v
	}
	return b
}

// WithRoleRef sets the Role this binding references.
func (b *RoleBindingBuilder) WithRoleRef(roleName string) *RoleBindingBuilder {
	b.RoleName = roleName
	return b
}

// AddSubject adds a subject (ServiceAccount, User, or Group) to the binding.
func (b *RoleBindingBuilder) AddSubject(subject rbacv1.Subject) *RoleBindingBuilder {
	b.Subjects = append(b.Subjects, subject)
	return b
}

// AddServiceAccountSubject adds a ServiceAccount subject.
func (b *RoleBindingBuilder) AddServiceAccountSubject(name, namespace string) *RoleBindingBuilder {
	return b.AddSubject(rbacv1.Subject{
		Kind:      rbacv1.ServiceAccountKind,
		Name:      name,
		Namespace: namespace,
	})
}

// Build constructs and returns the RoleBinding.
func (b *RoleBindingBuilder) Build() *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.Name,
			Namespace: b.Namespace,
			Labels:    maps.Clone(b.Labels),
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "Role",
			Name:     b.RoleName,
		},
		Subjects: cloneSlice(b.Subjects),
	}
}

// NamespacedName returns the NamespacedName for the RoleBinding.
func (b *RoleBindingBuilder) NamespacedName() types.NamespacedName {
	return types.NamespacedName{
		Name:      b.Name,
		Namespace: b.Namespace,
	}
}

// ClusterRoleBuilder builds a Kubernetes ClusterRole.
type ClusterRoleBuilder struct {
	Name   string
	Labels map[string]string
	Rules  []rbacv1.PolicyRule
}

// NewClusterRoleBuilder creates a new ClusterRoleBuilder.
func NewClusterRoleBuilder(name string) *ClusterRoleBuilder {
	return &ClusterRoleBuilder{Name: name, Labels: make(map[string]string)}
}

// WithLabels merges the given labels into the builder-owned label map.
func (b *ClusterRoleBuilder) WithLabels(labels map[string]string) *ClusterRoleBuilder {
	for k, v := range labels {
		b.Labels[k] = v
	}
	return b
}

// AddPolicyRule adds a policy rule to the ClusterRole.
func (b *ClusterRoleBuilder) AddPolicyRule(rule rbacv1.PolicyRule) *ClusterRoleBuilder {
	b.Rules = append(b.Rules, rule)
	return b
}

// Build constructs and returns the ClusterRole.
func (b *ClusterRoleBuilder) Build() *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:   b.Name,
			Labels: maps.Clone(b.Labels),
		},
		Rules: cloneSlice(b.Rules),
	}
}

// NamespacedName returns the NamespacedName for the ClusterRole. A ClusterRole is cluster-scoped,
// so the Namespace is always empty.
func (b *ClusterRoleBuilder) NamespacedName() types.NamespacedName {
	return types.NamespacedName{Name: b.Name}
}

// ClusterRoleBindingBuilder builds a Kubernetes ClusterRoleBinding.
type ClusterRoleBindingBuilder struct {
	Name            string
	Labels          map[string]string
	ClusterRoleName string
	Subjects        []rbacv1.Subject
}

// NewClusterRoleBindingBuilder creates a new ClusterRoleBindingBuilder.
func NewClusterRoleBindingBuilder(name string) *ClusterRoleBindingBuilder {
	return &ClusterRoleBindingBuilder{Name: name, Labels: make(map[string]string)}
}

// WithLabels merges the given labels into the builder-owned label map.
func (b *ClusterRoleBindingBuilder) WithLabels(labels map[string]string) *ClusterRoleBindingBuilder {
	for k, v := range labels {
		b.Labels[k] = v
	}
	return b
}

// WithClusterRoleRef sets the ClusterRole this binding references.
func (b *ClusterRoleBindingBuilder) WithClusterRoleRef(clusterRoleName string) *ClusterRoleBindingBuilder {
	b.ClusterRoleName = clusterRoleName
	return b
}

// AddSubject adds a subject to the binding.
func (b *ClusterRoleBindingBuilder) AddSubject(subject rbacv1.Subject) *ClusterRoleBindingBuilder {
	b.Subjects = append(b.Subjects, subject)
	return b
}

// AddServiceAccountSubject adds a ServiceAccount subject.
func (b *ClusterRoleBindingBuilder) AddServiceAccountSubject(name, namespace string) *ClusterRoleBindingBuilder {
	return b.AddSubject(rbacv1.Subject{
		Kind:      rbacv1.ServiceAccountKind,
		Name:      name,
		Namespace: namespace,
	})
}

// Build constructs and returns the ClusterRoleBinding.
func (b *ClusterRoleBindingBuilder) Build() *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   b.Name,
			Labels: maps.Clone(b.Labels),
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     b.ClusterRoleName,
		},
		Subjects: cloneSlice(b.Subjects),
	}
}

// NamespacedName returns the NamespacedName for the ClusterRoleBinding. A ClusterRoleBinding is
// cluster-scoped, so the Namespace is always empty.
func (b *ClusterRoleBindingBuilder) NamespacedName() types.NamespacedName {
	return types.NamespacedName{Name: b.Name}
}
