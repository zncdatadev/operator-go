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
	"regexp"
	"sort"
	"strings"
)

// EnvAdapter converts between map and environment variable format. It implements both
// ConfigMarshaler and the optional ConfigUnmarshaler.
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
//
// The output is meant to be sourced by a POSIX shell, so a key that is not a valid shell
// variable name is an error rather than a line the shell would choke on (see validateEnvKey).
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
		if err := validateEnvKey(key); err != nil {
			return "", err
		}
		if a.ExportPrefix {
			sb.WriteString("export ")
		}
		sb.WriteString(key)
		sb.WriteString("=")
		sb.WriteString(escapeEnvValue(value))
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

// Unmarshal converts environment variable format to a map. It is the optional ConfigUnmarshaler
// half of the adapter.
//
// Supports:
// - KEY=value
// - export KEY=value
// - Comments starting with #
//
// A single-quoted value is taken literally, as a POSIX shell does: no escape sequence inside it
// means anything, so 'a\nb' is the four characters a, backslash, n, b.
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
		literal := false
		if len(value) >= 2 {
			switch {
			case value[0] == '"' && value[len(value)-1] == '"':
				value = value[1 : len(value)-1]
			case value[0] == '\'' && value[len(value)-1] == '\'':
				value = value[1 : len(value)-1]
				literal = true
			}
		}

		if literal {
			result[key] = value
			continue
		}
		result[key] = unescapeEnvValue(value)
	}

	return result, nil
}

// envKeyPattern is the POSIX-portable shell variable name.
var envKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// validateEnvKey rejects keys that are not valid shell variable names. Keys are never quoted or
// escaped (the shell has no syntax for it), so an invalid name would produce a file that a
// `source` of the output rejects with a syntax error — or, worse, that the shell reads as a
// command.
func validateEnvKey(s string) error {
	if !envKeyPattern.MatchString(s) {
		return serializeError("env", fmt.Errorf(
			"invalid environment variable name %q: must match %s", s, envKeyPattern))
	}
	return nil
}

// envBareValuePattern is the set of characters a POSIX shell reads verbatim on the right-hand
// side of an assignment. It is an allowlist, not a denylist of the characters that happen to
// break today: a value carrying anything else — a command separator (';', '&', '|'), a
// redirection ('<', '>'), a subshell ('(', ')'), a tilde the shell would expand to a home
// directory, whitespace that ends the assignment word — must be quoted, or sourcing the file
// would run the rest of the value as a command instead of assigning it.
var envBareValuePattern = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

// escapeEnvValue escapes special characters in environment variable values.
//
// The quoted form is double quotes, where a POSIX shell still performs parameter expansion
// ("$VAR"), command substitution ("$(...)" and backticks) and line continuation. Every
// character carrying that meaning is therefore backslash-escaped, so sourcing the output can
// never expand or execute a config value. "\n", "\r" and "\t" are dotenv-style escapes rather
// than literal bytes (unescapeEnvValue reverses them); a shell reads them back as the two
// characters, not as the control character.
func escapeEnvValue(s string) string {
	if envBareValuePattern.MatchString(s) {
		return s
	}
	escaped := strings.ReplaceAll(s, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
	escaped = strings.ReplaceAll(escaped, "$", "\\$")
	escaped = strings.ReplaceAll(escaped, "`", "\\`")
	escaped = strings.ReplaceAll(escaped, "\n", "\\n")
	escaped = strings.ReplaceAll(escaped, "\r", "\\r")
	escaped = strings.ReplaceAll(escaped, "\t", "\\t")
	return fmt.Sprintf("\"%s\"", escaped)
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
