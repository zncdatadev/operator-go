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

package productlogging_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/productlogging"
)

var _ = Describe("LoggingGenerator", func() {
	Describe("NewLoggingGenerator", func() {
		It("should create a LoggingGenerator with log4j framework", func() {
			generator := productlogging.NewLoggingGenerator(productlogging.LoggingFrameworkLog4j)
			Expect(generator).NotTo(BeNil())
		})

		It("should create a LoggingGenerator with log4j2 framework", func() {
			generator := productlogging.NewLoggingGenerator(productlogging.LoggingFrameworkLog4j2)
			Expect(generator).NotTo(BeNil())
		})

		It("should create a LoggingGenerator with logback framework", func() {
			generator := productlogging.NewLoggingGenerator(productlogging.LoggingFrameworkLogback)
			Expect(generator).NotTo(BeNil())
		})

		It("should create a LoggingGenerator with python framework", func() {
			generator := productlogging.NewLoggingGenerator(productlogging.LoggingFrameworkPython)
			Expect(generator).NotTo(BeNil())
		})
	})

	Describe("Generate", func() {
		Context("with Log4j framework", func() {
			var generator *productlogging.LoggingGenerator

			BeforeEach(func() {
				generator = productlogging.NewLoggingGenerator(productlogging.LoggingFrameworkLog4j)
			})

			It("should generate log4j configuration with empty configs", func() {
				content, err := generator.Generate(map[string]productlogging.LoggerConfig{})
				Expect(err).ToNot(HaveOccurred())
				Expect(content).To(ContainSubstring("# Log4j Configuration"))
				Expect(content).To(ContainSubstring("log4j.rootLogger=INFO, CONSOLE"))
				Expect(content).To(ContainSubstring("log4j.appender.CONSOLE=org.apache.log4j.ConsoleAppender"))
			})

			It("should generate log4j configuration with single logger", func() {
				configs := map[string]productlogging.LoggerConfig{
					"com.example": {Name: "com.example", Level: productlogging.LogLevelDebug},
				}
				content, err := generator.Generate(configs)
				Expect(err).ToNot(HaveOccurred())
				Expect(content).To(ContainSubstring("log4j.logger.com.example=DEBUG"))
			})

			It("should generate log4j configuration with multiple loggers", func() {
				configs := map[string]productlogging.LoggerConfig{
					"com.example":      {Name: "com.example", Level: productlogging.LogLevelDebug},
					"org.apache.kafka": {Name: "org.apache.kafka", Level: productlogging.LogLevelWarn},
				}
				content, err := generator.Generate(configs)
				Expect(err).ToNot(HaveOccurred())
				Expect(content).To(ContainSubstring("log4j.logger.com.example=DEBUG"))
				Expect(content).To(ContainSubstring("log4j.logger.org.apache.kafka=WARN"))
			})

			It("should handle all log levels", func() {
				levels := []productlogging.LogLevel{
					productlogging.LogLevelTrace,
					productlogging.LogLevelDebug,
					productlogging.LogLevelInfo,
					productlogging.LogLevelWarn,
					productlogging.LogLevelError,
					productlogging.LogLevelFatal,
				}
				for _, level := range levels {
					configs := map[string]productlogging.LoggerConfig{
						"test": {Name: "test", Level: level},
					}
					content, err := generator.Generate(configs)
					Expect(err).ToNot(HaveOccurred())
					Expect(content).To(ContainSubstring("log4j.logger.test=" + string(level)))
				}
			})
		})

		Context("with Log4j2 framework", func() {
			var generator *productlogging.LoggingGenerator

			BeforeEach(func() {
				generator = productlogging.NewLoggingGenerator(productlogging.LoggingFrameworkLog4j2)
			})

			It("should generate log4j2 configuration with empty configs", func() {
				content, err := generator.Generate(map[string]productlogging.LoggerConfig{})
				Expect(err).ToNot(HaveOccurred())
				Expect(content).To(ContainSubstring("# Log4j2 Configuration"))
				Expect(content).To(ContainSubstring("rootLogger.level=INFO"))
				Expect(content).To(ContainSubstring("appender.console.type=Console"))
			})

			It("should generate log4j2 configuration with single logger", func() {
				configs := map[string]productlogging.LoggerConfig{
					"com.example": {Name: "com.example", Level: productlogging.LogLevelDebug},
				}
				content, err := generator.Generate(configs)
				Expect(err).ToNot(HaveOccurred())
				Expect(content).To(ContainSubstring("loggers=com.example"))
				Expect(content).To(ContainSubstring("logger.com_example.name=com.example"))
				Expect(content).To(ContainSubstring("logger.com_example.level=DEBUG"))
			})

			It("should generate log4j2 configuration with multiple loggers", func() {
				configs := map[string]productlogging.LoggerConfig{
					"com.example":      {Name: "com.example", Level: productlogging.LogLevelDebug},
					"org.apache.kafka": {Name: "org.apache.kafka", Level: productlogging.LogLevelWarn},
				}
				content, err := generator.Generate(configs)
				Expect(err).ToNot(HaveOccurred())
				Expect(content).To(ContainSubstring("loggers="))
				Expect(content).To(ContainSubstring("com.example"))
				Expect(content).To(ContainSubstring("org.apache.kafka"))
			})

			It("should escape special characters in logger names", func() {
				configs := map[string]productlogging.LoggerConfig{
					"com.example-module": {Name: "com.example-module", Level: productlogging.LogLevelInfo},
				}
				content, err := generator.Generate(configs)
				Expect(err).ToNot(HaveOccurred())
				Expect(content).To(ContainSubstring("logger.com_example_module.name=com.example-module"))
			})

			It("should handle all log levels", func() {
				levels := []productlogging.LogLevel{
					productlogging.LogLevelTrace,
					productlogging.LogLevelDebug,
					productlogging.LogLevelInfo,
					productlogging.LogLevelWarn,
					productlogging.LogLevelError,
					productlogging.LogLevelFatal,
				}
				for _, level := range levels {
					configs := map[string]productlogging.LoggerConfig{
						"test": {Name: "test", Level: level},
					}
					content, err := generator.Generate(configs)
					Expect(err).ToNot(HaveOccurred())
					Expect(content).To(ContainSubstring(string(level)))
				}
			})
		})

		Context("with Logback framework", func() {
			var generator *productlogging.LoggingGenerator

			BeforeEach(func() {
				generator = productlogging.NewLoggingGenerator(productlogging.LoggingFrameworkLogback)
			})

			It("should generate logback configuration with empty configs", func() {
				content, err := generator.Generate(map[string]productlogging.LoggerConfig{})
				Expect(err).ToNot(HaveOccurred())
				Expect(content).To(ContainSubstring("<?xml version=\"1.0\""))
				Expect(content).To(ContainSubstring("<configuration>"))
				Expect(content).To(ContainSubstring("<root level=\"INFO\">"))
				Expect(content).To(ContainSubstring("</configuration>"))
			})

			It("should generate logback configuration with single logger", func() {
				configs := map[string]productlogging.LoggerConfig{
					"com.example": {Name: "com.example", Level: productlogging.LogLevelDebug},
				}
				content, err := generator.Generate(configs)
				Expect(err).ToNot(HaveOccurred())
				Expect(content).To(ContainSubstring(`<logger name="com.example" level="DEBUG" />`))
			})

			It("should generate logback configuration with multiple loggers", func() {
				configs := map[string]productlogging.LoggerConfig{
					"com.example":      {Name: "com.example", Level: productlogging.LogLevelDebug},
					"org.apache.kafka": {Name: "org.apache.kafka", Level: productlogging.LogLevelWarn},
				}
				content, err := generator.Generate(configs)
				Expect(err).ToNot(HaveOccurred())
				Expect(content).To(ContainSubstring(`<logger name="com.example" level="DEBUG" />`))
				Expect(content).To(ContainSubstring(`<logger name="org.apache.kafka" level="WARN" />`))
			})

			It("should escape XML special characters in logger names", func() {
				configs := map[string]productlogging.LoggerConfig{
					"com.example<test>": {Name: "com.example<test>", Level: productlogging.LogLevelInfo},
				}
				content, err := generator.Generate(configs)
				Expect(err).ToNot(HaveOccurred())
				Expect(content).To(ContainSubstring("&lt;"))
				Expect(content).To(ContainSubstring("&gt;"))
			})

			It("should escape ampersand in logger names", func() {
				configs := map[string]productlogging.LoggerConfig{
					"com.example&test": {Name: "com.example&test", Level: productlogging.LogLevelInfo},
				}
				content, err := generator.Generate(configs)
				Expect(err).ToNot(HaveOccurred())
				Expect(content).To(ContainSubstring("&amp;"))
			})
		})

		Context("with Python framework", func() {
			var generator *productlogging.LoggingGenerator

			BeforeEach(func() {
				generator = productlogging.NewLoggingGenerator(productlogging.LoggingFrameworkPython)
			})

			It("should generate python logging configuration with empty configs", func() {
				content, err := generator.Generate(map[string]productlogging.LoggerConfig{})
				Expect(err).ToNot(HaveOccurred())
				Expect(content).To(ContainSubstring("# Python Logging Configuration"))
				Expect(content).To(ContainSubstring("LOGGING = {"))
				Expect(content).To(ContainSubstring("'version': 1"))
				Expect(content).To(ContainSubstring("'disable_existing_loggers': False"))
			})

			It("should generate python logging configuration with single logger", func() {
				configs := map[string]productlogging.LoggerConfig{
					"com.example": {Name: "com.example", Level: productlogging.LogLevelDebug},
				}
				content, err := generator.Generate(configs)
				Expect(err).ToNot(HaveOccurred())
				Expect(content).To(ContainSubstring("'com.example'"))
				Expect(content).To(ContainSubstring("'level': 'DEBUG'"))
			})

			It("should generate python logging configuration with multiple loggers", func() {
				configs := map[string]productlogging.LoggerConfig{
					"com.example":      {Name: "com.example", Level: productlogging.LogLevelDebug},
					"org.apache.kafka": {Name: "org.apache.kafka", Level: productlogging.LogLevelWarn},
				}
				content, err := generator.Generate(configs)
				Expect(err).ToNot(HaveOccurred())
				Expect(content).To(ContainSubstring("'com.example'"))
				Expect(content).To(ContainSubstring("'org.apache.kafka'"))
			})

			It("should map TRACE to DEBUG for Python", func() {
				configs := map[string]productlogging.LoggerConfig{
					"test": {Name: "test", Level: productlogging.LogLevelTrace},
				}
				content, err := generator.Generate(configs)
				Expect(err).ToNot(HaveOccurred())
				Expect(content).To(ContainSubstring("'level': 'DEBUG'"))
			})

			It("should map WARN to WARNING for Python", func() {
				configs := map[string]productlogging.LoggerConfig{
					"test": {Name: "test", Level: productlogging.LogLevelWarn},
				}
				content, err := generator.Generate(configs)
				Expect(err).ToNot(HaveOccurred())
				Expect(content).To(ContainSubstring("'level': 'WARNING'"))
			})

			It("should map FATAL to CRITICAL for Python", func() {
				configs := map[string]productlogging.LoggerConfig{
					"test": {Name: "test", Level: productlogging.LogLevelFatal},
				}
				content, err := generator.Generate(configs)
				Expect(err).ToNot(HaveOccurred())
				Expect(content).To(ContainSubstring("'level': 'CRITICAL'"))
			})

			It("should map ERROR to ERROR for Python", func() {
				configs := map[string]productlogging.LoggerConfig{
					"test": {Name: "test", Level: productlogging.LogLevelError},
				}
				content, err := generator.Generate(configs)
				Expect(err).ToNot(HaveOccurred())
				Expect(content).To(ContainSubstring("'level': 'ERROR'"))
			})

			It("should map INFO to INFO for Python", func() {
				configs := map[string]productlogging.LoggerConfig{
					"test": {Name: "test", Level: productlogging.LogLevelInfo},
				}
				content, err := generator.Generate(configs)
				Expect(err).ToNot(HaveOccurred())
				Expect(content).To(ContainSubstring("'level': 'INFO'"))
			})
		})

		Context("with unsupported framework", func() {
			It("should return error for unsupported framework", func() {
				generator := productlogging.NewLoggingGenerator(productlogging.LoggingFramework("unsupported"))
				content, err := generator.Generate(map[string]productlogging.LoggerConfig{})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unsupported logging framework"))
				Expect(content).To(BeEmpty())
			})
		})
	})
})

