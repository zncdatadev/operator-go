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
	"context"
	stderrors "errors"
	"fmt"
	"maps"
	"strings"

	"github.com/zncdatadev/operator-go/pkg/constant"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// workloadRoleKind is the RoleBinding roleRef Kind this helper writes. It is deliberately not
// ClusterRole: a namespaced CR cannot controller-own a cluster-scoped object, so the framework
// would have no lifecycle for one — no garbage collection with the cluster, and no ownership gate
// on the reclaim.
const workloadRoleKind = "Role"

// WorkloadRBACOption configures EnsureWorkloadRBAC.
type WorkloadRBACOption func(*workloadRBACOptions)

type workloadRBACOptions struct {
	productName string
	labels      map[string]string
}

// WithWorkloadRBACProductName sets app.kubernetes.io/name on the Role and RoleBinding.
func WithWorkloadRBACProductName(name string) WorkloadRBACOption {
	return func(o *workloadRBACOptions) { o.productName = name }
}

// WithWorkloadRBACExtraLabels merges extra labels onto the Role and RoleBinding. The canonical
// labels always win, the same rule EnsureDiscoveryConfigMap follows.
func WithWorkloadRBACExtraLabels(labels map[string]string) WorkloadRBACOption {
	return func(o *workloadRBACOptions) { o.labels = labels }
}

