package productlogging

import (
	"fmt"
	"strings"

	loggingv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
)

const log4j2Template = `appenders = FILE, CONSOLE

appender.CONSOLE.type = Console
appender.CONSOLE.name = CONSOLE
appender.CONSOLE.target = SYSTEM_ERR
appender.CONSOLE.layout.type = PatternLayout
appender.CONSOLE.layout.pattern = {{.ConsoleConversionPattern}}
appender.CONSOLE.filter.threshold.type = ThresholdFilter
appender.CONSOLE.filter.threshold.level = {{.ConsoleLogLevel}}

appender.FILE.type = RollingFile
appender.FILE.name = FILE
appender.FILE.fileName = {{.LogDir}}{{.LogFile}}
appender.FILE.filePattern = {{.LogDir}}{{.LogFile}}.%i
appender.FILE.layout.type = XMLLayout
appender.FILE.policies.type = Policies
appender.FILE.policies.size.type = SizeBasedTriggeringPolicy
appender.FILE.policies.size.size = {{.MaxLogFileSizeInMiB}}MB
appender.FILE.strategy.type = DefaultRolloverStrategy
appender.FILE.strategy.max = {{.NumberOfArchivedLogFiles}}
appender.FILE.filter.threshold.type = ThresholdFilter
appender.FILE.filter.threshold.level = {{.FileLogLevel}}
{{.Loggers}}
rootLogger.level={{.RootLogLevel}}
rootLogger.appenderRefs = CONSOLE, FILE
rootLogger.appenderRef.CONSOLE.ref = CONSOLE
rootLogger.appenderRef.FILE.ref = FILE`

func NewLog4j2ConfigGenerator(
	loggingConfigSpec *loggingv1alpha1.LoggingConfigSpec,
	containerName string,
	consoleConversionPattern string,
	maxLogFileSizeInMiB *float64,
	logFileName string,
	configFileName string) *Log4j2ConfigGenerator {
	impl := &Log4j2ConfigGenerator{configFileName: configFileName}
	impl.BaseLoggingConfigGenerator = *NewBaseLoggingConfigGenerator(loggingConfigSpec, containerName, consoleConversionPattern, maxLogFileSizeInMiB, logFileName, impl)
	return impl
}

var _ LoggingConfigGenerator = &Log4j2ConfigGenerator{}

type Log4j2ConfigGenerator struct {
	BaseLoggingConfigGenerator
	configFileName string
}

// GenerateLoggersConfig implements LoggingConfigGenerator.
func (l *Log4j2ConfigGenerator) GenerateLoggersConfig(LoggersSpec map[string]*loggingv1alpha1.LogLevelSpec) string {
	if len(LoggersSpec) == 0 {
		return ""
	}

	var loggerNames, loggersConfig []string
	for name, lvl := range LoggersSpec {
		loggerNames = append(loggerNames, name)
		loggersConfig = append(loggersConfig, fmt.Sprintf("logger.%s.name = %s", name, name))
		loggersConfig = append(loggersConfig, fmt.Sprintf("logger.%s.name = %s", name, lvl.Level))
	}
	return fmt.Sprintf("loggers = %s\n%s", strings.Join(loggerNames, ","), strings.Join(loggersConfig, "\n"))
}

// ConfigTemplate implements LoggingConfigGenerator.
func (l *Log4j2ConfigGenerator) ConfigTemplate() string {
	return log4j2Template
}

func (l *Log4j2ConfigGenerator) FileName() string {
	return l.configFileName
}
