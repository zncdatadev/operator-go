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

	"github.com/zncdatadev/operator-go/pkg/common"
)

// INIAdapter converts between map[string]string and flat INI file format (no sections).
// It writes flat key = value pairs and supports reading flat INI files.
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
		fmt.Fprintf(&sb, "%s = %s\n", key, data[key])
	}
	return sb.String(), nil
}

// Unmarshal converts flat INI file content to a map.
// Supports:
//   - key = value and key=value (with or without spaces)
//   - # and ; comment lines
//   - Blank lines
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
			return nil, common.ConfigParseError("ini", fmt.Errorf(
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
		return nil, common.ConfigParseError("ini", fmt.Errorf("failed to scan INI content: %w", err))
	}

	return result, nil
}
