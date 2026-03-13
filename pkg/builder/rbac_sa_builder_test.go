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

package builder_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/builder"
	rbacv1 "k8s.io/api/rbac/v1"
)

var _ = Describe("ServiceAccountBuilder", func() {
	It("should build a ServiceAccount with name and namespace", func() {
		sa := builder.NewServiceAccountBuilder("my-sa", "default").Build()
		Expect(sa.Name).To(Equal("my-sa"))
		Expect(sa.Namespace).To(Equal("default"))
	})

	It("should build a ServiceAccount with labels", func() {
		sa := builder.NewServiceAccountBuilder("sa", "ns").
			WithLabels(map[string]string{"app": "test"}).
			Build()
		Expect(sa.Labels).To(HaveKeyWithValue("app", "test"))
	})

	It("should build a ServiceAccount with annotations", func() {
		sa := builder.NewServiceAccountBuilder("sa", "ns").
			WithAnnotations(map[string]string{"iam.amazonaws.com/role": "arn:aws:iam::123:role/test"}).
			Build()
		Expect(sa.Annotations).To(HaveKey("iam.amazonaws.com/role"))
	})
})

var _ = Describe("RoleBuilder", func() {
	It("should build a Role with policy rules", func() {
		role := builder.NewRoleBuilder("my-role", "default").
			AddPolicyRule(rbacv1.PolicyRule{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "list"},
			}).
			Build()
		Expect(role.Name).To(Equal("my-role"))
		Expect(role.Namespace).To(Equal("default"))
		Expect(role.Rules).To(HaveLen(1))
		Expect(role.Rules[0].Resources).To(ContainElement("pods"))
	})
})

var _ = Describe("RoleBindingBuilder", func() {
	It("should build a RoleBinding with ServiceAccount subject", func() {
		rb := builder.NewRoleBindingBuilder("my-rb", "default").
			WithRoleRef("my-role").
			AddServiceAccountSubject("my-sa", "default").
			Build()
		Expect(rb.Name).To(Equal("my-rb"))
		Expect(rb.RoleRef.Name).To(Equal("my-role"))
		Expect(rb.RoleRef.Kind).To(Equal("Role"))
		Expect(rb.Subjects).To(HaveLen(1))
		Expect(rb.Subjects[0].Name).To(Equal("my-sa"))
		Expect(rb.Subjects[0].Kind).To(Equal(rbacv1.ServiceAccountKind))
	})
})

var _ = Describe("ClusterRoleBuilder", func() {
	It("should build a ClusterRole", func() {
		cr := builder.NewClusterRoleBuilder("cluster-reader").
			AddPolicyRule(rbacv1.PolicyRule{
				APIGroups: []string{""},
				Resources: []string{"nodes"},
				Verbs:     []string{"get", "list", "watch"},
			}).
			Build()
		Expect(cr.Name).To(Equal("cluster-reader"))
		Expect(cr.Rules).To(HaveLen(1))
		Expect(cr.Namespace).To(BeEmpty()) // ClusterRole has no namespace
	})
})

var _ = Describe("ClusterRoleBindingBuilder", func() {
	It("should build a ClusterRoleBinding", func() {
		crb := builder.NewClusterRoleBindingBuilder("cluster-reader-binding").
			WithClusterRoleRef("cluster-reader").
			AddServiceAccountSubject("my-sa", "default").
			Build()
		Expect(crb.Name).To(Equal("cluster-reader-binding"))
		Expect(crb.RoleRef.Kind).To(Equal("ClusterRole"))
		Expect(crb.RoleRef.Name).To(Equal("cluster-reader"))
		Expect(crb.Subjects).To(HaveLen(1))
	})
})
