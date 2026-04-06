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

var _ = Describe("ConfigGenerator", func() {
	Describe("NewConfigGenerator", func() {
		It("should create a ConfigGenerator with format", func() {
			format := config.NewPropertiesAdapter()
			generator := config.NewConfigGenerator(format)
			Expect(generator).NotTo(BeNil())
		})
	})

	Describe("NewConfigGeneratorWithType", func() {
		It("should create a ConfigGenerator with XML format", func() {
			generator := config.NewConfigGeneratorWithType(config.FormatXML)
			Expect(generator).NotTo(BeNil())
		})

		It("should create a ConfigGenerator with Properties format", func() {
			generator := config.NewConfigGeneratorWithType(config.FormatProperties)
			Expect(generator).NotTo(BeNil())
		})

		It("should create a ConfigGenerator with YAML format", func() {
			generator := config.NewConfigGeneratorWithType(config.FormatYAML)
			Expect(generator).NotTo(BeNil())
		})

		It("should create a ConfigGenerator with Env format", func() {
			generator := config.NewConfigGeneratorWithType(config.FormatEnv)
			Expect(generator).NotTo(BeNil())
		})

		It("should create a ConfigGenerator with default format for unknown type", func() {
			generator := config.NewConfigGeneratorWithType(config.ConfigFormatType("unknown"))
			Expect(generator).NotTo(BeNil())
		})
	})

	Describe("Generate", func() {
		It("should generate config content", func() {
			generator := config.NewConfigGeneratorWithType(config.FormatProperties)
			data := map[string]string{
				"key1": "value1",
				"key2": "value2",
			}
			content, err := generator.Generate(data)
			Expect(err).ToNot(HaveOccurred())
			Expect(content).To(ContainSubstring("key1=value1"))
			Expect(content).To(ContainSubstring("key2=value2"))
		})

		It("should return empty string for empty data", func() {
			generator := config.NewConfigGeneratorWithType(config.FormatProperties)
			content, err := generator.Generate(map[string]string{})
			Expect(err).ToNot(HaveOccurred())
			Expect(content).To(BeEmpty())
		})

		It("should return error when format is nil", func() {
			generator := config.NewConfigGenerator(nil)
			content, err := generator.Generate(map[string]string{"key": "value"})
			Expect(err).To(HaveOccurred())
			Expect(content).To(BeEmpty())
		})
	})

	Describe("Parse", func() {
		It("should parse config content", func() {
			generator := config.NewConfigGeneratorWithType(config.FormatProperties)
			content := "key1=value1\nkey2=value2\n"
			data, err := generator.Parse(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(data).To(HaveKeyWithValue("key1", "value1"))
			Expect(data).To(HaveKeyWithValue("key2", "value2"))
		})

		It("should return empty map for empty content", func() {
			generator := config.NewConfigGeneratorWithType(config.FormatProperties)
			data, err := generator.Parse("")
			Expect(err).ToNot(HaveOccurred())
			Expect(data).To(BeEmpty())
		})

		It("should return error when format is nil", func() {
			generator := config.NewConfigGenerator(nil)
			data, err := generator.Parse("key=value")
			Expect(err).To(HaveOccurred())
			Expect(data).To(BeNil())
		})
	})

	Describe("GenerateFiles", func() {
		It("should generate multiple config files", func() {
			generator := config.NewConfigGeneratorWithType(config.FormatProperties)
			files := map[string]map[string]string{
				"config1.properties": {"key1": "value1"},
				"config2.properties": {"key2": "value2"},
			}
			result, err := generator.GenerateFiles(files)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKey("config1.properties"))
			Expect(result).To(HaveKey("config2.properties"))
			Expect(result["config1.properties"]).To(ContainSubstring("key1=value1"))
			Expect(result["config2.properties"]).To(ContainSubstring("key2=value2"))
		})

		It("should return empty map for empty input", func() {
			generator := config.NewConfigGeneratorWithType(config.FormatProperties)
			result, err := generator.GenerateFiles(map[string]map[string]string{})
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(BeEmpty())
		})
	})
})

