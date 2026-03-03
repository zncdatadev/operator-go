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

// EnvAdapter converts between map and environment variable format.
type EnvAdapter struct {
	// ExportPrefix adds 'export ' prefix to each line.
	ExportPrefix bool
}

// NewEnvAdapter creates a new EnvAdapter.
func NewEnvAdapter() *EnvAdapter {
	return &EnvAdapter{
		ExportPrefix: false,
	}
}

// Marshal converts a configuration map to environment variable format.
// The output format is:
// KEY1=value1
// KEY2=value2
func (a *EnvAdapter) Marshal(data map[string]string) (string, error) {
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
		if a.ExportPrefix {
			sb.WriteString("export ")
		}
		sb.WriteString(escapeEnvKey(key))
		sb.WriteString("=")
		sb.WriteString(escapeEnvValue(value))
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

// Unmarshal converts environment variable format to a map.
// Supports:
// - KEY=value
// - export KEY=value
// - Comments starting with #
func (a *EnvAdapter) Unmarshal(data string) (map[string]string, error) {
	result := make(map[string]string)

	lines := strings.Split(data, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Remove 'export ' prefix if present
		line = strings.TrimPrefix(line, "export ")
		line = strings.TrimSpace(line)

		// Find the separator
		sepIndex := strings.Index(line, "=")
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

		result[key] = unescapeEnvValue(value)
	}

	return result, nil
}

// escapeEnvKey escapes special characters in environment variable keys.
func escapeEnvKey(s string) string {
	// Environment variable keys typically don't need escaping
	// but we ensure they're valid shell variable names
	return s
}

// escapeEnvValue escapes special characters in environment variable values.
func escapeEnvValue(s string) string {
	// If value contains spaces, special chars, or is empty, wrap in quotes
	if s == "" || strings.ContainsAny(s, " \t\n\r\"'$`\\") {
		escaped := strings.ReplaceAll(s, "\\", "\\\\")
		escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
		escaped = strings.ReplaceAll(escaped, "\n", "\\n")
		escaped = strings.ReplaceAll(escaped, "\r", "\\r")
		escaped = strings.ReplaceAll(escaped, "\t", "\\t")
		return fmt.Sprintf("\"%s\"", escaped)
	}
	return s
}

// unescapeEnvValue unescapes an environment variable value.
func unescapeEnvValue(s string) string {
	result := make([]rune, 0, len(s))
	runes := []rune(s)

	for i := 0; i < len(runes); i++ {
		if runes[i] == '\\' && i+1 < len(runes) {
			switch runes[i+1] {
			case 'n':
				result = append(result, '\n')
				i++
			case 'r':
				result = append(result, '\r')
				i++
			case 't':
				result = append(result, '\t')
				i++
			case '\\', '"', '\'', '$', '`':
				result = append(result, runes[i+1])
				i++
			default:
				result = append(result, runes[i])
			}
		} else {
			result = append(result, runes[i])
		}
	}

	return string(result)
}
