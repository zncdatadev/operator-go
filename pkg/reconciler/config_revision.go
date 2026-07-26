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
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"maps"
	"slices"

	"github.com/zncdatadev/operator-go/pkg/constant"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// AnnotationConfigRevision carries a digest of the role group ConfigMap's rendered contents on the
// StatefulSet's POD TEMPLATE. Changing a pod template annotation is what makes the StatefulSet
// controller roll the pods, which is the whole point: without it a configuration change updated
// the ConfigMap and stopped there.
//
// A mounted ConfigMap volume does eventually refresh the files on disk (kubelet sync, ~1 minute,
// and only without subPath), but none of the products this SDK targets re-read their configuration
// at runtime — they need the process restarted. So the observable behaviour was: the user edits
// configOverrides, the operator reports ReconcileComplete=True, and the cluster keeps serving the
// old configuration indefinitely. Worse, the on-disk files DO change, so the next unrelated pod
// restart silently splits the role group across two configurations with nothing in the API
// recording it.
const AnnotationConfigRevision = "config." + constant.KubedoopDomain + "/revision"

// ConfigRevisionPolicy selects whether the framework stamps AnnotationConfigRevision.
type ConfigRevisionPolicy string

const (
	// ConfigRevisionDisabled leaves the pod template untouched. This is the default for now
	// because turning the stamp on rolls every pod of every managed cluster exactly once, as the
	// annotation appears for the first time — an operational event that has to be scheduled, not
	// inherited from an operator upgrade.
	ConfigRevisionDisabled ConfigRevisionPolicy = ""

	// ConfigRevisionEnabled stamps the digest, so a configuration change restarts the pods that
	// consume it.
	ConfigRevisionEnabled ConfigRevisionPolicy = "Enabled"
)

// configRevision returns a stable digest of a ConfigMap's rendered contents, or "" when there is
// nothing to hash.
//
// Determinism is the load-bearing property, not collision resistance: the value is recomputed on
// every reconcile from a freshly built ConfigMap, so any dependence on Go's map iteration order
// would produce a different digest each pass and roll the pods forever. Keys are therefore sorted,
// and each key and value is length-prefixed so that {"ab":"c"} and {"a":"bc"} cannot collide by
// concatenation.
func configRevision(cm *corev1.ConfigMap) string {
	if cm == nil || (len(cm.Data) == 0 && len(cm.BinaryData) == 0) {
		return ""
	}

	h := sha256.New()
	writeField := func(b []byte) {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(b)))
		h.Write(length[:])
		h.Write(b)
	}

	for _, k := range slices.Sorted(maps.Keys(cm.Data)) {
		writeField([]byte(k))
		writeField([]byte(cm.Data[k]))
	}
	// BinaryData is hashed in its own pass, after a separator, so a key moving between the two
	// maps changes the digest rather than colliding with its former self.
	writeField([]byte("\x00binaryData"))
	for _, k := range slices.Sorted(maps.Keys(cm.BinaryData)) {
		writeField([]byte(k))
		writeField(cm.BinaryData[k])
	}

	return hex.EncodeToString(h.Sum(nil))
}

// stampConfigRevision writes the digest onto the StatefulSet's pod template annotations. It is a
// no-op when either object is absent or the ConfigMap renders to nothing.
func stampConfigRevision(sts *appsv1.StatefulSet, cm *corev1.ConfigMap) {
	if sts == nil {
		return
	}
	revision := configRevision(cm)
	if revision == "" {
		return
	}
	if sts.Spec.Template.Annotations == nil {
		sts.Spec.Template.Annotations = map[string]string{}
	}
	sts.Spec.Template.Annotations[AnnotationConfigRevision] = revision
}
