package util

import "testing"

func TestMarshal(t *testing.T) {

	tests := []struct {
		name string
		x    *XMLConfiguration
		want string
	}{
		{
			name: "empty",
			x:    &XMLConfiguration{},
			want: `<?xml version="1.0" encoding="UTF-8"?>
<?xml-stylesheet type="text/xsl" href="configuration.xsl"?>
<configuration></configuration>`,
		},
		{
			name: "one",
			x: &XMLConfiguration{
				Properties: []Property{
					{
						Name:        "name",
						Value:       "value",
						Description: "description",
						Comment:     "This is a comment",
					},
				},
			},
			want: `<?xml version="1.0" encoding="UTF-8"?>
<?xml-stylesheet type="text/xsl" href="configuration.xsl"?>
<configuration>
    <property>
        <!--This is a comment-->
        <name>name</name>
        <value>value</value>
        <description>description</description>
    </property>
</configuration>`,
		},
		{
			name: "two",
			x: &XMLConfiguration{
				Properties: []Property{
					{
						Name:  "name1",
						Value: "value1",
					},
				},
			},
			want: `<?xml version="1.0" encoding="UTF-8"?>
<?xml-stylesheet type="text/xsl" href="configuration.xsl"?>
<configuration>
    <property>
        <name>name1</name>
        <value>value1</value>
    </property>
</configuration>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.x.Marshal()
			if err != nil {
				t.Errorf("Marshal() error = %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("Marshal() got = %v, want %v", got, tt.want)
			}
		})
	}
}

// func TestNewXMLConfigurationFromString(t *testing.T) {
// 	xmlString := `<?xml version="1.0" encoding="UTF-8"?>
// <?xml-stylesheet type="text/xsl" href="configuration.xsl"?>
// <configuration>
//     <property>
//         <name>name</name>
//         <value>value</value>
//     </property>
// </configuration>`

// 	x, err := NewXMLConfigurationFromString(xmlString)
// 	if err != nil {
// 		t.Errorf("NewXMLConfigurationFromString() error = %v", err)
// 		return
// 	}

// 	if len(x.Properties) != 1 {
// 		t.Errorf("NewXMLConfigurationFromString() failed, expected 1 property, got %d", len(x.Properties))
// 	} else {
// 		p := x.Properties[0]
// 		if p.Name != "name" || p.Value != "value" || p.Description != "description" || p.Comment != "This is a comment" {
// 			t.Errorf("NewXMLConfigurationFromString() failed, expected property %v, got %v", Property{Name: "name", Value: "value", Description: "description", Comment: "This is a comment"}, p)
// 		}
// 	}
// }

func TestNewXMLConfigurationFromMap(t *testing.T) {
	properties := map[string]string{
		"name1": "value1",
		"name2": "value2",
		"name3": "value3",
	}

	x := NewXMLConfigurationFromMap(properties)

	if len(x.Properties) != len(properties) {
		t.Errorf("NewXMLConfigurationFromMap() failed, expected %d properties, got %d", len(properties), len(x.Properties))
	} else {
		for name, value := range properties {
			p, found := x.GetProperty(name)
			if !found {
				t.Errorf("NewXMLConfigurationFromMap() failed, property %s not found", name)
			} else if p.Value != value {
				t.Errorf("NewXMLConfigurationFromMap() failed, expected property %s value %s, got %s", name, value, p.Value)
			}
		}
	}
}

func TestAddProperty(t *testing.T) {
	x := NewXMLConfiguration()

	// Add a new property
	p1 := Property{Name: "name1", Value: "value1"}
	x.AddProperty(p1)

	// Verify that the property was added
	if len(x.Properties) != 1 {
		t.Errorf("AddProperty() failed, expected 1 property, got %d", len(x.Properties))
	} else if x.Properties[0] != p1 {
		t.Errorf("AddProperty() failed, expected property %v, got %v", p1, x.Properties[0])
	}

	// Add another property with the same name
	p2 := Property{Name: "name1", Value: "value2"}
	x.AddProperty(p2)

	// Verify that the existing property was updated
	if len(x.Properties) != 1 {
		t.Errorf("AddProperty() failed, expected 1 property, got %d", len(x.Properties))
	} else if x.Properties[0] != p2 {
		t.Errorf("AddProperty() failed, expected property %v, got %v", p2, x.Properties[0])
	}

	// Add a new property
	p3 := Property{Name: "name2", Value: "value3"}
	x.AddProperty(p3)

	// Verify that the new property was added
	if len(x.Properties) != 2 {
		t.Errorf("AddProperty() failed, expected 2 properties, got %d", len(x.Properties))
	} else if x.Properties[1] != p3 {
		t.Errorf("AddProperty() failed, expected property %v, got %v", p3, x.Properties[1])
	}
}

func TestAddPropertyWithString(t *testing.T) {
	x := NewXMLConfiguration()

	// Add a new property with string values
	name := "name1"
	value := "value1"
	description := "description1"
	comment := "This is a comment"
	x.AddPropertyWithString(name, value, description, comment)

	// Verify that the property was added
	if len(x.Properties) != 1 {
		t.Errorf("AddPropertyWithString() failed, expected 1 property, got %d", len(x.Properties))
	} else {
		p := x.Properties[0]
		if p.Name != name || p.Value != value || p.Description != description || p.Comment != comment {
			t.Errorf("AddPropertyWithString() failed, expected property %v, got %v", Property{Name: name, Value: value, Description: description, Comment: comment}, p)
		}
	}
}

func TestAddPropertiesWithMap(t *testing.T) {
	x := NewXMLConfiguration()

	// Define the properties map
	properties := map[string]string{
		"name1": "value1",
		"name2": "value2",
		"name3": "value3",
	}

	// Add properties using the map
	x.AddPropertiesWithMap(properties)

	// Verify that the properties were added
	if len(x.Properties) != len(properties) {
		t.Errorf("AddPropertiesWithMap() failed, expected %d properties, got %d", len(properties), len(x.Properties))
	} else {
		for name, value := range properties {
			p, found := x.GetProperty(name)
			if !found {
				t.Errorf("AddPropertiesWithMap() failed, property %s not found", name)
			} else if p.Value != value {
				t.Errorf("AddPropertiesWithMap() failed, expected property %s value %s, got %s", name, value, p.Value)
			}
		}
	}
}

func TestDeleteProperties(t *testing.T) {
	x := NewXMLConfiguration()

	// Add properties
	p1 := Property{Name: "name1", Value: "value1"}
	p2 := Property{Name: "name2", Value: "value2"}
	p3 := Property{Name: "name3", Value: "value3"}
	x.AddProperty(p1)
	x.AddProperty(p2)
	x.AddProperty(p3)

	// Delete properties
	x.DeleteProperties("name1", "name3")

	// Verify that the properties were deleted
	if len(x.Properties) != 1 {
		t.Errorf("DeleteProperties() failed, expected 1 property, got %d", len(x.Properties))
	} else if x.Properties[0] != p2 {
		t.Errorf("DeleteProperties() failed, expected property %v, got %v", p2, x.Properties[0])
	}
}
