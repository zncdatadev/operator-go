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
	"fmt"
	"reflect"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"

	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/productlogging"
)

// A role group's `config` block is resolved in two halves, by two owners.
//
// commons defines RoleGroupConfigSpec — resources, affinity, gracefulShutdownTimeout, logging — and
// a product extends it by COMPOSITION, embedding it inline in its own ConfigSpec and adding the
// fields specific to it (zookeeper's tickTime, hdfs's listenerClass, hive's warehouseDir). That
// composition IS the framework's extension point, and it draws the ownership line:
//
//   - the FRAMEWORK resolves the commons half generically, off RoleSpec.Config and
//     RoleGroupSpec.Config, because it defined those fields and knows what each one means;
//   - the PRODUCT resolves its own increment, because only it knows that `tickTime` belongs on a
//     particular line of zoo.cfg. The framework could read that field and still have nothing to do
//     with it.
//
// What the framework owes the product is therefore not the parse — it is the RULES. Nine downstream
// operators hand-wrote a config merge and five are wrong, two of them applying the product's value
// ON TOP of the user's and silently discarding what the user asked for. Those are not typos; they
// are what happens when a rule with no single implementation gets restated nine times. So both
// halves fold through machinery declared here: FoldCommonConfig for the half the framework owns,
// FoldProductConfig for the half the product owns.
//
// Neither half requires the framework to know the product's type, which is why nothing in this file
// is parameterised over one.

// ConfigFoldTag is the struct tag key FoldProductConfig reads on a product's config fields.
const ConfigFoldTag = "kubedoop"

// ConfigFoldAtomic is the only tag value: `kubedoop:"atomic"` opts a struct or pointer-to-struct
// field into wholesale replacement, which ValidateProductConfigType otherwise refuses.
const ConfigFoldAtomic = "atomic"

// commonConfigType is the framework-owned half's type. FoldProductConfig SKIPS a field of this type
// rather than folding it: that half is resolved by the framework and published on the build context,
// and folding it here as well would compute one answer in two places.
var commonConfigType = reflect.TypeFor[*v1alpha1.RoleGroupConfigSpec]()

// opaqueLeafTypes are struct types the fold treats as single values rather than composites.
//
// POINTERS to these are allowed; the bare forms are not, and that is the same rule that rejects a
// bare scalar rather than an exception to it. Each has a legal, statable zero — a zero Quantity, a
// zero Duration, the zero Time — so a bare field cannot distinguish "stated zero" from "absent".
// The repository already litigated this for resource.Quantity and came down the same way: the
// commons CPU/memory/storage fields are pointers because a bare Quantity is a struct that
// `omitempty` cannot omit and whose MarshalJSON renders the zero value as "0", so a Go-constructed
// spec would transmit an explicit zero and a role group would silently erase the role's value.
//
// reflect.Value.IsZero is not even a stable predicate on a bare Quantity: resource.MustParse("0")
// populates the cached format string and reads non-zero, while the same value produced by
// arithmetic has an empty cache and reads zero.
var opaqueLeafTypes = map[reflect.Type]struct{}{
	reflect.TypeFor[resource.Quantity]():       {},
	reflect.TypeFor[metav1.Duration]():         {},
	reflect.TypeFor[metav1.Time]():             {},
	reflect.TypeFor[metav1.MicroTime]():        {},
	reflect.TypeFor[k8sruntime.RawExtension](): {},
}

