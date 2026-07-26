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
	"bytes"
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

// yamlStringTag is the YAML resolved tag that forces a scalar to stay a string.
const yamlStringTag = "!!str"

// YAMLAdapter converts between map and YAML format. It implements both ConfigMarshaler and the
// optional ConfigUnmarshaler.
//
// Both directions go through a real YAML emitter/parser, so values are byte-faithful: a value
// containing a colon, a backslash, a quote or a newline is quoted (or folded into a block
// scalar) as the spec requires, and a value that looks like another scalar type ("true", "123",
// "null") keeps its string type through a round trip.
type YAMLAdapter struct{}

// NewYAMLAdapter creates a new YAMLAdapter.
func NewYAMLAdapter() *YAMLAdapter {
	return &YAMLAdapter{}
}

// Marshal converts a configuration map to a flat key-value YAML document.
// Keys are emitted in sorted order for deterministic output.
func (a *YAMLAdapter) Marshal(data map[string]string) (string, error) {
	if len(data) == 0 {
		return "", nil
	}

	// Sort keys for deterministic output
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build the mapping node by hand rather than marshalling the map directly, to fix the key
	// order. Values are emitted untagged so the encoder picks the plain form when it is
	// unambiguous and quotes only what would otherwise be invalid or re-read differently: the
	// values arrive as Go strings because configOverrides is map[string]map[string]string, but
	// the generated file is read by the product, and `port: 8080` must stay a YAML number there.
	// The empty string is the one case the plain form loses — it would emit a bare `key:`, which
	// re-reads as null — so it keeps the explicit string tag.
	root := &yaml.Node{Kind: yaml.MappingNode}
	for _, key := range keys {
		value := &yaml.Node{Kind: yaml.ScalarNode, Value: data[key]}
		if data[key] == "" {
			value.Tag = yamlStringTag
		}
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: yamlStringTag, Value: key},
			value,
		)
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return "", serializeError("yaml", err)
	}
	if err := enc.Close(); err != nil {
		return "", serializeError("yaml", err)
	}

	return buf.String(), nil
}

// Unmarshal converts a flat key-value YAML document to a map. It is the optional
// ConfigUnmarshaler half of the adapter.
//
// Every value is taken as its literal string, so a quoted, folded or type-looking scalar
// ("true", "123") yields the same string Marshal was given. Nested structures are NOT
// supported: a document that is not a mapping, or a mapping whose value is itself a mapping or
// a sequence, is rejected instead of being silently flattened into a wrong value.
func (a *YAMLAdapter) Unmarshal(data string) (map[string]string, error) {
	result := make(map[string]string)

	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(data), &doc); err != nil {
		return nil, parseError("yaml", err)
	}
	// An empty (or comment-only) document decodes to a zero node.
	if doc.Kind == 0 || len(doc.Content) == 0 {
		return result, nil
	}

	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, parseError("yaml", fmt.Errorf("expected a key-value mapping at the document root"))
	}

	for i := 0; i+1 < len(root.Content); i += 2 {
		key, value := root.Content[i], root.Content[i+1]
		if key.Kind != yaml.ScalarNode {
			return nil, parseError("yaml", fmt.Errorf("keys must be scalars; got a complex key"))
		}
		if value.Kind != yaml.ScalarNode {
			return nil, parseError("yaml", fmt.Errorf(
				"key %q has a nested value; only flat key-value documents are supported", key.Value))
		}
		// A duplicate key is invalid YAML: silently keeping one of the two would hand the caller
		// a value the document does not unambiguously carry.
		if _, exists := result[key.Value]; exists {
			return nil, parseError("yaml", fmt.Errorf("key %q is defined more than once", key.Value))
		}
		result[key.Value] = value.Value
	}

	return result, nil
}
