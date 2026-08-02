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
	"github.com/zncdatadev/operator-go/pkg/productlogging"
	"github.com/zncdatadev/operator-go/pkg/testutil"
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

	// These three pin the FIX for the defaulting bug the harness made observable. Before it, the
	// API server stamped CRD defaults into any config block that existed, which made "the group
	// did not set this" indistinguishable from "the group asked for the default" — so a role-level
	// value could never reach a group that declared a config block for any other reason.
	//
	// The defaults now live at consumption time (GetGracefulShutdownTimeout, GetCapacity), so the
	// stored spec faithfully records what the user actually wrote.
	It("does not stamp gracefulShutdownTimeout into a group that did not set it", func() {
		stored := createCluster(v1alpha1.GenericClusterSpec{
			Roles: map[string]v1alpha1.RoleSpec{
				"broker": {
					Config: &v1alpha1.RoleGroupConfigSpec{GracefulShutdownTimeout: ptr.To("5m")},
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{
						// A config block declared for an unrelated reason.
						"default": {Config: &v1alpha1.RoleGroupConfigSpec{
							Resources: &v1alpha1.ResourcesSpec{},
						}},
					},
				},
			},
		})

		group := stored.Spec.Roles["broker"].RoleGroups["default"]
		Expect(group.Config.GracefulShutdownTimeout).To(BeNil(),
			"nil is what lets the role's 5m win the merge; a stamped default would win instead")
		Expect(group.Config.GetGracefulShutdownTimeout()).To(Equal("30s"),
			"the framework default is still available, just applied when the value is consumed")
	})

	It("does not stamp a storage capacity into a YAML-authored group that sets only storageClass", func() {
		counter++
		name := fmt.Sprintf("schema-storage-yaml-%d", counter)
		// Unstructured on purpose: the kubectl/GitOps path, where `capacity` is genuinely absent.
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
		Expect(group.Config.Resources.Storage.Capacity).To(BeNil(),
			"overriding one leaf must not silently downgrade the capacity the role asked for")
		Expect(group.Config.Resources.Storage.StorageClass).To(HaveValue(Equal("fast-ssd")))
	})

	It("does not stamp a log level into a group that declared an empty console block", func() {
		// The same defect class as the two specs above, on the field where it survived longest.
		// LogLevelSpec.Level used to carry +kubebuilder:default:="INFO", and `console` is an object
		// a role group declares deliberately — so `console: {}` came back from the API server as
		// `console: {level: INFO}`, and the role's DEBUG lost the merge. mergeContainerLogging has
		// a guard written for exactly this case; the default made it unreachable, because the
		// field was already filled before the merge ever saw it.
		stored := createCluster(v1alpha1.GenericClusterSpec{
			Roles: map[string]v1alpha1.RoleSpec{
				"broker": {
					Config: &v1alpha1.RoleGroupConfigSpec{Logging: &v1alpha1.LoggingSpec{
						Containers: map[string]v1alpha1.LoggingConfigSpec{
							"broker": {Console: &v1alpha1.LogLevelSpec{Level: "DEBUG"}},
						},
					}},
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{
						"default": {Config: &v1alpha1.RoleGroupConfigSpec{Logging: &v1alpha1.LoggingSpec{
							Containers: map[string]v1alpha1.LoggingConfigSpec{
								"broker": {Console: &v1alpha1.LogLevelSpec{}},
							},
						}}},
					},
				},
			},
		})

		role := stored.Spec.Roles["broker"]
		group := role.RoleGroups["default"]
		Expect(group.Config.Logging.Containers["broker"].Console.Level).To(BeEmpty(),
			"an empty level is what lets the role's DEBUG win the merge")

		merged := productlogging.MergeLoggingSpec(role.Config.Logging, group.Config.Logging)
		Expect(merged.Containers["broker"].Console.Level).To(Equal("DEBUG"),
			"the role's threshold must survive a group that named console without setting a level")
	})

	It("keeps a level the group actually asked for", func() {
		// The other half of the contract: nothing about the fix may make a stated value inheritable.
		stored := createCluster(v1alpha1.GenericClusterSpec{
			Roles: map[string]v1alpha1.RoleSpec{
				"broker": {
					Config: &v1alpha1.RoleGroupConfigSpec{Logging: &v1alpha1.LoggingSpec{
						Containers: map[string]v1alpha1.LoggingConfigSpec{
							"broker": {Console: &v1alpha1.LogLevelSpec{Level: "DEBUG"}},
						},
					}},
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{
						"default": {Config: &v1alpha1.RoleGroupConfigSpec{Logging: &v1alpha1.LoggingSpec{
							Containers: map[string]v1alpha1.LoggingConfigSpec{
								"broker": {Console: &v1alpha1.LogLevelSpec{Level: "WARN"}},
							},
						}}},
					},
				},
			},
		})

		role := stored.Spec.Roles["broker"]
		merged := productlogging.MergeLoggingSpec(role.Config.Logging, role.RoleGroups["default"].Config.Logging)
		Expect(merged.Containers["broker"].Console.Level).To(Equal("WARN"))
	})

	It("omits an unset capacity from a Go-constructed spec", func() {
		stored := createCluster(v1alpha1.GenericClusterSpec{
			Roles: map[string]v1alpha1.RoleSpec{
				"broker": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{
						"default": {Config: &v1alpha1.RoleGroupConfigSpec{
							Resources: &v1alpha1.ResourcesSpec{
								Storage: &v1alpha1.StorageResource{StorageClass: ptr.To("fast-ssd")},
							},
						}},
					},
				},
			},
		})

		// As a bare resource.Quantity this transmitted "0": `omitempty` cannot omit a struct, and
		// Quantity.MarshalJSON renders the zero value. The field was therefore always present, the
		// API server never defaulted it, and a group persisted an explicit zero capacity. As a
		// pointer it is genuinely absent.
		group := stored.Spec.Roles["broker"].RoleGroups["default"]
		Expect(group.Config.Resources.Storage.Capacity).To(BeNil())
		capacity := group.Config.Resources.Storage.GetCapacity()
		Expect(capacity.String()).To(Equal("10Gi"),
			"the framework default applies at consumption time")
	})
})

