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

package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/config"
)

var _ = Describe("ConfigMerger", func() {
	var merger *config.ConfigMerger

	BeforeEach(func() {
		merger = config.NewConfigMerger()
	})

	Describe("NewConfigMerger", func() {
		It("should create a ConfigMerger with default replace strategy", func() {
			Expect(merger).NotTo(BeNil())
			Expect(merger.SliceMergeStrategy).To(Equal(config.MergeStrategyReplace))
		})
	})

	Describe("Merge", func() {
		It("should merge empty overrides", func() {
			roleOverrides := &v1alpha1.OverridesSpec{}
			groupOverrides := &v1alpha1.OverridesSpec{}

			result := merger.Merge(roleOverrides, groupOverrides)

			Expect(result).NotTo(BeNil())
		})

		It("should merge role and group overrides", func() {
			roleOverrides := &v1alpha1.OverridesSpec{
				ConfigOverrides: map[string]map[string]string{
					"config.yaml": {
						"role": "value",
					},
				},
			}
			groupOverrides := &v1alpha1.OverridesSpec{
				ConfigOverrides: map[string]map[string]string{
					"config.yaml": {
						"group": "value",
					},
				},
			}

			result := merger.Merge(roleOverrides, groupOverrides)

			Expect(result).NotTo(BeNil())
			Expect(result.ConfigFiles).To(HaveKey("config.yaml"))
			Expect(result.ConfigFiles["config.yaml"]).To(HaveKeyWithValue("role", "value"))
			Expect(result.ConfigFiles["config.yaml"]).To(HaveKeyWithValue("group", "value"))
		})

		It("should handle nil overrides", func() {
			result := merger.Merge(nil, nil)

			Expect(result).NotTo(BeNil())
		})

		It("should merge CLI overrides", func() {
			roleOverrides := &v1alpha1.OverridesSpec{
				CliOverrides: []string{"--role-flag=value"},
			}
			groupOverrides := &v1alpha1.OverridesSpec{
				CliOverrides: []string{"--group-flag=value"},
			}

			result := merger.Merge(roleOverrides, groupOverrides)

			Expect(result).NotTo(BeNil())
			Expect(result.CliArgs).To(Equal([]string{"--group-flag=value"}))
		})

		It("should merge env overrides", func() {
			roleOverrides := &v1alpha1.OverridesSpec{
				EnvOverrides: map[string]string{
					"ROLE_VAR": "role_value",
				},
			}
			groupOverrides := &v1alpha1.OverridesSpec{
				EnvOverrides: map[string]string{
					"GROUP_VAR": "group_value",
				},
			}

			result := merger.Merge(roleOverrides, groupOverrides)

			Expect(result).NotTo(BeNil())
			Expect(result.EnvVars).To(HaveKeyWithValue("ROLE_VAR", "role_value"))
			Expect(result.EnvVars).To(HaveKeyWithValue("GROUP_VAR", "group_value"))
		})

		It("should use role CLI overrides when group has none with Replace strategy", func() {
			merger.SliceMergeStrategy = config.MergeStrategyReplace
			roleOverrides := &v1alpha1.OverridesSpec{
				CliOverrides: []string{"--role-flag=value"},
			}
			groupOverrides := &v1alpha1.OverridesSpec{}

			result := merger.Merge(roleOverrides, groupOverrides)

			Expect(result.CliArgs).To(Equal([]string{"--role-flag=value"}))
		})

		It("should append CLI overrides with Append strategy", func() {
			merger.SliceMergeStrategy = config.MergeStrategyAppend
			roleOverrides := &v1alpha1.OverridesSpec{
				CliOverrides: []string{"--role-flag=value"},
			}
			groupOverrides := &v1alpha1.OverridesSpec{
				CliOverrides: []string{"--group-flag=value"},
			}

			result := merger.Merge(roleOverrides, groupOverrides)

			Expect(result.CliArgs).To(Equal([]string{"--role-flag=value", "--group-flag=value"}))
		})

		It("should override role env with group env", func() {
			roleOverrides := &v1alpha1.OverridesSpec{
				EnvOverrides: map[string]string{
					"VAR": "role_value",
				},
			}
			groupOverrides := &v1alpha1.OverridesSpec{
				EnvOverrides: map[string]string{
					"VAR": "group_value",
				},
			}

			result := merger.Merge(roleOverrides, groupOverrides)

			Expect(result.EnvVars).To(HaveKeyWithValue("VAR", "group_value"))
		})

		It("should merge config files from different files", func() {
			roleOverrides := &v1alpha1.OverridesSpec{
				ConfigOverrides: map[string]map[string]string{
					"config1.yaml": {"key1": "value1"},
				},
			}
			groupOverrides := &v1alpha1.OverridesSpec{
				ConfigOverrides: map[string]map[string]string{
					"config2.yaml": {"key2": "value2"},
				},
			}

			result := merger.Merge(roleOverrides, groupOverrides)

			Expect(result.ConfigFiles).To(HaveKey("config1.yaml"))
			Expect(result.ConfigFiles).To(HaveKey("config2.yaml"))
		})

		It("should override role config with group config in same file", func() {
			roleOverrides := &v1alpha1.OverridesSpec{
				ConfigOverrides: map[string]map[string]string{
					"config.yaml": {"key": "role_value"},
				},
			}
			groupOverrides := &v1alpha1.OverridesSpec{
				ConfigOverrides: map[string]map[string]string{
					"config.yaml": {"key": "group_value"},
				},
			}

			result := merger.Merge(roleOverrides, groupOverrides)

			Expect(result.ConfigFiles["config.yaml"]).To(HaveKeyWithValue("key", "group_value"))
		})
	})
})