var _ = Describe("GenerateLog4j", func() {
	It("should generate valid log4j 1.x properties format", func() {
		configs := map[string]productlogging.LoggerConfig{
			"com.example.app": {Name: "com.example.app", Level: productlogging.LogLevelInfo},
		}
		content, err := productlogging.GenerateLog4j(configs)
		Expect(err).ToNot(HaveOccurred())
		Expect(content).To(ContainSubstring("# Log4j Configuration"))
		Expect(content).To(ContainSubstring("log4j.rootLogger=INFO, CONSOLE"))
		Expect(content).To(ContainSubstring("log4j.appender.CONSOLE=org.apache.log4j.ConsoleAppender"))
		Expect(content).To(ContainSubstring("log4j.appender.CONSOLE.layout=org.apache.log4j.PatternLayout"))
		Expect(content).To(ContainSubstring("log4j.appender.CONSOLE.layout.ConversionPattern="))
		Expect(content).To(ContainSubstring("log4j.logger.com.example.app=INFO"))
	})

	It("should be console-only without a file output path", func() {
		content, err := productlogging.GenerateLog4j(nil)
		Expect(err).ToNot(HaveOccurred())
		Expect(content).NotTo(ContainSubstring("FILE"))
		Expect(content).NotTo(ContainSubstring("RollingFileAppender"))
	})

	It("should sort logger names alphabetically", func() {
		configs := map[string]productlogging.LoggerConfig{
			"zebra":  {Name: "zebra", Level: productlogging.LogLevelInfo},
			"alpha":  {Name: "alpha", Level: productlogging.LogLevelInfo},
			"middle": {Name: "middle", Level: productlogging.LogLevelInfo},
		}
		content, err := productlogging.GenerateLog4j(configs)
		Expect(err).ToNot(HaveOccurred())
		alpha := strings.Index(content, "log4j.logger.alpha=INFO")
		middle := strings.Index(content, "log4j.logger.middle=INFO")
		zebra := strings.Index(content, "log4j.logger.zebra=INFO")
		Expect(alpha).To(BeNumerically(">=", 0))
		Expect(alpha).To(BeNumerically("<", middle))
		Expect(middle).To(BeNumerically("<", zebra))
	})

	It("should emit valid properties format (every non-comment line is key=value)", func() {
		configs := map[string]productlogging.LoggerConfig{
			"org.apache.kafka": {Name: "org.apache.kafka", Level: productlogging.LogLevelWarn},
		}
		content, err := productlogging.GenerateLog4j(configs)
		Expect(err).ToNot(HaveOccurred())
		for _, line := range strings.Split(content, "\n") {
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			Expect(line).To(ContainSubstring("="), "line %q must be a key=value property", line)
		}
	})
})

