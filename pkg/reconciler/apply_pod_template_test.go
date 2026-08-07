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