// EnsureWorkloadRBAC maintains the namespaced Role and RoleBinding that give a cluster's WORKLOAD
// pods the Kubernetes API permissions they need — not the operator's own permissions, which come
// from the operator's ClusterRole and are a separate axis.
//
// Most products should NOT call this directly: set GenericReconcilerConfig.WorkloadRBACRules and
// the framework calls it at cluster level, with the ServiceAccount name it DERIVED, right after
// creating that ServiceAccount. That is what keeps the workload's identity and its permissions from
// drifting apart — the failure this signature cannot prevent is a caller passing a name that is not
// the workload's, leaving a Role that grants to a ServiceAccount no pod uses: both objects exist,
// the pods start, and the first API call 403s.
//
// It stays exported for a product driving the reconciler's pieces itself rather than through
// GenericReconciler. Call it from a common.ClusterExtension PreReconcile hook, where a ctx and
// client exist and the workload has not been built yet.
//
//	err := reconciler.EnsureWorkloadRBAC(ctx, c, scheme, cr, saName, []rbacv1.PolicyRule{{
//	    APIGroups: []string{"coordination.k8s.io"},
//	    Resources: []string{"leases"},
//	    Verbs:     []string{"get", "list", "watch", "create", "update"},
//	}}, reconciler.WithWorkloadRBACProductName("nifi"))
//
// Either way the framework owns the ensure semantics — naming, canonical labels, the controller
// owner reference, idempotent create-or-update, revocation — and the product owns the rules,
// because only the product knows what its pods call.
//
// Both objects are named after the ServiceAccount they serve and live in the CR's namespace, so a
// reader who sees the SA on a pod can find its permissions without knowing this API. Both carry a
// CONTROLLER owner reference to the CR, which is what puts them inside the framework's lifecycle:
// they are garbage-collected with the cluster, and — unlike a plain owner reference — a competing
// controller cannot claim them.
//
// # Revocation
//
// An empty rule set DELETES the Role and RoleBinding, provided this CR controller-owns them. That
// is the same reading the framework gives every other optional slot — a nil MetricsService reclaims
// the Service — and it is what makes "the rules converge" true in both directions: narrowing to
// zero is the largest narrowing there is.
//
// # A pre-existing RoleBinding is never adopted
//
// roleRef is immutable, so a RoleBinding already sitting at this name pointing somewhere else
// cannot be converged. This fails with a *ValidationError naming both refs and the one command
// that fixes it, rather than rebinding the existing object to the workload's ServiceAccount —
// which would hand those pods whatever the old ref allows.
//
// # The escalation footgun
//
// Kubernetes refuses to let a subject grant permissions it does not itself hold, so the OPERATOR's
// own ClusterRole must be a superset of every rule passed here. The failure is a 403 at Role
// create/update time, it is invisible at compile time, and it is invisible in any test that runs as
// cluster-admin (which envtest does). This re-explains that error rather than pre-checking it: the
// API server's own message already names the exact rule that is missing, and a pre-check would have
// to reimplement RBAC rule covering — wildcards, resourceNames, aggregated ClusterRoles,
// non-resource URLs — and would then be wrong in both directions.
//
// The operator needs TWO kinds of marker, and only one of them is about the rules:
//
//	// the rules being granted — Kubernetes forbids granting what the granter lacks
//	// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update
//	// write access to the RBAC API itself, without which nothing here can be created at all
//	// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings,verbs=get;list;watch;create;update;patch;delete
func EnsureWorkloadRBAC(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	owner client.Object,
	serviceAccountName string,
	rules []rbacv1.PolicyRule,
	opts ...WorkloadRBACOption,
) error {
	if serviceAccountName == "" {
		return fmt.Errorf("workload RBAC needs the name of the ServiceAccount it grants to: got an empty string")
	}
	options := &workloadRBACOptions{}
	for _, opt := range opts {
		opt(options)
	}

	// An empty rule set is an instruction to REVOKE, not "leave it alone". Returning early here
	// made it the one case that silently did nothing while the pods kept permissions the product
	// had stopped granting.
	if len(rules) == 0 {
		return reclaimWorkloadRBAC(ctx, c, owner, serviceAccountName)
	}

	key := types.NamespacedName{Namespace: owner.GetNamespace(), Name: serviceAccountName}
	want := rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: workloadRoleKind, Name: serviceAccountName}

	// Check the one thing that can never be converged BEFORE writing anything. The same check runs
	// again inside the mutate func below, and both are needed for different reasons: this one keeps
	// a doomed pass from leaving a Role behind ("a slot that half-converged and then failed is worse
	// than one that did not start" — the rule validateRoleGroupResources follows), while the one
	// below closes the window between this read and the write.
	if err := checkWorkloadRoleRef(ctx, c, key, want); err != nil {
		return err
	}

	// Build the desired objects ONCE, as pure data, before any I/O. Keeping construction separate
	// from the write is what makes the framework's own invariants checkable: buildWorkloadRBAC is a
	// pure function, so a test can hold both objects at once and prove they share no map and alias
	// none of the caller's data — which is impossible to observe once they have crossed the API
	// boundary, since every client deep-copies on serialize. Same split pkg/builder exists for.
	desiredRole, desiredBinding := buildWorkloadRBAC(owner, key, want, rules, options)

	role := &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace}}
	if _, err := controllerutil.CreateOrUpdate(ctx, c, role, func() error {
		if err := controllerutil.SetControllerReference(owner, role, scheme); err != nil {
			return err
		}
		role.Labels = desiredRole.Labels
		role.Rules = desiredRole.Rules
		return nil
	}); err != nil {
		return explainWorkloadRBACError(ctx, err, rules)
	}

	binding := &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace}}
	if _, err := controllerutil.CreateOrUpdate(ctx, c, binding, func() error {
		if err := controllerutil.SetControllerReference(owner, binding, scheme); err != nil {
			return err
		}
		binding.Labels = desiredBinding.Labels
		binding.Subjects = desiredBinding.Subjects

		// RoleRef is immutable, so it can only be SET on create — but it must still be CHECKED on
		// update, and checking it is the whole point.
		//
		// controllerutil.CreateOrUpdate Gets the live object into `binding` before running this
		// func, so on the update path RoleRef is never the zero value. Writing it only when empty
		// would therefore silently ADOPT whatever a pre-existing object points at: a RoleBinding
		// left at this name by an older operator, a Helm chart, or the ExtraResources route this
		// replaces would keep its roleRef while its subject was rewritten to the workload's
		// ServiceAccount — handing those pods permissions nobody granted them, and returning nil.
		// Migration is exactly the situation this helper exists for, so that is the main path.
		if binding.RoleRef == (rbacv1.RoleRef{}) {
			binding.RoleRef = desiredBinding.RoleRef
			return nil
		}
		if binding.RoleRef != want {
			return workloadRoleRefConflict(key, binding.RoleRef, want)
		}
		return nil
	}); err != nil {
		return explainWorkloadRBACError(ctx, err, rules)
	}

	return nil
}