var _ = Describe("GenerateLog4j2", func() {
	It("should generate valid log4j2 properties format", func() {
		configs := map[string]productlogging.LoggerConfig{
			"com.example.app": {Name: "com.example.app", Level: productlogging.LogLevelInfo},
		}
		content, err := productlogging.GenerateLog4j2(configs)
		Expect(err).ToNot(HaveOccurred())
		Expect(content).To(ContainSubstring("# Log4j2 Configuration"))
		Expect(content).To(ContainSubstring("rootLogger.level=INFO"))
		Expect(content).To(ContainSubstring("rootLogger.appenderRefs=stdout"))
		Expect(content).To(ContainSubstring("rootLogger.appenderRef.stdout.ref=STDOUT"))
		Expect(content).To(ContainSubstring("appenders=console"))
		Expect(content).To(ContainSubstring("appender.console.type=Console"))
		Expect(content).To(ContainSubstring("appender.console.layout.type=PatternLayout"))
		Expect(content).To(ContainSubstring("loggers=com.example.app"))
		Expect(content).To(ContainSubstring("logger.com_example_app.name=com.example.app"))
		Expect(content).To(ContainSubstring("logger.com_example_app.level=INFO"))
	})

	It("binds both stdout and file appenderRefs to the root logger when file output is enabled", func() {
		gen, err := productlogging.GeneratorFor(productlogging.LoggingFrameworkLog4j2)
		Expect(err).ToNot(HaveOccurred())
		content, err := gen.Render(
			productlogging.LogConfig{},
			productlogging.RenderOptions{FileOutputPath: "/kubedoop/log/zookeeper/zookeeper.log4j2.xml"},
		)
		Expect(err).ToNot(HaveOccurred())
		// Both identifiers must be declared AND bound; without the file binding the rolling file
		// appender is silently not wired to the root logger (empty log file, nothing to ship).
		Expect(content).To(ContainSubstring("rootLogger.appenderRefs=stdout,file"))
		Expect(content).To(ContainSubstring("rootLogger.appenderRef.stdout.ref=STDOUT"))
		Expect(content).To(ContainSubstring("rootLogger.appenderRef.file.ref=FILE"))
	})

	It("should handle logger names with dollar sign", func() {
		configs := map[string]productlogging.LoggerConfig{
			"com$example": {Name: "com$example", Level: productlogging.LogLevelDebug},
		}
		content, err := productlogging.GenerateLog4j2(configs)
		Expect(err).ToNot(HaveOccurred())
		Expect(content).To(ContainSubstring("logger.com_example.name=com$example"))
	})

	It("should sort logger names alphabetically", func() {
		configs := map[string]productlogging.LoggerConfig{
			"zebra":  {Name: "zebra", Level: productlogging.LogLevelInfo},
			"alpha":  {Name: "alpha", Level: productlogging.LogLevelInfo},
			"middle": {Name: "middle", Level: productlogging.LogLevelInfo},
		}
		content, err := productlogging.GenerateLog4j2(configs)
		Expect(err).ToNot(HaveOccurred())
		Expect(content).To(ContainSubstring("loggers=alpha,middle,zebra"))
	})
})

