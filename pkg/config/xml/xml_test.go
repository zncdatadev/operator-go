package xml

import (
	"testing"
)

func TestXMLConfiguration(t *testing.T) {
	xmlData := `
<?xml version="1.0" encoding="UTF-8" standalone="no"?>
<?xml-stylesheet type="text/xsl" href="configuration.xsl"?><!--
   Licensed to the Apache Software Foundation (ASF) under one or more
   contributor license agreements.  See the NOTICE file distributed with
   this work for additional information regarding copyright ownership.
   The ASF licenses this file to You under the Apache License, Version 2.0
   (the "License"); you may not use this file except in compliance with
   the License.  You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
-->
<configuration>
    <!-- WARNING!!! This file is auto generated for documentation purposes ONLY! -->
    <!-- WARNING!!! Any changes you make to this file will be ignored by Hive.   -->
    <!-- WARNING!!! You must make your changes in hive-site.xml instead.         -->
    <!-- Hive Execution Parameters -->
    <property>
        <name>hive.metastore.warehouse.dir</name>
        <value>/user/hive/warehouse</value>
        <description>location of default database for the warehouse</description>
    </property>
    <property>
        <name>javax.jdo.option.ConnectionURL</name>
        <value>jdbc:derby:;databaseName=metastore_db;create=true</value>
        <description>
          JDBC connect string for a JDBC metastore.
          To use SSL to encrypt/authenticate the connection, provide database-specific SSL flag in the connection URL.
          For example, jdbc:postgresql://myhost/db?ssl=true for postgres database.
        </description>
    </property>
</configuration>
`

	expectedXMLData := `<?xml version="1.0" encoding="UTF-8" standalone="no"?>
<?xml-stylesheet type="text/xsl" href="configuration.xsl"?><!--
   Licensed to the Apache Software Foundation (ASF) under one or more
   contributor license agreements.  See the NOTICE file distributed with
   this work for additional information regarding copyright ownership.
   The ASF licenses this file to You under the Apache License, Version 2.0
   (the "License"); you may not use this file except in compliance with
   the License.  You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
-->
<configuration>
    <property>
        <name>hive.metastore.db.type</name>
        <value>DERBY</value>
        <description>Expects one of [derby, oracle, mysql, mssql, postgres]. Type of database used by the metastore. Information schema &amp; JDBCStorageHandler depend on it.</description>
    </property>
    <property>
        <name>hive.metastore.warehouse.dir</name>
        <value>/user/hive/warehouse</value>
        <description>location of default database for the warehouse</description>
    </property>
    <property>
        <name>javax.jdo.option.ConnectionURL</name>
        <value>jdbc:derby:;databaseName=metastore_db;create=true</value>
        <description>
          JDBC connect string for a JDBC metastore.
          To use SSL to encrypt/authenticate the connection, provide database-specific SSL flag in the connection URL.
          For example, jdbc:postgresql://myhost/db?ssl=true for postgres database.
        </description>
    </property>
</configuration>
`

	// Note: Marshalling XML data no blank line

	config, err := NewXMLConfigurationFromString(xmlData)
	if err != nil {
		t.Errorf("Failed to create XML configuration from string: %v", err)
	}

	property, ok := config.GetProperty("hive.metastore.warehouse.dir")
	if !ok {
		t.Errorf("Expected property 'hive.metastore.warehouse.dir' not found")
	}

	if property.Value != "/user/hive/warehouse" {
		t.Errorf("Property 'hive.metastore.warehouse.dir' has incorrect value. Expected: /user/hive/warehouse, Got: %s", property.Value)
	}

	config.AddPropertyWithString("hive.metastore.db.type", "DERBY", "Expects one of [derby, oracle, mysql, mssql, postgres]. Type of database used by the metastore. Information schema & JDBCStorageHandler depend on it.")

	xmlData, err = config.Marshal()
	if err != nil {
		t.Errorf("Failed to marshal XML configuration: %v", err)
	}

	if xmlData != expectedXMLData {
		t.Errorf("Marshalled XML does not match expected XML. Expected:\n%s\nGot:\n%s", expectedXMLData, xmlData)
	}

}

func TestNewXMLConfigurationFromString(t *testing.T) {
	xmlData := `
    <?xml version="1.0" encoding="UTF-8"?>
    <configuration>
        <property>
            <name>property1</name>
            <value>value1</value>
        </property>
        <property>
            <name>property2</name>
            <value>value2</value>
        </property>
    </configuration>
    `

	expectedProperties := map[string]string{
		"property1": "value1",
		"property2": "value2",
	}

	config, err := NewXMLConfigurationFromString(xmlData)
	if err != nil {
		t.Errorf("Failed to create XML configuration from string: %v", err)
	}

	for name, value := range expectedProperties {
		property, ok := config.GetProperty(name)
		if !ok {
			t.Errorf("Expected property '%s' not found", name)
		}

		if property.Value != value {
			t.Errorf("Property '%s' has incorrect value. Expected: %s, Got: %s", name, value, property.Value)
		}
	}
}
func TestNewXMLConfigurationFromString_WithHead(t *testing.T) {
	xmlData := `
<?xml version="1.0" encoding="UTF-8" standalone="no"?>
<?xml-stylesheet type="text/xsl" href="configuration.xsl"?><!--
   Licensed to the Apache Software Foundation (ASF) under one or more
   contributor license agreements.  See the NOTICE file distributed with
   this work for additional information regarding copyright ownership.
   The ASF licenses this file to You under the Apache License, Version 2.0
   (the "License"); you may not use this file except in compliance with
   the License.  You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
-->
<configuration>
  <property>
    <name>hive.metastore.warehouse.dir</name>
    <value>/user/hive/warehouse</value>
    <description>location of default database for the warehouse</description>
  </property>
  <property>
    <name>javax.jdo.option.ConnectionURL</name>
    <value>jdbc:derby:;databaseName=metastore_db;create=true</value>
    <description>
      JDBC connect string for a JDBC metastore.
      To use SSL to encrypt/authenticate the connection, provide database-specific SSL flag in the connection URL.
      For example, jdbc:postgresql://myhost/db?ssl=true for postgres database.
    </description>
  </property>
</configuration>
`

	expectedProperties := map[string]string{
		"hive.metastore.warehouse.dir":   "/user/hive/warehouse",
		"javax.jdo.option.ConnectionURL": "jdbc:derby:;databaseName=metastore_db;create=true",
	}

	config, err := NewXMLConfigurationFromString(xmlData)
	if err != nil {
		t.Errorf("Failed to create XML configuration from string: %v", err)
	}

	for name, value := range expectedProperties {
		property, ok := config.GetProperty(name)
		if !ok {
			t.Errorf("Expected property '%s' not found", name)
		}

		if property.Value != value {
			t.Errorf("Property '%s' has incorrect value. Expected: %s, Got: %s", name, value, property.Value)
		}
	}
}

