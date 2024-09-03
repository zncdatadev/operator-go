package productlogging_test

import (
	loggingv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/productlogging"
)

var _ = Describe("Log4j2ConfigGenerator", func() {
	var (
		generator productlogging.Log4j2ConfigGenerator
	)

	Context("when parsing is successful", func() {
		It("should return parsed configuration string", func() {
			loggingConfigSpec := &loggingv1alpha1.LoggingConfigSpec{
				Loggers: map[string]*loggingv1alpha1.LogLevelSpec{
					"a": {
						Level: "INFO",
					},
					"b": {
						Level: "DEBUG",
					},
				},
				Console: &loggingv1alpha1.LogLevelSpec{
					Level: "INFO",
				},
				File: &loggingv1alpha1.LogLevelSpec{
					Level: "DEBUG",
				},
			}
			generator = *productlogging.NewLog4j2ConfigGenerator(loggingConfigSpec, "zoo_container", productlogging.DefaultLog4j2ConversionPattern, nil, "app.log", "logback.xml")
			result := generator.Generate()
			Expect(result).Should(ContainSubstring("appenders = FILE, CONSOLE"))
			Expect(result).Should(ContainSubstring("appender.CONSOLE.type = Console"))
			Expect(result).Should(ContainSubstring("appender.CONSOLE.name = CONSOLE"))
			Expect(result).Should(ContainSubstring("appender.CONSOLE.target = SYSTEM_ERR"))
			Expect(result).Should(ContainSubstring("appender.CONSOLE.layout.type = PatternLayout"))
			Expect(result).Should(ContainSubstring("appender.CONSOLE.layout.pattern = %d{ISO8601} %-5p %m%n"))
			Expect(result).Should(ContainSubstring("appender.CONSOLE.filter.threshold.type = ThresholdFilter"))
			Expect(result).Should(ContainSubstring("appender.CONSOLE.filter.threshold.level = INFO"))

			Expect(result).Should(ContainSubstring("appender.FILE.type = RollingFile"))
			Expect(result).Should(ContainSubstring("appender.FILE.name = FILE"))
			Expect(result).Should(ContainSubstring("appender.FILE.fileName = /kubedoop/log/zoo_container/app.log"))
			Expect(result).Should(ContainSubstring("appender.FILE.filePattern = /kubedoop/log/zoo_container/app.log.%i"))
			Expect(result).Should(ContainSubstring("appender.FILE.layout.type = XMLLayout"))
			Expect(result).Should(ContainSubstring("appender.FILE.policies.type = Policies"))
			Expect(result).Should(ContainSubstring("appender.FILE.policies.size.type = SizeBasedTriggeringPolicy"))
			Expect(result).Should(ContainSubstring("appender.FILE.policies.size.size = 5MB"))
			Expect(result).Should(ContainSubstring("appender.FILE.strategy.type = DefaultRolloverStrategy"))
			Expect(result).Should(ContainSubstring("appender.FILE.strategy.max = 1"))
			Expect(result).Should(ContainSubstring("appender.FILE.filter.threshold.type = ThresholdFilter"))
			Expect(result).Should(ContainSubstring("appender.FILE.filter.threshold.level = DEBUG"))

			Expect(result).Should(ContainSubstring("loggers = a,b"))
			Expect(result).Should(ContainSubstring("logger.a.name = a"))
			Expect(result).Should(ContainSubstring("logger.a.name = INFO"))
			Expect(result).Should(ContainSubstring("logger.b.name = b"))
			Expect(result).Should(ContainSubstring("logger.b.name = DEBUG"))
			Expect(result).Should(ContainSubstring("rootLogger.level=INFO"))
			Expect(result).Should(ContainSubstring("rootLogger.appenderRefs = CONSOLE, FILE"))
			Expect(result).Should(ContainSubstring("rootLogger.appenderRef.CONSOLE.ref = CONSOLE"))
			Expect(result).Should(ContainSubstring("rootLogger.appenderRef.FILE.ref = FILE"))

		})
	})

	Context("when parsing is successful", func() {
		It("should return default string when loggingConfigSpec is nil", func() {
			generator = *productlogging.NewLog4j2ConfigGenerator(nil, "zoo_container", productlogging.DefaultLog4j2ConversionPattern, nil, "app.log", "logback.xml")
			result := generator.Generate()
			Expect(result).Should(ContainSubstring("appenders = FILE, CONSOLE"))
			Expect(result).Should(ContainSubstring("appender.CONSOLE.type = Console"))
			Expect(result).Should(ContainSubstring("appender.CONSOLE.name = CONSOLE"))
			Expect(result).Should(ContainSubstring("appender.CONSOLE.target = SYSTEM_ERR"))
			Expect(result).Should(ContainSubstring("appender.CONSOLE.layout.type = PatternLayout"))
			Expect(result).Should(ContainSubstring("appender.CONSOLE.layout.pattern = %d{ISO8601} %-5p %m%n"))
			Expect(result).Should(ContainSubstring("appender.CONSOLE.filter.threshold.type = ThresholdFilter"))
			Expect(result).Should(ContainSubstring("appender.CONSOLE.filter.threshold.level = INFO"))

			Expect(result).Should(ContainSubstring("appender.FILE.type = RollingFile"))
			Expect(result).Should(ContainSubstring("appender.FILE.name = FILE"))
			Expect(result).Should(ContainSubstring("appender.FILE.fileName = /kubedoop/log/zoo_container/app.log"))
			Expect(result).Should(ContainSubstring("appender.FILE.filePattern = /kubedoop/log/zoo_container/app.log.%i"))
			Expect(result).Should(ContainSubstring("appender.FILE.layout.type = XMLLayout"))
			Expect(result).Should(ContainSubstring("appender.FILE.policies.type = Policies"))
			Expect(result).Should(ContainSubstring("appender.FILE.policies.size.type = SizeBasedTriggeringPolicy"))
			Expect(result).Should(ContainSubstring("appender.FILE.policies.size.size = 10MB"))
			Expect(result).Should(ContainSubstring("appender.FILE.strategy.type = DefaultRolloverStrategy"))
			Expect(result).Should(ContainSubstring("appender.FILE.strategy.max = 1"))
			Expect(result).Should(ContainSubstring("appender.FILE.filter.threshold.type = ThresholdFilter"))
			Expect(result).Should(ContainSubstring("appender.FILE.filter.threshold.level = INFO"))

			Expect(result).Should(ContainSubstring("rootLogger.level=INFO"))
			Expect(result).Should(ContainSubstring("rootLogger.appenderRefs = CONSOLE, FILE"))
			Expect(result).Should(ContainSubstring("rootLogger.appenderRef.CONSOLE.ref = CONSOLE"))
			Expect(result).Should(ContainSubstring("rootLogger.appenderRef.FILE.ref = FILE"))
		})
	})

})
