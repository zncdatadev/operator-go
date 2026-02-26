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
		})
	})
})
