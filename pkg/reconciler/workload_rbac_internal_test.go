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

package reconciler

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// These live in the INTERNAL test package because the properties they guard vanish the moment an
// object crosses the API boundary: every client — real, cached or fake — deep-copies on serialize,
// so two objects that shared one map in memory come back as two independent maps. An assertion
// written against what EnsureWorkloadRBAC wrote therefore passes with or without the copies.
//
// buildWorkloadRBAC exists so that is not the end of the story. It renders both objects as pure
// data, with no client and no live state, so a spec CAN hold them at once and prove they share
// nothing — the call-site discipline that was previously unreachable, and that the first draft of
// this file got wrong with both helpers already in place.
var _ = Describe("workload RBAC object construction", func() {
	owner := &metav1.PartialObjectMetadata{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "ns"},
	}
	key := types.NamespacedName{Namespace: "ns", Name: "mockcluster-cluster"}
	roleRef := rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: workloadRoleKind, Name: key.Name}

	It("gives the Role and the RoleBinding independent label maps", func() {
		// The exact defect this file's first draft shipped: one `labels := workloadRBACLabels(…)`
		// hoisted above both mutate funcs, so a single map was assigned to two API objects.
		role, binding := buildWorkloadRBAC(owner, key, roleRef,
			[]rbacv1.PolicyRule{{Verbs: []string{"get"}}}, &workloadRBACOptions{})

		Expect(role.Labels).To(Equal(binding.Labels))
		role.Labels["only-on-the-role"] = "x"
		Expect(binding.Labels).NotTo(HaveKey("only-on-the-role"),
			"the two objects must not be able to rewrite each other's labels")
	})

	It("does not store the caller's rule slice in the Role", func() {
		rules := []rbacv1.PolicyRule{{
			APIGroups: []string{"coordination.k8s.io"},
			Resources: []string{"leases"},
			Verbs:     []string{"get"},
		}}
		role, _ := buildWorkloadRBAC(owner, key, roleRef, rules, &workloadRBACOptions{})

		rules[0].Verbs[0] = "delete"
		Expect(role.Rules[0].Verbs).To(Equal([]string{"get"}),
			"the product keeps a reference to its rules; the built object must not alias them")
	})

	It("names both objects after the ServiceAccount and binds that ServiceAccount", func() {
		role, binding := buildWorkloadRBAC(owner, key, roleRef,
			[]rbacv1.PolicyRule{{Verbs: []string{"get"}}}, &workloadRBACOptions{})

		Expect(role.Name).To(Equal(key.Name))
		Expect(role.Namespace).To(Equal(key.Namespace))
		Expect(binding.Name).To(Equal(key.Name))
		Expect(binding.RoleRef).To(Equal(roleRef))
		Expect(binding.Subjects).To(Equal([]rbacv1.Subject{{
			Kind: rbacv1.ServiceAccountKind, Name: key.Name, Namespace: key.Namespace,
		}}), "the binding's subject IS the workload's ServiceAccount, by construction")
	})

	It("returns an independent label map on every call", func() {
		options := &workloadRBACOptions{productName: "nifi", labels: map[string]string{"extra": "v"}}

		first := workloadRBACLabels(owner, options)
		second := workloadRBACLabels(owner, options)
		Expect(first).To(Equal(second))

		first["only-on-first"] = "x"
		Expect(second).NotTo(HaveKey("only-on-first"),
			"two objects assigned these maps must not be able to rewrite each other's labels")
	})

	It("does not alias the caller's extra-label map", func() {
		extras := map[string]string{"extra": "v"}
		labels := workloadRBACLabels(owner, &workloadRBACOptions{labels: extras})

		labels["added-by-framework"] = "x"
		Expect(extras).NotTo(HaveKey("added-by-framework"),
			"the caller's map must not gain the canonical labels")
	})

	It("deep-copies the caller's rules, including the string slices inside them", func() {
		in := []rbacv1.PolicyRule{{
			APIGroups: []string{"coordination.k8s.io"},
			Resources: []string{"leases"},
			Verbs:     []string{"get"},
		}}
		out := cloneWorkloadRules(in)
		Expect(out).To(Equal(in))

		// A shallow copy would share the Verbs backing array, so this is what separates
		// cloneWorkloadRules from `copy()` or `return rules`.
		in[0].Verbs[0] = "delete"
		in[0].Resources[0] = "secrets"
		Expect(out[0].Verbs).To(Equal([]string{"get"}))
		Expect(out[0].Resources).To(Equal([]string{"leases"}))
	})

	It("survives an empty rule set", func() {
		Expect(cloneWorkloadRules(nil)).To(BeEmpty())
	})
})