// buildWorkloadRBAC renders the desired Role and RoleBinding as pure data — no client, no I/O, no
// live state — so that everything about WHAT the framework wants to write is decided in one place
// and is checkable without a cluster.
//
// The two objects deliberately share nothing: each gets its own label map, and the Role's rules are
// a deep copy of the caller's. Neither property is observable downstream of this function (an API
// round-trip re-materialises both objects independently), which is exactly why the construction is
// separated out rather than inlined into the two mutate funcs — that inlining is what let one
// hoisted `labels := …` put a single map on both objects.
func buildWorkloadRBAC(
	owner client.Object,
	key types.NamespacedName,
	roleRef rbacv1.RoleRef,
	rules []rbacv1.PolicyRule,
	options *workloadRBACOptions,
) (*rbacv1.Role, *rbacv1.RoleBinding) {
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      key.Name,
			Namespace: key.Namespace,
			Labels:    workloadRBACLabels(owner, options),
		},
		// Copied, not aliased: a product's rules are typically a package-level literal, and the
		// framework must not store its caller's backing array in an object it hands to a client.
		// Same rule pkg/builder's Build() follows for every map and slice it returns.
		Rules: cloneWorkloadRules(rules),
	}

	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      key.Name,
			Namespace: key.Namespace,
			// A second call, not the Role's map: two API objects must never share one Go map, or a
			// future edit to either silently rewrites the other.
			Labels: workloadRBACLabels(owner, options),
		},
		RoleRef: roleRef,
		Subjects: []rbacv1.Subject{{
			Kind:      rbacv1.ServiceAccountKind,
			Name:      key.Name,
			Namespace: key.Namespace,
		}},
	}

	return role, binding
}

// checkWorkloadRoleRef fails when a RoleBinding already sits at this name pointing somewhere the
// framework cannot converge to. A NotFound (nothing there yet) and any other read error are both
// "carry on": a read failure will surface again from the write path, with the API server's own
// message, rather than being reported here as a validation problem it is not.
func checkWorkloadRoleRef(ctx context.Context, c client.Client, key types.NamespacedName, want rbacv1.RoleRef) error {
	live := &rbacv1.RoleBinding{}
	if err := c.Get(ctx, key, live); err != nil {
		return nil //nolint:nilerr // deliberate: the write path reports read failures with better context
	}
	if live.RoleRef == want {
		return nil
	}
	return workloadRoleRefConflict(key, live.RoleRef, want)
}

// workloadRoleRefConflict builds the error both roleRef checks return, so the two cannot drift.
func workloadRoleRefConflict(key types.NamespacedName, have, want rbacv1.RoleRef) error {
	return NewValidationError("WorkloadRBAC", "", "", fmt.Errorf(
		"the RoleBinding %s/%s already exists and points at %s/%s, but this cluster's workload RBAC "+
			"requires %s/%s. roleRef is immutable, so the framework cannot converge it and will not "+
			"rebind the existing one — that would grant this cluster's pods whatever the old ref "+
			"allows. Delete it and let the framework recreate it: kubectl -n %s delete rolebinding %s",
		key.Namespace, key.Name, have.Kind, have.Name,
		want.Kind, want.Name, key.Namespace, key.Name))
}

// cloneWorkloadRules deep-copies the product's rules, including the string slices inside each rule,
// so nothing the framework hands to the API client aliases the caller's data.
func cloneWorkloadRules(rules []rbacv1.PolicyRule) []rbacv1.PolicyRule {
	cloned := make([]rbacv1.PolicyRule, len(rules))
	for i, rule := range rules {
		cloned[i] = *rule.DeepCopy()
	}
	return cloned
}

// reclaimWorkloadRBAC deletes the Role and RoleBinding this cluster's workload RBAC consists of,
// but only when they are controller-owned by this CR — the same ownership gate every other reclaim
// in this package applies, so a same-named object belonging to something else is never touched.
//
// Order mirrors the grant: the binding first, so the permission stops applying before the Role that
// spells it out disappears. A NotFound at either step is success.
func reclaimWorkloadRBAC(ctx context.Context, c client.Client, owner client.Object, name string) error {
	key := types.NamespacedName{Namespace: owner.GetNamespace(), Name: name}

	if err := deleteWorkloadRBACIfOwned(ctx, c, key, &rbacv1.RoleBinding{}, owner); err != nil {
		return err
	}
	return deleteWorkloadRBACIfOwned(ctx, c, key, &rbacv1.Role{}, owner)
}

// deleteWorkloadRBACIfOwned deletes obj at key when it exists and this CR is its controller owner.
func deleteWorkloadRBACIfOwned(ctx context.Context, c client.Client, key types.NamespacedName, obj client.Object, owner client.Object) error {
	// An owner with no UID cannot prove it owns anything, so it deletes nothing — the same rule the
	// cleaner's live orphan discovery and role-PDB reclaim follow. Without it, an owner built in
	// memory (which the exported helper permits) would match any object whose controller ref also
	// carries an empty UID.
	if owner.GetUID() == "" {
		return nil
	}
	if err := c.Get(ctx, key, obj); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	ref := metav1.GetControllerOf(obj)
	if ref == nil || ref.UID != owner.GetUID() {
		// Not ours. Leaving it is the safe answer: the alternative is deleting an object another
		// controller maintains because it happens to share a name.
		return nil
	}
	if err := c.Delete(ctx, obj); err != nil && !errors.IsNotFound(err) {
		return err
	}
	log.FromContext(ctx).Info("Revoked workload RBAC: the product supplied no rules",
		"name", key.Name, "namespace", key.Namespace)
	return nil
}

