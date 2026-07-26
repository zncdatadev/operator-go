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

package testutil_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/testutil"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
)

// These specs assert that the CRDs envtest installs carry a REAL schema — that the API server
// defaults, validates and prunes the mock cluster resources the way it would a product CR.
//
// They are not testing Kubernetes. They are testing this repository's test harness: the previous
// hand-written CRDs declared `x-kubernetes-preserve-unknown-fields: true` for spec and status and
// nothing else, so every `+kubebuilder:default`, `+kubebuilder:validation:*` and required-field
// marker in pkg/apis was inert throughout the suite. Any spec that believes it exercised
// defaulting or validation was exercising Go zero values. If these specs start failing, the CRDs
// have regressed to schema-free and the guarantees the rest of the suite thinks it has are gone.
var _ = Describe("Generated test CRD schema", func() {
	// crdSchemaNamespace is where these fixtures live; the suite's envtest provides it.
	const crdSchemaNamespace = "default"

	var counter int

	// createCluster persists a MockCluster under a unique name and returns it re-read from the API
	// server, so every field below is one the server actually stored.
	createCluster := func(spec v1alpha1.GenericClusterSpec) *testutil.MockCluster {
		counter++
		name := fmt.Sprintf("schema-%d", counter)
		cr := testutil.NewMockCluster(name, crdSchemaNamespace)
		cr.Spec = spec
		Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, cr) })

		stored := &testutil.MockCluster{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: crdSchemaNamespace, Name: name}, stored)).To(Succeed())
		return stored
	}

	It("applies +kubebuilder:default to an omitted field", func() {
		stored := createCluster(v1alpha1.GenericClusterSpec{
			Roles: map[string]v1alpha1.RoleSpec{
				"broker": {RoleGroups: map[string]v1alpha1.RoleGroupSpec{"default": {}}},
			},
		})

		group := stored.Spec.Roles["broker"].RoleGroups["default"]
		Expect(group.Replicas).NotTo(BeNil(), "the API server must stamp the replicas default")
		Expect(*group.Replicas).To(Equal(int32(1)))
	})

	It("enforces +kubebuilder:validation:Minimum", func() {
		counter++
		cr := testutil.NewMockCluster(fmt.Sprintf("schema-invalid-%d", counter), crdSchemaNamespace)
		cr.Spec = v1alpha1.GenericClusterSpec{
			Roles: map[string]v1alpha1.RoleSpec{
				"broker": {RoleGroups: map[string]v1alpha1.RoleGroupSpec{
					"default": {Replicas: ptr.To(int32(-1))},
				}},
			},
		}
		Expect(k8sClient.Create(ctx, cr)).NotTo(Succeed(),
			"replicas is marked Minimum=0, so a negative value must be rejected")
	})

	It("enforces a required field on a status condition", func() {
		stored := createCluster(v1alpha1.GenericClusterSpec{})
		stored.Status.Conditions = []metav1.Condition{
			{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Test"},
		}
		// lastTransitionTime is required by the metav1.Condition schema. Under the schema-free
		// CRD this write succeeded and the suite happily persisted invalid conditions.
		Expect(k8sClient.Status().Update(ctx, stored)).NotTo(Succeed())
	})

	// The next two reproduce, at the harness level, the defaulting behaviour that
	// reconciler.MergeRoleGroupConfig documents as a known caveat. Both were structurally
	// impossible to observe before: without a schema the API server stamped nothing, so the
	// merge logic was only ever tested against inputs that do not occur in production.
	It("stamps gracefulShutdownTimeout into every config block that exists", func() {
		stored := createCluster(v1alpha1.GenericClusterSpec{
			Roles: map[string]v1alpha1.RoleSpec{
				"broker": {
					// Role level declares the value the operator intends to inherit.
					Config: &v1alpha1.RoleGroupConfigSpec{GracefulShutdownTimeout: "5m"},
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{
						// The group declares a config block for an unrelated reason.
						"default": {Config: &v1alpha1.RoleGroupConfigSpec{
							Resources: &v1alpha1.ResourcesSpec{},
						}},
					},
				},
			},
		})

		group := stored.Spec.Roles["broker"].RoleGroups["default"]
		Expect(group.Config.GracefulShutdownTimeout).To(Equal("30s"),
			"the API server stamps the CRD default into the group's config block, which is then "+
				"indistinguishable from an explicit group-level value and wins the merge over the "+
				"role's 5m")
	})

	// Storage capacity has TWO distinct failure modes, and which one fires depends on how the CR
	// was written. Both lose the role-level value; only the first is reachable with kubectl.
	It("stamps the storage capacity default into a YAML-authored group that sets only storageClass", func() {
		counter++
		name := fmt.Sprintf("schema-storage-yaml-%d", counter)
		// Built as unstructured on purpose: this is the kubectl/GitOps path, where `capacity` is
		// genuinely ABSENT from the request, so structural defaulting fills it.
		cr := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "test.zncdata.dev/v1alpha1",
			"kind":       "MockCluster",
			"metadata":   map[string]any{"name": name, "namespace": crdSchemaNamespace},
			"spec": map[string]any{
				"roles": map[string]any{
					"broker": map[string]any{
						"config": map[string]any{
							"resources": map[string]any{
								"storage": map[string]any{"capacity": "500Gi"},
							},
						},
						"roleGroups": map[string]any{
							"default": map[string]any{
								"config": map[string]any{
									"resources": map[string]any{
										// Only a storageClass. The user is overriding one leaf.
										"storage": map[string]any{"storageClass": "fast-ssd"},
									},
								},
							},
						},
					},
				},
			},
		}}
		cr.SetGroupVersionKind(testutil.MockClusterGroupVersion.WithKind("MockCluster"))
		Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, cr) })

		stored := &testutil.MockCluster{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: crdSchemaNamespace, Name: name}, stored)).To(Succeed())

		group := stored.Spec.Roles["broker"].RoleGroups["default"]
		Expect(group.Config.Resources.Storage.Capacity.String()).To(Equal("10Gi"),
			"the group asked only for a storageClass, but the API server filled capacity with the "+
				"CRD default — the role's 500Gi never reaches it, and copyStatefulSetState then "+
				"pins the resulting PVC template forever")
	})

	It("sends an explicit zero capacity from a Go client, so the default never applies", func() {
		stored := createCluster(v1alpha1.GenericClusterSpec{
			Roles: map[string]v1alpha1.RoleSpec{
				"broker": {
					Config: &v1alpha1.RoleGroupConfigSpec{Resources: &v1alpha1.ResourcesSpec{
						Storage: &v1alpha1.StorageResource{Capacity: resource.MustParse("500Gi")},
					}},
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{
						"default": {Config: &v1alpha1.RoleGroupConfigSpec{
							Resources: &v1alpha1.ResourcesSpec{
								Storage: &v1alpha1.StorageResource{StorageClass: "fast-ssd"},
							},
						}},
					},
				},
			},
		})

		// The second failure mode, and the nastier one. Capacity is a bare resource.Quantity, so
		// `omitempty` cannot omit it (encoding/json does not treat a struct as empty) and its
		// MarshalJSON renders the zero value as "0". The field is therefore PRESENT in the
		// request, structural defaulting skips it, and the group keeps a zero capacity that then
		// wins the role/roleGroup merge.
		group := stored.Spec.Roles["broker"].RoleGroups["default"]
		Expect(group.Config.Resources.Storage.Capacity.String()).To(Equal("0"),
			"a Go-constructed spec persists capacity 0, not the 10Gi default and not the role's 500Gi")
	})
})
