package productlogging

import (
	"testing"

	"github.com/stretchr/testify/assert"
	loggingv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
)

func TestLog4jConfig_Content(t *testing.T) {
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

	log4jConfig := &Log4jConfig{productLogging: productLogging}
	content, err := log4jConfig.Content()
	assert.NoError(t, err)

	expectedContent := `log4j.rootLogger=DEBUG, CONSOLE, FILE

log4j.appender.CONSOLE=org.apache.log4j.ConsoleAppender
log4j.appender.CONSOLE.Threshold=INFO
log4j.appender.CONSOLE.layout=org.apache.log4j.PatternLayout
log4j.appender.CONSOLE.layout.ConversionPattern=%d{ISO8601} %-5p %m%n

log4j.appender.FILE=org.apache.log4j.RollingFileAppender
log4j.appender.FILE.Threshold=WARN
log4j.appender.FILE.File=/var/log/app.log
log4j.appender.FILE.MaxFileSize=10MB
log4j.appender.FILE.MaxBackupIndex=5
log4j.appender.FILE.layout=org.apache.log4j.xml.XMLLayout

log4j.logger.com.example=DEBUG
`
	assert.Equal(t, expectedContent, content)
}

func TestLog4jConfig_LoggerFormatter(t *testing.T) {
	log4jConfig := &Log4jConfig{}
	formattedLogger := log4jConfig.LoggerFormatter("com.example", "DEBUG")
	expectedFormattedLogger := "log4j.logger.com.example=DEBUG"
	assert.Equal(t, expectedFormattedLogger, formattedLogger)
}

func TestLog4jConfig_Template(t *testing.T) {
	log4jConfig := &Log4jConfig{}
	template := log4jConfig.Template()
	assert.Equal(t, log4jTemplate, template)
}
