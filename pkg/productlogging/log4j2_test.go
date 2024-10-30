package productlogging

import (
	"testing"

	"github.com/stretchr/testify/assert"
	loggingv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
)

func TestLog4j2Config_Content(t *testing.T) {
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

	log4j2Config := &Log4j2Config{productLogging: productLogging}
	content, err := log4j2Config.Content()
	assert.NoError(t, err)

	expectedContent := `appenders = FILE, CONSOLE

appender.CONSOLE.type = Console
appender.CONSOLE.name = CONSOLE
appender.CONSOLE.target = SYSTEM_ERR
appender.CONSOLE.layout.type = PatternLayout
appender.CONSOLE.layout.pattern = %d{ISO8601} %-5p %m%n
appender.CONSOLE.filter.threshold.type = ThresholdFilter
appender.CONSOLE.filter.threshold.level = INFO

appender.FILE.type = RollingFile
appender.FILE.name = FILE
appender.FILE.fileName = /var/log/app.log
appender.FILE.filePattern = /var/log/app.log.%i
appender.FILE.layout.type = XMLLayout
appender.FILE.policies.type = Policies
appender.FILE.policies.size.type = SizeBasedTriggeringPolicy
appender.FILE.policies.size.size = 10MB
appender.FILE.strategy.type = DefaultRolloverStrategy
appender.FILE.strategy.max = 5
appender.FILE.filter.threshold.type = ThresholdFilter
appender.FILE.filter.threshold.level = WARN
logger.com.example.name = com.example
logger.com.example.level = DEBUG
rootLogger.level=DEBUG
rootLogger.appenderRefs = CONSOLE, FILE
rootLogger.appenderRef.CONSOLE.ref = CONSOLE
rootLogger.appenderRef.FILE.ref = FILE
`
	assert.Equal(t, expectedContent, content)
}

func TestLog4j2Config_LoggerFormatter(t *testing.T) {
	log4j2Config := &Log4j2Config{}
	formattedLogger := log4j2Config.LoggerFormatter("com.example", "DEBUG")
	expectedFormattedLogger := "logger.com.example.name = com.example\nlogger.com.example.level = DEBUG"
	assert.Equal(t, expectedFormattedLogger, formattedLogger)
}

func TestLog4j2Config_Template(t *testing.T) {
	log4j2Config := &Log4j2Config{}
	template := log4j2Config.Template()
	assert.Equal(t, log4j2Template, template)
}
