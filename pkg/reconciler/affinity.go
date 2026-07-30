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
	"slices"

	corev1 "k8s.io/api/core/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
)

// affinityFields are the top-level members of corev1.Affinity. mergeAffinity folds a role's
// affinity into a role group's one member at a time, so the list has to be the complete set: a
// member missing from here would silently stop being inheritable.
var affinityFields = []string{"nodeAffinity", "podAffinity", "podAntiAffinity"}

// DecodeAffinity decodes the schema-free `config.affinity` RawExtension into a corev1.Affinity,
// REJECTING fields the type does not have.
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
	// require exactly one value first. It also keeps this function consistent with affinityMembers,
	// which uses json.Unmarshal and rejects trailing data already.
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

// mergeAffinity folds a role's affinity into a role group's, one top-level member at a time: the
// group wins for a member it declares, and inherits the role's for a member it does not.
//
// The previous rule was all-or-nothing — any affinity at the group level discarded the role's
// entirely — which is the exact behavior MergeRoleGroupConfig's own doc comment identifies as a bug
// for `resources`: "Overriding one knob is the normal way to use this API, and it silently dropped
// the rest." Affinity had the same shape and is where it hurts most: a role that spreads its pods
// with podAntiAffinity, plus a group that adds a nodeAffinity to pin itself to an instance type,
// silently lost the spreading and put the whole group on one node.
//
// Merging stops at the top level on purpose. Below it sit `requiredDuringSchedulingIgnoredDuringExecution`
// and its preferred counterpart, whose values are complete scheduling statements — a nodeSelectorTerm
// list is an OR of ANDs, and interleaving two of them produces a constraint neither author wrote.
// Choosing per member is the finest granularity where the result still means what both sides said.
//
// Neither input is mutated and the result shares no memory with them.
func mergeAffinity(role, group *k8sruntime.RawExtension) *k8sruntime.RawExtension {
	roleFields, roleOK := affinityMembers(role)
	groupFields, groupOK := affinityMembers(group)

	// Anything that is not a JSON object cannot be merged member-wise. Rather than guess, keep the
	// old precedence (group wins whole) and let DecodeAffinity fail the build with the real reason —
	// the same malformed value reaches it a moment later.
	if !roleOK || !groupOK {
		if group != nil {
			return group.DeepCopy()
		}
		return role.DeepCopy()
	}

	merged := make(map[string]json.RawMessage, len(affinityFields))
	for _, field := range affinityFields {
		if value, ok := groupFields[field]; ok {
			merged[field] = value
			continue
		}
		if value, ok := roleFields[field]; ok {
			merged[field] = value
		}
	}
	// Members outside the known set are preserved so the strict decode still gets to reject them
	// with a message naming the field, instead of this merge quietly deleting the evidence.
	for _, fields := range []map[string]json.RawMessage{roleFields, groupFields} {
		for field, value := range fields {
			if _, known := merged[field]; known {
				continue
			}
			if !isAffinityField(field) {
				merged[field] = value
			}
		}
	}

	if len(merged) == 0 {
		return nil
	}
	encoded, err := json.Marshal(merged)
	if err != nil {
		// A map of already-valid json.RawMessage values cannot fail to marshal; keep the group's
		// value rather than dropping the user's configuration on an impossible path.
		if group != nil {
			return group.DeepCopy()
		}
		return role.DeepCopy()
	}
	return &k8sruntime.RawExtension{Raw: encoded}
}

// affinityMembers splits a RawExtension into its top-level JSON members. The bool reports whether
// the value was a JSON object at all; an absent value is an empty object, which merges cleanly.
func affinityMembers(raw *k8sruntime.RawExtension) (map[string]json.RawMessage, bool) {
	if raw == nil || len(raw.Raw) == 0 {
		return map[string]json.RawMessage{}, true
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(raw.Raw, &members); err != nil {
		return nil, false
	}
	if members == nil {
		// Explicit JSON null.
		return map[string]json.RawMessage{}, true
	}
	return members, true
}

// isAffinityField reports whether name is a known top-level member of corev1.Affinity.
func isAffinityField(name string) bool {
	return slices.Contains(affinityFields, name)
}

// affinityFieldsAreComplete fails the build if corev1.Affinity grows a member affinityFields does
// not list, which would silently stop that member from being inheritable from the role level.
func affinityFieldsAreComplete() error {
	encoded, err := json.Marshal(corev1.Affinity{
		NodeAffinity:    &corev1.NodeAffinity{},
		PodAffinity:     &corev1.PodAffinity{},
		PodAntiAffinity: &corev1.PodAntiAffinity{},
	})
	if err != nil {
		return err
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &members); err != nil {
		return err
	}
	for name := range members {
		if !isAffinityField(name) {
			return fmt.Errorf("corev1.Affinity has member %q that affinityFields does not list, "+
				"so a role-level value for it would not be inherited by its role groups", name)
		}
	}
	if len(members) != len(affinityFields) {
		return fmt.Errorf("affinityFields lists %d members but corev1.Affinity marshals %d",
			len(affinityFields), len(members))
	}
	return nil
}
