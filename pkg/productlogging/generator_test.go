package productlogging

import (
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	loggingv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
)

func TestProductLogging(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ProductLogging Suite")
}

var _ = Describe("ConfigGenerator", func() {
	var (
		loggingConfigSpec *loggingv1alpha1.LoggingConfigSpec
		containerName     string
		logFileMaxBytes   float64
		logFileName       string
		configGenerator   *ConfigGenerator
		err               error
	)

	BeforeEach(func() {
		loggingConfigSpec = &loggingv1alpha1.LoggingConfigSpec{}
		containerName = "test-container"
		logFileMaxBytes = 10.0 * 1024 * 1024
		logFileName = "test.log"
	})

	DescribeTable("getProductLogging",
		func(logType LogType, expectedFormatter string) {
			loggingConfigSpec = &loggingv1alpha1.LoggingConfigSpec{
				Console: &loggingv1alpha1.LogLevelSpec{
					Level: "DEBUG",
				},
				File: &loggingv1alpha1.LogLevelSpec{
					Level: "ERROR",
				},
				Loggers: map[string]*loggingv1alpha1.LogLevelSpec{
					"ROOT": {Level: "WARN"},
					"app":  {Level: "INFO"},
				},
			}
			configGenerator, err = NewConfigGenerator(
				loggingConfigSpec,
				containerName,
				logFileName,
				logType,
				func(o *ConfigGeneratorOption) {
					o.LogFileMaxBytes = &logFileMaxBytes
				},
			)
			Expect(err).NotTo(HaveOccurred())

			productLogging, err := configGenerator.getProductLogging()
			Expect(err).NotTo(HaveOccurred())
			Expect(productLogging.RootLogLevel).To(Equal("WARN"))
			Expect(productLogging.ConsoleHandlerLevel).To(Equal("DEBUG"))
			Expect(productLogging.RotatingFileHandlerLevel).To(Equal("ERROR"))
			Expect(productLogging.ConsoleHandlerFormatter).To(Equal(expectedFormatter))
			Expect(productLogging.RotatingFileHandlerMaxBytes).To(Equal(10.0 * 1024 * 1024))
			Expect(productLogging.RotatingFileHandlerBackupCount).To(Equal(1))
			Expect(productLogging.Loggers["app"].Level).To(Equal("INFO"))
		},
		Entry("Log4j", LogTypeLog4j, DefaultLog4jConversionPattern),
		Entry("Log4j2", LogTypeLog4j2, DefaultLog4j2ConversionPattern),
		Entry("Logback", LogTypeLogback, DefaultLogbackConversionPattern),
	)

	DescribeTable("Content",
		func(logType LogType, expectedContent string) {
			loggingConfigSpec = &loggingv1alpha1.LoggingConfigSpec{
				Console: &loggingv1alpha1.LogLevelSpec{
					Level: "DEBUG",
				},
				File: &loggingv1alpha1.LogLevelSpec{
					Level: "ERROR",
				},
				Loggers: map[string]*loggingv1alpha1.LogLevelSpec{
					"ROOT": {Level: "WARN"},
					"app":  {Level: "INFO"},
				},
			}
			configGenerator, err = NewConfigGenerator(
				loggingConfigSpec,
				containerName,
				logFileName,
				logType,
				func(o *ConfigGeneratorOption) {
					o.LogFileMaxBytes = &logFileMaxBytes
				},
			)
			Expect(err).NotTo(HaveOccurred())

			content, err := configGenerator.Content()
			Expect(err).NotTo(HaveOccurred())
			Expect(content).To(ContainSubstring(expectedContent))
		},
		Entry("Log4j", LogTypeLog4j, fmt.Sprintf("log4j.appender.CONSOLE.layout.ConversionPattern=%s", DefaultLog4jConversionPattern)),
		Entry("Log4j2", LogTypeLog4j2, fmt.Sprintf("appender.CONSOLE.layout.pattern = %s", DefaultLog4j2ConversionPattern)),
		Entry("Logback", LogTypeLogback, fmt.Sprintf("      <pattern>%s</pattern>", DefaultLogbackConversionPattern)),
	)
})