// FoldCommonConfig folds the FRAMEWORK-owned half of the config block, lowest-precedence first —
// the product's role default, then the CR's role level, then its role group level. It is the only
// fold of that half on the reconcile path.
//
// Inputs are never mutated; the result is a fresh, fully deep-copied object with no field aliasing
// any input, so a resolved value can never write back into the live CR in the informer cache.
//
//   - resources folds per LEAF. Overriding one knob (cpu.max, a storageClass) and keeping its
//     siblings is the normal way to use this API, and a struct-level fold silently dropped the rest.
//   - affinity folds per MEMBER, and an empty value CLEARS. Each layer is decoded STRICTLY
//     (DecodeAffinity rejects an unknown field, so `nodeAffinty` is a build failure rather than pods
//     scheduled anywhere), then nodeAffinity / podAffinity / podAntiAffinity are resolved
//     independently. `affinity: {}` clears all three — the single-node development escape hatch —
//     and `affinity: {podAntiAffinity: {}}` clears one while keeping a sibling the product set. The
//     result is normalized, so a cleared member renders byte-identically to one that never existed
//     and produces no churn in the apply path's diff.
//   - gracefulShutdownTimeout is atomic: a non-nil upper value wins.
//   - logging folds through productlogging.MergeLoggingSpec — on THIS path and no other.
//
// CLEARING IS PER FIELD, and the rule follows the SCHEMA rather than taste.
//
// `affinity` is `x-kubernetes-preserve-unknown-fields`, so the API server never prunes inside it: a
// stored `{}` is always something a user wrote, and treating it as "no affinity" is honest.
//
// `resources` and its leaves are structural, so the API server PRUNES what it does not recognise —
// a mistyped `resources.cpu.maxx: "4"` is stored as `cpu: {}`. Reading that as a clear would let a
// typo silently delete the product's CPU policy, so an empty value there states nothing and
// inherits. `storage: {}` inherits for the same reason, which is also what lets a product use an
// empty storage block as its "this role has a data volume" marker.
//
// Per-member folding matters now in a way it did not before, and that is why an earlier attempt at
// it was reverted rather than wrong. With only the CR's two levels, both written by the same person
// in the same file, wholesale replacement is the simpler thing to understand. With a product default
// underneath them, written by someone else entirely, replacement means a user adding a nodeAffinity
// silently deletes the anti-affinity the product ships to spread a quorum — with no event and no
// log line.
//
// Framework floors are NOT applied here: DefaultGracefulShutdownTimeout and DefaultStorageCapacity
// stay at consumption time behind GetGracefulShutdownTimeout() / GetCapacity(), below the fold's
// bottom layer, so they fire only when nobody stated anything at all.
func FoldCommonConfig(layers ...*v1alpha1.RoleGroupConfigSpec) (*v1alpha1.RoleGroupConfigSpec, error) {
	out := &v1alpha1.RoleGroupConfigSpec{}
	var affinity *corev1.Affinity

	for _, layer := range layers {
		if layer == nil {
			continue
		}

		if layer.GracefulShutdownTimeout != nil {
			out.GracefulShutdownTimeout = copyString(layer.GracefulShutdownTimeout)
		}
		out.Resources = foldResources(out.Resources, layer.Resources)
		out.Logging = productlogging.MergeLoggingSpec(out.Logging, layer.Logging).DeepCopy()

		decoded, err := DecodeAffinity(layer.Affinity)
		if err != nil {
			return nil, err
		}
		affinity = foldAffinityMembers(affinity, decoded)
	}

	encoded, err := EncodeAffinity(NormalizeAffinity(affinity))
	if err != nil {
		return nil, err
	}
	out.Affinity = encoded
	return out, nil
}

// foldAffinityMembers resolves nodeAffinity / podAffinity / podAntiAffinity independently. A member
// the upper layer did not state inherits; a member it stated — including as an empty struct —
// replaces. An upper layer that states the whole affinity as empty clears all three.
//
// The two readings are one rule: an empty value at a level means "nothing at this level". `affinity:
// {}` is no affinity, `affinity: {podAntiAffinity: {}}` is no pod anti-affinity while a sibling the
// product set survives — which wholesale replacement could not express, so dropping a spreading rule
// also nuked a rack-awareness nodeAffinity declared beside it.
//
// Clearing works HERE and not in `resources` because the schemas differ, and the difference is
// Kubernetes' rather than ours: `affinity` is `x-kubernetes-preserve-unknown-fields`, so the API
// server never prunes inside it and a stored `{}` is always something the user wrote. `resources` is
// structural, so a mistyped `resources.cpu.maxx` is PRUNED down to a stored `cpu: {}` — treating
// that as a clear would let a typo silently delete the product's CPU policy.
func foldAffinityMembers(lower, upper *corev1.Affinity) *corev1.Affinity {
	if upper == nil {
		return lower
	}
	if upper.NodeAffinity == nil && upper.PodAffinity == nil && upper.PodAntiAffinity == nil {
		return nil
	}
	out := &corev1.Affinity{}
	if lower != nil {
		out = lower.DeepCopy()
	}
	if upper.NodeAffinity != nil {
		out.NodeAffinity = upper.NodeAffinity.DeepCopy()
	}
	if upper.PodAffinity != nil {
		out.PodAffinity = upper.PodAffinity.DeepCopy()
	}
	if upper.PodAntiAffinity != nil {
		out.PodAntiAffinity = upper.PodAntiAffinity.DeepCopy()
	}
	return out
}