var _ = Describe("MultiFormatConfigGenerator", func() {
	var generator *config.MultiFormatConfigGenerator

	BeforeEach(func() {
		generator = config.NewMultiFormatConfigGenerator()
	})

	Describe("NewMultiFormatConfigGenerator", func() {
		It("should create a MultiFormatConfigGenerator", func() {
			Expect(generator).NotTo(BeNil())
		})
	})

	Describe("RegisterFormat", func() {
		It("should register a custom format", func() {
			generator.RegisterFormat(".custom", config.NewPropertiesAdapter())
			// No error means success
		})
	})

	Describe("RegisterDefaultFormats", func() {
		It("should register all default formats", func() {
			generator.RegisterDefaultFormats()
			// No error means success
		})
	})

	Describe("Generate", func() {
		BeforeEach(func() {
			generator.RegisterDefaultFormats()
		})

		It("should generate XML format for .xml extension", func() {
			data := map[string]string{"key": "value"}
			content, err := generator.Generate("config.xml", data)
			Expect(err).ToNot(HaveOccurred())
			Expect(content).To(ContainSubstring("<?xml"))
			Expect(content).To(ContainSubstring("<name>key</name>"))
			Expect(content).To(ContainSubstring("<value>value</value>"))
		})

		It("should generate Properties format for .properties extension", func() {
			data := map[string]string{"key": "value"}
			content, err := generator.Generate("config.properties", data)
			Expect(err).ToNot(HaveOccurred())
			Expect(content).To(ContainSubstring("key=value"))
		})

		It("should generate YAML format for .yaml extension", func() {
			data := map[string]string{"key": "value"}
			content, err := generator.Generate("config.yaml", data)
			Expect(err).ToNot(HaveOccurred())
			Expect(content).To(ContainSubstring("key: value"))
		})

		It("should generate YAML format for .yml extension", func() {
			data := map[string]string{"key": "value"}
			content, err := generator.Generate("config.yml", data)
			Expect(err).ToNot(HaveOccurred())
			Expect(content).To(ContainSubstring("key: value"))
		})

		It("should generate Env format for .env extension", func() {
			data := map[string]string{"KEY": "value"}
			content, err := generator.Generate("config.env", data)
			Expect(err).ToNot(HaveOccurred())
			Expect(content).To(ContainSubstring("KEY=value"))
		})

		It("should generate INI format for .ini extension", func() {
			data := map[string]string{"key": "value"}
			content, err := generator.Generate("config.ini", data)
			Expect(err).ToNot(HaveOccurred())
			Expect(content).To(ContainSubstring("key = value"))
		})

		It("should fall back to Properties format for unknown extension", func() {
			data := map[string]string{"key": "value"}
			content, err := generator.Generate("config.unknown", data)
			Expect(err).ToNot(HaveOccurred())
			Expect(content).To(ContainSubstring("key=value"))
		})
	})

	Describe("GenerateFiles", func() {
		BeforeEach(func() {
			generator.RegisterDefaultFormats()
		})

		It("should generate multiple files with different formats", func() {
			files := map[string]map[string]string{
				"core-site.xml":          {"fs.defaultFS": "hdfs://localhost:8020"},
				"application.properties": {"server.port": "8080"},
				"config.yaml":            {"name": "test"},
				"app.env":                {"APP_ENV": "production"},
			}
			result, err := generator.GenerateFiles(files)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveLen(4))
			Expect(result["core-site.xml"]).To(ContainSubstring("<?xml"))
			Expect(result["application.properties"]).To(ContainSubstring("server.port=8080"))
			Expect(result["config.yaml"]).To(ContainSubstring("name: test"))
			Expect(result["app.env"]).To(ContainSubstring("APP_ENV=production"))
		})
	})
})
