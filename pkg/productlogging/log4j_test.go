package productlogging_test

import (
	"fmt"

	loggingv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/productlogging"
)

var _ = Describe("Log4jConfigGenerator", func() {
	var (
		generator productlogging.Log4jConfigGenerator
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
			generator = *productlogging.NewLog4jConfigGenerator(loggingConfigSpec, "zoo_container", productlogging.DefaultLog4jConversionPattern, nil, "app.log", "logback.xml")
			result := generator.Generate()
			Expect(result).Should(ContainSubstring("log4j.rootLogger=INFO, CONSOLE, FILE"))
			Expect(result).Should(ContainSubstring("log4j.appender.CONSOLE=org.apache.log4j.ConsoleAppender"))
			Expect(result).Should(ContainSubstring("log4j.appender.CONSOLE.Threshold=INFO"))
			Expect(result).Should(ContainSubstring("log4j.appender.CONSOLE.layout=org.apache.log4j.PatternLayout"))
			Expect(result).Should(ContainSubstring("log4j.appender.CONSOLE.layout.ConversionPattern=[%d] %p %m (%c)%n"))

			Expect(result).Should(ContainSubstring("log4j.appender.FILE=org.apache.log4j.RollingFileAppender"))
			Expect(result).Should(ContainSubstring("log4j.appender.FILE.Threshold=DEBUG"))
			Expect(result).Should(ContainSubstring("log4j.appender.FILE.File=/kubedoop/log/zoo_container/app.log"))
			Expect(result).Should(ContainSubstring("log4j.appender.FILE.MaxFileSize=5MB"))
			Expect(result).Should(ContainSubstring("log4j.appender.FILE.MaxBackupIndex=1"))
			Expect(result).Should(ContainSubstring("log4j.appender.FILE.layout=org.apache.log4j.xml.XMLLayout"))

			Expect(result).Should(ContainSubstring("log4j.logger.a=INFO"))
			Expect(result).Should(ContainSubstring("log4j.logger.b=DEBUG"))
		})
	})

	Context("when parsing is successful", func() {
		It("should return default string when loggingConfigSpec is nil", func() {
			generator = *productlogging.NewLog4jConfigGenerator(nil, "zoo_container", productlogging.DefaultLog4jConversionPattern, nil, "app.log", "logback.xml")
			result := generator.Generate()
			fmt.Println(result)
			Expect(result).Should(ContainSubstring("log4j.rootLogger=INFO, CONSOLE, FILE"))
			Expect(result).Should(ContainSubstring("log4j.appender.CONSOLE=org.apache.log4j.ConsoleAppender"))
			Expect(result).Should(ContainSubstring("log4j.appender.CONSOLE.Threshold=INFO"))
			Expect(result).Should(ContainSubstring("log4j.appender.CONSOLE.layout=org.apache.log4j.PatternLayout"))
			Expect(result).Should(ContainSubstring("log4j.appender.CONSOLE.layout.ConversionPattern=[%d] %p %m (%c)%n"))

			Expect(result).Should(ContainSubstring("log4j.appender.FILE=org.apache.log4j.RollingFileAppender"))
			Expect(result).Should(ContainSubstring("log4j.appender.FILE.Threshold=INFO"))
			Expect(result).Should(ContainSubstring("log4j.appender.FILE.File=/kubedoop/log/zoo_container/app.log"))
			Expect(result).Should(ContainSubstring("log4j.appender.FILE.MaxFileSize=10MB"))
			Expect(result).Should(ContainSubstring("log4j.appender.FILE.MaxBackupIndex=1"))
			Expect(result).Should(ContainSubstring("log4j.appender.FILE.layout=org.apache.log4j.xml.XMLLayout"))
		})
	})

})
