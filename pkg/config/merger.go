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

// Merge performs deep merge of role and role group configurations.
// RoleGroup configurations override Role configurations.
func (m *ConfigMerger) Merge(roleOverrides, roleGroupOverrides *v1alpha1.OverridesSpec) *MergedConfig {
	result := NewMergedConfig()

	// Normalize nil inputs
	if roleOverrides == nil {
		roleOverrides = &v1alpha1.OverridesSpec{}
	}
	if roleGroupOverrides == nil {
		roleGroupOverrides = &v1alpha1.OverridesSpec{}
	}

	// Merge config files (deep merge)
	result.ConfigFiles = m.mergeConfigFiles(
		roleOverrides.ConfigOverrides,
		roleGroupOverrides.ConfigOverrides,
	)

	// Merge environment variables (deep merge)
	result.EnvVars = m.mergeMaps(
		roleOverrides.EnvOverrides,
		roleGroupOverrides.EnvOverrides,
	)

	// Merge CLI arguments (replace or append)
	result.CliArgs = m.mergeSlices(
		roleOverrides.CliOverrides,
		roleGroupOverrides.CliOverrides,
	)

	// Merge pod overrides (strategic merge patch)
	result.PodOverrides = m.mergePodOverrides(
		roleOverrides.PodOverrides,
		roleGroupOverrides.PodOverrides,
	)

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
		result := make([]string, len(base))
		copy(result, base)
		return append(result, override...)
	case MergeStrategyReplace:
		fallthrough
	default:
		return override
	}
}

// mergePodOverrides performs strategic merge patch on pod overrides.
func (m *ConfigMerger) mergePodOverrides(base, override *k8sruntime.RawExtension) *corev1.PodTemplateSpec {
	if base == nil && override == nil {
		return nil
	}

	var basePod, overridePod corev1.PodTemplateSpec

	// Parse base
	if base != nil && base.Raw != nil {
		if err := json.Unmarshal(base.Raw, &basePod); err != nil {
			// Log error but continue
			basePod = corev1.PodTemplateSpec{}
		}
	}

	// Parse override
	if override != nil && override.Raw != nil {
		if err := json.Unmarshal(override.Raw, &overridePod); err != nil {
			// Log error but continue
			overridePod = corev1.PodTemplateSpec{}
		}
	}

	// If only one is set, return it
	if base == nil || base.Raw == nil {
		return &overridePod
	}
	if override == nil || override.Raw == nil {
		return &basePod
	}

	// Perform strategic merge patch
	baseBytes, err := json.Marshal(basePod)
	if err != nil {
		return &overridePod
	}

	overrideBytes, err := json.Marshal(overridePod)
	if err != nil {
		return &basePod
	}

	// Get the pod template schema
	podTemplateSchema, err := strategicpatch.NewPatchMetaFromStruct(basePod)
	if err != nil {
		// Fall back to simple override
		return &overridePod
	}

	// Perform the merge
	mergedBytes, err := strategicpatch.StrategicMergePatchUsingLookupPatchMeta(baseBytes, overrideBytes, podTemplateSchema)
	if err != nil {
		return &overridePod
	}

	var mergedPod corev1.PodTemplateSpec
	if err := json.Unmarshal(mergedBytes, &mergedPod); err != nil {
		return &overridePod
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
