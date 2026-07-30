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
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/config"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/testutil"
	corev1 "k8s.io/api/core/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
)

// raw wraps a JSON literal as the CRD carries it.
func raw(j string) *k8sruntime.RawExtension {
	return &k8sruntime.RawExtension{Raw: []byte(j)}
}

// members splits a RawExtension into its top-level keys so a merge result can be asserted on
// without depending on key order.
func members(r *k8sruntime.RawExtension) map[string]json.RawMessage {
	Expect(r).NotTo(BeNil())
	var m map[string]json.RawMessage
	Expect(json.Unmarshal(r.Raw, &m)).To(Succeed())
	return m
}

var _ = Describe("Affinity is decoded strictly", func() {
	var counter int

	newHandler := func() *reconciler.BaseRoleGroupHandler[*testutil.MockCluster] {
		return reconciler.NewBaseRoleGroupHandler[*testutil.MockCluster]("product:1", testScheme)
	}

	build := func(affinity *k8sruntime.RawExtension) error {
		counter++
		name := fmt.Sprintf("aff-%d", counter)
		_, err := newHandler().BuildResources(context.Background(), k8sClient,
			testutil.NewMockCluster(name, testNamespace),
			&reconciler.RoleGroupBuildContext{
				ClusterName:      name,
				ClusterNamespace: testNamespace,
				ClusterSpec:      &v1alpha1.GenericClusterSpec{},
				RoleName:         "worker",
				RoleSpec:         &v1alpha1.RoleSpec{},
				RoleGroupName:    "default",
				RoleGroupSpec: v1alpha1.RoleGroupSpec{
					Replicas: ptr.To(int32(1)),
					Config:   &v1alpha1.RoleGroupConfigSpec{Affinity: affinity},
				},
				MergedConfig: &config.MergedConfig{},
				ResourceName: reconciler.RoleGroupResourceName(name, "worker", "default"),
			})
		return err
	}

	It("rejects a misspelled affinity key instead of scheduling anywhere", func() {
		// This is the whole point. `nodeAffinty` passes admission (the CRD declares the field
		// x-kubernetes-preserve-unknown-fields, so the API server neither validates nor prunes it)
		// and a plain json.Unmarshal ignores it — so the user's rack awareness silently evaporated
		// and the pods went wherever the scheduler liked, with no event and no status change.
		err := build(raw(`{"nodeAffinty":{"requiredDuringSchedulingIgnoredDuringExecution":{"nodeSelectorTerms":[]}}}`))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("nodeAffinty"), "the message must name the bad field")
		Expect(err.Error()).To(ContainSubstring("affinity"))
	})

	It("rejects a misspelling nested inside a valid member", func() {
		// Strictness has to reach all the way down: the outer key being right is no protection.
		err := build(raw(`{"nodeAffinity":{"requiredDuringSchedulingIgnoredDuringExecutionn":{}}}`))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("requiredDuringSchedulingIgnoredDuringExecutionn"))
	})

	It("still accepts a valid affinity and puts it on the pod", func() {
		// The strictness must not cost the feature: everything that worked has to keep working.
		counter++
		name := fmt.Sprintf("aff-ok-%d", counter)
		resources, err := newHandler().BuildResources(context.Background(), k8sClient,
			testutil.NewMockCluster(name, testNamespace),
			&reconciler.RoleGroupBuildContext{
				ClusterName:      name,
				ClusterNamespace: testNamespace,
				ClusterSpec:      &v1alpha1.GenericClusterSpec{},
				RoleName:         "worker",
				RoleSpec:         &v1alpha1.RoleSpec{},
				RoleGroupName:    "default",
				RoleGroupSpec: v1alpha1.RoleGroupSpec{
					Replicas: ptr.To(int32(1)),
					Config: &v1alpha1.RoleGroupConfigSpec{
						Affinity: raw(`{"podAntiAffinity":{"requiredDuringSchedulingIgnoredDuringExecution":[` +
							`{"topologyKey":"kubernetes.io/hostname","labelSelector":{"matchLabels":{"a":"b"}}}]}}`),
					},
				},
				MergedConfig: &config.MergedConfig{},
				ResourceName: reconciler.RoleGroupResourceName(name, "worker", "default"),
			})
		Expect(err).NotTo(HaveOccurred())

		affinity := resources.StatefulSet.Spec.Template.Spec.Affinity
		Expect(affinity).NotTo(BeNil())
		Expect(affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution).To(HaveLen(1))
		Expect(affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution[0].TopologyKey).
			To(Equal("kubernetes.io/hostname"))
	})

	It("confirms the API server really does accept the typo", func() {
		// The premise of this whole change: admission is not the layer that catches it. If this spec
		// ever fails, the CRD gained a schema for affinity and the strict decode is belt-and-braces.
		cr := testutil.NewMockCluster("aff-admission", testNamespace)
		cr.Spec.Roles = map[string]v1alpha1.RoleSpec{
			"worker": {RoleGroups: map[string]v1alpha1.RoleGroupSpec{
				"default": {Config: &v1alpha1.RoleGroupConfigSpec{
					Affinity: raw(`{"nodeAffinty":{"whatever":true}}`),
				}},
			}},
		}
		Expect(k8sClient.Create(ctx, cr)).To(Succeed(),
			"the API server accepts the typo — it has no schema to check it against")
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, cr) })

		stored := &testutil.MockCluster{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Namespace: testNamespace, Name: "aff-admission"}, stored)).To(Succeed())
		Expect(string(stored.Spec.Roles["worker"].RoleGroups["default"].Config.Affinity.Raw)).
			To(ContainSubstring("nodeAffinty"), "and stores it verbatim")
	})
})

