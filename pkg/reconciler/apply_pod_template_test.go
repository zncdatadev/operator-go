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
	"testing"

	"github.com/zncdatadev/operator-go/pkg/constant"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func stsWithTemplateAnnotations(annotations map[string]string) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-role-default", Namespace: "ns"},
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Annotations: annotations},
			},
		},
	}
}

// The restarter's annotation is the framework's ONLY documented mechanism for delivering a
// configOverrides change to running pods: a ConfigMap rewrite leaves the pod template
// byte-identical, so commons-operator stamps "configmap.restarter.kubedoop.dev/<name>" into the pod
// template and the StatefulSet controller rolls. The handler never builds that key, so a wholesale
// spec assignment removes it — and the resulting Update wakes the restarter, which re-stamps, which
// wakes this reconciler through its own Owns(&appsv1.StatefulSet{}) watch. Neither side errors, so
// the workqueue Forgets every pass and nothing backs off.
func TestCopyStatefulSetState_KeepsRestarterPodTemplateAnnotation(t *testing.T) {
	restarterKey := constant.AnnotationConfigMapRestarterPrefix + "cluster-role-default"
	live := stsWithTemplateAnnotations(map[string]string{restarterKey: "uid-1/12345"})
	desired := stsWithTemplateAnnotations(nil)

	ignored, err := copyDesiredState(desired, live)
	if err != nil {
		t.Fatalf("copyDesiredState() error = %v", err)
	}
	// Touching the pod template is not an immutable-field change, so nothing may be reported to
	// the user as dropped.
	if len(ignored) != 0 {
		t.Errorf("ignored = %v, want none", ignored)
	}

	got, ok := live.Spec.Template.Annotations[restarterKey]
	if !ok {
		t.Fatalf("the restarter's pod-template annotation was removed; the pods roll on every reconcile")
	}
	if got != "uid-1/12345" {
		t.Errorf("annotation value = %q, want it untouched", got)
	}
}

func TestCopyStatefulSetState_DesiredPodTemplateAnnotationWins(t *testing.T) {
	// Same precedence the object-level merge uses: the handler's value beats the live one, so a
	// product can still change an annotation it owns.
	live := stsWithTemplateAnnotations(map[string]string{"product/a": "old", "foreign/b": "keep"})
	desired := stsWithTemplateAnnotations(map[string]string{"product/a": "new"})

	ignored, err := copyDesiredState(desired, live)
	if err != nil {
		t.Fatalf("copyDesiredState() error = %v", err)
	}
	// Touching the pod template is not an immutable-field change, so nothing may be reported to
	// the user as dropped.
	if len(ignored) != 0 {
		t.Errorf("ignored = %v, want none", ignored)
	}

	if got := live.Spec.Template.Annotations["product/a"]; got != "new" {
		t.Errorf("desired must win: got %q, want %q", got, "new")
	}
	if got := live.Spec.Template.Annotations["foreign/b"]; got != "keep" {
		t.Errorf("a foreign annotation must survive: got %q", got)
	}
}

func TestCopyStatefulSetState_NoAnnotationsStaysNil(t *testing.T) {
	// An object that never had pod-template annotations must not gain an empty map: that is a spec
	// difference, so it would bump the resourceVersion and roll the pods on the first reconcile
	// after this change ships.
	live := stsWithTemplateAnnotations(nil)
	desired := stsWithTemplateAnnotations(nil)

	ignored, err := copyDesiredState(desired, live)
	if err != nil {
		t.Fatalf("copyDesiredState() error = %v", err)
	}
	// Touching the pod template is not an immutable-field change, so nothing may be reported to
	// the user as dropped.
	if len(ignored) != 0 {
		t.Errorf("ignored = %v, want none", ignored)
	}
	if live.Spec.Template.Annotations != nil {
		t.Errorf("annotations = %v, want nil", live.Spec.Template.Annotations)
	}
}

