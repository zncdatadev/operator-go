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

package config

import (
	"encoding/json"
	"fmt"

	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
)

// MergeStrategy defines how slices are merged.
type MergeStrategy string

const (
	// MergeStrategyReplace completely replaces the parent slice.
	MergeStrategyReplace MergeStrategy = "Replace"
	// MergeStrategyAppend appends to the parent slice.
	MergeStrategyAppend MergeStrategy = "Append"
)

// MergedConfig represents the final merged configuration.
type MergedConfig struct {
	// ConfigFiles contains configuration file content indexed by filename.
	// Inner map is key-value pairs for the configuration.
	ConfigFiles map[string]map[string]string

	// EnvVars contains environment variables.
	EnvVars map[string]string

	// CliArgs contains CLI arguments.
	CliArgs []string

	// JvmArgs contains JVM arguments.
	JvmArgs []string

	// PodOverrides contains pod template overrides.
	PodOverrides *corev1.PodTemplateSpec

	// PodOverrideErrors collects the podOverrides layers that could not be applied: a layer
	// whose raw JSON does not decode into a PodTemplateSpec, or a strategic merge patch that
	// failed. Merge cannot reject the input (it has no error return), so it records the problem
	// here instead of dropping the layer silently; callers are expected to surface a non-empty
	// slice (Warning event / Degraded condition) rather than reconcile a pod the user did not
	// ask for.
	PodOverrideErrors []error

	// Logging is the per-container logging configuration, deep-merged from the Role and
	// RoleGroup levels (RoleGroup values win at the leaf). It drives both Vector sidecar
	// enablement and per-container logging config file generation. Nil when the product
	// CRD configured no logging.
	Logging *v1alpha1.LoggingSpec
}

// NewMergedConfig creates a new MergedConfig with initialized maps.
func NewMergedConfig() *MergedConfig {
	return &MergedConfig{
		ConfigFiles: make(map[string]map[string]string),
		EnvVars:     make(map[string]string),
		CliArgs:     make([]string, 0),
		JvmArgs:     make([]string, 0),
	}
}

// ConfigMerger merges role and role group configurations.
type ConfigMerger struct {
	// SliceMergeStrategy controls how slices (CLI arguments) are merged. It is settable by the
	// caller — the framework's own merger keeps the SDK default (MergeStrategyReplace), while a
	// product that treats CLI arguments as additive can construct a merger with
	// MergeStrategyAppend.
	SliceMergeStrategy MergeStrategy
}

// NewConfigMerger creates a new ConfigMerger.
func NewConfigMerger() *ConfigMerger {
	return &ConfigMerger{
		SliceMergeStrategy: MergeStrategyReplace,
	}
}

// Merge performs a deep merge of the given override layers in increasing order of
// precedence: each layer overrides the ones before it. The conventional order is
// product defaults (lowest), then role overrides, then role group overrides (highest),
// so a value set anywhere in the CRD always wins over a product default. nil layers are
// skipped, so callers may pass an absent layer (e.g. a missing product default) without
// a guard.
//
// Merge strategies follow the SDK contract: maps (config files, env) are deep-merged,
// slices (CLI) follow SliceMergeStrategy, and pod overrides use a strategic merge patch.
//
// Passing exactly (roleOverrides, roleGroupOverrides) reproduces the previous two-layer
// behavior, so existing callers are unaffected.
//
// A podOverrides layer that cannot be applied does not abort the merge (there is no error
// return): the failure is recorded in MergedConfig.PodOverrideErrors for the caller to surface.
func (m *ConfigMerger) Merge(overrides ...*v1alpha1.OverridesSpec) *MergedConfig {
	result := NewMergedConfig()

	for _, o := range overrides {
		if o == nil {
			continue
		}
		result.ConfigFiles = m.mergeConfigFiles(result.ConfigFiles, o.ConfigOverrides)
		result.EnvVars = m.mergeMaps(result.EnvVars, o.EnvOverrides)
		result.CliArgs = m.mergeSlices(result.CliArgs, o.CliOverrides)
		merged, err := m.mergePodOverrideInto(result.PodOverrides, o.PodOverrides)
		if err != nil {
			result.PodOverrideErrors = append(result.PodOverrideErrors, err)
		}
		result.PodOverrides = merged
	}

	return result
}

