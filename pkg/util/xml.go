package util

import (
	"encoding/xml"
	"slices"
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
	Comment     string   `xml:",comment"`
	Name        string   `xml:"name"`
	Value       string   `xml:"value"`
	Description string   `xml:"description,omitempty"`
}

type XMLConfiguration struct {
	Properties    []Property
	XMLStylesheet string
}

func NewXMLConfiguration() *XMLConfiguration {
	return &XMLConfiguration{
		Properties:    []Property{},
		XMLStylesheet: XMLStylesheet,
	}
}

func NewXMLConfigurationFromString(xmlString string) (*XMLConfiguration, error) {
	x := configuration{}
	err := xml.Unmarshal([]byte(xmlString), &x)
	if err != nil {
		return nil, err
	}
	return &XMLConfiguration{Properties: x.Properties, XMLStylesheet: XMLStylesheet}, nil
}

func NewXMLConfigurationFromMap(properties map[string]string) *XMLConfiguration {
	x := NewXMLConfiguration()
	x.AddPropertiesWithMap(properties)
	return x
}

func (x *XMLConfiguration) GetProperty(name string) (Property, bool) {
	for _, p := range x.Properties {
		if p.Name == name {
			return p, true
		}
	}
	return Property{}, false
}

func (x *XMLConfiguration) AddProperty(p Property) {
	for i, existingProperty := range x.Properties {
		if existingProperty.Name == p.Name {
			x.Properties[i] = p // update
			return
		}
	}
	x.Properties = append(x.Properties, p) // add
}

func (x *XMLConfiguration) AddPropertyWithString(name, value, description, comment string) {
	x.AddProperty(Property{Name: name, Value: value, Description: description, Comment: comment})
}

func (x *XMLConfiguration) AddPropertiesWithMap(properties map[string]string) {
	for name, value := range properties {
		x.AddProperty(Property{Name: name, Value: value})
	}
}

func (x *XMLConfiguration) DeleteProperties(names ...string) {
	s := slices.DeleteFunc(x.Properties, func(i Property) bool {
		for _, name := range names {
			if i.Name == name {
				return true
			}
		}
		return false
	})
	x.Properties = s
}

func (x *XMLConfiguration) getHeader() string {
	if x.XMLStylesheet == "" {
		return xml.Header + XMLStylesheet
	}
	return xml.Header + x.XMLStylesheet
}

func (x *XMLConfiguration) Marshal() (string, error) {
	c := &configuration{Properties: x.Properties}
	data, err := xml.MarshalIndent(c, "", "    ")
	if err != nil {
		return "", err
	}

	fullXML := x.getHeader() + string(data)

	return fullXML, nil
}
