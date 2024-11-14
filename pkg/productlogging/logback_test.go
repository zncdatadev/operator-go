package productlogging

import (
	"testing"

	"github.com/stretchr/testify/assert"
	loggingv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
)

func TestLogbackConfig_Content(t *testing.T) {
	productLogging := &ProductLogging{
		RootLogLevel:                   "DEBUG",
		ConsoleHandlerLevel:            "INFO",
		ConsoleHandlerFormatter:        "%d{ISO8601} %-5p %m%n",
		RotatingFileHandlerLevel:       "WARN",
		RotatingFileHandlerFile:        "/var/log/app.log",
		RotatingFileHandlerMaxBytes:    10 * 1024 * 1024,
		RotatingFileHandlerBackupCount: 5,
		Loggers: map[string]loggingv1alpha1.LogLevelSpec{
			"com.example": {Level: "DEBUG"},
		},
	}

	logbackConfig := &LogbackConfig{productLogging: productLogging}
	content, err := logbackConfig.Content()
	assert.NoError(t, err)

	expectedContent := `
<configuration>
    <appender name="CONSOLE" class="ch.qos.logback.core.ConsoleAppender">
        <encoder>
            <pattern>%d{ISO8601} %-5p %m%n</pattern>
        </encoder>
        <filter class="ch.qos.logback.classic.filter.ThresholdFilter">
            <level>INFO</level>
        </filter>
    </appender>

    <appender name="FILE" class="ch.qos.logback.core.rolling.RollingFileAppender">
        <File>/var/log/app.log</File>
        <encoder class="ch.qos.logback.core.encoder.LayoutWrappingEncoder">
            <layout class="ch.qos.logback.classic.log4j.XMLLayout" />
        </encoder>

        <filter class="ch.qos.logback.classic.filter.ThresholdFilter">
            <level>WARN</level>
        </filter>
        <rollingPolicy class="ch.qos.logback.core.rolling.FixedWindowRollingPolicy">
            <minIndex>1</minIndex>
            <maxIndex>5</maxIndex>
            <FileNamePattern>/var/log/app.log.%i</FileNamePattern>
        </rollingPolicy>

        <triggeringPolicy class="ch.qos.logback.core.rolling.SizeBasedTriggeringPolicy">
            <MaxFileSize>10MB</MaxFileSize>
        </triggeringPolicy>
    </appender>

    <logger name="com.example" level="DEBUG" />

    <root level="DEBUG">
        <appender-ref ref="CONSOLE" />
        <appender-ref ref="FILE" />
    </root>

</configuration>
`
	assert.Equal(t, expectedContent, content)
}

func TestLogbackConfig_LoggerFormatter(t *testing.T) {
	logbackConfig := &LogbackConfig{}
	formattedLogger := logbackConfig.LoggerFormatter("com.example", "DEBUG")
	expectedFormattedLogger := `<logger name="com.example" level="DEBUG" />`
	assert.Equal(t, expectedFormattedLogger, formattedLogger)
}

func TestLogbackConfig_Template(t *testing.T) {
	logbackConfig := &LogbackConfig{}
	template := logbackConfig.Template()
	assert.Equal(t, logbackTemplate, template)
}
