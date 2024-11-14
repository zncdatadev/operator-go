package productlogging

import (
	"github.com/zncdatadev/operator-go/pkg/config"
)

const logbackTemplate = `
<configuration>
    <appender name="CONSOLE" class="ch.qos.logback.core.ConsoleAppender">
        <encoder>
            <pattern>{{ .ConsoleHandlerFormatter }}</pattern>
        </encoder>
        <filter class="ch.qos.logback.classic.filter.ThresholdFilter">
            <level>{{ .ConsoleHandlerLevel }}</level>
        </filter>
    </appender>

    <appender name="FILE" class="ch.qos.logback.core.rolling.RollingFileAppender">
        <File>{{ .RotatingFileHandlerFile }}</File>
        <encoder class="ch.qos.logback.core.encoder.LayoutWrappingEncoder">
            <layout class="ch.qos.logback.classic.log4j.XMLLayout" />
        </encoder>

        <filter class="ch.qos.logback.classic.filter.ThresholdFilter">
            <level>{{ .RotatingFileHandlerLevel }}</level>
        </filter>
        <rollingPolicy class="ch.qos.logback.core.rolling.FixedWindowRollingPolicy">
            <minIndex>1</minIndex>
            <maxIndex>{{ .RotatingFileHandlerBackupCount }}</maxIndex>
            <FileNamePattern>{{ .RotatingFileHandlerFile }}.%i</FileNamePattern>
        </rollingPolicy>

        <triggeringPolicy class="ch.qos.logback.core.rolling.SizeBasedTriggeringPolicy">
            <MaxFileSize>{{ .RotatingFileHandlerMaxSizeInMiB }}MB</MaxFileSize>
        </triggeringPolicy>
    </appender>

    {{ .Loggers }}

    <root level="{{ .RootLogLevel }}">
        <appender-ref ref="CONSOLE" />
        <appender-ref ref="FILE" />
    </root>

</configuration>
`

var _ LoggingConfig = &LogbackConfig{}

// LogbackConfig is a struct that contains logback logging configuration
type LogbackConfig struct {
	productLogging *ProductLogging
}

// Content implements the LoggingConfig interface
func (l *LogbackConfig) Content() (string, error) {
	values := JavaLogTemplateValue(l, l.productLogging)

	p := config.TemplateParser{Template: l.Template(), Value: values}
	return p.Parse()
}

// LoggerFormatter implements the LoggingConfig interface
func (l *LogbackConfig) LoggerFormatter(name string, level string) string {
	return `<logger name="` + name + `" level="` + level + `" />`
}

// String implements the LoggingConfig interface
func (l *LogbackConfig) String() string {
	c, e := l.Content()
	if e != nil {
		panic(e)
	}
	return c
}

// Template implements the LoggingConfig interface
func (l *LogbackConfig) Template() string {
	return logbackTemplate
}
