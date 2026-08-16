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
	"bytes"
	"encoding/json"
	"fmt"
	"maps"

	"github.com/zncdatadev/operator-go/pkg/constant"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
)

// DecodeAffinity decodes the schema-free `config.affinity` RawExtension into a corev1.Affinity,
// REJECTING fields the type does not have.
//
// This file is only about READING that field. How it folds across the role and role group levels is
// a property of the config merge and lives with the other merge rules in role_group_handler.go:
// affinity is replaced wholesale, so there is no merge helper to keep alongside this one.
//
// The CRD declares this field `type: object` with `x-kubernetes-preserve-unknown-fields: true`, so
// the API server neither validates nor prunes it — and a plain json.Unmarshal ignores what it does
// not recognise. The two together made a typo completely silent: `nodeAffinty` (missing an `i`)
// passed admission, decoded into an empty Affinity, and the pods were scheduled anywhere at all.
// Affinity is a scheduling *contract* for these products — rack awareness, spreading a quorum
// across failure domains, keeping a worker near its data — so losing it is not a cosmetic problem,
// and it produced no event, no log line and no status change.
//
// A strict decode turns that into a build failure naming the field. The trade-off is deliberate: a
// field that exists in a newer Kubernetes API than the one this SDK is built against is now
// rejected rather than ignored. That is the honest answer — the framework cannot honor a field it
// does not know — and it fails where someone can read it instead of at 3am when a node drains.
func DecodeAffinity(raw *k8sruntime.RawExtension) (*corev1.Affinity, error) {
	if raw == nil || len(raw.Raw) == 0 {
		return nil, nil
	}
	// json.Decoder reads ONE value and leaves the rest of the stream untouched, so
	// `{"nodeAffinity":{...}} {"podAffinity":{...}}` decodes the first object and drops the second
	// without complaint — the same silent partial loss this function exists to prevent, arriving
	// through the decoder rather than through an unknown field. json.Valid is the cheap way to
	// require exactly one value first — json.Unmarshal would have rejected it, json.Decoder does not.
	//
	// A value that reached here through the API server is always a single re-serialized object, so
	// this guards a RawExtension built in Go: a product's webhook defaulter, or product code
	// assembling a config by hand.
	if !json.Valid(raw.Raw) {
		return nil, fmt.Errorf("affinity is not a single valid JSON value")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw.Raw))
	decoder.DisallowUnknownFields()

	affinity := &corev1.Affinity{}
	if err := decoder.Decode(affinity); err != nil {
		return nil, err
	}
	return affinity, nil
}

// EncodeAffinity is the inverse of DecodeAffinity: it renders an affinity into the schema-free
// RawExtension the CRD field holds, so a product can supply one as a role config default
// (RoleDeclaration.ConfigDefaults) using the typed Kubernetes API rather than a JSON literal. A nil affinity encodes to nil, which the merge reads as "no opinion".
//
// Three Gen 3 migrations wrote this by hand, each with its own comment explaining that the marshal
// cannot realistically fail. It can, in exactly one way — a value the Kubernetes API itself cannot
// serialize — so the error is returned rather than dropped, and the caller decides.
func EncodeAffinity(affinity *corev1.Affinity) (*k8sruntime.RawExtension, error) {
	if affinity == nil {
		return nil, nil
	}
	raw, err := json.Marshal(affinity)
	if err != nil {
		return nil, fmt.Errorf("encoding affinity: %w", err)
	}
	return &k8sruntime.RawExtension{Raw: raw}, nil
}

// TopologyKeyHostname spreads across nodes. It is the topology key every one of these products
// wants by default, and is named here so a product does not re-type the string.
const TopologyKeyHostname = "kubernetes.io/hostname"

// ClusterSelectorLabels is the framework's identity selector for every pod of one cluster
// (`app.kubernetes.io/instance`).
//
// Exported so an affinity term matches what the framework actually stamps: those keys are
// framework-owned, and a downstream operator re-typing them as string literals gets a selector
// matching nothing the moment they change — a silent scheduling failure, since a PREFERRED term
// that matches no pod is not an error.
func ClusterSelectorLabels(clusterName string) map[string]string {
	return map[string]string{constant.LabelKubernetesInstance: clusterName}
}

// RoleSelectorLabels is the same for one ROLE (instance + component).
//
// It selects on the role, not the role group: two groups of the same role are the same failure
// domain problem — three of five ZooKeeper servers on one node is an outage whether or not they
// share a group — and the role group's own marker key is not unique across roles anyway.
func RoleSelectorLabels(clusterName, roleName string) map[string]string {
	return map[string]string{
		constant.LabelKubernetesInstance:  clusterName,
		constant.LabelKubernetesComponent: roleName,
	}
}

// PreferredAffinityTerm builds one weighted term for a pod (anti-)affinity's
// PreferredDuringSchedulingIgnoredDuringExecution list.
//
// It replaces a single-shot DefaultAntiAffinity helper, which could emit exactly ONE anti-affinity
// term and so could not express the composite two of these products actually want: a cluster-wide
// pod affinity at one weight PLUS a role anti-affinity at another. Both hand-wrote it instead, and
// both got it wrong — one commented the merge out entirely, the other applied its default
// unconditionally and silently discarded the user's own `config.affinity`.
//
// PREFERRED only, and there is deliberately no Required constructor: a required spread turns a
// too-small cluster into pods that never schedule at all (three nodes, five replicas, two Pending
// forever), with no way to say "spread as far as you can". A product that genuinely requires it
// writes the corev1 term itself.
//
// `weight` and `topologyKey` are parameters with no defaults: the operators that hand-wrote this
// already disagree on the weight (20 and 70 both appear), and spreading across zones is a different
// guarantee from spreading across nodes.
func PreferredAffinityTerm(weight int32, topologyKey string, selector map[string]string) corev1.WeightedPodAffinityTerm {
	matchLabels := make(map[string]string, len(selector))
	maps.Copy(matchLabels, selector)
	return corev1.WeightedPodAffinityTerm{
		Weight: weight,
		PodAffinityTerm: corev1.PodAffinityTerm{
			LabelSelector: &metav1.LabelSelector{MatchLabels: matchLabels},
			TopologyKey:   topologyKey,
		},
	}
}

// NormalizeAffinity drops members that carry no term and returns nil when none is left, so a member
// a user cleared with `{}` renders byte-identically to one that never existed.
//
// This is what makes per-member folding free of churn. Without it a cleared member reaches the pod
// template as an empty struct, which differs from absent in the serialized spec and therefore shows
// up as a diff in the apply path on every single reconcile.
//
// Exported so a product assembling an affinity by hand produces the same bytes the fold does.
func NormalizeAffinity(a *corev1.Affinity) *corev1.Affinity {
	if a == nil {
		return nil
	}
	out := a.DeepCopy()

	if out.NodeAffinity != nil &&
		out.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil &&
		len(out.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution) == 0 {
		out.NodeAffinity = nil
	}
	if out.PodAffinity != nil &&
		len(out.PodAffinity.RequiredDuringSchedulingIgnoredDuringExecution) == 0 &&
		len(out.PodAffinity.PreferredDuringSchedulingIgnoredDuringExecution) == 0 {
		out.PodAffinity = nil
	}
	if out.PodAntiAffinity != nil &&
		len(out.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution) == 0 &&
		len(out.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution) == 0 {
		out.PodAntiAffinity = nil
	}

	if out.NodeAffinity == nil && out.PodAffinity == nil && out.PodAntiAffinity == nil {
		return nil
	}
	return out
}
