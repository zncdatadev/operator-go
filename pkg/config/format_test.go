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
	"errors"
	"fmt"
	"sort"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/config"
)

// bannerFormat is an emit-only format: it renders a config as a comment banner a product ships
// read-only. It deliberately has no Unmarshal — that is what makes it a legal ConfigMarshaler
// and an illegal parse target.
type bannerFormat struct{}

var _ config.ConfigMarshaler = bannerFormat{}

func (bannerFormat) Marshal(data map[string]string) (string, error) {
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var sb strings.Builder
	for _, key := range keys {
		fmt.Fprintf(&sb, "# %s -> %s\n", key, data[key])
	}
	return sb.String(), nil
}

var _ = Describe("Format contract", func() {
	Describe("ConfigMarshaler", func() {
		It("is the whole contract a registered format has to satisfy", func() {
			generator := config.NewMultiFormatConfigGenerator()
			generator.RegisterFormat(".banner", bannerFormat{})

			content, err := generator.Generate("notes.banner", map[string]string{"b": "2", "a": "1"})
			Expect(err).ToNot(HaveOccurred())
			Expect(content).To(Equal("# a -> 1\n# b -> 2\n"))
		})

		It("carries an emit-only format through GenerateFiles alongside a round-tripping one", func() {
			generator := config.NewMultiFormatConfigGenerator()
			generator.RegisterDefaultFormats()
			generator.RegisterFormat(".banner", bannerFormat{})

			files, err := generator.GenerateFiles(map[string]map[string]string{
				"notes.banner":   {"a": "1"},
				"app.properties": {"b": "2"},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(files).To(HaveKeyWithValue("notes.banner", "# a -> 1\n"))
			Expect(files).To(HaveKeyWithValue("app.properties", "b=2\n"))
		})

		It("is satisfied by a single-format generator that never parses", func() {
			generator := config.NewConfigGenerator(bannerFormat{})

			content, err := generator.Generate(map[string]string{"a": "1"})
			Expect(err).ToNot(HaveOccurred())
			Expect(content).To(Equal("# a -> 1\n"))
		})
	})

	Describe("ConfigUnmarshaler", func() {
		It("is implemented by every shipped adapter, so they all round-trip through Parse", func() {
			generator := config.NewMultiFormatConfigGenerator()
			generator.RegisterDefaultFormats()

			for _, filename := range []string{"a.xml", "a.properties", "a.yaml", "a.yml", "a.env", "a.ini"} {
				data := map[string]string{"KEY": "value"}
				content, err := generator.Generate(filename, data)
				Expect(err).ToNot(HaveOccurred(), filename)

				parsed, err := generator.Parse(filename, content)
				Expect(err).ToNot(HaveOccurred(), filename)
				Expect(parsed).To(Equal(data), filename)
			}
		})

		It("makes a multi-format parse of an emit-only file fail naming the file and the format", func() {
			generator := config.NewMultiFormatConfigGenerator()
			generator.RegisterFormat(".banner", bannerFormat{})

			parsed, err := generator.Parse("notes.banner", "# a -> 1\n")
			Expect(parsed).To(BeNil())
			Expect(err).To(HaveOccurred())

			var unsupported *config.UnsupportedParseError
			Expect(errors.As(err, &unsupported)).To(BeTrue())
			Expect(unsupported.File).To(Equal("notes.banner"))
			Expect(unsupported.Format).To(ContainSubstring(".banner"))
			Expect(err.Error()).To(ContainSubstring("notes.banner"))
			Expect(err.Error()).To(ContainSubstring(".banner"))
			Expect(err.Error()).To(ContainSubstring("bannerFormat"))
		})

		It("makes a single-format parse of an emit-only format fail naming the format", func() {
			generator := config.NewConfigGenerator(bannerFormat{})

			parsed, err := generator.Parse("# a -> 1\n")
			Expect(parsed).To(BeNil())
			Expect(err).To(HaveOccurred())

			var unsupported *config.UnsupportedParseError
			Expect(errors.As(err, &unsupported)).To(BeTrue())
			Expect(unsupported.File).To(BeEmpty())
			Expect(err.Error()).To(ContainSubstring("bannerFormat"))
		})
	})
})

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

	It("should return INIAdapter for FormatINI", func() {
		format := config.GetFormat(config.FormatINI)
		Expect(format).NotTo(BeNil())
		_, ok := format.(*config.INIAdapter)
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
