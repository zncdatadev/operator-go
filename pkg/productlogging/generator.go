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
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// boundedFileDefaults applies sensible defaults to the rolling-file bounds so a generated
// file appender can never grow without limit. Takes raw fields (rather than an options
// struct) so both RenderOptions and LogbackOptions callers can share it.
func boundedFileDefaults(maxFileSize string, maxHistory int, totalSizeCap string) (string, int, string) {
	if maxFileSize == "" {
		maxFileSize = "5MB"
	}
	if maxHistory <= 0 {
		maxHistory = 1
	}
	if totalSizeCap == "" {
		totalSizeCap = "8MB"
	}
	return maxFileSize, maxHistory, totalSizeCap
}

// parseSizeBytes converts a size string like "5MB" / "512KB" / "1024" into bytes, returning
// fallback when it cannot be parsed. Used where a numeric byte count is required (Python).
func parseSizeBytes(s string, fallback int64) int64 {
	s = strings.TrimSpace(strings.ToUpper(s))
	mult := int64(1)
	switch {
	case strings.HasSuffix(s, "KB"):
		mult, s = 1024, strings.TrimSuffix(s, "KB")
	case strings.HasSuffix(s, "MB"):
		mult, s = 1024*1024, strings.TrimSuffix(s, "MB")
	case strings.HasSuffix(s, "GB"):
		mult, s = 1024*1024*1024, strings.TrimSuffix(s, "GB")
	}
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || n <= 0 {
		return fallback
	}
	return n * mult
}

// escapeXML escapes the five XML special characters so caller-supplied strings (logger
// names, patterns, file paths) cannot produce invalid logback XML.
func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

// LoggingFramework defines the logging framework type.
type LoggingFramework string

