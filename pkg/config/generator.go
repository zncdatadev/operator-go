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
	"fmt"

	"github.com/zncdatadev/operator-go/pkg/common"
)

// ConfigGenerator generates configuration files using format adapters.
type ConfigGenerator struct {
	format ConfigFormat
}

// NewConfigGenerator creates a new ConfigGenerator with the specified format.
func NewConfigGenerator(format ConfigFormat) *ConfigGenerator {
	return &ConfigGenerator{format: format}
}

// NewConfigGeneratorWithType creates a new ConfigGenerator with a format type.
func NewConfigGeneratorWithType(formatType ConfigFormatType) *ConfigGenerator {
	return &ConfigGenerator{format: GetFormat(formatType)}
}

// Generate generates configuration file content from a map.
func (g *ConfigGenerator) Generate(config map[string]string) (string, error) {
	if g.format == nil {
		return "", fmt.Errorf("no format configured")
	}
	return g.format.Marshal(config)
}

// Parse parses configuration file content into a map.
func (g *ConfigGenerator) Parse(content string) (map[string]string, error) {
	if g.format == nil {
		return nil, fmt.Errorf("no format configured")
	}
	return g.format.Unmarshal(content)
}

// GenerateFiles generates multiple configuration files.
// Returns a map of filename to file content.
func (g *ConfigGenerator) GenerateFiles(configFiles map[string]map[string]string) (map[string]string, error) {
	result := make(map[string]string)

	for filename, config := range configFiles {
		content, err := g.Generate(config)
		if err != nil {
			return nil, common.ConfigParseError(filename, fmt.Errorf("failed to generate %s: %w", filename, err))
		}
		result[filename] = content
	}

	return result, nil
}

// MultiFormatConfigGenerator generates configuration files with different formats.
type MultiFormatConfigGenerator struct {
	formats map[string]ConfigFormat
}

// NewMultiFormatConfigGenerator creates a new MultiFormatConfigGenerator.
func NewMultiFormatConfigGenerator() *MultiFormatConfigGenerator {
	return &MultiFormatConfigGenerator{
		formats: make(map[string]ConfigFormat),
	}
}

// RegisterFormat registers a format for a specific file extension.
func (g *MultiFormatConfigGenerator) RegisterFormat(extension string, format ConfigFormat) {
	g.formats[extension] = format
}

// RegisterDefaultFormats registers all default formats.
func (g *MultiFormatConfigGenerator) RegisterDefaultFormats() {
	g.RegisterFormat(".xml", NewXMLAdapter())
	g.RegisterFormat(".properties", NewPropertiesAdapter())
	g.RegisterFormat(".yaml", NewYAMLAdapter())
	g.RegisterFormat(".yml", NewYAMLAdapter())
	g.RegisterFormat(".env", NewEnvAdapter())
	g.RegisterFormat(".ini", NewINIAdapter())
}

// Generate generates configuration file content with format detection based on extension.
func (g *MultiFormatConfigGenerator) Generate(filename string, config map[string]string) (string, error) {
	format := g.getFormatForFile(filename)
	if format == nil {
		format = NewPropertiesAdapter() // Default fallback
	}
	return format.Marshal(config)
}

// GenerateFiles generates multiple configuration files with format detection.
func (g *MultiFormatConfigGenerator) GenerateFiles(configFiles map[string]map[string]string) (map[string]string, error) {
	result := make(map[string]string)

	for filename, config := range configFiles {
		content, err := g.Generate(filename, config)
		if err != nil {
			return nil, common.ConfigParseError(filename, fmt.Errorf("failed to generate %s: %w", filename, err))
		}
		result[filename] = content
	}

	return result, nil
}

// getFormatForFile returns the format for a file based on its extension.
func (g *MultiFormatConfigGenerator) getFormatForFile(filename string) ConfigFormat {
	for ext, format := range g.formats {
		if len(filename) >= len(ext) && filename[len(filename)-len(ext):] == ext {
			return format
		}
	}
	return nil
}