var _ = Describe("GenerateLogback", func() {
	It("should generate valid logback XML format", func() {
		configs := map[string]productlogging.LoggerConfig{
			"com.example.app": {Name: "com.example.app", Level: productlogging.LogLevelDebug},
		}
		content, err := productlogging.GenerateLogback(configs)
		Expect(err).ToNot(HaveOccurred())
		Expect(content).To(ContainSubstring("<?xml version=\"1.0\""))
		Expect(content).To(ContainSubstring("<configuration>"))
		Expect(content).To(ContainSubstring(`<appender name="STDOUT" class="ch.qos.logback.core.ConsoleAppender">`))
		Expect(content).To(ContainSubstring("<root level=\"INFO\">"))
		Expect(content).To(ContainSubstring(`<logger name="com.example.app" level="DEBUG" />`))
		Expect(content).To(ContainSubstring("</configuration>"))
	})

	It("should escape double quotes in logger names", func() {
		configs := map[string]productlogging.LoggerConfig{
			`com.example"test`: {Name: `com.example"test`, Level: productlogging.LogLevelInfo},
		}
		content, err := productlogging.GenerateLogback(configs)
		Expect(err).ToNot(HaveOccurred())
		Expect(content).To(ContainSubstring("&quot;"))
	})

	It("should escape single quotes in logger names", func() {
		configs := map[string]productlogging.LoggerConfig{
			"com.example'test": {Name: "com.example'test", Level: productlogging.LogLevelInfo},
		}
		content, err := productlogging.GenerateLogback(configs)
		Expect(err).ToNot(HaveOccurred())
		Expect(content).To(ContainSubstring("&apos;"))
	})

	It("should sort logger names alphabetically", func() {
		configs := map[string]productlogging.LoggerConfig{
			"zebra":  {Name: "zebra", Level: productlogging.LogLevelInfo},
			"alpha":  {Name: "alpha", Level: productlogging.LogLevelInfo},
			"middle": {Name: "middle", Level: productlogging.LogLevelInfo},
		}
		content, err := productlogging.GenerateLogback(configs)
		Expect(err).ToNot(HaveOccurred())
		// Check that alpha appears before middle which appears before zebra
		Expect(content).To(ContainSubstring(`<logger name="alpha"`))
		Expect(content).To(ContainSubstring(`<logger name="middle"`))
		Expect(content).To(ContainSubstring(`<logger name="zebra"`))
	})
})