var _ = Describe("MergedConfig", func() {
	Describe("NewMergedConfig", func() {
		It("should create a MergedConfig with initialized maps", func() {
			merged := config.NewMergedConfig()
			Expect(merged).NotTo(BeNil())
			Expect(merged.ConfigFiles).NotTo(BeNil())
			Expect(merged.EnvVars).NotTo(BeNil())
			Expect(merged.CliArgs).NotTo(BeNil())
			Expect(merged.JvmArgs).NotTo(BeNil())
		})
	})

	Describe("Clone", func() {
		It("should create a deep copy of MergedConfig", func() {
			original := config.NewMergedConfig()
			original.ConfigFiles["test.yaml"] = map[string]string{"key": "value"}
			original.EnvVars["ENV_VAR"] = "env_value"
			original.CliArgs = []string{"--arg1", "--arg2"}
			original.JvmArgs = []string{"-Xmx1g"}

			cloned := original.Clone()

			Expect(cloned).NotTo(BeNil())
			Expect(cloned.ConfigFiles).To(Equal(original.ConfigFiles))
			Expect(cloned.EnvVars).To(Equal(original.EnvVars))
			Expect(cloned.CliArgs).To(Equal(original.CliArgs))
			Expect(cloned.JvmArgs).To(Equal(original.JvmArgs))
		})

		It("should not affect original when modifying clone", func() {
			original := config.NewMergedConfig()
			original.EnvVars["KEY"] = "original"

			cloned := original.Clone()
			cloned.EnvVars["KEY"] = "modified"

			Expect(original.EnvVars["KEY"]).To(Equal("original"))
			Expect(cloned.EnvVars["KEY"]).To(Equal("modified"))
		})

		It("should clone empty MergedConfig", func() {
			original := config.NewMergedConfig()
			cloned := original.Clone()

			Expect(cloned.ConfigFiles).To(BeEmpty())
			Expect(cloned.EnvVars).To(BeEmpty())
			Expect(cloned.CliArgs).To(BeEmpty())
			Expect(cloned.JvmArgs).To(BeEmpty())
		})
	})

	Describe("AddConfigFile", func() {
		It("should add a config file to MergedConfig", func() {
			merged := config.NewMergedConfig()
			configData := map[string]string{"key": "value"}

			merged.AddConfigFile("test.yaml", configData)

			Expect(merged.ConfigFiles).To(HaveKey("test.yaml"))
			Expect(merged.ConfigFiles["test.yaml"]).To(Equal(configData))
		})

		It("should initialize ConfigFiles if nil", func() {
			merged := &config.MergedConfig{}
			configData := map[string]string{"key": "value"}

			merged.AddConfigFile("test.yaml", configData)

			Expect(merged.ConfigFiles).NotTo(BeNil())
			Expect(merged.ConfigFiles["test.yaml"]).To(Equal(configData))
		})
	})

	Describe("AddEnvVar", func() {
		It("should add an environment variable to MergedConfig", func() {
			merged := config.NewMergedConfig()

			merged.AddEnvVar("ENV_VAR", "value")

			Expect(merged.EnvVars).To(HaveKeyWithValue("ENV_VAR", "value"))
		})

		It("should initialize EnvVars if nil", func() {
			merged := &config.MergedConfig{}

			merged.AddEnvVar("ENV_VAR", "value")

			Expect(merged.EnvVars).NotTo(BeNil())
			Expect(merged.EnvVars).To(HaveKeyWithValue("ENV_VAR", "value"))
		})
	})

	Describe("AddCliArg", func() {
		It("should add a CLI argument to MergedConfig", func() {
			merged := config.NewMergedConfig()

			merged.AddCliArg("--flag=value")

			Expect(merged.CliArgs).To(ContainElement("--flag=value"))
		})

		It("should append multiple CLI arguments", func() {
			merged := config.NewMergedConfig()

			merged.AddCliArg("--arg1")
			merged.AddCliArg("--arg2")

			Expect(merged.CliArgs).To(HaveLen(2))
			Expect(merged.CliArgs).To(ContainElements("--arg1", "--arg2"))
		})
	})

	Describe("AddJvmArg", func() {
		It("should add a JVM argument to MergedConfig", func() {
			merged := config.NewMergedConfig()

			merged.AddJvmArg("-Xmx1g")

			Expect(merged.JvmArgs).To(ContainElement("-Xmx1g"))
		})

		It("should append multiple JVM arguments", func() {
			merged := config.NewMergedConfig()

			merged.AddJvmArg("-Xmx1g")
			merged.AddJvmArg("-Xms512m")

			Expect(merged.JvmArgs).To(HaveLen(2))
			Expect(merged.JvmArgs).To(ContainElements("-Xmx1g", "-Xms512m"))
		})
	})

	Describe("GetConfigFile", func() {
		It("should return config file by name", func() {
			merged := config.NewMergedConfig()
			configData := map[string]string{"key": "value"}
			merged.ConfigFiles["test.yaml"] = configData

			result := merged.GetConfigFile("test.yaml")

			Expect(result).To(Equal(configData))
		})

		It("should return nil for non-existent file", func() {
			merged := config.NewMergedConfig()

			result := merged.GetConfigFile("nonexistent.yaml")

			Expect(result).To(BeNil())
		})

		It("should return nil when ConfigFiles is nil", func() {
			merged := &config.MergedConfig{}

			result := merged.GetConfigFile("test.yaml")

			Expect(result).To(BeNil())
		})
	})
})

var _ = Describe("MergeStrategy constants", func() {
	It("should have correct MergeStrategyReplace value", func() {
		Expect(string(config.MergeStrategyReplace)).To(Equal("Replace"))
	})

	It("should have correct MergeStrategyAppend value", func() {
		Expect(string(config.MergeStrategyAppend)).To(Equal("Append"))
	})
})