// foldResources folds the upper layer's resources into the lower's, leaf by leaf.
func foldResources(lower, upper *v1alpha1.ResourcesSpec) *v1alpha1.ResourcesSpec {
	switch {
	case lower == nil && upper == nil:
		return nil
	case lower == nil:
		return upper.DeepCopy()
	case upper == nil:
		return lower.DeepCopy()
	}

	merged := upper.DeepCopy()

	switch {
	case merged.CPU == nil:
		merged.CPU = lower.CPU.DeepCopy()
	case lower.CPU != nil:
		if merged.CPU.Min == nil {
			merged.CPU.Min = copyQuantity(lower.CPU.Min)
		}
		if merged.CPU.Max == nil {
			merged.CPU.Max = copyQuantity(lower.CPU.Max)
		}
	}

	switch {
	case merged.Memory == nil:
		merged.Memory = lower.Memory.DeepCopy()
	case lower.Memory != nil:
		if merged.Memory.Limit == nil {
			merged.Memory.Limit = copyQuantity(lower.Memory.Limit)
		}
	}

	switch {
	case merged.Storage == nil:
		merged.Storage = lower.Storage.DeepCopy()
	case lower.Storage != nil:
		if merged.Storage.Capacity == nil {
			merged.Storage.Capacity = copyQuantity(lower.Storage.Capacity)
		}
		if merged.Storage.StorageClass == nil {
			merged.Storage.StorageClass = copyString(lower.Storage.StorageClass)
		}
	}

	return merged
}

// FoldProductConfig folds the PRODUCT-owned half: the fields a product added to its own ConfigSpec
// alongside the embedded commons block. Layers are lowest-precedence first — the product's own role
// default, the CR's role level, its role group level.
//
//	cfg, err := reconciler.FoldProductConfig(defaults, roleCfg, groupCfg) // *v1alpha1.ConfigSpec
//	if err != nil { return nil, err }
//	tickTime := *cfg.TickTime   // folded, with the user's value winning at every level
//
// The rule is PRESENCE-WINS per field: the upper layer wins iff its value is not the zero value,
// otherwise the lower layer's is inherited, deep-copied. No depth, no vocabulary to get wrong.
// Every product config field surveyed across twelve operators is a scalar, so a merge ENGINE would
// be machinery for a case that does not exist; ValidateProductConfigType refuses the shapes this
// rule cannot handle honestly rather than guessing at them.
//
// It SKIPS an embedded *v1alpha1.RoleGroupConfigSpec. That half is the framework's, resolved by
// FoldCommonConfig and published on the build context; folding it here as well would compute one
// answer in two places, which is the defect that made the previous seam unreadable. The returned
// value's commons field is therefore nil — read the effective commons config from the build context.
//
// Nil layers are skipped, and the result is never nil, so a caller reads a field without a
// preliminary nil check.
func FoldProductConfig[T any](layers ...*T) (*T, error) {
	if err := ValidateProductConfigType[T](); err != nil {
		return nil, err
	}

	out := new(T)
	outElem := reflect.ValueOf(out).Elem()

	present := make([]*T, 0, len(layers))
	for _, l := range layers {
		if l != nil {
			present = append(present, l)
		}
	}

	for i := range outElem.NumField() {
		field := outElem.Type().Field(i)
		if !field.IsExported() || field.Type == commonConfigType {
			continue
		}
		// Highest layer first: the first layer that stated anything owns the field.
		for j := len(present) - 1; j >= 0; j-- {
			v := reflect.ValueOf(present[j]).Elem().Field(i)
			if v.IsZero() {
				continue
			}
			outElem.Field(i).Set(deepCopyValue(v))
			break
		}
	}

	return out, nil
}

