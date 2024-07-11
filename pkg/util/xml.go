package util

import (
	"encoding/xml"
	"slices"
	"strings"
)

const (
	XMLStylesheet = `<?xml-stylesheet type="text/xsl" href="configuration.xsl"?>` + "\n"
)

type configuration struct {
	XMLName    xml.Name   `xml:"configuration"`
	Properties []Property `xml:"property"`
}

type Property struct {
	XMLName     xml.Name `xml:"property"`
	Name        string   `xml:"name"`
	Value       string   `xml:"value"`
	Description string   `xml:"description,omitempty"`
}

type XMLConfiguration struct {
	Configuration *configuration
	Header        string
}

func NewXMLConfiguration() *XMLConfiguration {
	return &XMLConfiguration{
		Configuration: &configuration{},
		Header:        xml.Header + XMLStylesheet,
	}
}

func NewXMLConfigurationFromString(xmlData string) (*XMLConfiguration, error) {
	config := &XMLConfiguration{}
	headerEnd := strings.Index(xmlData, "<configuration>")
	if headerEnd != -1 {
		config.Header = xmlData[:headerEnd]
	}
	err := xml.Unmarshal([]byte(xmlData[headerEnd:]), &config.Configuration)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func NewXMLConfigurationFromMap(properties map[string]string) *XMLConfiguration {
	x := NewXMLConfiguration()
	x.AddPropertiesWithMap(properties)
	return x
}

func (x *XMLConfiguration) GetProperty(name string) (Property, bool) {
	for _, p := range x.Configuration.Properties {
		if p.Name == name {
			return p, true
		}
	}
	return Property{}, false
}

func (x *XMLConfiguration) AddProperty(p Property) {
	for i, existingProperty := range x.Configuration.Properties {
		if existingProperty.Name == p.Name {
			x.Configuration.Properties[i] = p // update
			return
		}
	}
	x.Configuration.Properties = append(x.Configuration.Properties, p) // add
}

func (x *XMLConfiguration) AddPropertyWithString(name, value, description string) {
	x.AddProperty(Property{Name: name, Value: value, Description: description})
}

func (x *XMLConfiguration) AddPropertiesWithMap(properties map[string]string) {
	for name, value := range properties {
		x.AddProperty(Property{Name: name, Value: value})
	}
}

func (x *XMLConfiguration) DeleteProperties(names ...string) {
	s := slices.DeleteFunc(x.Configuration.Properties, func(i Property) bool {
		for _, name := range names {
			if i.Name == name {
				return true
			}
		}
		return false
	})
	x.Configuration.Properties = s
}

func (x *XMLConfiguration) getHeader() string {
	if x.Header == "" {
		return xml.Header + XMLStylesheet
	}
	return x.Header
}

func (x *XMLConfiguration) Marshal() (string, error) {
	data, err := xml.MarshalIndent(x.Configuration, "", "    ")
	if err != nil {
		return "", err
	}

	fullXML := x.getHeader() + string(data) + "\n"

	// replace &#xA; with newline
	fixedXML := strings.ReplaceAll(fullXML, "&#xA;", "\n")

	return fixedXML, nil
}
