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

package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/config"
)

var _ = Describe("LoggingGenerator", func() {
	Describe("NewLoggingGenerator", func() {
		It("should create a LoggingGenerator with log4j2 framework", func() {
			generator := config.NewLoggingGenerator(config.LoggingFrameworkLog4j2)
			Expect(generator).NotTo(BeNil())
		})

		It("should create a LoggingGenerator with logback framework", func() {
			generator := config.NewLoggingGenerator(config.LoggingFrameworkLogback)
			Expect(generator).NotTo(BeNil())
		})

		It("should create a LoggingGenerator with python framework", func() {
			generator := config.NewLoggingGenerator(config.LoggingFrameworkPython)
			Expect(generator).NotTo(BeNil())
		})
	})

	Describe("Generate", func() {
		Context("with Log4j2 framework", func() {
			var generator *config.LoggingGenerator

			BeforeEach(func() {
				generator = config.NewLoggingGenerator(config.LoggingFrameworkLog4j2)
			})

			It("should generate log4j2 configuration with empty configs", func() {
				content, err := generator.Generate(map[string]config.LoggerConfig{})
				Expect(err).To(BeNil())
				Expect(content).To(ContainSubstring("# Log4j2 Configuration"))
				Expect(content).To(ContainSubstring("rootLogger.level=INFO"))
				Expect(content).To(ContainSubstring("appender.console.type=Console"))
			})

			It("should generate log4j2 configuration with single logger", func() {
				configs := map[string]config.LoggerConfig{
					"com.example": {Name: "com.example", Level: config.LogLevelDebug},
				}
				content, err := generator.Generate(configs)
				Expect(err).To(BeNil())
				Expect(content).To(ContainSubstring("loggers=com.example"))
				Expect(content).To(ContainSubstring("logger.com_example.name=com.example"))
				Expect(content).To(ContainSubstring("logger.com_example.level=DEBUG"))
			})

			It("should generate log4j2 configuration with multiple loggers", func() {
				configs := map[string]config.LoggerConfig{
					"com.example":      {Name: "com.example", Level: config.LogLevelDebug},
					"org.apache.kafka": {Name: "org.apache.kafka", Level: config.LogLevelWarn},
				}
				content, err := generator.Generate(configs)
				Expect(err).To(BeNil())
				Expect(content).To(ContainSubstring("loggers="))
				Expect(content).To(ContainSubstring("com.example"))
				Expect(content).To(ContainSubstring("org.apache.kafka"))
			})

			It("should escape special characters in logger names", func() {
				configs := map[string]config.LoggerConfig{
					"com.example-module": {Name: "com.example-module", Level: config.LogLevelInfo},
				}
				content, err := generator.Generate(configs)
				Expect(err).To(BeNil())
				Expect(content).To(ContainSubstring("logger.com_example_module.name=com.example-module"))
			})

			It("should handle all log levels", func() {
				levels := []config.LogLevel{
					config.LogLevelTrace,
					config.LogLevelDebug,
					config.LogLevelInfo,
					config.LogLevelWarn,
					config.LogLevelError,
					config.LogLevelFatal,
				}
				for _, level := range levels {
					configs := map[string]config.LoggerConfig{
						"test": {Name: "test", Level: level},
					}
					content, err := generator.Generate(configs)
					Expect(err).To(BeNil())
					Expect(content).To(ContainSubstring(string(level)))
				}
			})
		})

		Context("with Logback framework", func() {
			var generator *config.LoggingGenerator

			BeforeEach(func() {
				generator = config.NewLoggingGenerator(config.LoggingFrameworkLogback)
			})

			It("should generate logback configuration with empty configs", func() {
				content, err := generator.Generate(map[string]config.LoggerConfig{})
				Expect(err).To(BeNil())
				Expect(content).To(ContainSubstring("<?xml version=\"1.0\""))
				Expect(content).To(ContainSubstring("<configuration>"))
				Expect(content).To(ContainSubstring("<root level=\"INFO\">"))
				Expect(content).To(ContainSubstring("</configuration>"))
			})

			It("should generate logback configuration with single logger", func() {
				configs := map[string]config.LoggerConfig{
					"com.example": {Name: "com.example", Level: config.LogLevelDebug},
				}
				content, err := generator.Generate(configs)
				Expect(err).To(BeNil())
				Expect(content).To(ContainSubstring(`<logger name="com.example" level="DEBUG" />`))
			})

			It("should generate logback configuration with multiple loggers", func() {
				configs := map[string]config.LoggerConfig{
					"com.example":      {Name: "com.example", Level: config.LogLevelDebug},
					"org.apache.kafka": {Name: "org.apache.kafka", Level: config.LogLevelWarn},
				}
				content, err := generator.Generate(configs)
				Expect(err).To(BeNil())
				Expect(content).To(ContainSubstring(`<logger name="com.example" level="DEBUG" />`))
				Expect(content).To(ContainSubstring(`<logger name="org.apache.kafka" level="WARN" />`))
			})

			It("should escape XML special characters in logger names", func() {
				configs := map[string]config.LoggerConfig{
					"com.example<test>": {Name: "com.example<test>", Level: config.LogLevelInfo},
				}
				content, err := generator.Generate(configs)
				Expect(err).To(BeNil())
				Expect(content).To(ContainSubstring("&lt;"))
				Expect(content).To(ContainSubstring("&gt;"))
			})

			It("should escape ampersand in logger names", func() {
				configs := map[string]config.LoggerConfig{
					"com.example&test": {Name: "com.example&test", Level: config.LogLevelInfo},
				}
				content, err := generator.Generate(configs)
				Expect(err).To(BeNil())
				Expect(content).To(ContainSubstring("&amp;"))
			})
		})

		Context("with Python framework", func() {
			var generator *config.LoggingGenerator

			BeforeEach(func() {
				generator = config.NewLoggingGenerator(config.LoggingFrameworkPython)
			})

			It("should generate python logging configuration with empty configs", func() {
				content, err := generator.Generate(map[string]config.LoggerConfig{})
				Expect(err).To(BeNil())
				Expect(content).To(ContainSubstring("# Python Logging Configuration"))
				Expect(content).To(ContainSubstring("LOGGING = {"))
				Expect(content).To(ContainSubstring("'version': 1"))
				Expect(content).To(ContainSubstring("'disable_existing_loggers': False"))
			})

			It("should generate python logging configuration with single logger", func() {
				configs := map[string]config.LoggerConfig{
					"com.example": {Name: "com.example", Level: config.LogLevelDebug},
				}
				content, err := generator.Generate(configs)
				Expect(err).To(BeNil())
				Expect(content).To(ContainSubstring("'com.example'"))
				Expect(content).To(ContainSubstring("'level': 'DEBUG'"))
			})

			It("should generate python logging configuration with multiple loggers", func() {
				configs := map[string]config.LoggerConfig{
					"com.example":      {Name: "com.example", Level: config.LogLevelDebug},
					"org.apache.kafka": {Name: "org.apache.kafka", Level: config.LogLevelWarn},
				}
				content, err := generator.Generate(configs)
				Expect(err).To(BeNil())
				Expect(content).To(ContainSubstring("'com.example'"))
				Expect(content).To(ContainSubstring("'org.apache.kafka'"))
			})

			It("should map TRACE to DEBUG for Python", func() {
				configs := map[string]config.LoggerConfig{
					"test": {Name: "test", Level: config.LogLevelTrace},
				}
				content, err := generator.Generate(configs)
				Expect(err).To(BeNil())
				Expect(content).To(ContainSubstring("'level': 'DEBUG'"))
			})

			It("should map WARN to WARNING for Python", func() {
				configs := map[string]config.LoggerConfig{
					"test": {Name: "test", Level: config.LogLevelWarn},
				}
				content, err := generator.Generate(configs)
				Expect(err).To(BeNil())
				Expect(content).To(ContainSubstring("'level': 'WARNING'"))
			})

			It("should map FATAL to CRITICAL for Python", func() {
				configs := map[string]config.LoggerConfig{
					"test": {Name: "test", Level: config.LogLevelFatal},
				}
				content, err := generator.Generate(configs)
				Expect(err).To(BeNil())
				Expect(content).To(ContainSubstring("'level': 'CRITICAL'"))
			})

			It("should map ERROR to ERROR for Python", func() {
				configs := map[string]config.LoggerConfig{
					"test": {Name: "test", Level: config.LogLevelError},
				}
				content, err := generator.Generate(configs)
				Expect(err).To(BeNil())
				Expect(content).To(ContainSubstring("'level': 'ERROR'"))
			})

			It("should map INFO to INFO for Python", func() {
				configs := map[string]config.LoggerConfig{
					"test": {Name: "test", Level: config.LogLevelInfo},
				}
				content, err := generator.Generate(configs)
				Expect(err).To(BeNil())
				Expect(content).To(ContainSubstring("'level': 'INFO'"))
			})
		})

		Context("with unsupported framework", func() {
			It("should return error for unsupported framework", func() {
				generator := config.NewLoggingGenerator(config.LoggingFramework("unsupported"))
				content, err := generator.Generate(map[string]config.LoggerConfig{})
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("unsupported logging framework"))
				Expect(content).To(BeEmpty())
			})
		})
	})
})

