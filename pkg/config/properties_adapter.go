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

const (
	escapeBackslash = "\\\\"
	escapeEqual     = "\\="
	escapeColon     = "\\:"
	escapeSpace     = "\\ "
	escapeN         = "\\n"
	escapeR         = "\\r"
	escapeT         = "\\t"
)

// PropertiesAdapter converts between map and Java .properties format.
type PropertiesAdapter struct {
	// Separator is the character used to separate key and value.
	Separator string
}

// NewPropertiesAdapter creates a new PropertiesAdapter.
func NewPropertiesAdapter() *PropertiesAdapter {
	return &PropertiesAdapter{
		Separator: "=",
	}
}

// Marshal converts a configuration map to Java .properties format.
// The output format is:
// key1=value1
// key2=value2
func (a *PropertiesAdapter) Marshal(data map[string]string) (string, error) {
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
		sb.WriteString(escapePropertiesKey(key))
		sb.WriteString(a.Separator)
		sb.WriteString(escapePropertiesValue(value))
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

// Unmarshal converts Java .properties format to a map.
// Supports:
// - key=value
// - key:value
// - key value
// - Comments starting with # or !
// - Line continuations with backslash
func (a *PropertiesAdapter) Unmarshal(data string) (map[string]string, error) {
	result := make(map[string]string)

	scanner := bufio.NewScanner(strings.NewReader(data))
	var line string
	var continuedLine string

	for scanner.Scan() {
		line = scanner.Text()

		// Handle line continuation
		if strings.HasSuffix(line, "\\") {
			continuedLine += strings.TrimSuffix(line, "\\")
			continue
		}

		if continuedLine != "" {
			line = continuedLine + line
			continuedLine = ""
		}

		// Skip empty lines and comments
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}

		// Find the separator
		sepIndex := -1
		for i, c := range line {
			if c == '=' || c == ':' || (c == ' ' && i > 0) {
				sepIndex = i
				break
			}
		}

		if sepIndex == -1 {
			// Key with no value
			result[unescapeProperties(line)] = ""
			continue
		}

		key := strings.TrimSpace(line[:sepIndex])
		value := strings.TrimSpace(line[sepIndex+1:])

		// Don't trim the value - preserve leading spaces if escaped
		if len(value) > 0 && value[0] == ' ' {
			value = value[1:]
		}

		result[unescapeProperties(key)] = unescapeProperties(value)
	}

	if err := scanner.Err(); err != nil {
		return nil, common.ConfigParseError("properties", fmt.Errorf("failed to scan properties: %w", err))
	}

	return result, nil
}

// escapePropertiesKey escapes special characters in property keys.
func escapePropertiesKey(s string) string {
	s = strings.ReplaceAll(s, "\\", escapeBackslash)
	s = strings.ReplaceAll(s, "=", escapeEqual)
	s = strings.ReplaceAll(s, ":", escapeColon)
	s = strings.ReplaceAll(s, " ", escapeSpace)
	s = strings.ReplaceAll(s, "\n", escapeN)
	s = strings.ReplaceAll(s, "\r", escapeR)
	s = strings.ReplaceAll(s, "\t", escapeT)
	return s
}

// escapePropertiesValue escapes special characters in property values.
func escapePropertiesValue(s string) string {
	s = strings.ReplaceAll(s, "\\", escapeBackslash)
	s = strings.ReplaceAll(s, "\n", escapeN)
	s = strings.ReplaceAll(s, "\r", escapeR)
	s = strings.ReplaceAll(s, "\t", escapeT)
	return s
}

// unescapeProperties unescapes a properties key or value.
func unescapeProperties(s string) string {
	// Handle common escape sequences
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
			case '\\', '=', ':', ' ', '#', '!':
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
