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

	corev1 "k8s.io/api/core/v1"
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