// MergeBeneath folds an overrides layer UNDERNEATH an already-merged config: a value the higher
// layers set is kept, and only what they left unset is contributed.
//
// Merge itself cannot express this. It folds *OverridesSpec layers left to right, so the lowest
// layer must be known before the merge runs — which is exactly what a product computing its config
// from a live API lookup cannot promise, because the lookup happens inside BuildResources. Both
// hive-operator and spark-k8s-operator hand-wrote the same "set only keys the user did not set"
// helper for that reason, byte for byte including its doc comment.
//
// The per-dimension rules are the merge's own, applied with the product's layer as the base:
// config files fold per key, env vars per key, CLI args as a whole (an empty higher slice means
// "unset", so the product's is used), and podOverrides through the same strategic merge patch.
//
// JvmArgs has no OverridesSpec field, so the lower layer cannot contribute any; the higher config's
// are carried through unchanged for callers that populated them imperatively (MergedConfig.AddJvmArg).
//
// `higher` is not mutated; the result is a new MergedConfig.
func (m *ConfigMerger) MergeBeneath(higher *MergedConfig, lower *v1alpha1.OverridesSpec) *MergedConfig {
	base := m.Merge(lower)
	if higher == nil {
		return base
	}

	result := NewMergedConfig()
	result.ConfigFiles = m.mergeConfigFiles(base.ConfigFiles, higher.ConfigFiles)
	result.EnvVars = m.mergeMaps(base.EnvVars, higher.EnvVars)
	result.CliArgs = m.mergeSlices(base.CliArgs, higher.CliArgs)
	result.JvmArgs = m.mergeSlices(base.JvmArgs, higher.JvmArgs)

	// The product's pod template is the base and the already-merged one is the patch, which is
	// what puts the user's podOverrides on top.
	result.PodOverrides = higher.PodOverrides
	if base.PodOverrides != nil {
		merged, err := m.mergePodTemplates(base.PodOverrides, higher.PodOverrides)
		if err != nil {
			result.PodOverrideErrors = append(result.PodOverrideErrors, err)
		} else {
			result.PodOverrides = merged
		}
	}

	// Carry both layers' recorded failures: the caller surfaces them as one set.
	result.PodOverrideErrors = append(result.PodOverrideErrors, base.PodOverrideErrors...)
	result.PodOverrideErrors = append(result.PodOverrideErrors, higher.PodOverrideErrors...)
	result.Logging = higher.Logging
	return result
}

// mergeMaps performs deep merge of two maps.
// Values in the second map override values in the first map.
func (m *ConfigMerger) mergeMaps(base, override map[string]string) map[string]string {
	result := make(map[string]string)

	// Copy base values
	for k, v := range base {
		result[k] = v
	}

	// Override with new values
	for k, v := range override {
		result[k] = v
	}

	return result
}

// mergeConfigFiles performs deep merge of nested config file maps.
func (m *ConfigMerger) mergeConfigFiles(base, override map[string]map[string]string) map[string]map[string]string {
	result := make(map[string]map[string]string)

	// Copy base values
	for filename, config := range base {
		result[filename] = make(map[string]string)
		for k, v := range config {
			result[filename][k] = v
		}
	}

	// Merge/override with new values
	for filename, config := range override {
		if result[filename] == nil {
			result[filename] = make(map[string]string)
		}
		for k, v := range config {
			result[filename][k] = v
		}
	}

	return result
}

// mergeSlices merges two slices based on the merge strategy.
//
// An empty override (nil or []) is indistinguishable from an absent one: it leaves the base
// untouched, so a role group cannot clear the CLI arguments its role set. Only a non-empty
// override replaces (or, under MergeStrategyAppend, extends) them.
func (m *ConfigMerger) mergeSlices(base, override []string) []string {
	if len(override) == 0 {
		return base
	}

	switch m.SliceMergeStrategy {
	case MergeStrategyAppend:
		result := make([]string, len(base), len(base)+len(override))
		copy(result, base)
		return append(result, override...)
	case MergeStrategyReplace:
		fallthrough
	default:
		return override
	}
}

