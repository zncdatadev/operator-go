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
	"bufio"
	"fmt"
	"sort"
	"strings"
)

// INIAdapter converts between map[string]string and flat INI file format (no sections).
// It writes flat key = value pairs and supports reading flat INI files. It implements both
// ConfigMarshaler and the optional ConfigUnmarshaler.
//
// Note: This adapter only supports flat INI files (no [section] headers).
// If a section header is encountered during Unmarshal, an error is returned.
// This avoids silently losing data when multiple sections define the same key.
type INIAdapter struct{}

// NewINIAdapter creates a new INIAdapter.
func NewINIAdapter() *INIAdapter {
	return &INIAdapter{}
}

// Marshal converts a configuration map to flat INI format.
// Keys are sorted for deterministic output.
// Output format: key = value (one per line).
//
// INI has no standard escape syntax, so entries that cannot be written unambiguously are
// rejected instead of emitted as content that reads back as something else (see
// validateINIEntry).
func (a *INIAdapter) Marshal(data map[string]string) (string, error) {
	if len(data) == 0 {
		return "", nil
	}

	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	for _, key := range keys {
		if err := validateINIEntry(key, data[key]); err != nil {
			return "", err
		}
		fmt.Fprintf(&sb, "%s = %s\n", key, data[key])
	}
	return sb.String(), nil
}

// validateINIEntry rejects a key/value pair that flat INI cannot represent:
//   - a line break in either half would split the entry, injecting a second key;
//   - a separator ('=' or ':') in the key would move the split point, renaming the key and
//     prefixing the value;
//   - a key opening with '[', '#' or ';' would be re-read as a section header or a comment.
func validateINIEntry(key, value string) error {
	if strings.ContainsAny(key, "\n\r") || strings.ContainsAny(value, "\n\r") {
		return serializeError("ini", fmt.Errorf(
			"key %q: line breaks cannot be represented in INI (they would inject a new entry)", key))
	}
	if strings.ContainsAny(key, "=:") {
		return serializeError("ini", fmt.Errorf(
			"key %q: a key cannot contain the separator characters '=' or ':'", key))
	}
	if strings.HasPrefix(key, "[") || strings.HasPrefix(key, "#") || strings.HasPrefix(key, ";") {
		return serializeError("ini", fmt.Errorf(
			"key %q: a key cannot start with '[', '#' or ';' (it would be read as a section header or a comment)", key))
	}
	return nil
}

// Unmarshal converts flat INI file content to a map. It is the optional ConfigUnmarshaler half
// of the adapter.
//
// Supports:
//   - key = value and key=value (with or without spaces)
//   - # and ; comment lines
//   - Blank lines
//
// Surrounding whitespace is trimmed from both halves (the "key = value" spacing is not part of
// the data), so a value written with leading or trailing spaces does not round-trip.
//
// Returns an error if a [section] header is encountered, since section-aware
// INI files cannot be safely represented as a flat map[string]string.
func (a *INIAdapter) Unmarshal(data string) (map[string]string, error) {
	result := make(map[string]string)

	scanner := bufio.NewScanner(strings.NewReader(data))
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip blank lines and comments
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		// Reject section headers — flat INI only
		if strings.HasPrefix(line, "[") {
			return nil, parseError("ini", fmt.Errorf(
				"line %d: section headers are not supported; use flat INI (key = value) format only", lineNum))
		}

		sepIdx := strings.IndexAny(line, "=:")
		if sepIdx == -1 {
			// Key-only line (treat as empty value)
			result[strings.TrimSpace(line)] = ""
			continue
		}

		key := strings.TrimSpace(line[:sepIdx])
		value := strings.TrimSpace(line[sepIdx+1:])
		result[key] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, parseError("ini", fmt.Errorf("failed to scan INI content: %w", err))
	}

	return result, nil
}
