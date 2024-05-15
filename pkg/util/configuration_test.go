package util

import (
	"strings"
	"testing"
)

func TestOverrideXmlFileContent(t *testing.T) {
	const origin = `<?xml version="1.0"?>
	<configuration>
		<property>
			<name>key1</name>
			<value>value1</value>
		</property>
	</configuration>`

	type args struct {
		current  string
		override map[string]string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		// TODO: Add test cases.
		{
			name: "test1",
			args: args{
				current:  origin,
				override: map[string]string{"key2": "value2"},
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := OverrideXmlContent(tt.args.current, tt.args.override); strings.TrimSpace(got) != strings.TrimSpace(tt.want) {
				t.Errorf("OverrideXmlContent() = %v, want %v", got, tt.want)
			}
		})
	}
}