// mergePodOverrideInto strategically merges a raw pod override layer on top of an
// already-parsed base template, returning the merged result. This fold-friendly shape lets
// Merge accumulate any number of layers: the accumulator (base) is the running merged
// template and override is the next raw layer.
//
// Behavior:
//   - both empty            -> nil
//   - only the override set -> the parsed override
//   - only the base set     -> the base unchanged
//   - both set              -> strategic merge patch of override onto base
//
// The returned error reports a layer that could not be applied. The template returned alongside
// it is the best available fallback (the malformed layer is treated as absent, a failed patch
// keeps the higher-precedence layer) so the fold can continue, but the caller must not treat
// the result as the user's intent: a malformed override must neither win precedence nor
// surface downstream as a non-nil empty PodTemplateSpec.
func (m *ConfigMerger) mergePodOverrideInto(base *corev1.PodTemplateSpec, override *k8sruntime.RawExtension) (*corev1.PodTemplateSpec, error) {
	// Parse the override layer. An unmarshal failure keeps the layer out of the result.
	var overridePod *corev1.PodTemplateSpec
	if override != nil && override.Raw != nil {
		var parsed corev1.PodTemplateSpec
		if err := json.Unmarshal(override.Raw, &parsed); err != nil {
			return base, fmt.Errorf("podOverrides layer is not a valid PodTemplateSpec: %w", err)
		}
		overridePod = &parsed
	}

	return m.mergePodTemplates(base, overridePod)
}

// mergePodTemplates strategic-merges override onto base. Both callers need this — the RawExtension
// path above after it has decoded its layer, and MergeBeneath, whose "override" is an
// already-merged template rather than raw JSON — so it lives in one place rather than two that can
// drift.
func (m *ConfigMerger) mergePodTemplates(
	base, overridePod *corev1.PodTemplateSpec,
) (*corev1.PodTemplateSpec, error) {
	switch {
	case base == nil && overridePod == nil:
		return nil, nil
	case base == nil:
		return overridePod, nil
	case overridePod == nil:
		return base, nil
	}

	baseBytes, err := json.Marshal(base)
	if err != nil {
		return overridePod, fmt.Errorf("failed to encode the merged pod template: %w", err)
	}

	overrideBytes, err := json.Marshal(overridePod)
	if err != nil {
		return base, fmt.Errorf("failed to encode the podOverrides layer: %w", err)
	}

	podTemplateSchema, err := strategicpatch.NewPatchMetaFromStruct(*base)
	if err != nil {
		return overridePod, fmt.Errorf("failed to build the pod template patch metadata: %w", err)
	}

	mergedBytes, err := strategicpatch.StrategicMergePatchUsingLookupPatchMeta(baseBytes, overrideBytes, podTemplateSchema)
	if err != nil {
		return overridePod, fmt.Errorf("failed to strategic-merge the podOverrides layer: %w", err)
	}

	var mergedPod corev1.PodTemplateSpec
	if err := json.Unmarshal(mergedBytes, &mergedPod); err != nil {
		return overridePod, fmt.Errorf("failed to decode the strategic-merged pod template: %w", err)
	}

	return &mergedPod, nil
}

// Clone creates a deep copy of MergedConfig.
func (c *MergedConfig) Clone() *MergedConfig {
	result := NewMergedConfig()

	// Clone config files
	for filename, config := range c.ConfigFiles {
		result.ConfigFiles[filename] = make(map[string]string)
		for k, v := range config {
			result.ConfigFiles[filename][k] = v
		}
	}

	// Clone env vars
	for k, v := range c.EnvVars {
		result.EnvVars[k] = v
	}

	// Clone slices
	result.CliArgs = make([]string, len(c.CliArgs))
	copy(result.CliArgs, c.CliArgs)

	result.JvmArgs = make([]string, len(c.JvmArgs))
	copy(result.JvmArgs, c.JvmArgs)

	// Pod overrides are not cloned (reference copy is sufficient for most use cases)
	result.PodOverrides = c.PodOverrides
	result.PodOverrideErrors = append([]error(nil), c.PodOverrideErrors...)

	return result
}

// AddConfigFile adds a configuration file to the merged config.
func (c *MergedConfig) AddConfigFile(filename string, config map[string]string) {
	if c.ConfigFiles == nil {
		c.ConfigFiles = make(map[string]map[string]string)
	}
	c.ConfigFiles[filename] = config
}

// AddEnvVar adds an environment variable to the merged config.
func (c *MergedConfig) AddEnvVar(key, value string) {
	if c.EnvVars == nil {
		c.EnvVars = make(map[string]string)
	}
	c.EnvVars[key] = value
}

// AddCliArg adds a CLI argument to the merged config.
func (c *MergedConfig) AddCliArg(arg string) {
	c.CliArgs = append(c.CliArgs, arg)
}

// AddJvmArg adds a JVM argument to the merged config.
func (c *MergedConfig) AddJvmArg(arg string) {
	c.JvmArgs = append(c.JvmArgs, arg)
}

// GetConfigFile returns a configuration file by name, or nil if not found.
func (c *MergedConfig) GetConfigFile(filename string) map[string]string {
	if c.ConfigFiles == nil {
		return nil
	}
	return c.ConfigFiles[filename]
}