// workloadRBACLabels builds the canonical label set. Extra labels are merged underneath, so the
// canonical ones always win — the same rule EnsureDiscoveryConfigMap follows.
func workloadRBACLabels(owner client.Object, options *workloadRBACOptions) map[string]string {
	labels := maps.Clone(options.labels)
	if labels == nil {
		labels = map[string]string{}
	}
	labels[constant.LabelKubernetesInstance] = owner.GetName()
	labels[constant.LabelKubernetesManagedBy] = managedByValue
	if options.productName != "" {
		labels[constant.LabelKubernetesName] = options.productName
	}
	return labels
}

// explainWorkloadRBACError re-explains the failures a product author cannot diagnose from the raw
// message, and passes everything else through untouched.
//
// A 403 here has TWO possible causes needing opposite fixes, which is why the attribution has to be
// narrow rather than blanket:
//
//   - The operator lacks write access to roles/rolebindings themselves. Its own ClusterRole needs
//     `+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings,...`.
//   - The operator holds that, but not the permissions it is trying to grant. Kubernetes refuses to
//     let a subject grant what it does not itself hold (absent the `escalate` verb), so the
//     operator's ClusterRole must cover every rule passed here.
//
// Blanket-attributing every 403 to the second sends an author auditing rules they just added while
// the real gap is the first, so the escalation wording is used only when the API server's own
// message says so. That message has already done the exact rule-covering computation, against the
// operator's real effective permissions including aggregated ClusterRoles, and names the missing
// rule — which is why this re-explains rather than pre-checks.
func explainWorkloadRBACError(ctx context.Context, err error, rules []rbacv1.PolicyRule) error {
	var alreadyOwned *controllerutil.AlreadyOwnedError
	if stderrors.As(err, &alreadyOwned) {
		// The ServiceAccount name is derived from the CR, so two clusters can no longer be
		// configured onto one Role. What remains is an unrelated object squatting on the derived
		// name — which the framework must not adopt, since rebinding it would grant this cluster's
		// pods whatever it already allows.
		return fmt.Errorf(
			"the workload RBAC object %q is already controlled by %s %q, so this cluster cannot own it. "+
				"Workload RBAC is named after this cluster's ServiceAccount, whose name is derived from "+
				"the CR, so something else is occupying that name: delete it, or rename the cluster: %w",
			alreadyOwned.Object.GetName(), alreadyOwned.Owner.Kind, alreadyOwned.Owner.Name, err)
	}

	if !errors.IsForbidden(err) {
		return err
	}
	if !isRBACEscalation(err) {
		return fmt.Errorf(
			"the API server refused to write this cluster's workload RBAC. If the operator lacks access to "+
				"the RBAC API itself, add "+
				"`+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings,"+
				"verbs=get;list;watch;create;update;patch;delete` and regenerate its ClusterRole: %w", err)
	}
	log.FromContext(ctx).Error(err, "Workload RBAC was refused: the operator cannot grant permissions it does not hold itself",
		"rules", rules)
	return fmt.Errorf(
		"the API server refused these workload RBAC rules because this operator does not hold them itself — "+
			"Kubernetes forbids granting permissions the granter lacks. Add the matching "+
			"+kubebuilder:rbac markers to the operator and regenerate its ClusterRole: %w", err)
}

// isRBACEscalation reports whether a 403 is the RBAC escalation refusal rather than a plain
// missing-verb denial. The API server phrases the former distinctly; matching on it keeps the two
// causes apart without the framework guessing.
//
// The first phrase is what a current API server actually emits, verified against a real refusal
// rather than assumed — see the "REAL API server" spec, which mints an under-privileged user and
// asserts this function recognises the result. That spec exists because the obvious guesses (the
// two fallbacks below, taken from the escalation check's own error text and from the `escalate`
// verb) do NOT appear in the message the server returns, so an implementation written from memory
// silently attributes every escalation refusal to the wrong cause.
//
// The fallbacks are kept for older servers and for the paths that phrase it differently; the first
// is the one under test.
func isRBACEscalation(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "attempting to grant RBAC permissions not currently held") ||
		strings.Contains(msg, "attempt to grant extra privileges") ||
		strings.Contains(msg, "escalate")
}
