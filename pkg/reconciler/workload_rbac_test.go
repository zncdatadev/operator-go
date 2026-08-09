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
	"errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/constant"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/testutil"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// Coverage for the workload RBAC layer: the namespaced Role and RoleBinding that give a cluster's
// PODS their Kubernetes API permissions. This is a separate axis from the operator's own
// ClusterRole, and it is deliberately namespaced-only — a namespaced CR cannot controller-own a
// cluster-scoped object, so the framework would have no lifecycle for a ClusterRole.
var _ = Describe("GenericReconciler workload RBAC", func() {
	var (
		mockHandler *testutil.MockRoleGroupHandler
		namespace   string
		testID      string
	)

	leaseRules := []rbacv1.PolicyRule{{
		APIGroups: []string{"coordination.k8s.io"},
		Resources: []string{"leases"},
		Verbs:     []string{"get", "list", "watch", "create", "update"},
	}}

	newReconciler := func(prototype *testutil.MockCluster, rules func(cr *testutil.MockCluster) []rbacv1.PolicyRule) *reconciler.GenericReconciler[*testutil.MockCluster] {
		cfg := &reconciler.GenericReconcilerConfig[*testutil.MockCluster]{
			Client:            k8sClient,
			Scheme:            testScheme,
			Recorder:          recorder,
			RoleGroupHandler:  &handlerAdapter{handler: mockHandler},
			Prototype:         prototype,
			WorkloadRBACRules: rules,
		}
		r, err := reconciler.NewGenericReconciler(cfg)
		Expect(err).NotTo(HaveOccurred())
		return r
	}

	newCR := func(name string) *testutil.MockCluster {
		cr := testutil.NewMockCluster(name, namespace).
			WithRoles(map[string]v1alpha1.RoleSpec{
				"test-role": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{
						"default": {Replicas: ptr.To(int32(1))},
					},
				},
			})
		Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		return cr
	}

	reconcileReq := func(r *reconciler.GenericReconciler[*testutil.MockCluster], crName string) error {
		_, err := r.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Namespace: namespace, Name: crName},
		})
		return err
	}

	// Workload RBAC is named after the workload's ServiceAccount, so a reader who sees the SA on a
	// pod can find its permissions without knowing this API.
	rbacNameFor := func(crName string) string {
		return reconciler.ServiceAccountResourceName("MockCluster", crName)
	}

	getRole := func(name string) (*rbacv1.Role, error) {
		role := &rbacv1.Role{}
		err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, role)
		return role, err
	}

	getBinding := func(name string) (*rbacv1.RoleBinding, error) {
		binding := &rbacv1.RoleBinding{}
		err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, binding)
		return binding, err
	}

	cleanupRBAC := func(name string) {
		_ = k8sClient.Delete(ctx, &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		})
		_ = k8sClient.Delete(ctx, &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		})
	}

	BeforeEach(func() {
		namespace = testNamespace
		mockHandler = testutil.NewMockRoleGroupHandler()
		testID = fmt.Sprintf("%d", time.Now().UnixNano())
	})

	It("creates a Role and a RoleBinding bound to the derived ServiceAccount", func() {
		crName := "wl-rbac-" + testID
		name := rbacNameFor(crName)
		cr := newCR(crName)
		defer func() {
			Expect(k8sClient.Delete(ctx, cr)).To(Succeed())
			cleanupRBAC(name)
		}()

		Expect(reconcileReq(newReconciler(cr, func(*testutil.MockCluster) []rbacv1.PolicyRule {
			return leaseRules
		}), crName)).To(Succeed())

		role, err := getRole(name)
		Expect(err).NotTo(HaveOccurred())
		Expect(role.Rules).To(Equal(leaseRules))
		Expect(role.Labels).To(HaveKeyWithValue(constant.LabelKubernetesInstance, crName))
		Expect(role.Labels).To(HaveKeyWithValue(constant.LabelKubernetesManagedBy, "operator-go"))
		roleOwner := metav1.GetControllerOf(role)
		Expect(roleOwner).NotTo(BeNil())
		Expect(roleOwner.Name).To(Equal(crName))

		binding, err := getBinding(name)
		Expect(err).NotTo(HaveOccurred())
		// The binding points at the Role of the same name and grants to the workload's own
		// ServiceAccount — the identity settled one step earlier in the same reconcile.
		Expect(binding.RoleRef).To(Equal(rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName, Kind: "Role", Name: name,
		}))
		Expect(binding.Subjects).To(Equal([]rbacv1.Subject{{
			Kind:      rbacv1.ServiceAccountKind,
			Name:      name,
			Namespace: namespace,
		}}))
		bindingOwner := metav1.GetControllerOf(binding)
		Expect(bindingOwner).NotTo(BeNil())
		Expect(bindingOwner.Name).To(Equal(crName))
	})

	It("converges a changed rule set", func() {
		crName := "wl-converge-" + testID
		name := rbacNameFor(crName)
		cr := newCR(crName)
		defer func() {
			Expect(k8sClient.Delete(ctx, cr)).To(Succeed())
			cleanupRBAC(name)
		}()

		rules := leaseRules
		r := newReconciler(cr, func(*testutil.MockCluster) []rbacv1.PolicyRule { return rules })
		Expect(reconcileReq(r, crName)).To(Succeed())

		// Narrowing the verbs must reach the live Role, not just the newly created one.
		rules = []rbacv1.PolicyRule{{
			APIGroups: []string{"coordination.k8s.io"},
			Resources: []string{"leases"},
			Verbs:     []string{"get"},
		}}
		Expect(reconcileReq(r, crName)).To(Succeed())

		role, err := getRole(name)
		Expect(err).NotTo(HaveOccurred())
		Expect(role.Rules).To(Equal(rules))
	})

	It("revokes both objects when the rule set becomes empty", func() {
		// An empty rule set is an instruction, not "leave it alone" — the same reading a nil
		// MetricsService gets. Without it, narrowing to zero was the one change that silently did
		// nothing while the pods kept permissions the product had stopped granting.
		crName := "wl-revoke-" + testID
		name := rbacNameFor(crName)
		cr := newCR(crName)
		defer func() {
			Expect(k8sClient.Delete(ctx, cr)).To(Succeed())
			cleanupRBAC(name)
		}()

		rules := leaseRules
		r := newReconciler(cr, func(*testutil.MockCluster) []rbacv1.PolicyRule { return rules })
		Expect(reconcileReq(r, crName)).To(Succeed())
		_, err := getRole(name)
		Expect(err).NotTo(HaveOccurred())

		rules = nil
		Expect(reconcileReq(r, crName)).To(Succeed())

		_, err = getRole(name)
		Expect(k8serrors.IsNotFound(err)).To(BeTrue(), "the Role must be deleted when the rules go empty")
		_, err = getBinding(name)
		Expect(k8serrors.IsNotFound(err)).To(BeTrue(), "the RoleBinding must be deleted when the rules go empty")
	})

	It("creates nothing when the product declares no rules hook", func() {
		// A nil hook is "this product never opted in", which must not run a revoke every pass —
		// that would need RBAC read permission from operators that never asked for the feature.
		crName := "wl-optout-" + testID
		name := rbacNameFor(crName)
		cr := newCR(crName)
		defer func() {
			Expect(k8sClient.Delete(ctx, cr)).To(Succeed())
			cleanupRBAC(name)
		}()

		Expect(reconcileReq(newReconciler(cr, nil), crName)).To(Succeed())

		_, err := getRole(name)
		Expect(k8serrors.IsNotFound(err)).To(BeTrue())
		_, err = getBinding(name)
		Expect(k8serrors.IsNotFound(err)).To(BeTrue())
	})

	It("refuses to rebind a pre-existing RoleBinding pointing somewhere else", func() {
		// roleRef is immutable, so this cannot be converged. Rewriting only the subject would hand
		// this cluster's pods whatever the old ref allows, and return success.
		crName := "wl-squat-" + testID
		name := rbacNameFor(crName)
		cr := newCR(crName)
		defer func() {
			Expect(k8sClient.Delete(ctx, cr)).To(Succeed())
			cleanupRBAC(name)
		}()

		foreign := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			RoleRef: rbacv1.RoleRef{
				APIGroup: rbacv1.GroupName, Kind: "Role", Name: "some-other-role",
			},
			Subjects: []rbacv1.Subject{{
				Kind: rbacv1.ServiceAccountKind, Name: "someone-else", Namespace: namespace,
			}},
		}
		Expect(k8sClient.Create(ctx, foreign)).To(Succeed())

		err := reconcileReq(newReconciler(cr, func(*testutil.MockCluster) []rbacv1.PolicyRule {
			return leaseRules
		}), crName)
		Expect(err).To(HaveOccurred())
		Expect(reconciler.IsValidationError(err)).To(BeTrue(), "a non-convergeable roleRef is a validation failure")
		Expect(err.Error()).To(ContainSubstring("some-other-role"), "the error names the existing ref")
		Expect(err.Error()).To(ContainSubstring("roleRef is immutable"))
		Expect(err.Error()).To(ContainSubstring("delete rolebinding " + name))

		// The foreign binding is left exactly as it was.
		live, getErr := getBinding(name)
		Expect(getErr).NotTo(HaveOccurred())
		Expect(live.RoleRef.Name).To(Equal("some-other-role"))
		Expect(live.Subjects[0].Name).To(Equal("someone-else"))

		// And NOTHING was applied: the roleRef conflict is detected before the Role is written, so a
		// doomed pass leaves no half-converged state behind (the rule validateRoleGroupResources
		// follows for role group slots).
		_, roleErr := getRole(name)
		Expect(k8serrors.IsNotFound(roleErr)).To(BeTrue(),
			"the Role must not be created when the RoleBinding cannot be converged")
	})

	// NOTE: the deep-copy of the caller's rules and the per-object label maps are covered in
	// workload_rbac_internal_test.go, not here. Both are in-process aliasing properties, and every
	// path out of this package deep-copies on serialize, so a spec written at THIS level passes
	// identically with and without the copies. The helpers can be pinned down one level down; the
	// call-site discipline above them — calling workloadRBACLabels once per object rather than
	// hoisting one map — is what no test reaches.

	It("leaves a same-named object it does not control alone on revoke", func() {
		// The reclaim is gated on controller ownership, so a Role another controller maintains is
		// never deleted just because it shares the derived name.
		crName := "wl-notours-" + testID
		otherName := "wl-owner-" + testID
		name := rbacNameFor(crName)
		otherCR := newCR(otherName)
		cr := newCR(crName)
		defer func() {
			Expect(k8sClient.Delete(ctx, cr)).To(Succeed())
			Expect(k8sClient.Delete(ctx, otherCR)).To(Succeed())
			cleanupRBAC(name)
		}()

		foreignRole := &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Rules:      leaseRules,
		}
		Expect(controllerutil.SetControllerReference(otherCR, foreignRole, testScheme)).To(Succeed())
		Expect(k8sClient.Create(ctx, foreignRole)).To(Succeed())

		// This cluster declares no rules, so its reconcile runs the revoke path.
		Expect(reconcileReq(newReconciler(cr, func(*testutil.MockCluster) []rbacv1.PolicyRule {
			return nil
		}), crName)).To(Succeed())

		live, err := getRole(name)
		Expect(err).NotTo(HaveOccurred(), "a Role owned by something else must survive the reclaim")
		Expect(metav1.GetControllerOf(live).Name).To(Equal(otherName))
	})

	It("rejects an empty ServiceAccount name from the exported helper", func() {
		// The helper takes the name as a parameter, so it can be called with one that is not the
		// workload's. An empty one is the case it can actually detect: a Role granting to nothing.
		crName := "wl-noname-" + testID
		cr := newCR(crName)
		defer func() { Expect(k8sClient.Delete(ctx, cr)).To(Succeed()) }()

		err := reconciler.EnsureWorkloadRBAC(ctx, k8sClient, testScheme, cr, "", leaseRules)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("empty string"))
	})

	It("applies the product name label through the helper option", func() {
		crName := "wl-label-" + testID
		name := rbacNameFor(crName)
		cr := newCR(crName)
		defer func() {
			Expect(k8sClient.Delete(ctx, cr)).To(Succeed())
			cleanupRBAC(name)
		}()

		Expect(reconciler.EnsureWorkloadRBAC(ctx, k8sClient, testScheme, cr, name, leaseRules,
			reconciler.WithWorkloadRBACProductName("nifi"),
			reconciler.WithWorkloadRBACExtraLabels(map[string]string{
				"custom": "value",
				// A canonical key must not be overridable by an extra label.
				constant.LabelKubernetesInstance: "forged",
			}),
		)).To(Succeed())

		role, err := getRole(name)
		Expect(err).NotTo(HaveOccurred())
		Expect(role.Labels).To(HaveKeyWithValue(constant.LabelKubernetesName, "nifi"))
		Expect(role.Labels).To(HaveKeyWithValue("custom", "value"))
		Expect(role.Labels).To(HaveKeyWithValue(constant.LabelKubernetesInstance, crName),
			"canonical labels always win over extras")
	})

	It("refuses to adopt a foreign-owned object squatting on the derived name", func() {
		// A derived name cannot collide with another cluster, but something else can still occupy
		// it. Adopting it would give this cluster's pods whatever that object already grants.
		crName := "wl-adopt-" + testID
		otherName := "wl-adopt-owner-" + testID
		name := rbacNameFor(crName)
		otherCR := newCR(otherName)
		cr := newCR(crName)
		defer func() {
			Expect(k8sClient.Delete(ctx, cr)).To(Succeed())
			Expect(k8sClient.Delete(ctx, otherCR)).To(Succeed())
			cleanupRBAC(name)
		}()

		foreignRole := &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Rules:      leaseRules,
		}
		Expect(controllerutil.SetControllerReference(otherCR, foreignRole, testScheme)).To(Succeed())
		Expect(k8sClient.Create(ctx, foreignRole)).To(Succeed())

		err := reconciler.EnsureWorkloadRBAC(ctx, k8sClient, testScheme, cr, name, leaseRules)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("already controlled by"))
		Expect(err.Error()).To(ContainSubstring(otherName), "the error names the current owner")
		Expect(err.Error()).To(ContainSubstring("derived from"),
			"the error should say the name is derived, not blame a naming collision")

		// The squatter is untouched.
		live, getErr := getRole(name)
		Expect(getErr).NotTo(HaveOccurred())
		Expect(metav1.GetControllerOf(live).Name).To(Equal(otherName))
	})

	It("is refused by a REAL API server when the operator does not hold the rules it grants", func() {
		// The escalation footgun, exercised against the actual API server rather than a stub. The
		// suite's own client is cluster-admin, so this mints a restricted user — one that may write
		// RBAC objects but holds none of the permissions being granted — which is exactly the shape
		// of an under-privileged operator.
		//
		// This is the spec that keeps isRBACEscalation honest: it matches on the API server's own
		// wording, so if that wording ever changes, the framework silently starts attributing an
		// escalation refusal to the WRONG cause (a missing RBAC-API marker) and sends the author to
		// audit the wrong thing. Only a real refusal can catch that.
		restricted := restrictedRBACClient()

		crName := "wl-real403-" + testID
		cr := newCR(crName)
		name := rbacNameFor(crName)
		defer func() {
			Expect(k8sClient.Delete(ctx, cr)).To(Succeed())
			cleanupRBAC(name)
		}()

		err := reconciler.EnsureWorkloadRBAC(ctx, restricted, testScheme, cr, name, leaseRules)
		Expect(err).To(HaveOccurred())
		Expect(k8serrors.IsForbidden(err)).To(BeTrue(), "the API server must refuse the escalation")
		Expect(err.Error()).To(ContainSubstring("does not hold them itself"),
			"the framework must recognise the API server's escalation wording; if this fails, "+
				"isRBACEscalation has drifted from what Kubernetes actually says")

		// Nothing was granted.
		_, getErr := getRole(name)
		Expect(k8serrors.IsNotFound(getErr)).To(BeTrue())
	})

	// The 403 branches below use a stub because they pin the exact MESSAGE each cause produces,
	// which is about the framework's attribution rather than the API server's behaviour. The spec
	// above covers the half only a real refusal can.
	It("attributes a plain 403 to missing access to the RBAC API itself", func() {
		crName := "wl-403-plain-" + testID
		cr := newCR(crName)
		defer func() { Expect(k8sClient.Delete(ctx, cr)).To(Succeed()) }()

		c := &forbiddenRBACClient{Client: k8sClient, message: "roles.rbac.authorization.k8s.io is forbidden"}
		err := reconciler.EnsureWorkloadRBAC(ctx, c, testScheme, cr, rbacNameFor(crName), leaseRules)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("rbac.authorization.k8s.io,resources=roles;rolebindings"),
			"the fix for this cause is the RBAC-API marker")
		Expect(err.Error()).NotTo(ContainSubstring("does not hold them itself"),
			"a plain denial must not be blamed on escalation — the two fixes are opposite")
	})

	It("attributes an escalation 403 to the operator not holding the rules itself", func() {
		crName := "wl-403-esc-" + testID
		cr := newCR(crName)
		defer func() { Expect(k8sClient.Delete(ctx, cr)).To(Succeed()) }()

		// The phrasing the API server uses when it refuses to let a subject grant what it lacks.
		c := &forbiddenRBACClient{Client: k8sClient, message: "attempt to grant extra privileges"}
		err := reconciler.EnsureWorkloadRBAC(ctx, c, testScheme, cr, rbacNameFor(crName), leaseRules)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("does not hold them itself"))
		Expect(err.Error()).To(ContainSubstring("+kubebuilder:rbac"))
	})

	It("passes a non-403 write error through untouched", func() {
		crName := "wl-othererr-" + testID
		cr := newCR(crName)
		defer func() { Expect(k8sClient.Delete(ctx, cr)).To(Succeed()) }()

		c := &erroringRBACClient{Client: k8sClient, err: k8serrors.NewServiceUnavailable("etcd is down")}
		err := reconciler.EnsureWorkloadRBAC(ctx, c, testScheme, cr, rbacNameFor(crName), leaseRules)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("etcd is down"))
		Expect(err.Error()).NotTo(ContainSubstring("+kubebuilder:rbac"),
			"only a 403 gets the RBAC explanation; anything else must not be reframed")
	})
})

