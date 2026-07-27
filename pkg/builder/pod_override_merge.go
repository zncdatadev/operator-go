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

package builder

import (
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
)

// strategicMergePodTemplate merges the override pod template onto the base using the same
// strategic-merge-patch machinery Kubernetes applies to pod templates. The base must already have
// been passed through clearSupersededUnions.
func strategicMergePodTemplate(base, override *corev1.PodTemplateSpec) (*corev1.PodTemplateSpec, error) {
	baseBytes, err := json.Marshal(base)
	if err != nil {
		return nil, err
	}
	overrideBytes, err := json.Marshal(override)
	if err != nil {
		return nil, err
	}
	patchMeta, err := strategicpatch.NewPatchMetaFromStruct(corev1.PodTemplateSpec{})
	if err != nil {
		return nil, err
	}
	mergedBytes, err := strategicpatch.StrategicMergePatchUsingLookupPatchMeta(baseBytes, overrideBytes, patchMeta)
	if err != nil {
		return nil, err
	}
	var merged corev1.PodTemplateSpec
	if err := json.Unmarshal(mergedBytes, &merged); err != nil {
		return nil, err
	}
	return &merged, nil
}

// clearSupersededUnions removes from base every member of a Kubernetes "one of" struct that the
// override supersedes by declaring a different member of the same group.
//
// A strategic merge patch deep-merges those structs member by member, so a podOverrides entry
// naming a framework-owned volume (e.g. "config") with a different source type merges into one
// volume carrying two sources, which the API server rejects outright ("may not specify more than 1
// volume type") — the whole role group then fails to apply with an opaque error. PodSpec.Volumes
// is declared patchStrategy:"merge,retainKeys" exactly so the patch author's volume replaces the
// stored one, but a patch derived from a typed PodTemplateSpec carries no $retainKeys directive.
// Clearing the superseded base members reproduces the retainKeys outcome: the override's volume
// replaces the framework's wholesale, at its original position, while an override that names the
// same source type still merges field by field (e.g. adding a sizeLimit to an emptyDir).
//
// The same one-of shape occurs in the other pod template fields the framework populates by
// default — EnvVar (value vs valueFrom), probe handlers and lifecycle handlers — and is resolved
// the same way, so no override can produce an object the API server refuses.
func clearSupersededUnions(base, override *corev1.PodTemplateSpec) {
	for i := range base.Spec.Volumes {
		ov, ok := findByKey(override.Spec.Volumes, volumeName, base.Spec.Volumes[i].Name)
		if !ok {
			continue
		}
		if supersedes(base.Spec.Volumes[i].VolumeSource, ov.VolumeSource) {
			base.Spec.Volumes[i].VolumeSource = corev1.VolumeSource{}
		}
	}

	clearSupersededContainerUnions(base.Spec.Containers, override.Spec.Containers)
	clearSupersededContainerUnions(base.Spec.InitContainers, override.Spec.InitContainers)
}

// clearSupersededContainerUnions resolves the one-of collisions inside every base container the
// override addresses by name.
func clearSupersededContainerUnions(base, override []corev1.Container) {
	for i := range base {
		oc, ok := findByKey(override, containerName, base[i].Name)
		if !ok {
			continue
		}
		clearSupersededEnvUnions(base[i].Env, oc.Env)
		clearSupersededProbeUnion(base[i].LivenessProbe, oc.LivenessProbe)
		clearSupersededProbeUnion(base[i].ReadinessProbe, oc.ReadinessProbe)
		clearSupersededProbeUnion(base[i].StartupProbe, oc.StartupProbe)
		if base[i].Lifecycle != nil && oc.Lifecycle != nil {
			clearSupersededLifecycleUnion(base[i].Lifecycle.PostStart, oc.Lifecycle.PostStart)
			clearSupersededLifecycleUnion(base[i].Lifecycle.PreStop, oc.Lifecycle.PreStop)
		}
	}
}

// clearSupersededEnvUnions resolves EnvVar's value/valueFrom exclusion. Value is a plain string
// rather than a pointer member, so this group is compared by hand instead of through supersedes.
func clearSupersededEnvUnions(base, override []corev1.EnvVar) {
	for i := range base {
		oe, ok := findByKey(override, envVarName, base[i].Name)
		if !ok {
			continue
		}
		if oe.ValueFrom != nil && base[i].Value != "" {
			base[i].Value = ""
		}
		if oe.Value != "" && base[i].ValueFrom != nil {
			base[i].ValueFrom = nil
		}
	}
}

// clearSupersededProbeUnion clears the base handler when the override states another one, leaving
// the probe's timing fields to merge as usual.
func clearSupersededProbeUnion(base, override *corev1.Probe) {
	if base == nil || override == nil {
		return
	}
	if supersedes(base.ProbeHandler, override.ProbeHandler) {
		base.ProbeHandler = corev1.ProbeHandler{}
	}
}

// clearSupersededLifecycleUnion clears the base handler when the override states another one.
func clearSupersededLifecycleUnion(base, override *corev1.LifecycleHandler) {
	if base == nil || override == nil {
		return
	}
	if supersedes(*base, *override) {
		*base = corev1.LifecycleHandler{}
	}
}

