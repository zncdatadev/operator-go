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

// roleKind is the RoleBinding roleRef Kind for the namespaced Role this helper creates. It is
// deliberately not ClusterRole: a namespaced CR cannot controller-own a cluster-scoped object, so
// the framework has no lifecycle for one.
const roleKind = "Role"

// PodRBACOption configures EnsurePodRBAC.
type PodRBACOption func(*podRBACOptions)

type podRBACOptions struct {
	productName string
	labels      map[string]string
	annotations map[string]string
}

// WithPodRBACProductName sets app.kubernetes.io/name on the Role and RoleBinding.
func WithPodRBACProductName(name string) PodRBACOption {
	return func(o *podRBACOptions) { o.productName = name }
}

// WithPodRBACExtraLabels merges extra labels onto the Role and RoleBinding. The canonical labels
// always win.
func WithPodRBACExtraLabels(labels map[string]string) PodRBACOption {
	return func(o *podRBACOptions) { o.labels = labels }
}

// WithPodRBACExtraAnnotations merges extra annotations onto the Role and RoleBinding.
func WithPodRBACExtraAnnotations(annotations map[string]string) PodRBACOption {
	return func(o *podRBACOptions) { o.annotations = annotations }
}

// EnsurePodRBAC maintains the namespaced Role and RoleBinding that give a cluster's WORKLOAD pods
// the API permissions they need — not the operator's own permissions, which come from the
// operator's ClusterRole and are a separate axis entirely.
//
// Most products should NOT call this directly: set GenericReconcilerConfig.PodRBACRules instead and
// the framework calls it at cluster level, with the ServiceAccount name it resolved itself, right
// after ensuring the ServiceAccount. That is what keeps the workload's identity and its permissions
// from drifting apart — the failure this signature cannot prevent is a product that changes
// ServiceAccountNameFunc and forgets its extension, leaving a Role that grants to a ServiceAccount
// no pod uses: both objects exist, the pods start, and the first API call 403s.
//
// It stays exported for the one case the config field structurally cannot serve: a product that
// manages its OWN ServiceAccount — platform-provisioned, or externally managed for IRSA / Workload
// Identity — so the framework has no name to resolve. Call it from a common.ClusterExtension
// PreReconcile hook, where a ctx and client exist and the workload has not been built yet.
//
// Either way the framework owns the ensure semantics — naming, canonical labels, the controller
// owner reference, idempotent create-or-update, revocation — and the product owns the rules,
// because only the product knows what its pods call.
//
//	err := reconciler.EnsurePodRBAC(ctx, c, scheme, cr, saName, []rbacv1.PolicyRule{{
//	    APIGroups: []string{"coordination.k8s.io"},
//	    Resources: []string{"leases"},
//	    Verbs:     []string{"get", "list", "watch", "create", "update"},
//	}}, reconciler.WithPodRBACProductName("nifi"))
//
// Both objects are named after the ServiceAccount they serve and live in the CR's namespace, so a
// reader who sees the SA on a pod can find its permissions without knowing this API. Both carry a
// CONTROLLER owner reference to the CR, which is what puts them inside the framework's lifecycle:
// they are garbage-collected with the cluster, and — unlike a plain owner reference — a competing
// controller cannot claim them.
//
// # Revocation
//
// An empty rule set deletes the Role and RoleBinding, provided this CR controller-owns them. That
// is the same reading the framework gives every other optional slot — a nil MetricsService reclaims
// the Service — and it is what makes "the rules converge" true in both directions: narrowing to
// zero is the largest narrowing there is.
//
// # A pre-existing RoleBinding is never adopted
//
// roleRef is immutable, so a RoleBinding already sitting at this name pointing somewhere else
// cannot be converged. The helper fails with a *ValidationError naming both refs and the one
// command that fixes it, rather than rebinding the existing object to the workload's
// ServiceAccount — which would hand those pods whatever the old ref allows.
//
// # The escalation footgun
//
// Kubernetes refuses to let a subject grant permissions it does not itself hold, so the OPERATOR's
// own ClusterRole must be a superset of every rule passed here. The failure is a 403 at Role
// create/update time, it is invisible at compile time, and it is invisible in any test that runs as
// cluster-admin (which envtest does). EnsurePodRBAC re-explains that error rather than pre-checking
// it: the API server's own message already names the exact rule that is missing, and a pre-check
// would have to reimplement RBAC rule covering — wildcards, resourceNames, aggregated ClusterRoles,
// non-resource URLs — and would then be wrong in both directions.
//
// The operator needs TWO kinds of marker, and only one of them is about the rules:
//
//	// the rules being granted — Kubernetes forbids granting what the granter lacks
//	// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update
//	// write access to the RBAC API itself, without which nothing here can be created at all
//	// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings,verbs=get;list;watch;create;update;patch;delete
func EnsurePodRBAC(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	owner client.Object,
	serviceAccountName string,
	rules []rbacv1.PolicyRule,
	opts ...PodRBACOption,
) error {
	if serviceAccountName == "" {
		return fmt.Errorf("pod RBAC needs the name of the ServiceAccount it grants to: got an empty string")
	}
	options := &podRBACOptions{}
	for _, opt := range opts {
		opt(options)
	}

	// An empty rule set is an instruction to REVOKE, not "leave it alone". That is the framework's
	// own rule for every optional slot — a nil MetricsService reclaims the Service, a nil
	// PodDisruptionBudget reclaims the PDB — and it is the only reading that makes "the rules
	// converge" true in both directions: narrowing to zero is the largest narrowing there is, and
	// returning early here made it the one case that silently did nothing while the pods kept the
	// permissions the product had stopped granting.
	if len(rules) == 0 {
		return reclaimPodRBAC(ctx, c, owner, serviceAccountName)
	}

	meta := metav1.ObjectMeta{
		Name:        serviceAccountName,
		Namespace:   owner.GetNamespace(),
		Labels:      podRBACLabels(owner, options),
		Annotations: maps.Clone(options.annotations),
	}

	role := &rbacv1.Role{ObjectMeta: meta}
	if _, err := controllerutil.CreateOrUpdate(ctx, c, role, func() error {
		if err := controllerutil.SetControllerReference(owner, role, scheme); err != nil {
			return err
		}
		role.Labels = podRBACLabels(owner, options)
		role.Annotations = mergeAnnotations(role.Annotations, options.annotations)
		role.Rules = rules
		return nil
	}); err != nil {
		return explainPodRBACError(ctx, err, rules)
	}

	binding := &rbacv1.RoleBinding{ObjectMeta: meta}
	if _, err := controllerutil.CreateOrUpdate(ctx, c, binding, func() error {
		if err := controllerutil.SetControllerReference(owner, binding, scheme); err != nil {
			return err
		}
		binding.Labels = podRBACLabels(owner, options)
		binding.Annotations = mergeAnnotations(binding.Annotations, options.annotations)
		binding.Subjects = []rbacv1.Subject{{
			Kind:      rbacv1.ServiceAccountKind,
			Name:      serviceAccountName,
			Namespace: owner.GetNamespace(),
		}}

		// RoleRef is immutable, so it can only be SET on create — but it must still be CHECKED on
		// update, and checking it is the whole point.
		//
		// controllerutil.CreateOrUpdate Gets the live object into `binding` before running this
		// func, so on the update path RoleRef is never the zero value. Writing it only when empty
		// therefore silently ADOPTS whatever a pre-existing object points at: a RoleBinding left at
		// this name by an older operator, a Helm chart, or the ExtraResources route this helper
		// replaces would keep its roleRef while its subject was rewritten to the workload's
		// ServiceAccount — handing those pods permissions nobody granted them, and returning nil.
		// Migration is exactly the situation this helper exists for, so that is the main path.
		want := rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: roleKind, Name: serviceAccountName}
		if binding.RoleRef == (rbacv1.RoleRef{}) {
			binding.RoleRef = want
			return nil
		}
		if binding.RoleRef != want {
			return NewValidationError("PodRBAC", "", "", fmt.Errorf(
				"the RoleBinding %s/%s already exists and points at %s/%s, but this cluster's workload RBAC "+
					"requires %s/%s. roleRef is immutable, so the framework cannot converge it and will not "+
					"rebind the existing one — that would grant this cluster's pods whatever the old ref "+
					"allows. Delete it and let the framework recreate it: kubectl -n %s delete rolebinding %s",
				binding.Namespace, binding.Name, binding.RoleRef.Kind, binding.RoleRef.Name,
				want.Kind, want.Name, binding.Namespace, binding.Name))
		}
		return nil
	}); err != nil {
		return explainPodRBACError(ctx, err, rules)
	}

	return nil
}

