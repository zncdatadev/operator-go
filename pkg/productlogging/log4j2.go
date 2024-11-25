/*
Copyright 2024 ZNCDataDev.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package productlogging

import (
	"github.com/zncdatadev/operator-go/pkg/config"
)

const log4j2Template = `appenders = FILE, CONSOLE

appender.CONSOLE.type = Console
appender.CONSOLE.name = CONSOLE
appender.CONSOLE.target = SYSTEM_ERR
appender.CONSOLE.layout.type = PatternLayout
appender.CONSOLE.layout.pattern = {{.ConsoleHandlerFormatter}}
appender.CONSOLE.filter.threshold.type = ThresholdFilter
appender.CONSOLE.filter.threshold.level = {{.ConsoleHandlerLevel}}

appender.FILE.type = RollingFile
appender.FILE.name = FILE
appender.FILE.fileName = {{.RotatingFileHandlerFile}}
appender.FILE.filePattern = {{.RotatingFileHandlerFile}}.%i
appender.FILE.layout.type = XMLLayout
appender.FILE.policies.type = Policies
appender.FILE.policies.size.type = SizeBasedTriggeringPolicy
appender.FILE.policies.size.size = {{.RotatingFileHandlerMaxSizeInMiB}}MB
appender.FILE.strategy.type = DefaultRolloverStrategy
appender.FILE.strategy.max = {{.RotatingFileHandlerBackupCount}}
appender.FILE.filter.threshold.type = ThresholdFilter
appender.FILE.filter.threshold.level = {{.RotatingFileHandlerLevel}}
{{.Loggers}}
rootLogger.level={{.RootLogLevel}}
rootLogger.appenderRefs = CONSOLE, FILE
rootLogger.appenderRef.CONSOLE.ref = CONSOLE
rootLogger.appenderRef.FILE.ref = FILE
`

var _ LoggingConfig = &Log4j2Config{}

// Log4j2Config is a struct that contains log4j2 logging configuration
type Log4j2Config struct {
	productLogging *ProductLogging
}

// Content implements the LoggingConfig interface
func (l *Log4j2Config) Content() (string, error) {
	values := JavaLogTemplateValue(l, l.productLogging)

	p := config.TemplateParser{Template: l.Template(), Value: values}
	return p.Parse()
}

// LoggerFormatter implements the LoggingConfig interface
func (l *Log4j2Config) LoggerFormatter(name string, level string) string {
	return `logger.` + name + `.name = ` + name + "\nlogger." + name + `.level = ` + level
}

// String implements the LoggingConfig interface
func (l *Log4j2Config) String() string {
	c, e := l.Content()
	if e != nil {
		panic(e)
	}
	return c
}

// Template implements the LoggingConfig interface
func (l *Log4j2Config) Template() string {
	return log4j2Template
}