var _ = Describe("GeneratePythonLogging", func() {
	It("should generate valid Python logging format", func() {
		configs := map[string]productlogging.LoggerConfig{
			"com.example.app": {Name: "com.example.app", Level: productlogging.LogLevelInfo},
		}
		content, err := productlogging.GeneratePythonLogging(configs)
		Expect(err).ToNot(HaveOccurred())
		Expect(content).To(ContainSubstring("# Python Logging Configuration"))
		Expect(content).To(ContainSubstring("LOGGING = {"))
		Expect(content).To(ContainSubstring("'version': 1"))
		Expect(content).To(ContainSubstring("'disable_existing_loggers': False"))
		Expect(content).To(ContainSubstring("'formatters'"))
		Expect(content).To(ContainSubstring("'handlers'"))
		Expect(content).To(ContainSubstring("'loggers'"))
		Expect(content).To(ContainSubstring("'root'"))
	})

	It("should include console handler configuration", func() {
		content, err := productlogging.GeneratePythonLogging(map[string]productlogging.LoggerConfig{})
		Expect(err).ToNot(HaveOccurred())
		Expect(content).To(ContainSubstring("'console'"))
		Expect(content).To(ContainSubstring("'class': 'logging.StreamHandler'"))
	})

	It("should include standard formatter configuration", func() {
		content, err := productlogging.GeneratePythonLogging(map[string]productlogging.LoggerConfig{})
		Expect(err).ToNot(HaveOccurred())
		Expect(content).To(ContainSubstring("'standard'"))
		Expect(content).To(ContainSubstring("'format'"))
	})

	It("should sort logger names alphabetically", func() {
		configs := map[string]productlogging.LoggerConfig{
			"zebra":  {Name: "zebra", Level: productlogging.LogLevelInfo},
			"alpha":  {Name: "alpha", Level: productlogging.LogLevelInfo},
			"middle": {Name: "middle", Level: productlogging.LogLevelInfo},
		}
		content, err := productlogging.GeneratePythonLogging(configs)
		Expect(err).ToNot(HaveOccurred())
		// Verify all loggers are present
		Expect(content).To(ContainSubstring("'alpha'"))
		Expect(content).To(ContainSubstring("'middle'"))
		Expect(content).To(ContainSubstring("'zebra'"))
	})
})