func TestCopyStatefulSetState_PodTemplateLabelsStillReplaced(t *testing.T) {
	// Labels are NOT given the annotation treatment: the pod template's labels have to match the
	// StatefulSet's immutable .spec.selector, so they are framework-owned and come from desired —
	// exactly like the object's own labels.
	live := stsWithTemplateAnnotations(nil)
	live.Spec.Template.Labels = map[string]string{"stale": "yes", "app": "old"}
	desired := stsWithTemplateAnnotations(nil)
	desired.Spec.Template.Labels = map[string]string{"app": "new"}

	ignored, err := copyDesiredState(desired, live)
	if err != nil {
		t.Fatalf("copyDesiredState() error = %v", err)
	}
	// Touching the pod template is not an immutable-field change, so nothing may be reported to
	// the user as dropped.
	if len(ignored) != 0 {
		t.Errorf("ignored = %v, want none", ignored)
	}

	if _, present := live.Spec.Template.Labels["stale"]; present {
		t.Error("pod template labels must be replaced wholesale, not merged")
	}
	if got := live.Spec.Template.Labels["app"]; got != "new" {
		t.Errorf("app label = %q, want %q", got, "new")
	}
}

// A RoleBinding fell through to copyGenericState, which copies every top-level field except
// apiVersion/kind/metadata/status — so it copied the immutable roleRef, and had no
// preserve-and-report path, so ImmutableFieldIgnored never fired either. A product that shipped a
// RoleBinding through the documented ExtraResources route and later changed its roleRef wedged
// every existing cluster's role group at the extras step, permanently.
func TestCopyRoleBindingState_PreservesImmutableRoleRefAndReportsIt(t *testing.T) {
	live := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "rb", Namespace: "ns"},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: "old"},
		Subjects:   []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: "stale", Namespace: "ns"}},
	}
	desired := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "rb", Namespace: "ns"},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: "new"},
		Subjects:   []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: "fresh", Namespace: "ns"}},
	}

	ignored, err := copyDesiredState(desired, live)
	if err != nil {
		t.Fatalf("copyDesiredState() error = %v", err)
	}

	if live.RoleRef.Name != "old" || live.RoleRef.Kind != "Role" {
		t.Errorf("roleRef must keep its live value (Kubernetes refuses the Update otherwise); got %+v", live.RoleRef)
	}
	// Silently preserving is what made a storage resize look successful; the same rule applies here.
	if len(ignored) != 1 || ignored[0] != "roleRef" {
		t.Errorf("ignored = %v, want [roleRef] so the user is told their change was dropped", ignored)
	}
	// Subjects ARE mutable: a subject the product removed must stop being bound.
	if len(live.Subjects) != 1 || live.Subjects[0].Name != "fresh" {
		t.Errorf("subjects must converge to desired; got %+v", live.Subjects)
	}
}

func TestCopyRoleBindingState_UnsetRoleRefIsNotReported(t *testing.T) {
	// An unset desired field is the product declining to have an opinion, not a change request —
	// the same rule copyStatefulSetState applies to its four preserved fields.
	live := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "rb", Namespace: "ns"},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: "old"},
	}
	desired := &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "rb", Namespace: "ns"}}

	ignored, err := copyDesiredState(desired, live)
	if err != nil {
		t.Fatalf("copyDesiredState() error = %v", err)
	}
	if len(ignored) != 0 {
		t.Errorf("ignored = %v, want none for an unset roleRef", ignored)
	}
	if live.RoleRef.Name != "old" {
		t.Errorf("roleRef = %+v, want it untouched", live.RoleRef)
	}
}

func TestCopyRoleState_ReplacesRulesWholesale(t *testing.T) {
	live := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "ns"},
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"get", "delete"}},
		},
	}
	desired := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "ns"},
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"get"}},
		},
	}

	if _, err := copyDesiredState(desired, live); err != nil {
		t.Fatalf("copyDesiredState() error = %v", err)
	}
	// Narrowing a permission must actually narrow it: merging would make a granted verb
	// unrevokable.
	if len(live.Rules) != 1 || len(live.Rules[0].Verbs) != 1 || live.Rules[0].Verbs[0] != "get" {
		t.Errorf("rules = %+v, want the desired set wholesale", live.Rules)
	}
}
