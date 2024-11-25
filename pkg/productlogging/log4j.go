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

const log4jTemplate = `log4j.rootLogger={{.RootLogLevel}}, CONSOLE, FILE

log4j.appender.CONSOLE=org.apache.log4j.ConsoleAppender
log4j.appender.CONSOLE.Threshold={{.ConsoleHandlerLevel}}
log4j.appender.CONSOLE.layout=org.apache.log4j.PatternLayout
log4j.appender.CONSOLE.layout.ConversionPattern={{.ConsoleHandlerFormatter}}

log4j.appender.FILE=org.apache.log4j.RollingFileAppender
log4j.appender.FILE.Threshold={{.RotatingFileHandlerLevel}}
log4j.appender.FILE.File={{.RotatingFileHandlerFile}}
log4j.appender.FILE.MaxFileSize={{.RotatingFileHandlerMaxSizeInMiB}}MB
log4j.appender.FILE.MaxBackupIndex={{.RotatingFileHandlerBackupCount}}
log4j.appender.FILE.layout=org.apache.log4j.xml.XMLLayout

{{.Loggers}}
`

var _ LoggingConfig = &Log4jConfig{}

// Log4jConfig is a struct that holds the configuration for log4j logging
type Log4jConfig struct {
	productLogging *ProductLogging
}

// Content implements LoggingConfig.
func (l *Log4jConfig) Content() (string, error) {
	values := JavaLogTemplateValue(l, l.productLogging)

	p := config.TemplateParser{Template: l.Template(), Value: values}
	content, err := p.Parse()
	if err != nil {
		return "", err
	}
	return content, nil
}

// LoggerFormatter implements LoggingConfig.
func (l *Log4jConfig) LoggerFormatter(name string, level string) string {
	return `log4j.logger.` + name + `=` + level
}

// String implements LoggingConfig.
func (l *Log4jConfig) String() string {
	c, e := l.Content()
	if e != nil {
		panic(e)
	}
	return c
}

// Template implements LoggingConfig.
func (l *Log4jConfig) Template() string {
	return log4jTemplate
}
