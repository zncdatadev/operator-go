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
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/yaml"
)

// grantSet flattens a ClusterRole into the set of "group/resource:verb" triples it allows.
//
// Expanding rather than matching is what makes the comparison work in BOTH directions. The previous
// shape asked "does the role allow X?" per required triple, which is only half a check: it passes
// for a role that also grants ten things nobody needs, and — because a wildcard matches anything —
// it passes for `apiGroups: ["*"], resources: ["*"], verbs: ["*"]`. A wildcard is therefore recorded
// verbatim here, so it compares unequal to the enumerated set instead of satisfying it.
func grantSet(role *rbacv1.ClusterRole) []string {
	var grants []string
	for _, rule := range role.Rules {
		for _, group := range rule.APIGroups {
			for _, resource := range rule.Resources {
				for _, verb := range rule.Verbs {
					grants = append(grants, fmt.Sprintf("%s/%s:%s", group, resource, verb))
				}
			}
		}
	}
	sort.Strings(grants)
	return slices.Compact(grants)
}

// The generated manager ClusterRole is what the operator actually runs with. It is also the set
// downstream operators copy — docs/security.md §3.3 publishes it as the canonical operator-side
// permission set — so this spec pins it EXACTLY rather than as a lower bound.
//
// Two failure directions, both real:
//   - missing a grant: the GenericReconciler cannot start its informers or cannot write what it
//     owns. Most of those fail loudly at boot; `events` and `pods` do not (§3.3.3), which is
//     precisely why they need a test rather than a first deployment to catch.
//   - granting extra: the published minimum stops being minimal, and every adopter copying it
//     inherits privileges the framework never asked for.
var _ = Describe("Manager ClusterRole", func() {
	var role *rbacv1.ClusterRole

	BeforeEach(func() {
		content, err := os.ReadFile(filepath.Join("..", "..", "config", "rbac", "role.yaml"))
		Expect(err).NotTo(HaveOccurred())

		role = &rbacv1.ClusterRole{}
		Expect(yaml.Unmarshal(content, role)).To(Succeed())
		Expect(role.Name).To(Equal("manager-role"))
	})

	It("grants exactly what the framework consumes, and nothing more", func() {
		// Derived from the framework's call sites, not from the markers this file checks — see
		// docs/security.md §3.3 for the evidence behind each verb. The narrow spots are deliberate:
		//
		//   - no `update`/`patch` on the CR body: the framework Gets the CR and writes only
		//     Status().Update. Nothing in pkg/ writes the body.
		//   - no `get`/`patch` on /status: the 409 refresh re-reads the MAIN resource.
		//   - no `patch` on the owned kinds: the apply path is CreateOrUpdate (Get + Create/Update).
		//   - no `delete` on serviceaccounts: nothing deletes one; owner-reference GC reclaims it.
		//   - no `get` on pods or PVCs: both are only ever Listed.
		expected := []string{
			// The CR, its status, and its finalizers. The last is not about SDK finalizers — there
			// are none — but about SetControllerReference stamping blockOwnerDeletion, which the
			// OwnerReferencesPermissionEnforcement admission plugin gates on the owner's
			// finalizers subresource.
			"trino.kubedoop.dev/trinoclusters:get",
			"trino.kubedoop.dev/trinoclusters:list",
			"trino.kubedoop.dev/trinoclusters:watch",
			"trino.kubedoop.dev/trinoclusters/status:update",
			"trino.kubedoop.dev/trinoclusters/finalizers:update",

			// The workload the framework builds and reclaims.
			"apps/statefulsets:get", "apps/statefulsets:list", "apps/statefulsets:watch",
			"apps/statefulsets:create", "apps/statefulsets:update", "apps/statefulsets:delete",
			"/configmaps:get", "/configmaps:list", "/configmaps:watch",
			"/configmaps:create", "/configmaps:update", "/configmaps:delete",
			"/services:get", "/services:list", "/services:watch",
			"/services:create", "/services:update", "/services:delete",
			"policy/poddisruptionbudgets:get", "policy/poddisruptionbudgets:list",
			"policy/poddisruptionbudgets:watch", "policy/poddisruptionbudgets:create",
			"policy/poddisruptionbudgets:update", "policy/poddisruptionbudgets:delete",

			// The workload identity, which every cluster gets and nothing ever deletes.
			"/serviceaccounts:get", "/serviceaccounts:list", "/serviceaccounts:watch",
			"/serviceaccounts:create", "/serviceaccounts:update",

			// Orphaned PVCs, when the delete-pvcs annotation is set on the CR at runtime.
			"/persistentvolumeclaims:list", "/persistentvolumeclaims:watch",
			"/persistentvolumeclaims:delete",

			// Health evaluation. Without this, Degraded cannot be computed and a failed List is
			// deliberately not reported as the cluster's fault — so it goes quiet, not loud.
			"/pods:list", "/pods:watch",

			// Events. Without this, client-go discards every one with no error and no retry.
			"/events:create", "/events:patch",
		}
		sort.Strings(expected)

		Expect(grantSet(role)).To(Equal(expected),
			"the generated ClusterRole drifted from the set docs/security.md §3.3 publishes; "+
				"regenerate with `make manifests` after editing the markers, and update both if the "+
				"framework's API usage genuinely changed")
	})
})