// supersedes reports whether the override declares a member the base does not, in which case
// merging the two would leave more than one member of a mutually exclusive group set.
func supersedes[T any](base, override T) bool {
	baseMembers := declaredMembers(base)
	if baseMembers.Len() == 0 {
		return false
	}
	return declaredMembers(override).Difference(baseMembers).Len() > 0
}

// declaredMembers returns the JSON field names v sets. Every member of a Kubernetes one-of struct
// is an omitempty pointer, so the marshalled object holds exactly the members v declares.
func declaredMembers[T any](v T) sets.Set[string] {
	raw, err := json.Marshal(v)
	if err != nil {
		// Pod template types are plain data and cannot fail to marshal; reporting no members on a
		// hypothetical failure leaves the merge exactly as it would have been.
		return nil
	}
	members := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &members); err != nil {
		return nil
	}
	return sets.KeySet(members)
}

// findByKey returns the first element of items whose strategic-merge key equals want.
func findByKey[T any](items []T, key func(T) string, want string) (*T, bool) {
	for i := range items {
		if key(items[i]) == want {
			return &items[i], true
		}
	}
	return nil, false
}

func volumeName(v corev1.Volume) string       { return v.Name }
func containerName(c corev1.Container) string { return c.Name }
func envVarName(e corev1.EnvVar) string       { return e.Name }

// checkPodOverrideMountInvariants reports the framework-owned volume mounts a podOverrides layer
// displaced, and any mount left pointing at a volume that does not exist.
//
// Strategic merge patch keys volumeMounts by **mountPath**, not by name. A user adding a mount at
// a path the framework already owns therefore does not append a second mount — it REWRITES the
// framework's entry to point at a different volume. Both outcomes are invisible in the override
// the user actually wrote:
//
//   - the override also declares that volume: the pod spec stays perfectly valid and the API
//     server accepts it, while the framework's ConfigMap (or CSI secret, or shared log volume) is
//     no longer mounted anywhere. The product starts with an empty config directory and
//     crash-loops, or silently runs on its built-in defaults. Nothing anywhere reports a problem;
//   - the override declares only the mount: the API server rejects the StatefulSet with
//     `volumeMounts[i].name: Not found`, naming a field the user never wrote, with no mention of
//     podOverrides.
//
// Only the framework knows which volume was supposed to be at that path, so only the framework
// can say what went wrong. Both cases are reported against the pre-merge template.
func checkPodOverrideMountInvariants(
	base, merged *corev1.PodTemplateSpec,
	claims []corev1.PersistentVolumeClaim,
) []error {
	declared := make(map[string]struct{}, len(merged.Spec.Volumes)+len(claims))
	for _, v := range merged.Spec.Volumes {
		declared[v.Name] = struct{}{}
	}
	// A StatefulSet's volumeClaimTemplates are mountable by name without ever appearing in
	// .spec.volumes — the framework's own data PVC among them.
	for _, c := range claims {
		declared[c.Name] = struct{}{}
	}

	var errs []error

	// Displaced or deleted framework mounts, walked in the built template's order so the report is
	// stable across reconciles.
	for _, baseContainer := range allPodContainers(base) {
		mergedContainer, found := findByKey(allPodContainers(merged), containerName, baseContainer.Name)
		if !found {
			// The override deleted the container outright. That is a different (and loud) kind of
			// mistake; this check is about mounts.
			continue
		}
		mergedMounts := make(map[string]string, len(mergedContainer.VolumeMounts))
		for _, m := range mergedContainer.VolumeMounts {
			mergedMounts[m.MountPath] = m.Name
		}
		for _, want := range baseContainer.VolumeMounts {
			switch got, present := mergedMounts[want.MountPath]; {
			case !present:
				errs = append(errs, fmt.Errorf(
					"podOverrides removed the framework's volume mount %q at %q in container %q",
					want.Name, want.MountPath, baseContainer.Name))
			case got != want.Name:
				errs = append(errs, fmt.Errorf(
					"podOverrides displaced the framework's volume mount at %q in container %q: "+
						"volume %q now mounts there instead of %q. Strategic merge patch keys "+
						"volumeMounts by mountPath, not by name, so a mount declared at a path the "+
						"framework already owns replaces it rather than being added alongside it — "+
						"mount %q somewhere else",
					want.MountPath, baseContainer.Name, got, want.Name, got))
			}
		}
	}

	// Mounts left pointing at nothing. The API server would reject these too, but only after a
	// round trip and without naming podOverrides.
	for _, c := range allPodContainers(merged) {
		for _, m := range c.VolumeMounts {
			if _, ok := declared[m.Name]; !ok {
				errs = append(errs, fmt.Errorf(
					"volume mount %q at %q in container %q references no declared volume "+
						"(check podOverrides: a mount at a mountPath the framework owns replaces "+
						"the framework's mount instead of being added alongside it)",
					m.Name, m.MountPath, c.Name))
			}
		}
	}

	return errs
}

// allPodContainers returns the init containers followed by the regular ones. Sidecars are injected
// as init containers with RestartPolicy Always, so both lists carry framework-owned mounts.
func allPodContainers(t *corev1.PodTemplateSpec) []corev1.Container {
	all := make([]corev1.Container, 0, len(t.Spec.InitContainers)+len(t.Spec.Containers))
	all = append(all, t.Spec.InitContainers...)
	all = append(all, t.Spec.Containers...)
	return all
}
