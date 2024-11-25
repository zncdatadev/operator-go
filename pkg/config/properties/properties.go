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

package properties

import (
	"bufio"
	"fmt"
	"os"
	"slices"
	"strings"
)

type Properties struct {
	data map[string]string
	keys []string
}

func NewProperties() *Properties {
	return &Properties{
		data: make(map[string]string),
		keys: []string{},
	}
}

func NewPropertiesFromFile(filename string) (*Properties, error) {
	p := NewProperties()
	err := p.LoadFromFile(filename)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func NewPropertiesFromMap(data map[string]string) *Properties {
	p := NewProperties()
	p.LoadFromMap(data)
	return p
}

func (p *Properties) LoadFromFile(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer func() {
		if e := file.Close(); e != nil {
			err = e
		}
	}()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			p.data[key] = value
			p.keys = append(p.keys, key)
		}
	}
	p.Sort()
	return scanner.Err()
}

func (p *Properties) LoadFromMap(data map[string]string) {
	for key, value := range data {
		p.data[key] = value
		p.keys = append(p.keys, key)
	}
	p.Sort()
}

func (p *Properties) Save(filename string) error {
	content, err := p.Marshal()
	if err != nil {
		return err
	}

	return os.WriteFile(filename, []byte(content), 0644)
}

func (p *Properties) Marshal() (string, error) {
	var builder strings.Builder
	for _, key := range p.keys {
		value := p.data[key]
		if _, err := fmt.Fprintf(&builder, "%s=%s\n", key, value); err != nil {
			return "", err
		}
	}
	return builder.String(), nil
}

func (p *Properties) Add(key, value string) {
	if _, exists := p.data[key]; !exists {
		p.keys = append(p.keys, key)
	}
	p.data[key] = value
	p.Sort()
}

func (p *Properties) Delete(key string) {
	if _, exists := p.data[key]; exists {
		delete(p.data, key)
		for i, k := range p.keys {
			if k == key {
				p.keys = append(p.keys[:i], p.keys[i+1:]...)
				break
			}
		}
	}
	p.Sort()
}

func (p *Properties) Get(key string) (string, bool) {
	value, exists := p.data[key]
	return value, exists
}

func (p *Properties) Keys() []string {
	return p.keys
}

func (p *Properties) Sort() {
	slices.Sort(p.keys)
}