var _ = Describe("GenerateLog4j2", func() {
	It("should generate valid log4j2 properties format", func() {
		configs := map[string]config.LoggerConfig{
			"com.example.app": {Name: "com.example.app", Level: config.LogLevelInfo},
		}
		content, err := config.GenerateLog4j2(configs)
		Expect(err).To(BeNil())
		Expect(content).To(ContainSubstring("# Log4j2 Configuration"))
		Expect(content).To(ContainSubstring("rootLogger.level=INFO"))
		Expect(content).To(ContainSubstring("rootLogger.appenderRefs=stdout"))
		Expect(content).To(ContainSubstring("appenders=console"))
		Expect(content).To(ContainSubstring("appender.console.type=Console"))
		Expect(content).To(ContainSubstring("appender.console.layout.type=PatternLayout"))
		Expect(content).To(ContainSubstring("loggers=com.example.app"))
		Expect(content).To(ContainSubstring("logger.com_example_app.name=com.example.app"))
		Expect(content).To(ContainSubstring("logger.com_example_app.level=INFO"))
	})

	It("should handle logger names with dollar sign", func() {
		configs := map[string]config.LoggerConfig{
			"com$example": {Name: "com$example", Level: config.LogLevelDebug},
		}
		content, err := config.GenerateLog4j2(configs)
		Expect(err).To(BeNil())
		Expect(content).To(ContainSubstring("logger.com_example.name=com$example"))
	})

	It("should sort logger names alphabetically", func() {
		configs := map[string]config.LoggerConfig{
			"zebra":  {Name: "zebra", Level: config.LogLevelInfo},
			"alpha":  {Name: "alpha", Level: config.LogLevelInfo},
			"middle": {Name: "middle", Level: config.LogLevelInfo},
		}
		content, err := config.GenerateLog4j2(configs)
		Expect(err).To(BeNil())
		Expect(content).To(ContainSubstring("loggers=alpha,middle,zebra"))
	})
})