func TestNewXMLConfigurationFromMap(t *testing.T) {
	properties := map[string]string{
		"property1": "value1",
		"property2": "value2",
	}

	config := NewXMLConfigurationFromMap(properties)

	for name, value := range properties {
		property, ok := config.GetProperty(name)
		if !ok {
			t.Errorf("Expected property '%s' not found", name)
		}

		if property.Value != value {
			t.Errorf("Property '%s' has incorrect value. Expected: %s, Got: %s", name, value, property.Value)
		}
	}
}

func TestXMLConfiguration_AddPropertyWithString(t *testing.T) {
	config := NewXMLConfiguration()

	name := "property1"
	value := "value1"
	description := "This is property 1"

	config.AddPropertyWithString(name, value, description)

	property, ok := config.GetProperty(name)
	if !ok {
		t.Errorf("Expected property '%s' not found", name)
	}

	if property.Value != value {
		t.Errorf("Property '%s' has incorrect value. Expected: %s, Got: %s", name, value, property.Value)
	}

	if property.Description != description {
		t.Errorf("Property '%s' has incorrect description. Expected: %s, Got: %s", name, description, property.Description)
	}
}

func TestXMLConfiguration_AddProperty(t *testing.T) {
	config := NewXMLConfiguration()

	property := Property{
		Name:  "property1",
		Value: "value1",
	}

	config.AddProperty(property)

	p, ok := config.GetProperty("property1")
	if !ok {
		t.Errorf("Expected property 'property1' not found")
	}

	if p.Value != "value1" {
		t.Errorf("Property 'property1' has incorrect value. Expected: value1, Got: %s", p.Value)
	}

	property.Value = "value2"
	config.AddProperty(property)

	p, ok = config.GetProperty("property1")
	if !ok {
		t.Errorf("Expected property 'property1' not found")
	}
}

func TestXMLConfiguration_DeleteProperties(t *testing.T) {
	properties := map[string]string{
		"property1": "value1",
		"property2": "value2",
	}

	config := NewXMLConfigurationFromMap(properties)

	config.DeleteProperties("property1")

	_, ok := config.GetProperty("property1")
	if ok {
		t.Errorf("Deleted property 'property1' still exists")
	}

	_, ok = config.GetProperty("property2")
	if !ok {
		t.Errorf("Expected property 'property2' not found")
	}
}

func TestXMLConfiguration_Marshal(t *testing.T) {
	properties := map[string]string{
		"property1": "value1",
		"property2": "value2",
	}

	config := NewXMLConfigurationFromMap(properties)

	expectedXML := `<?xml version="1.0" encoding="UTF-8"?>
<?xml-stylesheet type="text/xsl" href="configuration.xsl"?>
<configuration>
    <property>
        <name>property1</name>
        <value>value1</value>
    </property>
    <property>
        <name>property2</name>
        <value>value2</value>
    </property>
</configuration>
`

	xmlData, err := config.Marshal()
	if err != nil {
		t.Errorf("Failed to marshal XML configuration: %v", err)
	}

	if xmlData != expectedXML {
		t.Errorf("Marshalled XML does not match expected XML. Expected:\n%s\nGot:\n%s", expectedXML, xmlData)
	}
}

func TestXMLConfiguration_Marshal_NoHeader(t *testing.T) {
	xmlData := `
<configuration>
    <property>
        <name>property1</name>
        <value>value1</value>
    </property>
</configuration>
`

	expectedXML := `<?xml version="1.0" encoding="UTF-8"?>
<?xml-stylesheet type="text/xsl" href="configuration.xsl"?>
<configuration>
    <property>
        <name>property1</name>
        <value>value1</value>
    </property>
</configuration>
`

	config, err := NewXMLConfigurationFromString(xmlData)
	if err != nil {
		t.Errorf("Failed to create XML configuration from string: %v", err)
	}

	xmlData, err = config.Marshal()
	if err != nil {
		t.Errorf("Failed to marshal XML configuration: %v", err)
	}

	if xmlData != expectedXML {
		t.Errorf("Marshalled XML does not match expected XML. Expected:\n%s\nGot:\n%s", expectedXML, xmlData)
	}
}
