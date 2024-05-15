package util

import (
	"bytes"
	"encoding/xml"
)

type XmlNameValuePair struct {
	Name  string `xml:"name"`
	Value string `xml:"value"`
}

type XmlConfiguration struct {
	XMLName    xml.Name           `xml:"configuration"`
	Properties []XmlNameValuePair `xml:"property"`
}

func NewXmlConfiguration(properties []XmlNameValuePair) *XmlConfiguration {
	return &XmlConfiguration{
		Properties: properties,
	}
}

func (c *XmlConfiguration) String(properties []XmlNameValuePair) string {
	if len(c.Properties) != 0 {
		c.Properties = c.DistinctProperties(properties)
	}
	buf := new(bytes.Buffer)
	if _, err := buf.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n"); err != nil {
		logger.Error(err, "failed to write xml document head")
	}
	enc := xml.NewEncoder(buf)
	enc.Indent("", "  ")
	if err := enc.Encode(c); err != nil {
		logger.Error(err, "failed to encode xml document")
		panic(err)
	}
	return buf.String()
}

// DistinctProperties distinct properties by name,
func (c *XmlConfiguration) DistinctProperties(properties []XmlNameValuePair) []XmlNameValuePair {
	var collect []XmlNameValuePair
	collect = append(collect, c.Properties...)
	collect = append(collect, properties...)

	var distinctProperties []XmlNameValuePair
	var distinctKeys map[string]int
	for idx, v := range collect {
		if distinctKeys == nil {
			distinctKeys = make(map[string]int)
		}
		if existIdx, ok := distinctKeys[v.Name]; !ok {
			distinctKeys[v.Name] = idx
			distinctProperties = append(distinctProperties, v)
		} else {
			distinctProperties[existIdx] = v
		}
	}
	return distinctProperties

	//var distinctMap = make(map[string]XmlNameValuePair)
	//for _, v := range collect {
	//	distinctMap[v.Name] = v
	//}
	//return maps.Values(distinctMap)
}

func (c *XmlConfiguration) StringWithProperties(properties map[string]string) string {
	var pairs []XmlNameValuePair
	for k, v := range properties {
		pairs = append(pairs, XmlNameValuePair{
			Name:  k,
			Value: v,
		})
	}
	return c.String(pairs)
}

// Append  to exist xml dom
func Append(originXml string, properties []XmlNameValuePair) string {
	var xmlDom XmlConfiguration
	//string -> dom
	if err := xml.Unmarshal([]byte(originXml), &xmlDom); err != nil {
		panic(err)
	}
	return xmlDom.String(properties)
}

// OverrideXmlContent overrides the content of a xml file
// append the override properties to the current xml dom
func OverrideXmlContent(current string, overrideProperties map[string]string) string {
	var xmlDom XmlConfiguration
	//string -> dom
	if err := xml.Unmarshal([]byte(current), &xmlDom); err != nil {
		panic(err)
	}
	// do override
	for k, v := range overrideProperties {
		overridePair := XmlNameValuePair{
			Name:  k,
			Value: v,
		}
		xmlDom.Properties = append(xmlDom.Properties, overridePair)
	}
	// dom -> string
	var b bytes.Buffer
	if _, err := b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n"); err != nil {
		logger.Error(err, "failed to write string")
	}
	encoder := xml.NewEncoder(&b)
	encoder.Indent("", "  ")
	if err := encoder.Encode(xmlDom); err != nil {
		logger.Error(err, "failed to encode xml")
	}
	return b.String()
}