var _ = Describe("Affinity merges per member, not wholesale", func() {
	// MergeRoleGroupConfig's own doc comment says why leaf granularity matters for `resources`:
	// "Overriding one knob is the normal way to use this API, and it silently dropped the rest."
	// Affinity had the same shape and is where it hurts most.
	roleAntiAffinity := `{"podAntiAffinity":{"requiredDuringSchedulingIgnoredDuringExecution":[` +
		`{"topologyKey":"kubernetes.io/hostname","labelSelector":{"matchLabels":{"role":"worker"}}}]}}`
	groupNodeAffinity := `{"nodeAffinity":{"requiredDuringSchedulingIgnoredDuringExecution":` +
		`{"nodeSelectorTerms":[{"matchExpressions":[{"key":"instance-type","operator":"In","values":["r5.xlarge"]}]}]}}}`

	mergedConfig := func(role, group *k8sruntime.RawExtension) *v1alpha1.RoleGroupConfigSpec {
		return reconciler.MergeRoleGroupConfig(
			&v1alpha1.RoleGroupConfigSpec{Affinity: role},
			&v1alpha1.RoleGroupConfigSpec{Affinity: group},
		)
	}

	It("keeps the role's spreading when a group adds node affinity", func() {
		// The bug: a role spreads its pods with podAntiAffinity, a group pins itself to an instance
		// type, and the whole group ended up on ONE node because the group's affinity replaced the
		// role's entirely.
		merged := mergedConfig(raw(roleAntiAffinity), raw(groupNodeAffinity))

		m := members(merged.Affinity)
		Expect(m).To(HaveKey("podAntiAffinity"), "inherited from the role")
		Expect(m).To(HaveKey("nodeAffinity"), "declared by the group")

		// And the merged value is still a valid Affinity with both constraints intact.
		affinity, err := reconciler.DecodeAffinity(merged.Affinity)
		Expect(err).NotTo(HaveOccurred())
		Expect(affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution).To(HaveLen(1))
		Expect(affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms).To(HaveLen(1))
	})

	It("lets the group win for a member both declare", func() {
		// Per-member precedence, not per-field-inside-a-member: a nodeSelectorTerm list is an OR of
		// ANDs, and interleaving two of them produces a constraint neither author wrote.
		role := `{"nodeAffinity":{"requiredDuringSchedulingIgnoredDuringExecution":` +
			`{"nodeSelectorTerms":[{"matchExpressions":[{"key":"zone","operator":"In","values":["a"]}]}]}}}`
		group := `{"nodeAffinity":{"requiredDuringSchedulingIgnoredDuringExecution":` +
			`{"nodeSelectorTerms":[{"matchExpressions":[{"key":"zone","operator":"In","values":["b"]}]}]}}}`

		affinity, err := reconciler.DecodeAffinity(mergedConfig(raw(role), raw(group)).Affinity)
		Expect(err).NotTo(HaveOccurred())
		terms := affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
		Expect(terms).To(HaveLen(1), "the group's term replaces the role's, they are not concatenated")
		Expect(terms[0].MatchExpressions[0].Values).To(Equal([]string{"b"}))
	})

	It("inherits the role's affinity when the group declares none", func() {
		merged := mergedConfig(raw(roleAntiAffinity), nil)
		Expect(members(merged.Affinity)).To(HaveKey("podAntiAffinity"))
	})

	It("keeps the group's affinity when the role declares none", func() {
		merged := mergedConfig(nil, raw(groupNodeAffinity))
		Expect(members(merged.Affinity)).To(HaveKey("nodeAffinity"))
	})

	It("produces no affinity when neither declares one", func() {
		Expect(mergedConfig(nil, nil).Affinity).To(BeNil())
	})

	It("does not mutate either input", func() {
		role, group := raw(roleAntiAffinity), raw(groupNodeAffinity)
		roleBefore, groupBefore := string(role.Raw), string(group.Raw)

		merged := mergedConfig(role, group)
		merged.Affinity.Raw[0] = 'X'

		Expect(string(role.Raw)).To(Equal(roleBefore))
		Expect(string(group.Raw)).To(Equal(groupBefore))
	})

	It("carries an unknown member through so the decode can still reject it", func() {
		// The merge must not become a second place where a typo disappears: dropping the unknown key
		// here would hand DecodeAffinity a clean object and the user would be back to silence.
		merged := mergedConfig(raw(`{"nodeAffinty":{"x":1}}`), raw(groupNodeAffinity))
		Expect(members(merged.Affinity)).To(HaveKey("nodeAffinty"))

		_, err := reconciler.DecodeAffinity(merged.Affinity)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("nodeAffinty"))
	})

	It("falls back to the group's value when a side is not a JSON object", func() {
		// Nothing sensible can be merged member-wise here. Rather than guess, keep the old
		// precedence and let the strict decode fail the build with the real reason.
		merged := mergedConfig(raw(`"not-an-object"`), raw(groupNodeAffinity))
		Expect(members(merged.Affinity)).To(HaveKey("nodeAffinity"))

		merged = mergedConfig(raw(roleAntiAffinity), raw(`["also-not-an-object"]`))
		Expect(merged.Affinity).NotTo(BeNil())
		_, err := reconciler.DecodeAffinity(merged.Affinity)
		Expect(err).To(HaveOccurred(), "the malformed value reaches the loud check")
	})

	It("treats an explicit JSON null as no affinity at all", func() {
		merged := mergedConfig(raw(roleAntiAffinity), raw(`null`))
		Expect(members(merged.Affinity)).To(HaveKey("podAntiAffinity"),
			"a null group value inherits rather than blanking the role's")
	})
})

