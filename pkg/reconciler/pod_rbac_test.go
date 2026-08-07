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
	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/constant"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/testutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
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

var _ = Describe("Workload RBAC lifecycle", func() {
	var ctx context.Context
	var cr *testutil.MockCluster
	var name string

	rules := []rbacv1.PolicyRule{{
		APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"get", "list"},
	}}

	BeforeEach(func() {
		ctx = context.Background()
		name = fmt.Sprintf("rbac-life-%d", time.Now().UnixNano())
		cr = testutil.NewMockCluster(name, testNamespace)
		Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, cr)
			meta := metav1.ObjectMeta{Name: name, Namespace: testNamespace}
			_ = k8sClient.Delete(ctx, &rbacv1.RoleBinding{ObjectMeta: meta})
			_ = k8sClient.Delete(ctx, &rbacv1.Role{ObjectMeta: meta})
		})
	})

	key := func() types.NamespacedName {
		return types.NamespacedName{Namespace: testNamespace, Name: name}
	}

	It("revokes when the rule set becomes empty", func() {
		Expect(reconciler.EnsurePodRBAC(ctx, k8sClient, testScheme, cr, name, rules)).To(Succeed())
		Expect(k8sClient.Get(ctx, key(), &rbacv1.Role{})).To(Succeed())

		// The product turned the feature off. Narrowing to zero is the largest narrowing there
		// is; leaving the Role in place means the pods keep permissions nobody grants any more.
		Expect(reconciler.EnsurePodRBAC(ctx, k8sClient, testScheme, cr, name, nil)).To(Succeed())

		Expect(k8serrors.IsNotFound(k8sClient.Get(ctx, key(), &rbacv1.Role{}))).To(BeTrue(),
			"the Role must be deleted when no rules are declared")
		Expect(k8serrors.IsNotFound(k8sClient.Get(ctx, key(), &rbacv1.RoleBinding{}))).To(BeTrue(),
			"the RoleBinding must be deleted too, or it binds a Role that no longer exists")
	})

	It("does not revoke an object it does not control", func() {
		foreign := &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
			Rules:      rules,
		}
		Expect(k8sClient.Create(ctx, foreign)).To(Succeed())

		Expect(reconciler.EnsurePodRBAC(ctx, k8sClient, testScheme, cr, name, nil)).To(Succeed())

		// Deleting an object another controller maintains because it happens to share a name is
		// the worse failure; the ownership gate is the same one every reclaim here applies.
		Expect(k8sClient.Get(ctx, key(), &rbacv1.Role{})).To(Succeed())
	})

	It("refuses to adopt a pre-existing RoleBinding pointing somewhere else", func() {
		// A RoleBinding left at this name by an older operator, a Helm chart, or the
		// ExtraResources route this replaces. Rebinding it to the workload SA while keeping its
		// roleRef would hand those pods whatever the old ref allows.
		pre := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
			RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: "cluster-admin"},
			Subjects:   []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: "someone-else", Namespace: testNamespace}},
		}
		Expect(k8sClient.Create(ctx, pre)).To(Succeed())

		err := reconciler.EnsurePodRBAC(ctx, k8sClient, testScheme, cr, name, rules)
		Expect(err).To(HaveOccurred(), "adopting a foreign roleRef is a privilege escalation")
		Expect(reconciler.IsValidationError(err)).To(BeTrue(), "want a *ValidationError, got %T", err)
		for _, want := range []string{"cluster-admin", "immutable", "kubectl"} {
			Expect(err.Error()).To(ContainSubstring(want))
		}

		got := &rbacv1.RoleBinding{}
		Expect(k8sClient.Get(ctx, key(), got)).To(Succeed())
		Expect(got.Subjects[0].Name).To(Equal("someone-else"),
			"the existing binding's subject must not be rewritten to the workload ServiceAccount")
	})
})

