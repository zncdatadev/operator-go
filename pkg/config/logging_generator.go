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

package config

import (
	"fmt"
	"sort"
	"strings"
)

// LoggingFramework defines the logging framework type.
type LoggingFramework string

const (
	LoggingFrameworkLog4j2  LoggingFramework = "log4j2"
	LoggingFrameworkLogback LoggingFramework = "logback"
	LoggingFrameworkPython  LoggingFramework = "python"
)

// LogLevel defines the logging level.
type LogLevel string

const (
	LogLevelTrace LogLevel = "TRACE"
	LogLevelDebug LogLevel = "DEBUG"
	LogLevelInfo  LogLevel = "INFO"
	LogLevelWarn  LogLevel = "WARN"
	LogLevelError LogLevel = "ERROR"
	LogLevelFatal LogLevel = "FATAL"
)

// LoggerConfig defines configuration for a single logger.
type LoggerConfig struct {
	Name  string   `json:"name"`
	Level LogLevel `json:"level"`
}

// LoggingGenerator generates logging configuration files.
type LoggingGenerator struct {
	framework LoggingFramework
}

// NewLoggingGenerator creates a new LoggingGenerator.
func NewLoggingGenerator(framework LoggingFramework) *LoggingGenerator {
	return &LoggingGenerator{
		framework: framework,
	}
}

// Generate generates the logging configuration content based on the configured framework.
func (g *LoggingGenerator) Generate(configs map[string]LoggerConfig) (string, error) {
	switch g.framework {
	case LoggingFrameworkLog4j2:
		return GenerateLog4j2(configs)
	case LoggingFrameworkLogback:
		return GenerateLogback(configs)
	case LoggingFrameworkPython:
		return GeneratePythonLogging(configs)
	default:
		return "", fmt.Errorf("unsupported logging framework: %s", g.framework)
	}
}

// GenerateLog4j2 generates Log4j2 properties format.
// The output format is:
// rootLogger.level=INFO
// rootLogger.appenderRefs=stdout
// appenders=console
// appender.console.type=Console
// appender.console.name=STDOUT
// appender.console.layout.type=PatternLayout
// appender.console.layout.pattern=%d{yyyy-MM-dd HH:mm:ss} %-5p %c{1}:%L - %m%n
// loggers=com.example,org.apache
// logger.com.example.name=com.example
// logger.com.example.level=DEBUG
func GenerateLog4j2(configs map[string]LoggerConfig) (string, error) {
	var sb strings.Builder

	// Root logger configuration
	sb.WriteString("# Log4j2 Configuration\n")
	sb.WriteString("rootLogger.level=INFO\n")
	sb.WriteString("rootLogger.appenderRefs=stdout\n\n")

	// Appender configuration
	sb.WriteString("# Appenders\n")
	sb.WriteString("appenders=console\n")
	sb.WriteString("appender.console.type=Console\n")
	sb.WriteString("appender.console.name=STDOUT\n")
	sb.WriteString("appender.console.layout.type=PatternLayout\n")
	sb.WriteString("appender.console.layout.pattern=%d{yyyy-MM-dd HH:mm:ss} %-5p %c{1}:%L - %m%n\n\n")

	if len(configs) == 0 {
		return sb.String(), nil
	}

	// Sort logger names for deterministic output
	names := make([]string, 0, len(configs))
	for name := range configs {
		names = append(names, name)
	}
	sort.Strings(names)

	// List all loggers
	sb.WriteString("# Loggers\n")
	sb.WriteString("loggers=")
	sb.WriteString(strings.Join(names, ","))
	sb.WriteString("\n\n")

	// Logger configurations
	for _, name := range names {
		config := configs[name]
		safeName := escapeLoggerName(name)
		fmt.Fprintf(&sb, "logger.%s.name=%s\n", safeName, name)
		fmt.Fprintf(&sb, "logger.%s.level=%s\n\n", safeName, config.Level)
	}

	return sb.String(), nil
}

// GenerateLogback generates Logback XML format.
// The output format is:
// <configuration>
//
//	<appender name="STDOUT" class="ch.qos.logback.core.ConsoleAppender">
//	  <encoder>
//	    <pattern>%d{yyyy-MM-dd HH:mm:ss} %-5level %logger{36} - %msg%n</pattern>
//	  </encoder>
//	</appender>
//	<root level="INFO">
//	  <appender-ref ref="STDOUT" />
//	</root>
//	<logger name="com.example" level="DEBUG" />
//
// </configuration>
func GenerateLogback(configs map[string]LoggerConfig) (string, error) {
	return GenerateLogbackWithOptions(configs, LogbackOptions{})
}

// LogbackOptions tunes logback generation. The zero value reproduces the console-only
// output of GenerateLogback.
type LogbackOptions struct {
	// FileOutputPath, when set, adds a bounded RollingFileAppender writing to this path in
	// addition to the console appender. The filename must match the log consumer's glob —
	// e.g. the Vector sidecar collects "<LogDir>/*.stdout.log", so pass
	// "<LogDir>/<app>.stdout.log". Without this, log aggregation has nothing to read.
	FileOutputPath string

	// Pattern overrides the encoder pattern for both appenders.
	Pattern string

	// MaxFileSize / MaxHistory / TotalSizeCap bound the rolling file appender so it cannot
	// exhaust the log volume. Sensible defaults are applied when left zero.
	MaxFileSize  string
	MaxHistory   int
	TotalSizeCap string
}