var _ = Describe("DecodeAffinity", func() {
	It("returns nil for an absent or empty value", func() {
		affinity, err := reconciler.DecodeAffinity(nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(affinity).To(BeNil())

		affinity, err = reconciler.DecodeAffinity(&k8sruntime.RawExtension{})
		Expect(err).NotTo(HaveOccurred())
		Expect(affinity).To(BeNil())
	})

	It("rejects malformed JSON", func() {
		_, err := reconciler.DecodeAffinity(raw(`{`))
		Expect(err).To(HaveOccurred())
	})

	It("accepts an empty object", func() {
		affinity, err := reconciler.DecodeAffinity(raw(`{}`))
		Expect(err).NotTo(HaveOccurred())
		Expect(affinity).To(Equal(&corev1.Affinity{}))
	})
})

var _ = Describe("Role-level affinity reaches a role group through the reconciler", func() {
	It("applies a role's affinity to a group that declares none", func() {
		// End to end through MergeRoleGroupConfig and buildStatefulSet, which is the path the
		// reconciler takes: role-level config was silently dropped before the merge existed, and
		// this is the guarantee that keeps it delivered.
		handler := reconciler.NewBaseRoleGroupHandler[*testutil.MockCluster]("product:1", testScheme)
		roleAffinity := raw(`{"podAntiAffinity":{"preferredDuringSchedulingIgnoredDuringExecution":[` +
			`{"weight":100,"podAffinityTerm":{"topologyKey":"kubernetes.io/hostname"}}]}}`)

		groupSpec := v1alpha1.RoleGroupSpec{Replicas: ptr.To(int32(2))}
		roleConfig := &v1alpha1.RoleGroupConfigSpec{Affinity: roleAffinity}
		groupSpec.Config = reconciler.MergeRoleGroupConfig(roleConfig, groupSpec.GetConfig())

		resources, err := handler.BuildResources(context.Background(), k8sClient,
			testutil.NewMockCluster("aff-role", testNamespace),
			&reconciler.RoleGroupBuildContext{
				ClusterName:      "aff-role",
				ClusterNamespace: testNamespace,
				ClusterSpec:      &v1alpha1.GenericClusterSpec{},
				RoleName:         "worker",
				RoleSpec:         &v1alpha1.RoleSpec{Config: roleConfig},
				RoleGroupName:    "default",
				RoleGroupSpec:    groupSpec,
				MergedConfig:     &config.MergedConfig{},
				ResourceName:     reconciler.RoleGroupResourceName("aff-role", "worker", "default"),
			})
		Expect(err).NotTo(HaveOccurred())

		podAffinity := resources.StatefulSet.Spec.Template.Spec.Affinity
		Expect(podAffinity).NotTo(BeNil())
		Expect(podAffinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution).To(HaveLen(1))
		Expect(podAffinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution[0].Weight).
			To(Equal(int32(100)))
	})
})
