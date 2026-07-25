/*
Copyright 2026 ZNCDataDev.

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

package controller

import (
	"os"
	"path/filepath"
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/yaml"
)

// allows reports whether the ClusterRole grants verb on group/resource.
func allows(role *rbacv1.ClusterRole, group, resource, verb string) bool {
	matches := func(values []string, want string) bool {
		return slices.Contains(values, want) || slices.Contains(values, rbacv1.ResourceAll)
	}
	for _, rule := range role.Rules {
		if matches(rule.APIGroups, group) && matches(rule.Resources, resource) && matches(rule.Verbs, verb) {
			return true
		}
	}
	return false
}

// The generated manager ClusterRole is what the operator actually runs with; a missing kind here
// means the GenericReconciler cannot start its informers or cannot write the resources it owns.
var _ = Describe("Manager ClusterRole", func() {
	var role *rbacv1.ClusterRole

	BeforeEach(func() {
		content, err := os.ReadFile(filepath.Join("..", "..", "config", "rbac", "role.yaml"))
		Expect(err).NotTo(HaveOccurred())

		role = &rbacv1.ClusterRole{}
		Expect(yaml.Unmarshal(content, role)).To(Succeed())
		Expect(role.Name).To(Equal("manager-role"))
	})

	It("grants the permissions the reconciled kinds need", func() {
		required := []struct {
			group    string
			resource string
			verbs    []string
		}{
			// The CR itself, its status and its finalizers.
			{"trino.kubedoop.dev", "trinoclusters", []string{"get", "list", "watch", "update", "patch"}},
			{"trino.kubedoop.dev", "trinoclusters/status", []string{"get", "update", "patch"}},
			{"trino.kubedoop.dev", "trinoclusters/finalizers", []string{"update"}},
			// The kinds the GenericReconciler owns and watches.
			{"apps", "statefulsets", []string{"get", "list", "watch", "create", "update", "patch", "delete"}},
			{"", "configmaps", []string{"get", "list", "watch", "create", "update", "patch", "delete"}},
			{"", "services", []string{"get", "list", "watch", "create", "update", "patch", "delete"}},
			{"", "serviceaccounts", []string{"get", "list", "watch", "create", "update", "patch", "delete"}},
			{"policy", "poddisruptionbudgets", []string{"get", "list", "watch", "create", "update", "patch", "delete"}},
			// Orphaned PVCs are cleaned up when a role group shrinks.
			{"", "persistentvolumeclaims", []string{"get", "list", "watch", "delete"}},
			// Health checks list pods; the reconciler records events.
			{"", "pods", []string{"get", "list", "watch"}},
			{"", "events", []string{"create", "patch"}},
		}

		for _, r := range required {
			for _, verb := range r.verbs {
				Expect(allows(role, r.group, r.resource, verb)).To(BeTrue(),
					"manager-role must allow %q on %q (apiGroup %q)", verb, r.resource, r.group)
			}
		}
	})
})
