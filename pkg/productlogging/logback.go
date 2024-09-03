package productlogging

import (
	"fmt"

	loggingv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
)

const logbackTemplate = `<configuration>
<appender name="CONSOLE" class="ch.qos.logback.core.ConsoleAppender">
<encoder>
  <pattern>{{.ConsoleConversionPattern}}</pattern>
</encoder>
<filter class="ch.qos.logback.classic.filter.ThresholdFilter">
  <level>{{.ConsoleLogLevel}}</level>
</filter>
</appender>

<appender name="FILE" class="ch.qos.logback.core.rolling.RollingFileAppender">
<File>{{.LogDir}}{{.LogFile}}</File>
<encoder class="ch.qos.logback.core.encoder.LayoutWrappingEncoder">
  <layout class="ch.qos.logback.classic.log4j.XMLLayout" />
</encoder>
<filter class="ch.qos.logback.classic.filter.ThresholdFilter">
  <level>{{.FileLogLevel}}</level>
</filter>
<rollingPolicy class="ch.qos.logback.core.rolling.FixedWindowRollingPolicy">
  <minIndex>1</minIndex>
  <maxIndex>{{.NumberOfArchivedLogFiles}}</maxIndex>
  <FileNamePattern>{{.LogDir}}{{.LogFile}}.%i</FileNamePattern>
</rollingPolicy>
<triggeringPolicy class="ch.qos.logback.core.rolling.SizeBasedTriggeringPolicy">
  <MaxFileSize>{{.MaxLogFileSizeInMiB}}MB</MaxFileSize>
</triggeringPolicy>
</appender>

{{.Loggers}}

<root level="{{.RootLogLevel}}">
<appender-ref ref="CONSOLE" />
<appender-ref ref="FILE" />
</root>
</configuration>
`

func NewLogbackConfigGenerator(
	loggingConfigSpec *loggingv1alpha1.LoggingConfigSpec,
	containerName string,
	consoleConversionPattern string,
	maxLogFileSizeInMiB *float64,
	logFileName string,
	configFileName string) *LogbackConfigGenerator {
	impl := &LogbackConfigGenerator{configFileName: configFileName}
	impl.BaseLoggingConfigGenerator = *NewBaseLoggingConfigGenerator(loggingConfigSpec, containerName, consoleConversionPattern, maxLogFileSizeInMiB, logFileName, impl)
	return impl
}

var _ LoggingConfigGenerator = &LogbackConfigGenerator{}

type LogbackConfigGenerator struct {
	BaseLoggingConfigGenerator
	configFileName string
}

// GenerateLoggersConfig implements LoggingConfigGenerator.
func (l *LogbackConfigGenerator) GenerateLoggersConfig(LoggersSpec map[string]*loggingv1alpha1.LogLevelSpec) string {
	if len(LoggersSpec) == 0 {
		return ""
	}
	return createLoggerConfig(LoggersSpec, func(name, lvl string) string {
		return fmt.Sprintf("<logger name=\"%s\" level=\"%s\"/>", name, lvl)
	})
}

// ConfigTemplate implements LoggingConfigGenerator.
func (l *LogbackConfigGenerator) ConfigTemplate() string {
	return logbackTemplate
}

func (l *LogbackConfigGenerator) FileName() string {
	return l.configFileName
}
