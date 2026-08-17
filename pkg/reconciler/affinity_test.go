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
		return reconciler.NewBaseRoleGroupHandler[*testutil.MockCluster](testScheme)
	}

	build := func(affinity *k8sruntime.RawExtension) error {
		counter++
		name := fmt.Sprintf("aff-%d", counter)
		_, err := newHandler().BuildResources(context.Background(), k8sClient,
			testutil.NewMockCluster(name, testNamespace),
			&reconciler.RoleGroupBuildContext{
				ResolvedImage:    reconciler.ResolvedImage{Reference: "test-image:latest"},
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
				ResolvedImage:    reconciler.ResolvedImage{Reference: "test-image:latest"},
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

var _ = Describe("Affinity is replaced wholesale, and the replacement is reported", func() {
	// `affinity` follows the Kubernetes rule: any layer that states one replaces the layer beneath
	// it entirely. Per-member inheritance was implemented here once and reverted, and the objection
	// that carried the revert still stands — PodSpec.affinity, a Helm value and a Kustomize patch
	// all replace wholesale, so folding per member obliges a user to learn a semantic for exactly
	// one field of one CRD. What the replacement DISCARDS is reported instead, so the product
	// default a user overrode is not lost in silence.
	roleAntiAffinity := `{"podAntiAffinity":{"requiredDuringSchedulingIgnoredDuringExecution":[` +
		`{"topologyKey":"kubernetes.io/hostname","labelSelector":{"matchLabels":{"role":"worker"}}}]}}`
	groupNodeAffinity := `{"nodeAffinity":{"requiredDuringSchedulingIgnoredDuringExecution":` +
		`{"nodeSelectorTerms":[{"matchExpressions":[{"key":"instance-type","operator":"In","values":["r5.xlarge"]}]}]}}}`

	fold := func(role, group *k8sruntime.RawExtension) (
		*v1alpha1.RoleGroupConfigSpec, []reconciler.AffinityReplacement) {
		out, replaced, err := reconciler.FoldCommonConfig(
			&v1alpha1.RoleGroupConfigSpec{Affinity: role},
			&v1alpha1.RoleGroupConfigSpec{Affinity: group},
		)
		Expect(err).NotTo(HaveOccurred())
		return out, replaced
	}
	mergedConfig := func(role, group *k8sruntime.RawExtension) *v1alpha1.RoleGroupConfigSpec {
		out, _ := fold(role, group)
		return out
	}

	It("inherits the role's affinity when the group declares none", func() {
		cfg, replaced := fold(raw(roleAntiAffinity), nil)
		Expect(members(cfg.Affinity)).To(HaveKey("podAntiAffinity"))
		Expect(replaced).To(BeEmpty(), "a layer that states nothing discards nothing")
	})

	It("replaces the role's affinity wholesale when the group states one", func() {
		// The Kubernetes rule, and the cost it carries: asking for an instance type at the group
		// level takes the whole field over, so the role's spread is gone.
		m := members(mergedConfig(raw(roleAntiAffinity), raw(groupNodeAffinity)).Affinity)
		Expect(m).To(HaveKey("nodeAffinity"), "the member the group stated")
		Expect(m).NotTo(HaveKey("podAntiAffinity"), "and the role's, replaced wholesale")
	})

	It("REPORTS the members a wholesale replacement discarded", func() {
		// The mitigation the revert to wholesale is paid for with. Without it, a user pinning an
		// instance type silently stops their quorum being spread across nodes — invisible in the CR,
		// in the pod spec and in every status condition.
		_, replaced := fold(raw(roleAntiAffinity), raw(groupNodeAffinity))
		Expect(replaced).To(HaveLen(1))
		Expect(replaced[0].Layer).To(Equal(1), "the layer that did the replacing")
		Expect(replaced[0].Dropped).To(Equal([]string{"podAntiAffinity"}))
	})

	It("reports every discarded member in a fixed order, so the message is byte-stable", func() {
		// An unstable message would defeat the no-op guard in updateStatus and make the controller
		// reschedule itself forever, so the order is the struct's, never a map's.
		both := `{"nodeAffinity":{"requiredDuringSchedulingIgnoredDuringExecution":` +
			`{"nodeSelectorTerms":[{"matchExpressions":[{"key":"rack","operator":"Exists"}]}]}},` +
			`"podAntiAffinity":{"requiredDuringSchedulingIgnoredDuringExecution":[` +
			`{"topologyKey":"kubernetes.io/hostname"}]}}`
		for range 20 {
			_, replaced := fold(raw(both), raw(`{"podAffinity":{"requiredDuringSchedulingIgnoredDuringExecution":[`+
				`{"topologyKey":"kubernetes.io/hostname"}]}}`))
			Expect(replaced).To(HaveLen(1))
			Expect(replaced[0].Dropped).To(Equal([]string{"nodeAffinity", "podAntiAffinity"}))
		}
	})

	It("lets a group clear the role's affinity entirely with an empty object, and reports nothing", func() {
		// The single-node development escape hatch, preserved. No report: clearing is exactly what
		// `affinity: {}` asks for, and warning about an explicit request is noise. It works here and
		// NOT in `resources` because the schemas differ: `affinity` is
		// x-kubernetes-preserve-unknown-fields, so the API server never prunes inside it and a
		// stored `{}` is always something the user wrote.
		cfg, replaced := fold(raw(roleAntiAffinity), raw(`{}`))
		Expect(cfg.Affinity).To(BeNil(), "no affinity at all, not the role's")
		Expect(replaced).To(BeEmpty(), "an explicit clear is the user's own visible decision")
	})

	It("keeps the group's affinity when the role declares none, and reports nothing", func() {
		cfg, replaced := fold(nil, raw(groupNodeAffinity))
		Expect(members(cfg.Affinity)).To(HaveKey("nodeAffinity"))
		Expect(replaced).To(BeEmpty(), "there was nothing beneath to discard")
	})

	It("produces no affinity when neither declares one", func() {
		Expect(mergedConfig(nil, nil).Affinity).To(BeNil())
	})

	It("does not alias the role's value into the merged config", func() {
		// The merged config is handed to a handler that may edit it; the CR's spec must not move.
		role := raw(roleAntiAffinity)
		before := string(role.Raw)

		merged := mergedConfig(role, nil)
		merged.Affinity.Raw[0] = 'X'

		Expect(string(role.Raw)).To(Equal(before))
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

	It("rejects two concatenated objects instead of decoding only the first", func() {
		// json.Decoder reads ONE value and leaves the rest of the stream alone, so without the
		// json.Valid guard this decodes nodeAffinity and drops podAffinity silently — the same
		// partial loss the strict decode exists to prevent, arriving by a different door. Only
		// reachable for a RawExtension built in Go (a webhook defaulter, product code): a value that
		// came through the API server is always a single re-serialized object.
		_, err := reconciler.DecodeAffinity(raw(
			`{"nodeAffinity":{"requiredDuringSchedulingIgnoredDuringExecution":{"nodeSelectorTerms":[]}}} ` +
				`{"podAffinity":{}}`))
		Expect(err).To(HaveOccurred())
	})

	It("rejects trailing garbage after a valid object", func() {
		_, err := reconciler.DecodeAffinity(raw(`{"nodeAffinity":{}} oops`))
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
		// End to end through FoldCommonConfig and buildStatefulSet, which is the path the reconciler
		// takes: role-level config was silently dropped before the fold existed, and this is the
		// guarantee that keeps it delivered.
		handler := reconciler.NewBaseRoleGroupHandler[*testutil.MockCluster](testScheme)
		roleAffinity := raw(`{"podAntiAffinity":{"preferredDuringSchedulingIgnoredDuringExecution":[` +
			`{"weight":100,"podAffinityTerm":{"topologyKey":"kubernetes.io/hostname"}}]}}`)

		groupSpec := v1alpha1.RoleGroupSpec{Replicas: ptr.To(int32(2))}
		roleConfig := &v1alpha1.RoleGroupConfigSpec{Affinity: roleAffinity}
		folded, _, foldErr := reconciler.FoldCommonConfig(roleConfig, groupSpec.GetConfig())
		Expect(foldErr).NotTo(HaveOccurred())
		groupSpec.Config = folded

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
				ResolvedImage:    reconciler.ResolvedImage{Reference: "product:1"},
			})
		Expect(err).NotTo(HaveOccurred())

		podAffinity := resources.StatefulSet.Spec.Template.Spec.Affinity
		Expect(podAffinity).NotTo(BeNil())
		Expect(podAffinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution).To(HaveLen(1))
		Expect(podAffinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution[0].Weight).
			To(Equal(int32(100)))
	})
})