const (
	LoggingFrameworkLog4j   LoggingFramework = "log4j"
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
	case LoggingFrameworkLog4j:
		return GenerateLog4j(configs)
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

// GenerateLog4j generates Log4j 1.x properties format (as consumed by log4j 1.2 / reload4j).
// The output format is:
// log4j.rootLogger=INFO, CONSOLE
// log4j.appender.CONSOLE=org.apache.log4j.ConsoleAppender
// log4j.appender.CONSOLE.layout=org.apache.log4j.PatternLayout
// log4j.appender.CONSOLE.layout.ConversionPattern=%d{yyyy-MM-dd HH:mm:ss} %-5p %c{1}:%L - %m%n
// log4j.logger.com.example=DEBUG
func GenerateLog4j(configs map[string]LoggerConfig) (string, error) {
	return renderLog4j(LogConfig{Loggers: loggerConfigsToLevels(configs)}, RenderOptions{})
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
	return renderLog4j2(LogConfig{Loggers: loggerConfigsToLevels(configs)}, RenderOptions{})
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

	// RootLevel overrides the level of the root logger. Defaults to INFO when empty.
	RootLevel LogLevel

	// ConsoleLevel, when set, adds a ThresholdFilter to the console (STDOUT) appender so
	// messages below this level are dropped. Empty means no threshold.
	ConsoleLevel LogLevel

	// FileLevel, when set, adds a ThresholdFilter to the rolling file appender. Only
	// applies when FileOutputPath is set. Empty means no threshold.
	FileLevel LogLevel

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
%s    <encoder>
      <pattern>%s</pattern>
    </encoder>
  </appender>
`, logbackThresholdFilter(opts.ConsoleLevel), pattern)

	hasFile := opts.FileOutputPath != ""
	if hasFile {
		maxFileSize, maxHistory, totalSizeCap := boundedFileDefaults(opts.MaxFileSize, opts.MaxHistory, opts.TotalSizeCap)
		fmt.Fprintf(&sb, `
  <appender name="FILE" class="ch.qos.logback.core.rolling.RollingFileAppender">
    <file>%s</file>
%s    <encoder>
      <pattern>%s</pattern>
    </encoder>
    <rollingPolicy class="ch.qos.logback.core.rolling.SizeAndTimeBasedRollingPolicy">
      <fileNamePattern>%s.%%d{yyyy-MM-dd}.%%i</fileNamePattern>
      <maxFileSize>%s</maxFileSize>
      <maxHistory>%d</maxHistory>
      <totalSizeCap>%s</totalSizeCap>
    </rollingPolicy>
  </appender>
`, escapeXML(opts.FileOutputPath), logbackThresholdFilter(opts.FileLevel), pattern, escapeXML(opts.FileOutputPath), maxFileSize, maxHistory, totalSizeCap)
	}

	rootLevel := opts.RootLevel
	if rootLevel == "" {
		rootLevel = LogLevelInfo
	}
	fmt.Fprintf(&sb, "\n  <root level=\"%s\">\n    <appender-ref ref=\"STDOUT\" />\n", escapeXML(string(rootLevel)))
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
	return renderPython(LogConfig{Loggers: loggerConfigsToLevels(configs)}, RenderOptions{})
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
		return string(LogLevelDebug) // Python doesn't have TRACE, map to DEBUG
	case LogLevelDebug:
		return string(LogLevelDebug)
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

// loggerConfigsToLevels adapts the legacy LoggerConfig map to a plain name->level map.
func loggerConfigsToLevels(configs map[string]LoggerConfig) map[string]LogLevel {
	if len(configs) == 0 {
		return nil
	}
	out := make(map[string]LogLevel, len(configs))
	for name, c := range configs {
		out[name] = c.Level
	}
	return out
}

// sortedLoggerNames returns the logger names in deterministic (sorted) order.
func sortedLoggerNames(loggers map[string]LogLevel) []string {
	names := make([]string, 0, len(loggers))
	for name := range loggers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// logbackThresholdFilter renders a logback ThresholdFilter element (indented to sit inside
// an <appender>), or an empty string when no threshold is requested.
func logbackThresholdFilter(level LogLevel) string {
	if level == "" {
		return ""
	}
	return fmt.Sprintf("    <filter class=\"ch.qos.logback.classic.filter.ThresholdFilter\">\n      <level>%s</level>\n    </filter>\n", escapeXML(string(level)))
}

// renderLogback renders logback XML from the framework-neutral model.
func renderLogback(cfg LogConfig, opts RenderOptions) (string, error) {
	return GenerateLogbackWithOptions(loggersToLoggerConfigs(cfg.Loggers), LogbackOptions{
		Pattern:        opts.Pattern,
		RootLevel:      cfg.RootLevel,
		ConsoleLevel:   cfg.ConsoleLevel,
		FileLevel:      cfg.FileLevel,
		FileOutputPath: opts.FileOutputPath,
		MaxFileSize:    opts.MaxFileSize,
		MaxHistory:     opts.MaxHistory,
		TotalSizeCap:   opts.TotalSizeCap,
	})
}

// renderLog4j renders log4j 1.x properties (log4j 1.2 / reload4j) from the framework-neutral
// model. When opts.FileOutputPath is set it adds a bounded RollingFileAppender wired to the
// root logger. Both appenders use a plain-text PatternLayout so the file output matches the
// Vector "files_stdout" consumer (see LogFileSuffix). Patterns are emitted verbatim: '%' has
// no special meaning in log4j properties values, so conversion patterns like
// "[%d] %p %m (%c)%n" need no escaping.
func renderLog4j(cfg LogConfig, opts RenderOptions) (string, error) {
	pattern := opts.Pattern
	if pattern == "" {
		pattern = "%d{yyyy-MM-dd HH:mm:ss} %-5p %c{1}:%L - %m%n"
	}
	rootLevel := cfg.RootLevel
	if rootLevel == "" {
		rootLevel = LogLevelInfo
	}
	hasFile := opts.FileOutputPath != ""

	var sb strings.Builder
	sb.WriteString("# Log4j Configuration\n")
	if hasFile {
		fmt.Fprintf(&sb, "log4j.rootLogger=%s, CONSOLE, FILE\n\n", rootLevel)
	} else {
		fmt.Fprintf(&sb, "log4j.rootLogger=%s, CONSOLE\n\n", rootLevel)
	}

	sb.WriteString("# Appenders\n")
	sb.WriteString("log4j.appender.CONSOLE=org.apache.log4j.ConsoleAppender\n")
	if cfg.ConsoleLevel != "" {
		fmt.Fprintf(&sb, "log4j.appender.CONSOLE.Threshold=%s\n", cfg.ConsoleLevel)
	}
	sb.WriteString("log4j.appender.CONSOLE.layout=org.apache.log4j.PatternLayout\n")
	fmt.Fprintf(&sb, "log4j.appender.CONSOLE.layout.ConversionPattern=%s\n", pattern)

	if hasFile {
		// Bounded rollover (MaxFileSize + MaxBackupIndex) so the file cannot grow without limit.
		maxFileSize, maxHistory, _ := boundedFileDefaults(opts.MaxFileSize, opts.MaxHistory, opts.TotalSizeCap)
		sb.WriteString("\nlog4j.appender.FILE=org.apache.log4j.RollingFileAppender\n")
		if cfg.FileLevel != "" {
			fmt.Fprintf(&sb, "log4j.appender.FILE.Threshold=%s\n", cfg.FileLevel)
		}
		fmt.Fprintf(&sb, "log4j.appender.FILE.File=%s\n", opts.FileOutputPath)
		fmt.Fprintf(&sb, "log4j.appender.FILE.MaxFileSize=%s\n", maxFileSize)
		fmt.Fprintf(&sb, "log4j.appender.FILE.MaxBackupIndex=%d\n", maxHistory)
		sb.WriteString("log4j.appender.FILE.layout=org.apache.log4j.PatternLayout\n")
		fmt.Fprintf(&sb, "log4j.appender.FILE.layout.ConversionPattern=%s\n", pattern)
	}

	if len(cfg.Loggers) == 0 {
		return sb.String(), nil
	}
	sb.WriteString("\n# Loggers\n")
	for _, name := range sortedLoggerNames(cfg.Loggers) {
		fmt.Fprintf(&sb, "log4j.logger.%s=%s\n", name, cfg.Loggers[name])
	}
	return sb.String(), nil
}

// renderLog4j2 renders log4j2 properties from the framework-neutral model. When
// opts.FileOutputPath is set it adds a rolling file appender wired to the root logger.
func renderLog4j2(cfg LogConfig, opts RenderOptions) (string, error) {
	pattern := opts.Pattern
	if pattern == "" {
		pattern = "%d{yyyy-MM-dd HH:mm:ss} %-5p %c{1}:%L - %m%n"
	}
	rootLevel := cfg.RootLevel
	if rootLevel == "" {
		rootLevel = LogLevelInfo
	}
	hasFile := opts.FileOutputPath != ""

	var sb strings.Builder
	sb.WriteString("# Log4j2 Configuration\n")
	fmt.Fprintf(&sb, "rootLogger.level=%s\n", rootLevel)
	if hasFile {
		sb.WriteString("rootLogger.appenderRefs=stdout,file\n\n")
	} else {
		sb.WriteString("rootLogger.appenderRefs=stdout\n\n")
	}

	sb.WriteString("# Appenders\n")
	if hasFile {
		sb.WriteString("appenders=console,file\n")
	} else {
		sb.WriteString("appenders=console\n")
	}
	sb.WriteString("appender.console.type=Console\n")
	sb.WriteString("appender.console.name=STDOUT\n")
	sb.WriteString("appender.console.layout.type=PatternLayout\n")
	fmt.Fprintf(&sb, "appender.console.layout.pattern=%s\n", pattern)
	if cfg.ConsoleLevel != "" {
		sb.WriteString("appender.console.filter.threshold.type=ThresholdFilter\n")
		fmt.Fprintf(&sb, "appender.console.filter.threshold.level=%s\n", cfg.ConsoleLevel)
	}
	sb.WriteString("\n")
	if hasFile {
		maxFileSize, maxHistory, _ := boundedFileDefaults(opts.MaxFileSize, opts.MaxHistory, opts.TotalSizeCap)
		sb.WriteString("appender.file.type=RollingFile\n")
		sb.WriteString("appender.file.name=FILE\n")
		fmt.Fprintf(&sb, "appender.file.fileName=%s\n", opts.FileOutputPath)
		fmt.Fprintf(&sb, "appender.file.filePattern=%s.%%d{yyyy-MM-dd}.%%i\n", opts.FileOutputPath)
		sb.WriteString("appender.file.layout.type=PatternLayout\n")
		fmt.Fprintf(&sb, "appender.file.layout.pattern=%s\n", pattern)
		if cfg.FileLevel != "" {
			sb.WriteString("appender.file.filter.threshold.type=ThresholdFilter\n")
			fmt.Fprintf(&sb, "appender.file.filter.threshold.level=%s\n", cfg.FileLevel)
		}
		// Bounded rollover so the file cannot grow without limit.
		sb.WriteString("appender.file.policies.type=Policies\n")
		sb.WriteString("appender.file.policies.size.type=SizeBasedTriggeringPolicy\n")
		fmt.Fprintf(&sb, "appender.file.policies.size.size=%s\n", maxFileSize)
		sb.WriteString("appender.file.strategy.type=DefaultRolloverStrategy\n")
		fmt.Fprintf(&sb, "appender.file.strategy.max=%d\n", maxHistory)
		sb.WriteString("\n")
	}

	if len(cfg.Loggers) == 0 {
		return sb.String(), nil
	}
	names := sortedLoggerNames(cfg.Loggers)
	sb.WriteString("# Loggers\n")
	sb.WriteString("loggers=")
	sb.WriteString(strings.Join(names, ","))
	sb.WriteString("\n\n")
	for _, name := range names {
		safeName := escapeLoggerName(name)
		fmt.Fprintf(&sb, "logger.%s.name=%s\n", safeName, name)
		fmt.Fprintf(&sb, "logger.%s.level=%s\n\n", safeName, cfg.Loggers[name])
	}
	return sb.String(), nil
}

// renderPython renders a Python logging.config dictConfig from the framework-neutral model.
// When opts.FileOutputPath is set it adds a file handler wired to the root logger.
func renderPython(cfg LogConfig, opts RenderOptions) (string, error) {
	rootLevel := toPythonLogLevel(cfg.RootLevel)
	consoleLevel := string(LogLevelDebug)
	if cfg.ConsoleLevel != "" {
		consoleLevel = toPythonLogLevel(cfg.ConsoleLevel)
	}
	hasFile := opts.FileOutputPath != ""

	var sb strings.Builder
	sb.WriteString("# Python Logging Configuration\n")
	sb.WriteString("LOGGING = {\n")
	sb.WriteString("    'version': 1,\n")
	sb.WriteString("    'disable_existing_loggers': False,\n")
	sb.WriteString("    'formatters': {\n")
	sb.WriteString("        'standard': {\n")
	sb.WriteString("            'format': '%(asctime)s [%(levelname)s] %(name)s: %(message)s'\n")
	sb.WriteString("        },\n")
	sb.WriteString("    },\n")
	sb.WriteString("    'handlers': {\n")
	sb.WriteString("        'console': {\n")
	fmt.Fprintf(&sb, "            'level': '%s',\n", consoleLevel)
	sb.WriteString("            'class': 'logging.StreamHandler',\n")
	sb.WriteString("            'formatter': 'standard',\n")
	sb.WriteString("        },\n")
	if hasFile {
		fileLevel := string(LogLevelDebug)
		if cfg.FileLevel != "" {
			fileLevel = toPythonLogLevel(cfg.FileLevel)
		}
		maxFileSize, maxHistory, _ := boundedFileDefaults(opts.MaxFileSize, opts.MaxHistory, opts.TotalSizeCap)
		maxBytes := parseSizeBytes(maxFileSize, 5*1024*1024)
		sb.WriteString("        'file': {\n")
		fmt.Fprintf(&sb, "            'level': '%s',\n", fileLevel)
		sb.WriteString("            'class': 'logging.handlers.RotatingFileHandler',\n")
		fmt.Fprintf(&sb, "            'filename': '%s',\n", opts.FileOutputPath)
		// Bound the file so rotation is actually enabled (maxBytes=0 would disable it).
		fmt.Fprintf(&sb, "            'maxBytes': %d,\n", maxBytes)
		fmt.Fprintf(&sb, "            'backupCount': %d,\n", maxHistory)
		sb.WriteString("            'formatter': 'standard',\n")
		sb.WriteString("        },\n")
	}
	sb.WriteString("    },\n")
	sb.WriteString("    'loggers': {\n")
	rootHandlers := "['console']"
	if hasFile {
		rootHandlers = "['console', 'file']"
	}
	for _, name := range sortedLoggerNames(cfg.Loggers) {
		fmt.Fprintf(&sb, "        '%s': {\n", name)
		fmt.Fprintf(&sb, "            'level': '%s',\n", toPythonLogLevel(cfg.Loggers[name]))
		fmt.Fprintf(&sb, "            'handlers': %s,\n", rootHandlers)
		sb.WriteString("            'propagate': True,\n")
		sb.WriteString("        },\n")
	}
	sb.WriteString("    },\n")
	fmt.Fprintf(&sb, "    'root': {\n        'level': '%s',\n        'handlers': %s,\n    },\n}\n", rootLevel, rootHandlers)
	return sb.String(), nil
}
