package util

import (
	"bufio"
	"fmt"
	"strings"
)

type NameValuePair struct {
	comment string
	Name    string
	Value   string
}

// MakeConfigFileContent returns the content of a configuration file
// content such as:
// ```
// key1 value1
// key2 value2
// ```
func MakeConfigFileContent(config map[string]string) string {
	content := ""
	if len(config) == 0 {
		return content
	}
	for k, v := range config {
		content += fmt.Sprintf("%s %s\n", k, v)
	}
	return content
}

// MakePropertiesFileContent returns the content of a properties file
// content such as:
// ```properties
// key1=value1
// key2=value2
// ```
func MakePropertiesFileContent(config map[string]string) string {
	content := ""
	if len(config) == 0 {
		return content
	}
	for k, v := range config {
		content += fmt.Sprintf("%s=%s\n", k, v)
	}
	return content
}

func OverrideConfigFileContent(current string, override string) string {
	if current == "" {
		return override
	}
	if override == "" {
		return current
	}
	return current + "\n" + override
}

// OverridePropertiesFileContent use bufio resolve properties
func OverridePropertiesFileContent(current string, override []NameValuePair) (string, error) {
	var properties []NameValuePair
	//scan current
	if err := ScanProperties(current, &properties); err != nil {
		logger.Error(err, "failed to scan current properties")
		return "", err
	}
	// override
	OverrideProperties(override, &properties)

	// to string
	var res string
	for _, v := range properties {
		res += fmt.Sprintf("%s%s=%s\n", v.comment, v.Name, v.Value)
	}
	return res, nil
}

func ScanProperties(current string, properties *[]NameValuePair) error {
	scanner := bufio.NewScanner(strings.NewReader(current))

	var comment string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") || len(line) == 0 {
			comment += line + "\n"
			continue
		}

		items := strings.Split(line, "=")
		if len(items) == 2 {
			*properties = append(*properties, NameValuePair{
				comment: comment,
				Name:    items[0],
				Value:   items[1],
			})
			comment = ""
		} else {
			return fmt.Errorf("invalid property line: %s", line)
		}
	}
	return scanner.Err()
}

func OverrideProperties(override []NameValuePair, current *[]NameValuePair) {
	if len(override) == 0 {
		return
	}
	var currentKeys = make(map[string]int)
	for i, v := range *current {
		currentKeys[v.Name] = i
	}

	for _, v := range override {
		if _, ok := currentKeys[v.Name]; ok {
			(*current)[currentKeys[v.Name]].Value = v.Value // override
		} else {
			// append new
			*current = append(*current, NameValuePair{
				Name:  v.Name,
				Value: v.Value,
			})
		}
	}
}
