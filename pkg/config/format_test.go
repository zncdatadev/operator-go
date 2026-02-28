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
	"github.com/zncdatadev/operator-go/pkg/config"
)

var _ = Describe("GetFormat", func() {
	It("should return XMLAdapter for FormatXML", func() {
		format := config.GetFormat(config.FormatXML)
		Expect(format).NotTo(BeNil())
		_, ok := format.(*config.XMLAdapter)
		Expect(ok).To(BeTrue())
	})

	It("should return PropertiesAdapter for FormatProperties", func() {
		format := config.GetFormat(config.FormatProperties)
		Expect(format).NotTo(BeNil())
		_, ok := format.(*config.PropertiesAdapter)
		Expect(ok).To(BeTrue())
	})

	It("should return YAMLAdapter for FormatYAML", func() {
		format := config.GetFormat(config.FormatYAML)
		Expect(format).NotTo(BeNil())
		_, ok := format.(*config.YAMLAdapter)
		Expect(ok).To(BeTrue())
	})

	It("should return EnvAdapter for FormatEnv", func() {
		format := config.GetFormat(config.FormatEnv)
		Expect(format).NotTo(BeNil())
		_, ok := format.(*config.EnvAdapter)
		Expect(ok).To(BeTrue())
	})

	It("should return PropertiesAdapter as default for unknown format", func() {
		format := config.GetFormat(config.ConfigFormatType("unknown"))
		Expect(format).NotTo(BeNil())
		_, ok := format.(*config.PropertiesAdapter)
		Expect(ok).To(BeTrue())
	})

	It("should return PropertiesAdapter for FormatINI (not implemented)", func() {
		format := config.GetFormat(config.FormatINI)
		Expect(format).NotTo(BeNil())
		// INI format falls back to Properties
		_, ok := format.(*config.PropertiesAdapter)
		Expect(ok).To(BeTrue())
	})
})

var _ = Describe("ConfigFormatType constants", func() {
	It("should have correct FormatXML value", func() {
		Expect(string(config.FormatXML)).To(Equal("xml"))
	})

	It("should have correct FormatProperties value", func() {
		Expect(string(config.FormatProperties)).To(Equal("properties"))
	})

	It("should have correct FormatYAML value", func() {
		Expect(string(config.FormatYAML)).To(Equal("yaml"))
	})

	It("should have correct FormatEnv value", func() {
		Expect(string(config.FormatEnv)).To(Equal("env"))
	})

	It("should have correct FormatINI value", func() {
		Expect(string(config.FormatINI)).To(Equal("ini"))
	})
})