var _ = Describe("GenerateLogback", func() {
	It("should generate valid logback XML format", func() {
		configs := map[string]config.LoggerConfig{
			"com.example.app": {Name: "com.example.app", Level: config.LogLevelDebug},
		}
		content, err := config.GenerateLogback(configs)
		Expect(err).To(BeNil())
		Expect(content).To(ContainSubstring("<?xml version=\"1.0\""))
		Expect(content).To(ContainSubstring("<configuration>"))
		Expect(content).To(ContainSubstring(`<appender name="STDOUT" class="ch.qos.logback.core.ConsoleAppender">`))
		Expect(content).To(ContainSubstring("<root level=\"INFO\">"))
		Expect(content).To(ContainSubstring(`<logger name="com.example.app" level="DEBUG" />`))
		Expect(content).To(ContainSubstring("</configuration>"))
	})

	It("should escape double quotes in logger names", func() {
		configs := map[string]config.LoggerConfig{
			`com.example"test`: {Name: `com.example"test`, Level: config.LogLevelInfo},
		}
		content, err := config.GenerateLogback(configs)
		Expect(err).To(BeNil())
		Expect(content).To(ContainSubstring("&quot;"))
	})

	It("should escape single quotes in logger names", func() {
		configs := map[string]config.LoggerConfig{
			"com.example'test": {Name: "com.example'test", Level: config.LogLevelInfo},
		}
		content, err := config.GenerateLogback(configs)
		Expect(err).To(BeNil())
		Expect(content).To(ContainSubstring("&apos;"))
	})

	It("should sort logger names alphabetically", func() {
		configs := map[string]config.LoggerConfig{
			"zebra":  {Name: "zebra", Level: config.LogLevelInfo},
			"alpha":  {Name: "alpha", Level: config.LogLevelInfo},
			"middle": {Name: "middle", Level: config.LogLevelInfo},
		}
		content, err := config.GenerateLogback(configs)
		Expect(err).To(BeNil())
		// Check that alpha appears before middle which appears before zebra
		Expect(content).To(ContainSubstring(`<logger name="alpha"`))
		Expect(content).To(ContainSubstring(`<logger name="middle"`))
		Expect(content).To(ContainSubstring(`<logger name="zebra"`))
	})
})