// reclaimPodRBAC deletes the Role and RoleBinding this cluster's workload RBAC consists of, but
// only when they are controller-owned by this CR — the same ownership gate every other reclaim in
// this package applies, so a same-named object belonging to something else is never touched.
//
// Order mirrors the grant: the binding first, so the permission stops applying before the Role that
// spells it out disappears. A NotFound at either step is success.
func reclaimPodRBAC(ctx context.Context, c client.Client, owner client.Object, name string) error {
	key := types.NamespacedName{Namespace: owner.GetNamespace(), Name: name}

	binding := &rbacv1.RoleBinding{}
	if err := deleteIfOwned(ctx, c, key, binding, owner); err != nil {
		return err
	}
	role := &rbacv1.Role{}
	return deleteIfOwned(ctx, c, key, role, owner)
}

// deleteIfOwned deletes obj at key when it exists and this CR is its controller owner.
func deleteIfOwned(ctx context.Context, c client.Client, key types.NamespacedName, obj client.Object, owner client.Object) error {
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

// podRBACLabels builds the canonical label set. Extra labels are merged underneath, so the
// canonical ones always win — the same rule EnsureDiscoveryConfigMap follows.
func podRBACLabels(owner client.Object, options *podRBACOptions) map[string]string {
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

// explainPodRBACError re-explains the two failures a product author cannot diagnose from the raw
// message, and passes everything else through untouched.
//
// A 403 here has TWO possible causes and they need opposite fixes, which is why the attribution has
// to be narrow rather than blanket:
//
//   - The operator lacks write access to roles/rolebindings themselves. Its own ClusterRole needs
//     `+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings,...`.
//   - The operator holds that, but not the permissions it is trying to grant. Kubernetes refuses to
//     let a subject grant what it does not itself hold (unless it has the `escalate` verb), so the
//     operator's ClusterRole must cover every rule passed here.
//
// Blanket-attributing every 403 to the second sends an author auditing rules they just added while
// the real gap is the first, so the escalation wording is only used when the API server's own
// message says so. That message has already done the exact rule-covering computation, against the
// operator's real effective permissions including aggregated ClusterRoles, and names the missing
// rule — which is why this re-explains rather than pre-checks. A framework pre-check would have to
// reimplement wildcards, resourceNames and non-resource URLs, and be wrong in both directions.
func explainPodRBACError(ctx context.Context, err error, rules []rbacv1.PolicyRule) error {
	var alreadyOwned *controllerutil.AlreadyOwnedError
	if stderrors.As(err, &alreadyOwned) {
		// Same root cause the ServiceAccount path already explains: a static ServiceAccountName
		// shared by two CRs in one namespace makes both want the same Role.
		return fmt.Errorf(
			"the workload RBAC object %q is already controlled by %s %q, so this cluster cannot own it. "+
				"Workload RBAC is named after the ServiceAccount, so two clusters sharing a static "+
				"ServiceAccountName collide here. Give each CR its own name with "+
				"GenericReconcilerConfig.ServiceAccountNameFunc (e.g. \"<product>-<cluster>\"): %w",
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
func isRBACEscalation(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "attempt to grant extra privileges") ||
		strings.Contains(msg, "escalate")
}
