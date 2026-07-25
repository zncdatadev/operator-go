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
	escapeHash      = "\\#"
	escapeBang      = "\\!"
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
//
// It parses the output of Marshal back into the original map, including keys and values holding
// a separator character, an edge space or a trailing backslash. Whitespace that is not escaped
// is layout (indentation, padding around the separator) and is dropped, which is what a
// hand-written file expects.
func (a *PropertiesAdapter) Unmarshal(data string) (map[string]string, error) {
	result := make(map[string]string)

	scanner := bufio.NewScanner(strings.NewReader(data))
	var continuedLine string

	for scanner.Scan() {
		line := scanner.Text()

		// A trailing backslash continues the line only when it is not itself escaped: an even
		// count ends in an escaped backslash (e.g. the value "C:\" is written as "C:\\"), which
		// terminates the entry.
		if trailingBackslashes(line)%2 == 1 {
			continuedLine += line[:len(line)-1]
			continue
		}

		if continuedLine != "" {
			line = continuedLine + line
			continuedLine = ""
		}

		parsePropertiesLine(line, result)
	}

	if err := scanner.Err(); err != nil {
		return nil, common.ConfigParseError("properties", fmt.Errorf("failed to scan properties: %w", err))
	}

	// A continuation that is never terminated (the input ends on a backslash) still carries an
	// entry; dropping it would silently lose the last key.
	if continuedLine != "" {
		parsePropertiesLine(continuedLine, result)
	}

	return result, nil
}

// parsePropertiesLine parses one logical (continuation-joined) line into result. Empty lines
// and comments contribute nothing.
func parsePropertiesLine(line string, result map[string]string) {
	line = trimPropertiesSpace(line)
	if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
		return
	}

	sepIndex := propertiesSeparatorIndex(line)
	if sepIndex == -1 {
		// Key with no value
		result[unescapeProperties(line)] = ""
		return
	}

	key := trimPropertiesSpace(line[:sepIndex])
	value := trimPropertiesSpace(line[sepIndex+1:])
	result[unescapeProperties(key)] = unescapeProperties(value)
}

// trimPropertiesSpace removes the whitespace .properties treats as layout — indentation before
// the key and the padding around the separator — while keeping whitespace that belongs to the
// token. An edge space is written escaped ("a\ =v" for the key "a "), so a trailing space
// preceded by an odd number of backslashes terminates the trim instead of being stripped along
// with its escape.
func trimPropertiesSpace(s string) string {
	start := 0
	for start < len(s) && isPropertiesSpace(s[start]) {
		start++
	}
	end := len(s)
	for end > start && isPropertiesSpace(s[end-1]) {
		if trailingBackslashes(s[start:end-1])%2 == 1 {
			break
		}
		end--
	}
	return s[start:end]
}

func isPropertiesSpace(c byte) bool {
	return c == ' ' || c == '\t'
}

// propertiesSeparatorIndex returns the byte index of the first UNESCAPED key/value separator,
// or -1 when the line carries a key only. Escaped separators belong to the key (Marshal writes
// "a\=b=v" for the key "a=b"), so skipping them is what makes the round trip work.
func propertiesSeparatorIndex(line string) int {
	escaped := false
	for i, c := range line {
		if escaped {
			escaped = false
			continue
		}
		switch c {
		case '\\':
			escaped = true
		case '=', ':':
			return i
		case ' ':
			if i > 0 {
				return i
			}
		}
	}
	return -1
}

// trailingBackslashes counts the consecutive backslashes at the end of s.
func trailingBackslashes(s string) int {
	n := 0
	for i := len(s) - 1; i >= 0 && s[i] == '\\'; i-- {
		n++
	}
	return n
}

// escapePropertiesKey escapes special characters in property keys. The comment markers are
// escaped too: a key starting with '#' or '!' would otherwise turn its whole entry into a
// comment and disappear on the way back in.
func escapePropertiesKey(s string) string {
	s = strings.ReplaceAll(s, "\\", escapeBackslash)
	s = strings.ReplaceAll(s, "=", escapeEqual)
	s = strings.ReplaceAll(s, ":", escapeColon)
	s = strings.ReplaceAll(s, " ", escapeSpace)
	s = strings.ReplaceAll(s, "#", escapeHash)
	s = strings.ReplaceAll(s, "!", escapeBang)
	s = strings.ReplaceAll(s, "\n", escapeN)
	s = strings.ReplaceAll(s, "\r", escapeR)
	s = strings.ReplaceAll(s, "\t", escapeT)
	return s
}

// escapePropertiesValue escapes special characters in property values. Only the value's edge
// spaces need escaping (a reader strips the padding around the separator); interior spaces are
// left readable, which matters for the values products actually write (JVM argument lists).
func escapePropertiesValue(s string) string {
	s = strings.ReplaceAll(s, "\\", escapeBackslash)
	s = strings.ReplaceAll(s, "\n", escapeN)
	s = strings.ReplaceAll(s, "\r", escapeR)
	s = strings.ReplaceAll(s, "\t", escapeT)
	return escapeEdgeSpaces(s)
}

// escapeEdgeSpaces escapes the leading and trailing spaces of an already-escaped value (tabs are
// escape sequences by then, so only spaces can still sit raw at an edge).
func escapeEdgeSpaces(s string) string {
	lead := 0
	for lead < len(s) && s[lead] == ' ' {
		lead++
	}
	trail := len(s)
	for trail > lead && s[trail-1] == ' ' {
		trail--
	}
	if lead == 0 && trail == len(s) {
		return s
	}
	return strings.Repeat(escapeSpace, lead) + s[lead:trail] + strings.Repeat(escapeSpace, len(s)-trail)
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
