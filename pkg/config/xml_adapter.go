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

// XMLAdapter converts between map and Hadoop XML configuration format.
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
		sb.WriteString("  <property>\n")
		fmt.Fprintf(&sb, "    <name>%s</name>\n", escapeXML(key))
		fmt.Fprintf(&sb, "    <value>%s</value>\n", escapeXML(value))
		sb.WriteString("  </property>\n")
	}

	sb.WriteString("</configuration>\n")
	return sb.String(), nil
}

// Unmarshal converts Hadoop XML configuration to a map.
func (a *XMLAdapter) Unmarshal(data string) (map[string]string, error) {
	var config XMLConfiguration
	if err := xml.Unmarshal([]byte(data), &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal XML: %w", err)
	}

	result := make(map[string]string, len(config.Properties))
	for _, prop := range config.Properties {
		result[prop.Name] = prop.Value
	}

	return result, nil
}

// escapeXML escapes special characters for XML content.
func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}
