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

package reconciler_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/constant"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/testutil"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("EnsurePodRBAC", func() {
	var ctx context.Context
	var cr *testutil.MockCluster
	var saName string

	leaseRules := []rbacv1.PolicyRule{{
		APIGroups: []string{"coordination.k8s.io"},
		Resources: []string{"leases"},
		Verbs:     []string{"get", "list", "watch", "create", "update"},
	}}

	BeforeEach(func() {
		ctx = context.Background()
		saName = fmt.Sprintf("rbac-sa-%d", time.Now().UnixNano())
		cr = testutil.NewMockCluster(saName, testNamespace)
		Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, cr)
			meta := metav1.ObjectMeta{Name: saName, Namespace: testNamespace}
			_ = k8sClient.Delete(ctx, &rbacv1.RoleBinding{ObjectMeta: meta})
			_ = k8sClient.Delete(ctx, &rbacv1.Role{ObjectMeta: meta})
		})
	})

	key := func() types.NamespacedName {
		return types.NamespacedName{Namespace: testNamespace, Name: saName}
	}

	It("creates a Role and a RoleBinding named after the ServiceAccount, bound to it", func() {
		Expect(reconciler.EnsurePodRBAC(ctx, k8sClient, testScheme, cr, saName, leaseRules,
			reconciler.WithPodRBACProductName("nifi"))).To(Succeed())

		role := &rbacv1.Role{}
		Expect(k8sClient.Get(ctx, key(), role)).To(Succeed())
		Expect(role.Rules).To(Equal(leaseRules))
		Expect(role.Labels).To(HaveKeyWithValue(constant.LabelKubernetesName, "nifi"))
		Expect(role.Labels).To(HaveKeyWithValue(constant.LabelKubernetesInstance, cr.Name))

		binding := &rbacv1.RoleBinding{}
		Expect(k8sClient.Get(ctx, key(), binding)).To(Succeed())
		Expect(binding.RoleRef).To(Equal(rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName, Kind: "Role", Name: saName,
		}))
		// The binding is useless unless it names the SA the pods actually run as.
		Expect(binding.Subjects).To(ConsistOf(rbacv1.Subject{
			Kind: rbacv1.ServiceAccountKind, Name: saName, Namespace: testNamespace,
		}))

		// A CONTROLLER reference, not a plain one: every reclaim in this package tests for it
		// (isOwnedByCluster / GetControllerOf), so a plain owner ref puts the object outside the
		// framework's lifecycle entirely.
		ctrlRef := metav1.GetControllerOf(role)
		Expect(ctrlRef).NotTo(BeNil(), "the Role must be controller-owned by the CR")
		Expect(ctrlRef.UID).To(Equal(cr.UID))
		Expect(metav1.GetControllerOf(binding)).NotTo(BeNil())
	})

	It("is idempotent and converges the rules", func() {
		Expect(reconciler.EnsurePodRBAC(ctx, k8sClient, testScheme, cr, saName, leaseRules)).To(Succeed())

		narrowed := []rbacv1.PolicyRule{{
			APIGroups: []string{"coordination.k8s.io"},
			Resources: []string{"leases"},
			Verbs:     []string{"get"},
		}}
		Expect(reconciler.EnsurePodRBAC(ctx, k8sClient, testScheme, cr, saName, narrowed)).To(Succeed())

		role := &rbacv1.Role{}
		Expect(k8sClient.Get(ctx, key(), role)).To(Succeed())
		// Rules are replaced wholesale: a verb the product removed must stop granting, or
		// narrowing a permission would be impossible.
		Expect(role.Rules).To(Equal(narrowed))
	})

	It("does nothing for an empty rule set", func() {
		Expect(reconciler.EnsurePodRBAC(ctx, k8sClient, testScheme, cr, saName, nil)).To(Succeed())

		// Creating an empty Role would be a grant of nothing that nothing reclaims until the CR
		// is deleted.
		err := k8sClient.Get(ctx, key(), &rbacv1.Role{})
		Expect(err).To(HaveOccurred())
	})

	It("rejects an empty ServiceAccount name", func() {
		err := reconciler.EnsurePodRBAC(ctx, k8sClient, testScheme, cr, "", leaseRules)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("ServiceAccount"))
	})

	It("keeps canonical labels winning over caller-supplied ones", func() {
		Expect(reconciler.EnsurePodRBAC(ctx, k8sClient, testScheme, cr, saName, leaseRules,
			reconciler.WithPodRBACProductName("nifi"),
			reconciler.WithPodRBACExtraLabels(map[string]string{
				constant.LabelKubernetesInstance: "hijacked",
				"example.com/team":               "data",
			}),
		)).To(Succeed())

		role := &rbacv1.Role{}
		Expect(k8sClient.Get(ctx, key(), role)).To(Succeed())
		Expect(role.Labels).To(HaveKeyWithValue(constant.LabelKubernetesInstance, cr.Name))
		Expect(role.Labels).To(HaveKeyWithValue("example.com/team", "data"))
	})
})