var _ = Describe("LogLevel constants", func() {
	It("should have correct LogLevelTrace value", func() {
		Expect(string(productlogging.LogLevelTrace)).To(Equal("TRACE"))
	})

	It("should have correct LogLevelDebug value", func() {
		Expect(string(productlogging.LogLevelDebug)).To(Equal("DEBUG"))
	})

	It("should have correct LogLevelInfo value", func() {
		Expect(string(productlogging.LogLevelInfo)).To(Equal("INFO"))
	})

	It("should have correct LogLevelWarn value", func() {
		Expect(string(productlogging.LogLevelWarn)).To(Equal("WARN"))
	})

	It("should have correct LogLevelError value", func() {
		Expect(string(productlogging.LogLevelError)).To(Equal("ERROR"))
	})

	It("should have correct LogLevelFatal value", func() {
		Expect(string(productlogging.LogLevelFatal)).To(Equal("FATAL"))
	})
})

var _ = Describe("LoggingFramework constants", func() {
	It("should have correct LoggingFrameworkLog4j value", func() {
		Expect(string(productlogging.LoggingFrameworkLog4j)).To(Equal("log4j"))
	})

	It("should have correct LoggingFrameworkLog4j2 value", func() {
		Expect(string(productlogging.LoggingFrameworkLog4j2)).To(Equal("log4j2"))
	})

	It("should have correct LoggingFrameworkLogback value", func() {
		Expect(string(productlogging.LoggingFrameworkLogback)).To(Equal("logback"))
	})

	It("should have correct LoggingFrameworkPython value", func() {
		Expect(string(productlogging.LoggingFrameworkPython)).To(Equal("python"))
	})
})

