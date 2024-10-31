package productlogging

import (
	"fmt"
	"path"
	"strings"

	loggingv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/constants"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	DefalutLoggerLevel = "INFO"
	RootLoggerName     = "ROOT"

	DefaultLog4jConversionPattern   = "[%d] %p %m (%c)%n"
	DefaultLog4j2ConversionPattern  = "%d{ISO8601} %-5p %m%n"
	DefaultLogbackConversionPattern = "%d{ISO8601} %-5p [%t:%C{1}@%L] - %m%n"
)

type LogType int

const (
	LogTypeLog4j LogType = iota
	LogTypeLog4j2
	LogTypeLogback
	LogTypePythonLoging
)

type ProductLogging struct {
	RootLogLevel string
	// log msg format
	ConsoleHandlerFormatter string
	ConsoleHandlerLevel     string

	RotatingFileHandlerLevel       string
	RotatingFileHandlerFile        string
	RotatingFileHandlerMaxBytes    float64
	RotatingFileHandlerBackupCount int

	Loggers map[string]loggingv1alpha1.LogLevelSpec
}

type LoggingConfig interface {
	Template() string
	LoggerFormatter(name, level string) string
	String() string
	Content() (string, error)
}

func NewConfigGenerator(
	loggingConfigSpec *loggingv1alpha1.LoggingConfigSpec,
	containerName string,
	consoleHandlerFormatter string,
	logFileMaxBytes *float64,
	logFileName string,
	logType LogType) (*ConfigGenerator, error) {

	return &ConfigGenerator{
		loggingConfigSpec:       loggingConfigSpec,
		contaienrName:           containerName,
		consoleHandlerFormatter: consoleHandlerFormatter,
		logFileMaxBytes:         logFileMaxBytes,
		logFileName:             logFileName,
	}, nil
}

type ConfigGenerator struct {
	loggingConfigSpec       *loggingv1alpha1.LoggingConfigSpec
	contaienrName           string
	consoleHandlerFormatter string
	logFileMaxBytes         *float64
	logFileName             string

	logType LogType
}

func (b *ConfigGenerator) getLoggingConfig() (LoggingConfig, error) {
	productLogging := b.getProductLogging()
	var loggingConfig LoggingConfig
	switch b.logType {
	case LogTypeLog4j:
		loggingConfig = &Log4jConfig{productLogging: productLogging}
	case LogTypeLog4j2:
		loggingConfig = &Log4j2Config{productLogging: productLogging}
	case LogTypeLogback:
		loggingConfig = &LogbackConfig{productLogging: productLogging}
	default:
		return nil, fmt.Errorf("unsupported log type: %v", b.logType)
	}

	return loggingConfig, nil
}

func (l *ConfigGenerator) Content() (string, error) {
	loggingConfig, err := l.getLoggingConfig()
	if err != nil {
		return "", err
	}
	return loggingConfig.Content()
}

func (b *ConfigGenerator) getProductLogging() *ProductLogging {
	productLogging := &ProductLogging{
		RootLogLevel:            DefalutLoggerLevel,
		ConsoleHandlerLevel:     DefalutLoggerLevel,
		ConsoleHandlerFormatter: b.consoleHandlerFormatter,

		RotatingFileHandlerLevel: DefalutLoggerLevel,
		RotatingFileHandlerFile:  path.Join(constants.KubedoopLogDir, strings.ToLower(b.contaienrName), b.logFileName),
		// Default File size is 10MB
		RotatingFileHandlerMaxBytes:    10 * 1024 * 1024,
		RotatingFileHandlerBackupCount: 1,

		Loggers: make(map[string]loggingv1alpha1.LogLevelSpec),
	}

	if b.loggingConfigSpec != nil {
		// If console and file log levels are defined, use them. Otherwise, use the default log level.
		if b.loggingConfigSpec.Console != nil && b.loggingConfigSpec.Console.Level != "" {
			productLogging.ConsoleHandlerLevel = b.loggingConfigSpec.Console.Level
		}
		if b.loggingConfigSpec.File != nil && b.loggingConfigSpec.File.Level != "" {
			productLogging.RotatingFileHandlerLevel = b.loggingConfigSpec.File.Level
		}

		if b.loggingConfigSpec.Loggers != nil {
			for name, level := range b.loggingConfigSpec.Loggers {
				if name == RootLoggerName {
					productLogging.RootLogLevel = level.Level
				} else {
					productLogging.Loggers[name] = *level
				}
			}
		}
	}

	return productLogging
}

func JavaLogTemplateValue(loggingConfig LoggingConfig, productLogging *ProductLogging) map[string]interface{} {
	values := map[string]interface{}{}
	values["RootLogLevel"] = productLogging.RootLogLevel
	values["ConsoleHandlerLevel"] = productLogging.ConsoleHandlerLevel
	values["ConsoleHandlerFormatter"] = productLogging.ConsoleHandlerFormatter
	values["RotatingFileHandlerLevel"] = productLogging.RotatingFileHandlerLevel
	values["RotatingFileHandlerFile"] = productLogging.RotatingFileHandlerFile
	values["RotatingFileHandlerMaxSizeInMiB"] = productLogging.RotatingFileHandlerMaxBytes / 1024 / 1024
	values["RotatingFileHandlerBackupCount"] = productLogging.RotatingFileHandlerBackupCount
	loggers := []string{}

	for name, level := range productLogging.Loggers {
		loggers = append(loggers, loggingConfig.LoggerFormatter(name, level.Level))
	}
	values["Loggers"] = strings.Join(loggers, "\n")

	return values
}

// calculate_log_volume_size_limit calculates the log volume size limit based on the given max log file sizes.
// The limit is calculated by summing up all the given sizes, scaling the result to MEBI and multiplying it by 3.0.
// The result is then ceiled to avoid bulky numbers due to floating-point arithmetic.
func CalculateLogVolumeSizeLimit(maxLogFilesSize []resource.Quantity) resource.Quantity {
	logVolumeSizeLimit := resource.Quantity{}
	for _, q := range maxLogFilesSize {
		logVolumeSizeLimit.Add(q)
	}
	// According to the reasons mentioned in the function documentation, the multiplier must be
	// greater than 2. Manual tests with ZooKeeper 3.8 in an OpenShift cluster showed that 3 is
	// absolutely sufficient.
	logVolumeSizeLimit.Set(logVolumeSizeLimit.Value() * 3.0)
	return logVolumeSizeLimit
}