// deepCopyValue copies a reflected field so the fold's result never aliases a layer — which, for
// the CR's own two levels, means never aliasing the live object in the informer cache.
func deepCopyValue(v reflect.Value) reflect.Value {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return v
		}
		out := reflect.New(v.Type().Elem())
		out.Elem().Set(deepCopyValue(v.Elem()))
		return out
	case reflect.Slice:
		if v.IsNil() {
			return v
		}
		out := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		for i := range v.Len() {
			out.Index(i).Set(deepCopyValue(v.Index(i)))
		}
		return out
	case reflect.Map:
		if v.IsNil() {
			return v
		}
		out := reflect.MakeMapWithSize(v.Type(), v.Len())
		iter := v.MapRange()
		for iter.Next() {
			out.SetMapIndex(iter.Key(), deepCopyValue(iter.Value()))
		}
		return out
	default:
		return v
	}
}

// ValidateProductConfigType reports whether T is a shape FoldProductConfig can fold honestly.
//
// Call it once at operator start-up, next to where the reconciler is built, so a shape the fold
// cannot handle stops the process on the author's laptop instead of half-folding in production.
// FoldProductConfig calls it too, so a caller that skips the start-up check still gets an error
// rather than a wrong answer.
//
//	V1  T is not a struct.
//	V2  An unexported field: unreadable and unsettable by reflection, and invisible to the API
//	    server, so it can never carry user intent.
//	V3  A field of kind Func, Chan, UnsafePointer or Interface: no honest zero test, no safe copy.
//	V4  A bare (non-pointer) scalar. The fold's "was this stated?" test is the zero value, and a
//	    bare scalar cannot distinguish `tickTime: 0` from an absent tickTime. Make it a pointer.
//	V5  A struct or pointer-to-struct field other than the embedded commons block, unless it is a
//	    POINTER to an allowlisted opaque leaf or carries `kubedoop:"atomic"`. Replacing a composite
//	    wholesale drops the sibling fields a user did not restate — the defect this fold exists to
//	    prevent — and deep-folding a product struct needs rules the framework cannot infer.
//
// An embedded *v1alpha1.RoleGroupConfigSpec is always accepted and always skipped by the fold: it
// is the framework's half.
func ValidateProductConfigType[T any]() error {
	t := reflect.TypeFor[T]()
	if t.Kind() != reflect.Struct {
		return fmt.Errorf("product config type %s must be a struct", t)
	}

	var problems []string
	for i := range t.NumField() {
		f := t.Field(i)

		if f.Type == commonConfigType {
			continue // the framework's half
		}
		if !f.IsExported() {
			problems = append(problems, fmt.Sprintf(
				"field %s is unexported: the fold cannot read it and the API server never sees it", f.Name))
			continue
		}

		switch f.Type.Kind() {
		case reflect.Func, reflect.Chan, reflect.UnsafePointer, reflect.Interface:
			problems = append(problems, fmt.Sprintf(
				"field %s has kind %s: no honest zero test and no safe copy", f.Name, f.Type.Kind()))
		case reflect.Bool, reflect.String,
			reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
			reflect.Float32, reflect.Float64:
			problems = append(problems, fmt.Sprintf(
				"field %s is a bare %s: the fold cannot tell a stated zero from an absent value, make it *%s",
				f.Name, f.Type, f.Type))
		case reflect.Struct:
			if _, ok := opaqueLeafTypes[f.Type]; ok {
				problems = append(problems, fmt.Sprintf(
					"field %s is a bare %s: it has a statable zero, make it *%s", f.Name, f.Type, f.Type))
				continue
			}
			if !hasAtomicTag(f) {
				problems = append(problems, compositeProblem(f))
			}
		case reflect.Pointer:
			if f.Type.Elem().Kind() != reflect.Struct {
				continue
			}
			if _, ok := opaqueLeafTypes[f.Type.Elem()]; ok {
				continue
			}
			if !hasAtomicTag(f) {
				problems = append(problems, compositeProblem(f))
			}
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("product config type %s cannot be folded:\n  - %s",
			t, strings.Join(problems, "\n  - "))
	}
	return nil
}

func hasAtomicTag(f reflect.StructField) bool {
	return f.Tag.Get(ConfigFoldTag) == ConfigFoldAtomic
}

func compositeProblem(f reflect.StructField) string {
	return fmt.Sprintf(
		"field %s is a composite (%s): the fold would replace it wholesale and drop the sibling "+
			"fields a user did not restate. Flatten it into scalar pointers, or tag it "+
			"`%s:%q` to accept wholesale replacement",
		f.Name, f.Type, ConfigFoldTag, ConfigFoldAtomic)
}
