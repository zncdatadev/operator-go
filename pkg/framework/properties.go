package framework

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"
)

// PropertiesCodec writes deterministic UTF-8 Java properties syntax. Properties
// are encoded as data: line breaks, separators and edge spaces cannot introduce
// another entry. Runtime support is provided by the matching materialization helper.
type PropertiesCodec struct{}

func (PropertiesCodec) Encode(values map[string]string) (string, error) {
	var output strings.Builder
	for _, key := range slices.Sorted(maps.Keys(values)) {
		value := values[key]
		if !utf8.ValidString(key) || !utf8.ValidString(value) {
			return "", fmt.Errorf("properties keys and values must be valid UTF-8")
		}
		output.WriteString(propertyKeyEscapes.Replace(key))
		output.WriteByte('=')
		output.WriteString(escapePropertyValue(value))
		output.WriteByte('\n')
	}
	return output.String(), nil
}

var propertyKeyEscapes = strings.NewReplacer(
	"\\", `\\`, "=", `\=`, ":", `\:`, " ", `\ `, "#", `\#`, "!", `\!`,
	"\n", `\n`, "\r", `\r`, "\t", `\t`, "\f", `\f`,
)

var propertyValueEscapes = strings.NewReplacer("\\", `\\`, "\n", `\n`, "\r", `\r`, "\t", `\t`, "\f", `\f`)

func escapePropertyValue(value string) string {
	escaped := propertyValueEscapes.Replace(value)
	first, last := 0, len(escaped)
	for first < last && escaped[first] == ' ' {
		first++
	}
	for last > first && escaped[last-1] == ' ' {
		last--
	}
	return strings.Repeat(`\ `, first) + escaped[first:last] + strings.Repeat(`\ `, len(escaped)-last)
}
