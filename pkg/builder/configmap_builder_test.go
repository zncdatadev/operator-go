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

package builder_test

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/config"
)

// failingFormat is a mock format that always returns an error
type failingFormat struct{}

func (f *failingFormat) Marshal(data map[string]string) (string, error) {
	return "", errors.New("mock marshal error")
}

func (f *failingFormat) Unmarshal(data string) (map[string]string, error) {
	return nil, errors.New("mock unmarshal error")
}

var _ = Describe("ConfigMapBuilder", func() {
	const (
		name      = "test-cm"
		namespace = "test-namespace"
	)

	var cmBuilder *builder.ConfigMapBuilder

	BeforeEach(func() {
		cmBuilder = builder.NewConfigMapBuilder(name, namespace)
	})

	Describe("NewConfigMapBuilder", func() {
		It("should create a builder with default values", func() {
			Expect(cmBuilder.Name).To(Equal(name))
			Expect(cmBuilder.Namespace).To(Equal(namespace))
		})
	})

	Describe("WithLabels", func() {
		It("should add labels to the builder", func() {
			labels := map[string]string{"app": "test"}
			result := cmBuilder.WithLabels(labels)

			Expect(result).To(Equal(cmBuilder))
			Expect(cmBuilder.Labels).To(HaveKeyWithValue("app", "test"))
		})
	})

	Describe("WithAnnotations", func() {
		It("should add annotations to the builder", func() {
			annotations := map[string]string{"description": "test"}
			result := cmBuilder.WithAnnotations(annotations)

			Expect(result).To(Equal(cmBuilder))
			Expect(cmBuilder.Annotations).To(HaveKeyWithValue("description", "test"))
		})
	})

	Describe("AddData", func() {
		It("should add a data entry", func() {
			result := cmBuilder.AddData("config.yaml", "key: value")

			Expect(result).To(Equal(cmBuilder))
			Expect(cmBuilder.Data).To(HaveKeyWithValue("config.yaml", "key: value"))
		})
	})

	Describe("AddBinaryData", func() {
		It("should add a binary data entry", func() {
			binaryData := []byte{0x01, 0x02, 0x03}
			result := cmBuilder.AddBinaryData("binary.dat", binaryData)

			Expect(result).To(Equal(cmBuilder))
			Expect(cmBuilder.BinaryData).To(HaveKey("binary.dat"))
		})
	})

	Describe("WithConfigFiles", func() {
		It("should set multiple config files", func() {
			files := map[string]string{
				"config.yaml": "key: value",
				"app.conf":    "setting=true",
			}
			result := cmBuilder.WithConfigFiles(files)

			Expect(result).To(Equal(cmBuilder))
			Expect(cmBuilder.Data).To(HaveKeyWithValue("config.yaml", "key: value"))
			Expect(cmBuilder.Data).To(HaveKeyWithValue("app.conf", "setting=true"))
		})
	})

	Describe("Build", func() {
		It("should build a valid ConfigMap", func() {
			cm := cmBuilder.
				WithLabels(map[string]string{"app": "test"}).
				AddData("config.yaml", "key: value").
				Build()

			Expect(cm).NotTo(BeNil())
			Expect(cm.Name).To(Equal(name))
			Expect(cm.Namespace).To(Equal(namespace))
			Expect(cm.Data).To(HaveKeyWithValue("config.yaml", "key: value"))
		})

		It("should include binary data in the ConfigMap", func() {
			binaryData := []byte{0x01, 0x02, 0x03}
			cm := cmBuilder.
				AddBinaryData("binary.dat", binaryData).
				Build()

			Expect(cm.BinaryData).To(HaveKey("binary.dat"))
		})
	})

	Describe("NamespacedName", func() {
		It("should return the correct NamespacedName", func() {
			nn := cmBuilder.NamespacedName()

			Expect(nn.Name).To(Equal(name))
			Expect(nn.Namespace).To(Equal(namespace))
		})
	})

	Describe("WithMergedConfig", func() {
		It("should return builder unchanged when config is nil", func() {
			result, err := cmBuilder.WithMergedConfig(nil, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(cmBuilder))
		})

		It("should return builder unchanged when generator is nil", func() {
			cfg := &config.MergedConfig{
				ConfigFiles: map[string]map[string]string{
					"test.properties": {"key": "value"},
				},
			}
			result, err := cmBuilder.WithMergedConfig(cfg, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(cmBuilder))
		})

		It("should add config files from merged config", func() {
			cfg := &config.MergedConfig{
				ConfigFiles: map[string]map[string]string{
					"server.properties": {"port": "8080", "host": "localhost"},
				},
			}
			generator := config.NewMultiFormatConfigGenerator()
			generator.RegisterDefaultFormats()

			result, err := cmBuilder.WithMergedConfig(cfg, generator)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(cmBuilder))
			Expect(cmBuilder.Data).To(HaveKey("server.properties"))
			Expect(cmBuilder.Data["server.properties"]).To(ContainSubstring("port=8080"))
		})

		It("should add multiple config files from merged config", func() {
			cfg := &config.MergedConfig{
				ConfigFiles: map[string]map[string]string{
					"server.properties": {"port": "8080"},
					"app.yaml":          {"name": "test-app"},
				},
			}
			generator := config.NewMultiFormatConfigGenerator()
			generator.RegisterDefaultFormats()

			result, err := cmBuilder.WithMergedConfig(cfg, generator)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(cmBuilder))
			Expect(cmBuilder.Data).To(HaveKey("server.properties"))
			Expect(cmBuilder.Data).To(HaveKey("app.yaml"))
		})

		It("should build ConfigMap with merged config data", func() {
			cfg := &config.MergedConfig{
				ConfigFiles: map[string]map[string]string{
					"server.properties": {"port": "8080", "host": "localhost"},
				},
			}
			generator := config.NewMultiFormatConfigGenerator()
			generator.RegisterDefaultFormats()

			cmBuilder.WithMergedConfig(cfg, generator)
			cm := cmBuilder.Build()

			Expect(cm.Data).To(HaveKey("server.properties"))
		})

		It("should handle empty config files", func() {
			cfg := &config.MergedConfig{
				ConfigFiles: map[string]map[string]string{},
			}
			generator := config.NewMultiFormatConfigGenerator()
			generator.RegisterDefaultFormats()

			result, err := cmBuilder.WithMergedConfig(cfg, generator)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(cmBuilder))
			Expect(cmBuilder.Data).To(BeEmpty())
		})

		It("should return error when generator fails", func() {
			cfg := &config.MergedConfig{
				ConfigFiles: map[string]map[string]string{
					"test.fail": {"key": "value"},
				},
			}
			generator := config.NewMultiFormatConfigGenerator()
			// Register a failing format for .fail extension
			generator.RegisterFormat(".fail", &failingFormat{})

			result, err := cmBuilder.WithMergedConfig(cfg, generator)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to generate config files"))
			Expect(result).To(Equal(cmBuilder))
		})
	})
})
