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

// ConfigFormat defines the interface for configuration serialization.
// Implementations convert between map[string]string and file content.
type ConfigFormat interface {
	// Marshal converts a configuration map to file content.
	Marshal(data map[string]string) (string, error)

	// Unmarshal converts file content to a configuration map.
	Unmarshal(data string) (map[string]string, error)
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

// GetFormat returns the appropriate ConfigFormat for the given type.
func GetFormat(formatType ConfigFormatType) ConfigFormat {
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
