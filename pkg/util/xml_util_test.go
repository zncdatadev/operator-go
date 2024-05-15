package util

import (
	"testing"
)

func TestXmlConfiguration_Append(t *testing.T) {

	const origin = `<?xml version="1.0" encoding="UTF-8"?>
	<configuration>
		<property>
			<name>key1</name>
			<value>value1</value>
		</property>
	</configuration>`

	type args struct {
		originXml  string
		properties []XmlNameValuePair
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		// TODO: Add test cases.
		{
			name: "append a new property",
			args: args{
				originXml:  origin,
				properties: []XmlNameValuePair{{Name: "key2", Value: "value2"}},
			},
			want: `<?xml version="1.0" encoding="UTF-8"?>
<configuration>
  <property>
    <name>key1</name>
    <value>value1</value>
  </property>
  <property>
    <name>key2</name>
    <value>value2</value>
  </property>
</configuration>`,
		},
		{
			name: "append a new property, but the name exists in origin xml, should override it",
			args: args{
				originXml:  origin,
				properties: []XmlNameValuePair{{Name: "key1", Value: "value2"}, {Name: "key3", Value: "value3"}},
			},
			want: `<?xml version="1.0" encoding="UTF-8"?>
<configuration>
  <property>
    <name>key1</name>
    <value>value2</value>
  </property>
  <property>
    <name>key3</name>
    <value>value3</value>
  </property>
</configuration>`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Append(tt.args.originXml, tt.args.properties); got != tt.want {
				t.Errorf("XmlConfiguration.Append() = %v, want %v", got, tt.want)
			}
		})
	}
}
