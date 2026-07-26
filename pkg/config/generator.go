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
	"strings"
)

// defaultFormatExtension is the extension whose adapter renders a file no registered extension
// matches. Properties is the fallback because it is the only format that escapes every key and
// value it is given, so an unrecognized file is still rendered deterministically.
const defaultFormatExtension = ".properties"

// ConfigGenerator generates configuration files using a single format adapter.
type ConfigGenerator struct {
	format ConfigMarshaler

	// name labels the format in errors: the requested ConfigFormatType, or the adapter's Go type
	// when the caller supplied an adapter directly.
	name string
}

// NewConfigGenerator creates a new ConfigGenerator with the specified format. The format only
// has to emit; Parse additionally requires it to implement ConfigUnmarshaler.
func NewConfigGenerator(format ConfigMarshaler) *ConfigGenerator {
	return &ConfigGenerator{format: format, name: formatName("", format)}
}

// NewConfigGeneratorWithType creates a new ConfigGenerator with a format type.
func NewConfigGeneratorWithType(formatType ConfigFormatType) *ConfigGenerator {
	format := GetFormat(formatType)
	// An unknown type falls back to properties, so the label carries both the requested type and
	// the adapter that actually answered it.
	return &ConfigGenerator{format: format, name: formatName(string(formatType), format)}
}

// Generate generates configuration file content from a map.
func (g *ConfigGenerator) Generate(config map[string]string) (string, error) {
	if g.format == nil {
		return "", ErrNoFormat
	}
	content, err := g.format.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to generate configuration with the %s format: %w", g.name, err)
	}
	return content, nil
}

// Parse parses configuration file content into a map.
//
// Parsing is the optional half of the format contract, so a format that only emits fails here
// with an *UnsupportedParseError naming it, rather than returning an empty map the caller would
// read as "the file held nothing".
func (g *ConfigGenerator) Parse(content string) (map[string]string, error) {
	if g.format == nil {
		return nil, ErrNoFormat
	}
	unmarshaler, err := unmarshalerFor(g.format, g.name, "")
	if err != nil {
		return nil, err
	}
	data, err := unmarshaler.Unmarshal(content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse configuration with the %s format: %w", g.name, err)
	}
	return data, nil
}

// GenerateFiles generates multiple configuration files.
// Returns a map of filename to file content.
func (g *ConfigGenerator) GenerateFiles(configFiles map[string]map[string]string) (map[string]string, error) {
	result := make(map[string]string, len(configFiles))

	for filename, config := range configFiles {
		content, err := g.Generate(config)
		if err != nil {
			return nil, fmt.Errorf("config file %q: %w", filename, err)
		}
		result[filename] = content
	}

	return result, nil
}

// MultiFormatConfigGenerator generates configuration files with different formats, selecting the
// adapter from the file name.
type MultiFormatConfigGenerator struct {
	formats map[string]ConfigMarshaler
}

// NewMultiFormatConfigGenerator creates a new MultiFormatConfigGenerator.
func NewMultiFormatConfigGenerator() *MultiFormatConfigGenerator {
	return &MultiFormatConfigGenerator{
		formats: make(map[string]ConfigMarshaler),
	}
}

// RegisterFormat registers a format for a specific file extension. The extension is matched as a
// file name suffix, so a whole file name ("server.properties") is a legal registration and takes
// precedence over a plain extension (".properties").
//
// The format only has to emit; registering an adapter that cannot parse is legal, and only Parse
// on that extension fails.
func (g *MultiFormatConfigGenerator) RegisterFormat(extension string, format ConfigMarshaler) {
	g.formats[extension] = format
}

// RegisterDefaultFormats registers all default formats.
func (g *MultiFormatConfigGenerator) RegisterDefaultFormats() {
	g.RegisterFormat(".xml", NewXMLAdapter())
	g.RegisterFormat(defaultFormatExtension, NewPropertiesAdapter())
	g.RegisterFormat(".yaml", NewYAMLAdapter())
	g.RegisterFormat(".yml", NewYAMLAdapter())
	g.RegisterFormat(".env", NewEnvAdapter())
	g.RegisterFormat(".ini", NewINIAdapter())
}

// Generate generates configuration file content with format detection based on the file name.
func (g *MultiFormatConfigGenerator) Generate(filename string, config map[string]string) (string, error) {
	format, name := g.formatForFile(filename)
	content, err := format.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to generate config file %q with the %s format: %w", filename, name, err)
	}
	return content, nil
}

// Parse parses configuration file content with format detection based on the file name.
//
// Parsing is the optional half of the format contract: a file whose format only emits fails with
// an *UnsupportedParseError naming both the file and the format.
func (g *MultiFormatConfigGenerator) Parse(filename, content string) (map[string]string, error) {
	format, name := g.formatForFile(filename)
	unmarshaler, err := unmarshalerFor(format, name, filename)
	if err != nil {
		return nil, err
	}
	data, err := unmarshaler.Unmarshal(content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config file %q with the %s format: %w", filename, name, err)
	}
	return data, nil
}

// GenerateFiles generates multiple configuration files with format detection.
func (g *MultiFormatConfigGenerator) GenerateFiles(configFiles map[string]map[string]string) (map[string]string, error) {
	result := make(map[string]string, len(configFiles))

	for filename, config := range configFiles {
		content, err := g.Generate(filename, config)
		if err != nil {
			return nil, err
		}
		result[filename] = content
	}

	return result, nil
}

// formatForFile returns the adapter for a file together with the label naming it in errors.
//
// The match is the LONGEST registered suffix, so a file-specific registration
// ("server.properties") wins over a generic one (".properties") whatever order Go's map iteration
// produces — an extension-length tie is impossible, since two suffixes of equal length matching
// the same name are the same string. A file that matches nothing falls back to properties.
func (g *MultiFormatConfigGenerator) formatForFile(filename string) (ConfigMarshaler, string) {
	var (
		best    ConfigMarshaler
		bestExt string
	)

	for ext, format := range g.formats {
		// A nil registration can never emit; treating it as unmatched keeps Generate off a nil
		// receiver and falls back to the default adapter.
		if format == nil || !strings.HasSuffix(filename, ext) {
			continue
		}
		if best == nil || len(ext) > len(bestExt) {
			best, bestExt = format, ext
		}
	}

	if best == nil {
		fallback := NewPropertiesAdapter()
		return fallback, formatName(defaultFormatExtension+" (default)", fallback)
	}

	return best, formatName(bestExt, best)
}
