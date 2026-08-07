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
	"fmt"
	"maps"

	"github.com/zncdatadev/operator-go/pkg/constant"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
// It is the third member of the Ensure* family (EnsureDiscoveryConfigMap, EnsureGeneratedSecret):
// the framework owns the ensure semantics — naming, canonical labels, the controller owner
// reference, idempotent create-or-update — and the product owns the rules, because only the product
// knows what its pods call. Call it from a common.ClusterExtension PreReconcile hook, where a ctx
// and client exist and the workload has not been built yet.
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
// # Why this is a helper and not a GenericReconcilerConfig field
//
// A config field would make every operator pay for a permanent reconcile-flow step, two more
// informers, a slot label and two reclaim paths, to serve the products that actually need it. A
// helper costs nothing to the operators that never call it, and it composes: a product needing a
// ClusterRole (which a namespaced CR cannot controller-own at all) writes that itself instead of
// discovering halfway through that the framework's shape does not fit.
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
// Add the matching kubebuilder marker to the operator, e.g.:
//
//	// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update
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
	if len(rules) == 0 {
		// Not an error: a product may compute an empty rule set for a cluster that turned the
		// feature off. Creating an empty Role would be a grant of nothing that nothing reclaims
		// until the CR is deleted, so do nothing instead.
		return nil
	}

	options := &podRBACOptions{}
	for _, opt := range opts {
		opt(options)
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
		return explainRBACEscalation(ctx, err, rules)
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
		// RoleRef is immutable, so it is set only on create. CreateOrUpdate runs this func against
		// the not-yet-created object on the create path, where the zero value is what we see.
		if binding.RoleRef.Name == "" {
			binding.RoleRef = rbacv1.RoleRef{
				APIGroup: rbacv1.GroupName,
				Kind:     roleKind,
				Name:     serviceAccountName,
			}
		}
		return nil
	}); err != nil {
		return explainRBACEscalation(ctx, err, rules)
	}

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

// explainRBACEscalation turns the API server's escalation refusal into a message that names the
// cause, because the raw one does not mention the operator's own ClusterRole and this is the single
// most common way workload RBAC fails.
//
// It re-explains rather than pre-checks deliberately: the API server has already done the exact
// rule-covering computation, against the operator's real effective permissions including every
// aggregated ClusterRole, and its message names the specific missing rule. Any pre-check the
// framework wrote would duplicate that logic — wildcards, resourceNames, non-resource URLs — and
// be wrong in both directions, while costing a request per reconcile.
func explainRBACEscalation(ctx context.Context, err error, rules []rbacv1.PolicyRule) error {
	if !errors.IsForbidden(err) {
		return err
	}
	log.FromContext(ctx).Error(err, "Pod RBAC was refused; the operator cannot grant permissions it does not hold itself",
		"rules", rules)
	return fmt.Errorf(
		"the API server refused these pod RBAC rules because this operator does not hold them itself — "+
			"Kubernetes forbids granting permissions the granter lacks. Add the matching "+
			"+kubebuilder:rbac markers to the operator and regenerate its ClusterRole: %w", err)
}
