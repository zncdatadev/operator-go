package productlogging

import (
	"fmt"

	loggingv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
)

const log4jTemplate = `log4j.rootLogger={{.RootLogLevel}}, CONSOLE, FILE

log4j.appender.CONSOLE=org.apache.log4j.ConsoleAppender
log4j.appender.CONSOLE.Threshold={{.ConsoleLogLevel}}
log4j.appender.CONSOLE.layout=org.apache.log4j.PatternLayout
log4j.appender.CONSOLE.layout.ConversionPattern={{.ConsoleConversionPattern}}

log4j.appender.FILE=org.apache.log4j.RollingFileAppender
log4j.appender.FILE.Threshold={{.FileLogLevel}}
log4j.appender.FILE.File={{.LogDir}}{{.LogFile}}
log4j.appender.FILE.MaxFileSize={{.MaxLogFileSizeInMiB}}MB
log4j.appender.FILE.MaxBackupIndex={{.NumberOfArchivedLogFiles}}
log4j.appender.FILE.layout=org.apache.log4j.xml.XMLLayout

{{.Loggers}}`

func NewLog4jConfigGenerator(
	loggingConfigSpec *loggingv1alpha1.LoggingConfigSpec,
	containerName string,
	consoleConversionPattern string,
	maxLogFileSizeInMiB *float64,
	logFileName string,
	configFileName string) *Log4jConfigGenerator {
	impl := &Log4jConfigGenerator{configFileName: configFileName}
	impl.BaseLoggingConfigGenerator = *NewBaseLoggingConfigGenerator(loggingConfigSpec, containerName, consoleConversionPattern, maxLogFileSizeInMiB, logFileName, impl)
	return impl
}

var _ LoggingConfigGenerator = &Log4jConfigGenerator{}

type Log4jConfigGenerator struct {
	BaseLoggingConfigGenerator
	configFileName string
}

// GenerateLoggersConfig implements LoggingConfigGenerator.
func (l *Log4jConfigGenerator) GenerateLoggersConfig(LoggersSpec map[string]*loggingv1alpha1.LogLevelSpec) string {
	if len(LoggersSpec) == 0 {
		return ""
	}
	return createLoggerConfig(LoggersSpec, func(name, lvl string) string {
		return fmt.Sprintf("log4j.logger.%s=%s", name, lvl)
	})
}

// ConfigTemplate implements LoggingConfigGenerator.
func (l *Log4jConfigGenerator) ConfigTemplate() string {
	return log4jTemplate
}

func (l *Log4jConfigGenerator) FileName() string {
	return l.configFileName
}
