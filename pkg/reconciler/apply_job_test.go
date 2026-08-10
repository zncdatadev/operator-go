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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// A Job is the object downstream operators actually put in ExtraResources — a database migration, a
// one-shot registration — and the framework's generic fallback could not carry one: it assigns
// `spec` wholesale, and the API server GENERATES spec.selector and injects four UID-derived labels
// into spec.template at creation, so the SECOND pass with a byte-identical desired object was
// rejected. Reproduced against envtest before the typed rule existed:
//
//	spec.selector: Required value
//	spec.template.metadata.labels: Invalid value: null: `selector` does not match template `labels`
//	spec.template: ... field is immutable
var _ = Describe("Job through ExtraResources", func() {
	var ctx context.Context

	BeforeEach(func() { ctx = context.Background() })

	jobFor := func(name, namespace, image string) *batchv1.Job {
		return &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: batchv1.JobSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						RestartPolicy: corev1.RestartPolicyNever,
						Containers:    []corev1.Container{{Name: "migrate", Image: image}},
					},
				},
			},
		}
	}

	It("survives a second reconcile with nothing changed", func() {
		name := slotCRName("job-extra")
		createSlotCR(ctx, name, nil, map[string]v1alpha1.RoleSpec{"worker": defaultGroupRole()})
		jobName := name + "-migrate"
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: testNamespace}})
		})

		r := slotReconciler(k8sClient, nil, func(buildCtx *reconciler.RoleGroupBuildContext, res *reconciler.RoleGroupResources) {
			res.ExtraResources = []client.Object{jobFor(jobName, buildCtx.ClusterNamespace, "busybox:1")}
		})

		Expect(reconcileSlotCR(ctx, r, name)).To(Succeed())

		live := &batchv1.Job{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: jobName}, live)).To(Succeed())
		Expect(live.Spec.Selector).NotTo(BeNil(), "the API server generates the selector at creation")
		Expect(live.Spec.Template.Labels).To(HaveKey("batch.kubernetes.io/controller-uid"),
			"and injects UID-derived labels the handler cannot have built")

		// The handler builds the IDENTICAL object again. Before the typed rule this pass failed and
		// the role group went permanently Degraded quoting spec.selector.
		Expect(reconcileSlotCR(ctx, r, name)).To(Succeed())
	})

	It("leaves the live Job alone when the handler changes it, and says so", func() {
		// Create-once is the only semantics a Job has: its work is a side effect that already
		// happened. A product that needs it re-run changes the NAME.
		name := slotCRName("job-immutable")
		createSlotCR(ctx, name, nil, map[string]v1alpha1.RoleSpec{"worker": defaultGroupRole()})
		jobName := name + "-migrate"
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: testNamespace}})
		})

		image := "busybox:1"
		r := slotReconciler(k8sClient, nil, func(buildCtx *reconciler.RoleGroupBuildContext, res *reconciler.RoleGroupResources) {
			res.ExtraResources = []client.Object{jobFor(jobName, buildCtx.ClusterNamespace, image)}
		})

		Expect(reconcileSlotCR(ctx, r, name)).To(Succeed())

		image = "busybox:2"
		Expect(reconcileSlotCR(ctx, r, name)).To(Succeed(), "an immutable-field change must not wedge the role group")

		live := &batchv1.Job{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: jobName}, live)).To(Succeed())
		Expect(live.Spec.Template.Spec.Containers[0].Image).To(Equal("busybox:1"),
			"the live template is preserved — the work already ran")
	})

	It("still converges the fields batch DOES allow changing", func() {
		name := slotCRName("job-mutable")
		createSlotCR(ctx, name, nil, map[string]v1alpha1.RoleSpec{"worker": defaultGroupRole()})
		jobName := name + "-migrate"
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: testNamespace}})
		})

		suspend := false
		r := slotReconciler(k8sClient, nil, func(buildCtx *reconciler.RoleGroupBuildContext, res *reconciler.RoleGroupResources) {
			job := jobFor(jobName, buildCtx.ClusterNamespace, "busybox:1")
			job.Spec.Suspend = ptr.To(suspend)
			job.Spec.BackoffLimit = ptr.To(int32(6))
			res.ExtraResources = []client.Object{job}
		})

		Expect(reconcileSlotCR(ctx, r, name)).To(Succeed())

		// Preserving the whole spec would be the opposite failure: a Job could never be suspended,
		// and the knobs Kubernetes deliberately left mutable would be dead.
		suspend = true
		Expect(reconcileSlotCR(ctx, r, name)).To(Succeed())

		live := &batchv1.Job{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: jobName}, live)).To(Succeed())
		Expect(live.Spec.Suspend).To(HaveValue(BeTrue()))
	})
})