// restrictedRBACClient builds a client authenticated as a user that may create Roles and
// RoleBindings but holds none of the permissions those Roles grant — an under-privileged operator.
// Writing such a Role makes the API server apply its escalation prevention, which is the one thing
// the suite's cluster-admin client can never trigger.
func restrictedRBACClient() client.Client {
	user, err := testEnv.Env.AddUser(envtest.User{
		Name:   "under-privileged-operator",
		Groups: []string{"kubedoop-test"},
	}, nil)
	Expect(err).NotTo(HaveOccurred())

	// Write access to the RBAC API itself, and nothing else. In particular NOT the
	// coordination.k8s.io/leases rules the specs try to grant.
	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "kubedoop-test-rbac-writer"},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{rbacv1.GroupName},
			Resources: []string{"roles", "rolebindings"},
			Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
		}},
	}
	if err := k8sClient.Create(ctx, role); err != nil && !k8serrors.IsAlreadyExists(err) {
		Expect(err).NotTo(HaveOccurred())
	}
	binding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "kubedoop-test-rbac-writer"},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: role.Name},
		Subjects: []rbacv1.Subject{{
			Kind: rbacv1.GroupKind, APIGroup: rbacv1.GroupName, Name: "kubedoop-test",
		}},
	}
	if err := k8sClient.Create(ctx, binding); err != nil && !k8serrors.IsAlreadyExists(err) {
		Expect(err).NotTo(HaveOccurred())
	}

	restricted, err := client.New(user.Config(), client.Options{Scheme: testScheme})
	Expect(err).NotTo(HaveOccurred())
	return restricted
}

// forbiddenRBACClient answers writes with a 403 carrying a caller-chosen message, so each cause of
// an RBAC refusal maps to a checkable message. The real-refusal half is covered by
// restrictedRBACClient above.
type forbiddenRBACClient struct {
	client.Client
	message string
}

func (c *forbiddenRBACClient) forbidden() error {
	return k8serrors.NewForbidden(
		schema.GroupResource{Group: rbacv1.GroupName, Resource: "roles"}, "", errors.New(c.message))
}

func (c *forbiddenRBACClient) Create(_ context.Context, _ client.Object, _ ...client.CreateOption) error {
	return c.forbidden()
}

func (c *forbiddenRBACClient) Update(_ context.Context, _ client.Object, _ ...client.UpdateOption) error {
	return c.forbidden()
}

// erroringRBACClient answers writes with an arbitrary non-403 error.
type erroringRBACClient struct {
	client.Client
	err error
}

func (c *erroringRBACClient) Create(_ context.Context, _ client.Object, _ ...client.CreateOption) error {
	return c.err
}

func (c *erroringRBACClient) Update(_ context.Context, _ client.Object, _ ...client.UpdateOption) error {
	return c.err
}
