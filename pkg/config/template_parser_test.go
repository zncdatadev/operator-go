package config

import "testing"

const log4jProperties = `log4j.rootLogger=INFO, stdout
log4j.appender.stdout=org.apache.log4j.ConsoleAppender
{{- if .LogDir}}
log4j.appender.stdout.File={{.LogDir}}/{{.LogName}}.log
{{- else}}
log4j.appender.stdout.File=logs/app.log
{{- end}}
log4j.appender.stdout.Threshold=INFO
log4j.appender.stdout.Target=System.out
log4j.appender.stdout.layout=org.apache.log4j.PatternLayout
log4j.appender.stdout.layout.ConversionPattern=%d{yyyy-MM-dd HH:mm:ss} %-5p %c{1}:%L - %m%n
{{ range .Loggers }}
log4j.logger.{{.Logger}}={{.Level}}
{{- end}}
`

func TestTemplateParser_Parse(t1 *testing.T) {
	type fields struct {
		Value    interface{}
		Template string
	}
	tests := []struct {
		name    string
		fields  fields
		want    string
		wantErr bool
	}{
		// TODO: Add test cases.
		{
			name: "parse template simplely",
			fields: fields{
				Value: map[string]interface{}{
					"LogDir":  "customLogs",
					"LogName": "customApp",
					"Loggers": []map[string]string{
						{"Logger": "test1", "Level": "WARN"},
						{"Logger": "test2", "Level": "DEBUG"},
					},
				},
				Template: log4jProperties,
			},
			want: `log4j.rootLogger=INFO, stdout
log4j.appender.stdout=org.apache.log4j.ConsoleAppender
log4j.appender.stdout.File=customLogs/customApp.log
log4j.appender.stdout.Threshold=INFO
log4j.appender.stdout.Target=System.out
log4j.appender.stdout.layout=org.apache.log4j.PatternLayout
log4j.appender.stdout.layout.ConversionPattern=%d{yyyy-MM-dd HH:mm:ss} %-5p %c{1}:%L - %m%n

log4j.logger.test1=WARN
log4j.logger.test2=DEBUG
`,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t1.Run(tt.name, func(t1 *testing.T) {
			t := &TemplateParser{
				Value:    tt.fields.Value,
				Template: tt.fields.Template,
			}
			got, err := t.Parse()
			if (err != nil) != tt.wantErr {
				t1.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t1.Errorf("Parse() got = %v, want %v", got, tt.want)
			}
		})
	}
}
