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
	"encoding/xml"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// XMLProperty represents a single property in Hadoop XML configuration.
type XMLProperty struct {
	Name  string `xml:"name"`
	Value string `xml:"value"`
}

// XMLConfiguration represents the root element of Hadoop XML configuration.
type XMLConfiguration struct {
	XMLName    xml.Name      `xml:"configuration"`
	Properties []XMLProperty `xml:"property"`
	XMLHeader  string        `xml:",innerxml"`
}

// XMLAdapter converts between map and Hadoop XML configuration format. It implements both
// ConfigMarshaler and the optional ConfigUnmarshaler.
type XMLAdapter struct{}

// NewXMLAdapter creates a new XMLAdapter.
func NewXMLAdapter() *XMLAdapter {
	return &XMLAdapter{}
}

// Marshal converts a configuration map to Hadoop XML format.
// The output format is:
// <?xml version="1.0" encoding="UTF-8"?>
// <?xml-stylesheet type="text/xsl" href="configuration.xsl"?>
// <configuration>
//
//	<property>
//	  <name>key</name>
//	  <value>value</value>
//	</property>
//
// </configuration>
//
// A key or value XML 1.0 cannot carry is rejected rather than written into a document no parser
// would accept (see validateXMLText).
func (a *XMLAdapter) Marshal(data map[string]string) (string, error) {
	if len(data) == 0 {
		return `<?xml version="1.0" encoding="UTF-8"?>
<?xml-stylesheet type="text/xsl" href="configuration.xsl"?>
<configuration>
</configuration>`, nil
	}

	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<?xml-stylesheet type="text/xsl" href="configuration.xsl"?>
<configuration>
`)

	// Sort keys for deterministic output
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		value := data[key]
		if err := validateXMLText(key, key); err != nil {
			return "", err
		}
		if err := validateXMLText(key, value); err != nil {
			return "", err
		}
		sb.WriteString("  <property>\n")
		fmt.Fprintf(&sb, "    <name>%s</name>\n", escapeXML(key))
		fmt.Fprintf(&sb, "    <value>%s</value>\n", escapeXML(value))
		sb.WriteString("  </property>\n")
	}

	sb.WriteString("</configuration>\n")
	return sb.String(), nil
}

// Unmarshal converts Hadoop XML configuration to a map. It is the optional ConfigUnmarshaler
// half of the adapter.
func (a *XMLAdapter) Unmarshal(data string) (map[string]string, error) {
	var config XMLConfiguration
	if err := xml.Unmarshal([]byte(data), &config); err != nil {
		return nil, parseError("xml", err)
	}

	result := make(map[string]string, len(config.Properties))
	for _, prop := range config.Properties {
		result[prop.Name] = prop.Value
	}

	return result, nil
}

// validateXMLText rejects text XML 1.0 cannot represent at all: the C0 control characters other
// than tab, newline and carriage return have no character reference and no literal form, and a
// byte sequence that is not UTF-8 cannot appear in a document declared as UTF-8. Emitting either
// would produce a file every conforming parser rejects — including this adapter's own Unmarshal.
// The key names the offending entry whichever half of it the text came from.
func validateXMLText(key, text string) error {
	if !utf8.ValidString(text) {
		return serializeError("xml", fmt.Errorf(
			"key %q: a name or value must be valid UTF-8 to appear in an XML document", key))
	}
	for _, r := range text {
		if r < 0x20 && r != '\t' && r != '\n' && r != '\r' {
			return serializeError("xml", fmt.Errorf(
				"key %q: the control character %#U cannot be represented in XML 1.0", key, r))
		}
	}
	return nil
}

// escapeXML escapes special characters for XML content.
//
// A carriage return is written as a character reference because an XML parser normalizes literal
// "\r\n" and "\r" in content to "\n" (XML 1.0 §2.11): left literal, it would come back as a
// different string than it went out as.
func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	s = strings.ReplaceAll(s, "\r", "&#13;")
	return s
}
