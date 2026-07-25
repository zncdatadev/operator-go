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

import "fmt"

// ConfigMarshaler emits configuration file content from a key-value map. Emitting is the entire
// required contract of a configuration format: the framework's write path — the generators,
// BaseRoleGroupHandler and ConfigMapBuilder — never reads a generated file back, so a format a
// product only needs to WRITE is complete with Marshal alone.
type ConfigMarshaler interface {
	// Marshal converts a configuration map to file content.
	Marshal(data map[string]string) (string, error)
}

// ConfigUnmarshaler reads a format's own output back into a key-value map. It is an optional
// capability layered on ConfigMarshaler: the Parse paths discover it at runtime, so an emit-only
// format registers and generates like any other and only an actual parse attempt fails, with an
// UnsupportedParseError naming the format.
type ConfigUnmarshaler interface {
	// Unmarshal converts file content to a configuration map.
	Unmarshal(data string) (map[string]string, error)
}

// Every adapter shipped with the SDK round-trips, so the Parse paths accept all of them. A
// product's own adapter is free to implement ConfigMarshaler only.
var (
	_ ConfigUnmarshaler = (*XMLAdapter)(nil)
	_ ConfigUnmarshaler = (*PropertiesAdapter)(nil)
	_ ConfigUnmarshaler = (*YAMLAdapter)(nil)
	_ ConfigUnmarshaler = (*EnvAdapter)(nil)
	_ ConfigUnmarshaler = (*INIAdapter)(nil)
)

// unmarshalerFor upgrades a format to its optional parsing half. Whether a format can parse is
// not expressible in the static type of a registered adapter — registration requires
// ConfigMarshaler alone — so this is the single place the package inspects a dynamic type, and
// every Parse path funnels through it to fail the same, named way.
func unmarshalerFor(format ConfigMarshaler, formatName, file string) (ConfigUnmarshaler, error) {
	unmarshaler, ok := format.(ConfigUnmarshaler)
	if !ok {
		return nil, &UnsupportedParseError{Format: formatName, File: file}
	}
	return unmarshaler, nil
}

// formatName labels a format in an error message. The name the caller knows (a registered file
// extension or a requested ConfigFormatType) says which entry is at fault; the adapter's Go type
// says which implementation has to change, which is the part a product cannot guess when the
// format was registered elsewhere.
func formatName(requested string, format ConfigMarshaler) string {
	if requested == "" {
		return fmt.Sprintf("%T", format)
	}
	return fmt.Sprintf("%s (%T)", requested, format)
}

// ConfigFormatType represents a configuration file format type.
type ConfigFormatType string

const (
	// FormatXML represents Hadoop XML configuration format.
	FormatXML ConfigFormatType = "xml"
	// FormatProperties represents Java .properties format.
	FormatProperties ConfigFormatType = "properties"
	// FormatYAML represents YAML format.
	FormatYAML ConfigFormatType = "yaml"
	// FormatEnv represents environment variable format.
	FormatEnv ConfigFormatType = "env"
	// FormatINI represents INI format.
	FormatINI ConfigFormatType = "ini"
)

// GetFormat returns the adapter for the given format type. Every shipped adapter also implements
// ConfigUnmarshaler, so the result can be parsed with as well as generated from.
func GetFormat(formatType ConfigFormatType) ConfigMarshaler {
	switch formatType {
	case FormatXML:
		return NewXMLAdapter()
	case FormatProperties:
		return NewPropertiesAdapter()
	case FormatYAML:
		return NewYAMLAdapter()
	case FormatEnv:
		return NewEnvAdapter()
	case FormatINI:
		return NewINIAdapter()
	default:
		return NewPropertiesAdapter() // Default fallback
	}
}