// The maintainer's model in one spec: workload RBAC is created at CLUSTER level from the CR, and
// the role group merely CONSUMES it — the ServiceAccount on the pod template is the very one the
// RoleBinding grants to. That cross-check is what the config field exists for; with the exported
// helper the product supplies the name a second time and nothing verifies the two agree.
var _ = Describe("PodRBACRules wiring", func() {
	var ctx context.Context

	BeforeEach(func() { ctx = context.Background() })

	It("grants to the same ServiceAccount the role group puts on the pod template", func() {
		name := fmt.Sprintf("rbac-wire-%d", time.Now().UnixNano())
		saName := name + "-sa"
		cr := createSlotCR(ctx, name, nil, map[string]v1alpha1.RoleSpec{"w": defaultGroupRole()})
		DeferCleanup(func() {
			meta := metav1.ObjectMeta{Name: saName, Namespace: testNamespace}
			_ = k8sClient.Delete(ctx, &rbacv1.RoleBinding{ObjectMeta: meta})
			_ = k8sClient.Delete(ctx, &rbacv1.Role{ObjectMeta: meta})
			_ = k8sClient.Delete(ctx, &corev1.ServiceAccount{ObjectMeta: meta})
		})

		base := reconciler.NewBaseRoleGroupHandler[*testutil.MockCluster]("product:1", testScheme)
		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:                 k8sClient,
			Scheme:                 testScheme,
			Recorder:               recorder,
			RoleGroupHandler:       base,
			Prototype:              testutil.NewMockCluster("proto", testNamespace),
			ServiceAccountNameFunc: func(*testutil.MockCluster) string { return saName },
			PodRBACRules: func(*testutil.MockCluster) []rbacv1.PolicyRule {
				return []rbacv1.PolicyRule{{
					APIGroups: []string{"coordination.k8s.io"},
					Resources: []string{"leases"},
					Verbs:     []string{"get", "create", "update"},
				}}
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(reconcileSlotCR(ctx, r, name)).To(Succeed())

		key := types.NamespacedName{Namespace: testNamespace, Name: saName}
		role := &rbacv1.Role{}
		Expect(k8sClient.Get(ctx, key, role)).To(Succeed(), "the framework must create the Role itself")
		Expect(role.Rules[0].Resources).To(ConsistOf("leases"))

		binding := &rbacv1.RoleBinding{}
		Expect(k8sClient.Get(ctx, key, binding)).To(Succeed())

		sts := &appsv1.StatefulSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Namespace: testNamespace,
			Name:      reconciler.RoleGroupResourceName(name, "w", "default"),
		}, sts)).To(Succeed())

		// The cross-check the two-copies shape cannot make. If these ever diverge, the Role grants
		// to a ServiceAccount no pod uses: both objects exist, the pods start, and the first API
		// call 403s with nothing anywhere reporting why.
		Expect(binding.Subjects[0].Name).To(Equal(sts.Spec.Template.Spec.ServiceAccountName))
		Expect(binding.Subjects[0].Name).To(Equal(saName))
		Expect(metav1.GetControllerOf(role).UID).To(Equal(cr.UID))
	})

	It("warns instead of silently granting nothing when SA management is off", func() {
		name := fmt.Sprintf("rbac-nosa-%d", time.Now().UnixNano())
		createSlotCR(ctx, name, nil, map[string]v1alpha1.RoleSpec{"w": defaultGroupRole()})

		fake := record.NewFakeRecorder(100)
		base := reconciler.NewBaseRoleGroupHandler[*testutil.MockCluster]("product:1", testScheme)
		r, err := reconciler.NewGenericReconciler(&reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:           k8sClient,
			Scheme:           testScheme,
			Recorder:         fake,
			RoleGroupHandler: base,
			Prototype:        testutil.NewMockCluster("proto", testNamespace),
			// No ServiceAccountName, no ServiceAccountNameFunc: nothing to grant to.
			PodRBACRules: func(*testutil.MockCluster) []rbacv1.PolicyRule {
				return []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}}}
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(reconcileSlotCR(ctx, r, name)).To(Succeed())

		Expect(drainRecorder(fake)).To(ContainElement(SatisfyAll(
			ContainSubstring("Warning"), ContainSubstring("PodRBACSkipped"))),
			"declaring rules with no identity to grant them to is always a mistake")
	})
})
