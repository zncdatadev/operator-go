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
	// SliceMergeStrategy controls how slices are merged.
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
func (m *ConfigMerger) Merge(overrides ...*v1alpha1.OverridesSpec) *MergedConfig {
	result := NewMergedConfig()

	for _, o := range overrides {
		if o == nil {
			continue
		}
		result.ConfigFiles = m.mergeConfigFiles(result.ConfigFiles, o.ConfigOverrides)
		result.EnvVars = m.mergeMaps(result.EnvVars, o.EnvOverrides)
		result.CliArgs = m.mergeSlices(result.CliArgs, o.CliOverrides)
		result.PodOverrides = m.mergePodOverrideInto(result.PodOverrides, o.PodOverrides)
	}

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
// On any marshal/patch error it falls back to the override, so the higher-precedence layer
// still wins. A malformed override (invalid JSON) is treated as absent — it must neither win
// precedence nor surface downstream as a non-nil empty PodTemplateSpec.
func (m *ConfigMerger) mergePodOverrideInto(base *corev1.PodTemplateSpec, override *k8sruntime.RawExtension) *corev1.PodTemplateSpec {
	// Parse the override layer. An unmarshal failure leaves overridePod nil (layer absent).
	var overridePod *corev1.PodTemplateSpec
	if override != nil && override.Raw != nil {
		var parsed corev1.PodTemplateSpec
		if err := json.Unmarshal(override.Raw, &parsed); err == nil {
			overridePod = &parsed
		}
	}

	switch {
	case base == nil && overridePod == nil:
		return nil
	case base == nil:
		return overridePod
	case overridePod == nil:
		return base
	}

	baseBytes, err := json.Marshal(base)
	if err != nil {
		return overridePod
	}

	overrideBytes, err := json.Marshal(overridePod)
	if err != nil {
		return base
	}

	podTemplateSchema, err := strategicpatch.NewPatchMetaFromStruct(*base)
	if err != nil {
		return overridePod
	}

	mergedBytes, err := strategicpatch.StrategicMergePatchUsingLookupPatchMeta(baseBytes, overrideBytes, podTemplateSchema)
	if err != nil {
		return overridePod
	}

	var mergedPod corev1.PodTemplateSpec
	if err := json.Unmarshal(mergedBytes, &mergedPod); err != nil {
		return overridePod
	}

	return &mergedPod
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