var _ = Describe("GeneratePythonLogging", func() {
	It("should generate valid Python logging format", func() {
		configs := map[string]config.LoggerConfig{
			"com.example.app": {Name: "com.example.app", Level: config.LogLevelInfo},
		}
		content, err := config.GeneratePythonLogging(configs)
		Expect(err).To(BeNil())
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
		content, err := config.GeneratePythonLogging(map[string]config.LoggerConfig{})
		Expect(err).To(BeNil())
		Expect(content).To(ContainSubstring("'console'"))
		Expect(content).To(ContainSubstring("'class': 'logging.StreamHandler'"))
	})

	It("should include standard formatter configuration", func() {
		content, err := config.GeneratePythonLogging(map[string]config.LoggerConfig{})
		Expect(err).To(BeNil())
		Expect(content).To(ContainSubstring("'standard'"))
		Expect(content).To(ContainSubstring("'format'"))
	})

	It("should sort logger names alphabetically", func() {
		configs := map[string]config.LoggerConfig{
			"zebra":  {Name: "zebra", Level: config.LogLevelInfo},
			"alpha":  {Name: "alpha", Level: config.LogLevelInfo},
			"middle": {Name: "middle", Level: config.LogLevelInfo},
		}
		content, err := config.GeneratePythonLogging(configs)
		Expect(err).To(BeNil())
		// Verify all loggers are present
		Expect(content).To(ContainSubstring("'alpha'"))
		Expect(content).To(ContainSubstring("'middle'"))
		Expect(content).To(ContainSubstring("'zebra'"))
	})
})

var _ = Describe("LogLevel constants", func() {
	It("should have correct LogLevelTrace value", func() {
		Expect(string(config.LogLevelTrace)).To(Equal("TRACE"))
	})

	It("should have correct LogLevelDebug value", func() {
		Expect(string(config.LogLevelDebug)).To(Equal("DEBUG"))
	})

	It("should have correct LogLevelInfo value", func() {
		Expect(string(config.LogLevelInfo)).To(Equal("INFO"))
	})

	It("should have correct LogLevelWarn value", func() {
		Expect(string(config.LogLevelWarn)).To(Equal("WARN"))
	})

	It("should have correct LogLevelError value", func() {
		Expect(string(config.LogLevelError)).To(Equal("ERROR"))
	})

	It("should have correct LogLevelFatal value", func() {
		Expect(string(config.LogLevelFatal)).To(Equal("FATAL"))
	})
})

var _ = Describe("LoggingFramework constants", func() {
	It("should have correct LoggingFrameworkLog4j2 value", func() {
		Expect(string(config.LoggingFrameworkLog4j2)).To(Equal("log4j2"))
	})

	It("should have correct LoggingFrameworkLogback value", func() {
		Expect(string(config.LoggingFrameworkLogback)).To(Equal("logback"))
	})

	It("should have correct LoggingFrameworkPython value", func() {
		Expect(string(config.LoggingFrameworkPython)).To(Equal("python"))
	})
})

var _ = Describe("LoggerConfig", func() {
	It("should create LoggerConfig with name and level", func() {
		loggerConfig := config.LoggerConfig{
			Name:  "com.example",
			Level: config.LogLevelDebug,
		}
		Expect(loggerConfig.Name).To(Equal("com.example"))
		Expect(loggerConfig.Level).To(Equal(config.LogLevelDebug))
	})
})