// GenerateLogbackWithOptions generates logback XML with optional file output.
func GenerateLogbackWithOptions(configs map[string]LoggerConfig, opts LogbackOptions) (string, error) {
	pattern := opts.Pattern
	if pattern == "" {
		pattern = "%d{yyyy-MM-dd HH:mm:ss} %-5level %logger{36} - %msg%n"
	}
	// Escape the (possibly caller-supplied) pattern so reserved XML characters cannot
	// produce invalid logback XML.
	pattern = escapeXML(pattern)

	var sb strings.Builder
	sb.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<configuration>\n")
	fmt.Fprintf(&sb, `  <appender name="STDOUT" class="ch.qos.logback.core.ConsoleAppender">
    <encoder>
      <pattern>%s</pattern>
    </encoder>
  </appender>
`, pattern)

	hasFile := opts.FileOutputPath != ""
	if hasFile {
		maxFileSize := opts.MaxFileSize
		if maxFileSize == "" {
			maxFileSize = "5MB"
		}
		totalSizeCap := opts.TotalSizeCap
		if totalSizeCap == "" {
			totalSizeCap = "8MB"
		}
		maxHistory := opts.MaxHistory
		if maxHistory <= 0 {
			maxHistory = 1
		}
		fmt.Fprintf(&sb, `
  <appender name="FILE" class="ch.qos.logback.core.rolling.RollingFileAppender">
    <file>%s</file>
    <encoder>
      <pattern>%s</pattern>
    </encoder>
    <rollingPolicy class="ch.qos.logback.core.rolling.SizeAndTimeBasedRollingPolicy">
      <fileNamePattern>%s.%%d{yyyy-MM-dd}.%%i</fileNamePattern>
      <maxFileSize>%s</maxFileSize>
      <maxHistory>%d</maxHistory>
      <totalSizeCap>%s</totalSizeCap>
    </rollingPolicy>
  </appender>
`, escapeXML(opts.FileOutputPath), pattern, escapeXML(opts.FileOutputPath), maxFileSize, maxHistory, totalSizeCap)
	}

	sb.WriteString("\n  <root level=\"INFO\">\n    <appender-ref ref=\"STDOUT\" />\n")
	if hasFile {
		sb.WriteString("    <appender-ref ref=\"FILE\" />\n")
	}
	sb.WriteString("  </root>\n")

	if len(configs) == 0 {
		sb.WriteString("</configuration>\n")
		return sb.String(), nil
	}

	// Sort logger names for deterministic output
	names := make([]string, 0, len(configs))
	for name := range configs {
		names = append(names, name)
	}
	sort.Strings(names)

	// Logger configurations
	for _, name := range names {
		config := configs[name]
		fmt.Fprintf(&sb, "  <logger name=\"%s\" level=\"%s\" />\n", escapeXML(name), config.Level)
	}

	sb.WriteString("</configuration>\n")
	return sb.String(), nil
}

// GeneratePythonLogging generates Python logging config.
// The output format is:
//
//	LOGGING = {
//	    'version': 1,
//	    'disable_existing_loggers': False,
//	    'formatters': {...},
//	    'handlers': {...},
//	    'loggers': {...},
//	    'root': {...}
//	}
func GeneratePythonLogging(configs map[string]LoggerConfig) (string, error) {
	var sb strings.Builder

	sb.WriteString(`# Python Logging Configuration
LOGGING = {
    'version': 1,
    'disable_existing_loggers': False,
    'formatters': {
        'standard': {
            'format': '%(asctime)s [%(levelname)s] %(name)s: %(message)s'
        },
    },
    'handlers': {
        'console': {
            'level': 'DEBUG',
            'class': 'logging.StreamHandler',
            'formatter': 'standard',
        },
    },
    'loggers': {
`)

	if len(configs) == 0 {
		sb.WriteString("    },\n")
	} else {
		// Sort logger names for deterministic output
		names := make([]string, 0, len(configs))
		for name := range configs {
			names = append(names, name)
		}
		sort.Strings(names)

		for i, name := range names {
			config := configs[name]
			pythonLevel := toPythonLogLevel(config.Level)
			fmt.Fprintf(&sb, "        '%s': {\n", name)
			fmt.Fprintf(&sb, "            'level': '%s',\n", pythonLevel)
			sb.WriteString("            'handlers': ['console'],\n")
			sb.WriteString("            'propagate': True,\n")
			if i < len(names)-1 {
				sb.WriteString("        },\n")
			} else {
				sb.WriteString("        },\n")
			}
		}
		sb.WriteString("    },\n")
	}

	sb.WriteString(`    'root': {
        'level': 'INFO',
        'handlers': ['console'],
    },
}
`)

	return sb.String(), nil
}

// escapeLoggerName escapes special characters in logger names for use as property keys.
func escapeLoggerName(s string) string {
	s = strings.ReplaceAll(s, ".", "_")
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, "$", "_")
	return s
}

// toPythonLogLevel converts a LogLevel to Python logging level.
func toPythonLogLevel(level LogLevel) string {
	switch level {
	case LogLevelTrace:
		return "DEBUG" // Python doesn't have TRACE, map to DEBUG
	case LogLevelDebug:
		return "DEBUG"
	case LogLevelInfo:
		return "INFO"
	case LogLevelWarn:
		return "WARNING"
	case LogLevelError:
		return "ERROR"
	case LogLevelFatal:
		return "CRITICAL"
	default:
		return "INFO"
	}
}
