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
	"sort"
	"strings"
)

// YAMLAdapter converts between map and YAML format.
type YAMLAdapter struct{}

// NewYAMLAdapter creates a new YAMLAdapter.
func NewYAMLAdapter() *YAMLAdapter {
	return &YAMLAdapter{}
}

// Marshal converts a configuration map to YAML format.
// This produces a simple key-value YAML file.
func (a *YAMLAdapter) Marshal(data map[string]string) (string, error) {
	if len(data) == 0 {
		return "", nil
	}

	var sb strings.Builder

	// Sort keys for deterministic output
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		value := data[key]
		sb.WriteString(formatYAMLKeyValue(key, value))
	}

	return sb.String(), nil
}

// Unmarshal converts YAML format to a map.
// This supports simple key-value pairs only.
// Note: Nested YAML structures (indentation-based) are NOT supported.
// For complex YAML with nested structures, consider using sigs.k8s.io/yaml or gopkg.in/yaml.v3.
func (a *YAMLAdapter) Unmarshal(data string) (map[string]string, error) {
	result := make(map[string]string)

	lines := strings.Split(data, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Find the separator
		sepIndex := strings.Index(line, ":")
		if sepIndex == -1 {
			continue
		}

		key := strings.TrimSpace(line[:sepIndex])
		value := strings.TrimSpace(line[sepIndex+1:])

		// Remove quotes from value if present
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		result[key] = value
	}

	return result, nil
}

// formatYAMLKeyValue formats a key-value pair for YAML output.
func formatYAMLKeyValue(key, value string) string {
	// Check if the value needs quoting
	needsQuoting := strings.ContainsAny(value, ":#{}\n") ||
		strings.HasPrefix(value, " ") ||
		strings.HasSuffix(value, " ")

	if needsQuoting {
		// Escape quotes and use double quotes
		escaped := strings.ReplaceAll(value, "\"", "\\\"")
		return fmt.Sprintf("%s: \"%s\"\n", key, escaped)
	}

	// If value is empty, output empty string
	if value == "" {
		return fmt.Sprintf("%s: \"\"\n", key)
	}

	return fmt.Sprintf("%s: %s\n", key, value)
}
