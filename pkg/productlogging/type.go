package productlogging

import (
	"fmt"
	"maps"
	"math"
	"strings"

	loggingv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/constants"
)

const (
	DefalutLoggerLevel = "INFO"
	RootLoggerName     = "ROOT"

	DefaultLog4jConversionPattern   = "[%d] %p %m (%c)%n"
	DefaultLog4j2ConversionPattern  = "%d{ISO8601} %-5p %m%n"
	DefaultLogbackConversionPattern = "%d{ISO8601} %-5p [%t:%C{1}@%L] - %m%n"

	DefaultMaxLogFileSizeInMiB = 10
)

type TemplateData struct {
	RootLogLevel             string
	ConsoleLogLevel          string
	ConsoleConversionPattern string
	FileLogLevel             string
	LogDir                   string
	LogFile                  string
	MaxLogFileSizeInMiB      int
	NumberOfArchivedLogFiles int
	Loggers                  string
}

type LoggingConfigGenerator interface {
	Generate() string

	FileName() string
}

func NewBaseLoggingConfigGenerator(
	loggingConfigSpec *loggingv1alpha1.LoggingConfigSpec,
	containerName string,
	consoleConversionPattern string,
	maxLogFileSizeInMiB *float64,
	logFileName string,
	impl any) *BaseLoggingConfigGenerator {
	return &BaseLoggingConfigGenerator{
		LoggingConfigSpec:        loggingConfigSpec,
		contaienrName:            containerName,
		consoleConversionPattern: consoleConversionPattern,
		maxLogFileSizeInMiB:      maxLogFileSizeInMiB,
		logFileName:              logFileName,
		impl:                     impl,
	}
}

type BaseLoggingConfigGenerator struct {
	*loggingv1alpha1.LoggingConfigSpec
	contaienrName            string
	consoleConversionPattern string
	maxLogFileSizeInMiB      *float64
	logFileName              string

	impl any
}

func (b *BaseLoggingConfigGenerator) Config() *TemplateData {
	rootLogLevel := DefalutLoggerLevel
	consoleLogLevel := DefalutLoggerLevel
	fileLogLevel := DefalutLoggerLevel
	numberOfArchivedLogFiles := 1
	maxLogFileSizeInMiB := 10.0
	var loggerConfig string
	if b.LoggingConfigSpec != nil {
		loggersSpec := b.LoggingConfigSpec.Loggers
		consoleLogSpec := b.LoggingConfigSpec.Console
		fileLogSpec := b.LoggingConfigSpec.File
		// if console or file level is not empty, use it, otherwise use default
		consoleLogLevel = GetLoggerLevel(consoleLogSpec.Level != "", func() string { return consoleLogSpec.Level }, DefalutLoggerLevel)
		fileLogLevel = GetLoggerLevel(fileLogSpec.Level != "", func() string { return fileLogSpec.Level }, DefalutLoggerLevel)
		// extract root log level and logger names
		if len(loggersSpec) != 0 {
			// check root logger exists in loggers, if not, use default.
			if _, ok := loggersSpec[RootLoggerName]; ok {
				definedRootLogLevel := loggersSpec[RootLoggerName].Level
				if definedRootLogLevel != "" {
					rootLogLevel = definedRootLogLevel
				}
			}
			cloneLoggers := maps.Clone(loggersSpec)
			// Deletes the logger associated with the RootLogger key from the cloneLoggers map.
			// If the cloneLoggers map is not empty after deletion, concatenates the keys of the remaining loggers
			// with a comma and assigns the result to loggerNames.
			maps.DeleteFunc(cloneLoggers, func(key string, value *loggingv1alpha1.LogLevelSpec) bool { return key == RootLoggerName })
			if len(cloneLoggers) != 0 {
				loggerConfig = GetLoggers(b.impl, cloneLoggers)

			}
		}
		// compute max log file size
		var maxLogFileSize float64 = DefaultMaxLogFileSizeInMiB
		if b.maxLogFileSizeInMiB != nil {
			maxLogFileSize = *b.maxLogFileSizeInMiB
		}
		maxLogFileSizeInMiB = math.Max(1, float64(maxLogFileSize)/(1+float64(numberOfArchivedLogFiles)))
	}

	return &TemplateData{
		RootLogLevel:             rootLogLevel,
		ConsoleConversionPattern: b.consoleConversionPattern,
		ConsoleLogLevel:          consoleLogLevel,
		FileLogLevel:             fileLogLevel,
		LogDir:                   constants.KubedoopLogDir + strings.ToLower(string(b.contaienrName)) + "/",
		LogFile:                  b.logFileName,
		MaxLogFileSizeInMiB:      int(maxLogFileSizeInMiB),
		NumberOfArchivedLogFiles: numberOfArchivedLogFiles,
		Loggers:                  loggerConfig,
	}
}

// get Loggers
func createLoggerConfig(loggers map[string]*loggingv1alpha1.LogLevelSpec, createFunc func(name, lvl string) string) string {
	var configs = make([]string, 0)
	for name, lvl := range loggers {
		logger := createFunc(name, lvl.Level)
		configs = append(configs, logger)
	}
	return strings.Join(configs, "\n")
}

func GetLoggers(b any, loggers map[string]*loggingv1alpha1.LogLevelSpec) string {
	switch interface{}(b).(type) {
	case *LogbackConfigGenerator:
		return createLoggerConfig(loggers, func(name, lvl string) string {
			return fmt.Sprintf("<logger name=\"%s\" level=\"%s\"/>", name, lvl)
		})
	// case *Log4jConfigGenerator:
	// }
	// switch b.(type) {
	default:
		return ""
	}
}
