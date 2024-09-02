package productlogging_test

import (
	"fmt"

	loggingv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/productlogging"
)

var _ = Describe("LogbackConfigGenerator", func() {
	var (
		generator productlogging.LogbackConfigGenerator
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
			generator = *productlogging.NewLogbackConfigGenerator(loggingConfigSpec, "zoo_container", productlogging.DefaultLogbackConversionPattern, nil, "app.log", "logback.xml")
			result := generator.Generate()
			fmt.Println(result)
			Expect(result).ShouldNot(BeEmpty())
			Expect(result).Should(ContainSubstring("<logger name=\"a\" level=\"INFO\"/>"))
			Expect(result).Should(ContainSubstring("<logger name=\"b\" level=\"DEBUG\"/>"))
			Expect(result).Should(ContainSubstring("<root level=\"INFO\">"))
			Expect(result).Should(ContainSubstring(`<filter class="ch.qos.logback.classic.filter.ThresholdFilter">
  <level>DEBUG</level>
</filter>`))
			Expect(result).Should(ContainSubstring(`<filter class="ch.qos.logback.classic.filter.ThresholdFilter">
  <level>INFO</level>
</filter>`))
			Expect(result).Should(ContainSubstring("<File>/kubedoop/log/zoo_container/app.log</File>"))
			Expect(result).Should(ContainSubstring("<FileNamePattern>/kubedoop/log/zoo_container/app.log.%i</FileNamePattern>"))

			Expect(generator.FileName()).Should(Equal("logback.xml"))
		})
	})

	Context("when parsing is successful", func() {
		It("should return default string when loggingConfigSpec is nil", func() {
			generator = *productlogging.NewLogbackConfigGenerator(nil, "zoo_container", productlogging.DefaultLogbackConversionPattern, nil, "app.log", "logback.xml")
			result := generator.Generate()
			fmt.Println(result)

			Expect(result).ShouldNot(BeEmpty())
			// todo assert

			// Expect(result).Should(ContainSubstring("<root level=\"INFO\">"))
		})
	})

})