var _ = Describe("LoggerConfig", func() {
	It("should create LoggerConfig with name and level", func() {
		loggerConfig := productlogging.LoggerConfig{
			Name:  "com.example",
			Level: productlogging.LogLevelDebug,
		}
		Expect(loggerConfig.Name).To(Equal("com.example"))
		Expect(loggerConfig.Level).To(Equal(productlogging.LogLevelDebug))
	})
})

var _ = Describe("GenerateLogbackWithOptions", func() {
	It("emits only a console appender when no file output is requested (matches GenerateLogback)", func() {
		withOpts, err := productlogging.GenerateLogbackWithOptions(nil, productlogging.LogbackOptions{})
		Expect(err).NotTo(HaveOccurred())
		plain, err := productlogging.GenerateLogback(nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(withOpts).To(Equal(plain))
		Expect(withOpts).NotTo(ContainSubstring("RollingFileAppender"))
	})

	It("adds a bounded rolling file appender matching the consumer glob", func() {
		out, err := productlogging.GenerateLogbackWithOptions(nil, productlogging.LogbackOptions{
			FileOutputPath: "/kubedoop/log/zookeeper/zookeeper.log4j.xml",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring(`class="ch.qos.logback.core.rolling.RollingFileAppender"`))
		Expect(out).To(ContainSubstring("<file>/kubedoop/log/zookeeper/zookeeper.log4j.xml</file>"))
		// The stable FILE encoder is the log4j-compatible XMLLayout (edge-parsed by Vector's
		// files_log4j source), bounded by FixedWindowRollingPolicy + SizeBasedTriggeringPolicy.
		Expect(out).To(ContainSubstring(`<layout class="ch.qos.logback.classic.log4j.XMLLayout" />`))
		Expect(out).To(ContainSubstring(`class="ch.qos.logback.core.rolling.FixedWindowRollingPolicy"`))
		Expect(out).To(ContainSubstring("<fileNamePattern>/kubedoop/log/zookeeper/zookeeper.log4j.xml.%i</fileNamePattern>"))
		Expect(out).To(ContainSubstring("<maxFileSize>5MB</maxFileSize>"))
		Expect(out).To(ContainSubstring(`<appender-ref ref="FILE" />`))
	})

	It("defaults the root logger level to INFO when RootLevel is empty", func() {
		out, err := productlogging.GenerateLogbackWithOptions(nil, productlogging.LogbackOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring(`<root level="INFO">`))
	})

	It("honors a RootLevel override on the root logger", func() {
		out, err := productlogging.GenerateLogbackWithOptions(
			map[string]productlogging.LoggerConfig{
				"org.apache.zookeeper": {Name: "org.apache.zookeeper", Level: productlogging.LogLevelDebug},
			},
			productlogging.LogbackOptions{RootLevel: productlogging.LogLevelWarn},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring(`<root level="WARN">`))
		Expect(out).To(ContainSubstring(`<logger name="org.apache.zookeeper" level="DEBUG" />`))
	})
})