// A role or role group name is not free-form: it becomes a segment of every resource name the
// framework derives ("<cluster>-<role>-<group>") and the value of an app.kubernetes.io label.
// Names that are not lowercase RFC 1123 labels produce resource names the API server refuses, and
// before the CEL rules below that refusal only surfaced mid-reconcile — as a permanently Degraded
// role quoting a metadata.name the user never wrote. These specs assert the rejection now happens
// at admission, where the message reaches the person who can fix it.
var _ = Describe("Role and role group name validation", func() {
	const nameValidationNamespace = "default"

	var counter int

	// applyWithRole attempts to persist a cluster declaring exactly one role and one role group,
	// and returns the API server's verdict.
	applyWithRole := func(roleName, groupName string) error {
		counter++
		cr := testutil.NewMockCluster(fmt.Sprintf("names-%d", counter), nameValidationNamespace)
		cr.Spec = v1alpha1.GenericClusterSpec{
			Roles: map[string]v1alpha1.RoleSpec{
				roleName: {RoleGroups: map[string]v1alpha1.RoleGroupSpec{groupName: {}}},
			},
		}
		err := k8sClient.Create(ctx, cr)
		if err == nil {
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, cr) })
		}
		return err
	}

	DescribeTable("rejects a role name that cannot become a resource name",
		func(roleName string) {
			err := applyWithRole(roleName, "default")
			Expect(err).To(HaveOccurred(), "role name %q must be rejected at admission", roleName)
			Expect(err.Error()).To(ContainSubstring("lowercase RFC 1123 label"),
				"the message has to name the rule, not just the field")
		},
		Entry("uppercase", "Coordinator"),
		Entry("underscore", "my_role"),
		Entry("dot", "a.b"),
		Entry("leading dash", "-worker"),
		Entry("trailing dash", "worker-"),
		Entry("empty", ""),
	)

	DescribeTable("rejects a role group name that cannot become a resource name",
		func(groupName string) {
			err := applyWithRole("worker", groupName)
			Expect(err).To(HaveOccurred(), "role group name %q must be rejected at admission", groupName)
			Expect(err.Error()).To(ContainSubstring("lowercase RFC 1123 label"))
		},
		Entry("uppercase", "Default"),
		Entry("underscore", "high_mem"),
		Entry("dot", "rack.a"),
	)

	It("accepts the names products actually use", func() {
		// The rule must not be stricter than the resource names it protects: everything here has
		// always worked and must keep working.
		Expect(applyWithRole("namenode", "default")).To(Succeed())
		Expect(applyWithRole("journal-node", "rack-a-1")).To(Succeed())
		Expect(applyWithRole("s3", "0")).To(Succeed())
	})
})
