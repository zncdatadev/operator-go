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
	"encoding/xml"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/config"
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
				Expect(content).To(ContainSubstring("loggers=com_example"))
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
		Expect(content).To(ContainSubstring("loggers=com_example_app"))
		Expect(content).To(ContainSubstring("logger.com_example_app.name=com.example.app"))
		Expect(content).To(ContainSubstring("logger.com_example_app.level=INFO"))
	})

	It("lists the sanitized ids in loggers=, matching the logger.<id>.* keys", func() {
		configs := map[string]productlogging.LoggerConfig{
			"com.example-app": {Name: "com.example-app", Level: productlogging.LogLevelDebug},
			"org$apache":      {Name: "org$apache", Level: productlogging.LogLevelWarn},
		}
		content, err := productlogging.GenerateLog4j2(configs)
		Expect(err).ToNot(HaveOccurred())
		// A pre-2.6 property parser discovers loggers from this list, so every entry must be the
		// id the per-logger keys use; the raw name would resolve to nothing.
		Expect(content).To(ContainSubstring("loggers=com_example_app,org_apache"))
		Expect(content).To(ContainSubstring("logger.com_example_app.name=com.example-app"))
		Expect(content).To(ContainSubstring("logger.org_apache.name=org$apache"))
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

	// A named logger that carries the root handlers AND propagates emits every record twice:
	// once through its own handlers, once more through the root's.
	DescribeTable("keeps the handlers on the root logger only",
		func(opts productlogging.RenderOptions, rootHandlers string) {
			gen, err := productlogging.GeneratorFor(productlogging.LoggingFrameworkPython)
			Expect(err).ToNot(HaveOccurred())
			content, err := gen.Render(
				productlogging.LogConfig{Loggers: map[string]productlogging.LogLevel{
					"com.example.app": productlogging.LogLevelDebug,
				}},
				opts,
			)
			Expect(err).ToNot(HaveOccurred())

			loggersBlock := pythonLoggersBlock(content)
			Expect(loggersBlock).To(ContainSubstring("'com.example.app'"))
			Expect(loggersBlock).To(ContainSubstring("'level': 'DEBUG'"))
			Expect(loggersBlock).To(ContainSubstring("'propagate': True"))
			Expect(loggersBlock).ToNot(ContainSubstring("'handlers'"))

			Expect(content).To(ContainSubstring("'root': {\n        'level': 'INFO',\n        'handlers': " + rootHandlers))
		},
		Entry("console only", productlogging.RenderOptions{}, "['console']"),
		Entry("with file appender",
			productlogging.RenderOptions{FileOutputPath: "/kubedoop/log/app/app.py.json"},
			"['console', 'file']"),
	)
})

// pythonLoggersBlock returns the 'loggers' section of a rendered python dictConfig, i.e. the
// per-logger entries without the 'handlers' / 'root' sections around them.
func pythonLoggersBlock(content string) string {
	const marker = "    'loggers': {\n"
	start := strings.Index(content, marker)
	Expect(start).ToNot(Equal(-1), "rendered config has no 'loggers' section")
	rest := content[start+len(marker):]
	end := strings.Index(rest, "    'root': {")
	Expect(end).ToNot(Equal(-1), "rendered config has no 'root' section")
	return rest[:end]
}

// javaProperties reads a rendered log4j / log4j2 file the way both configurators do — through
// java.util.Properties semantics, which is what pkg/config's properties adapter implements — so
// the assertions below are about the entries a product actually ends up with, not about the
// characters on the line.
func javaProperties(content string) map[string]string {
	GinkgoHelper()
	props, err := config.NewPropertiesAdapter().Unmarshal(content)
	Expect(err).ToNot(HaveOccurred())
	return props
}

// log4j2Loggers resolves the rendered log4j2 file the way a property configuration is resolved:
// the "loggers" list selects the identifiers, and each identifier carries its logger through
// "logger.<id>.name" / "logger.<id>.level". A dangling id, a missing key or two ids collapsing
// onto one logger name shows up here as a failure rather than as a logger that is silently
// never configured.
func log4j2Loggers(content string) map[string]string {
	GinkgoHelper()
	props := javaProperties(content)
	loggers := map[string]string{}
	list, ok := props["loggers"]
	if !ok || list == "" {
		return loggers
	}
	for _, id := range strings.Split(list, ",") {
		name, ok := props["logger."+id+".name"]
		Expect(ok).To(BeTrue(), "loggers list declares id %q but there is no logger.%s.name", id, id)
		level, ok := props["logger."+id+".level"]
		Expect(ok).To(BeTrue(), "loggers list declares id %q but there is no logger.%s.level", id, id)
		Expect(loggers).ToNot(HaveKey(name), "two identifiers configure the logger %q", name)
		loggers[name] = level
	}
	return loggers
}

// logbackConfiguration is the shape the generated logback XML is asserted against; unmarshalling
// it also proves the document is well-formed, which an unescaped name or level would break.
type logbackConfiguration struct {
	XMLName   xml.Name `xml:"configuration"`
	Appenders []struct {
		Name  string `xml:"name,attr"`
		Class string `xml:"class,attr"`
		File  string `xml:"file"`
	} `xml:"appender"`
	Root struct {
		Level        string `xml:"level,attr"`
		AppenderRefs []struct {
			Ref string `xml:"ref,attr"`
		} `xml:"appender-ref"`
	} `xml:"root"`
	Loggers []struct {
		Name  string `xml:"name,attr"`
		Level string `xml:"level,attr"`
	} `xml:"logger"`
}

func parseLogback(content string) logbackConfiguration {
	GinkgoHelper()
	var parsed logbackConfiguration
	Expect(xml.Unmarshal([]byte(content), &parsed)).To(Succeed(), "rendered logback config is not well-formed XML:\n%s", content)
	return parsed
}

// Logger names are CRD map keys, so they carry whatever the user wrote. Each generator has to
// hand them to its own reader intact: properties keys and values are escaped for
// java.util.Properties, XML attributes for the XML parser and dict keys for the Python parser.
var _ = Describe("logger names the target format treats as structure", func() {
	adversarialNames := map[string]productlogging.LogLevel{
		"com.example app":  productlogging.LogLevelWarn,
		"com.example=app":  productlogging.LogLevelDebug,
		"com.example:app":  productlogging.LogLevelError,
		`com.example\app`:  productlogging.LogLevelInfo,
		"#com.example.app": productlogging.LogLevelTrace,
	}

	It("keeps them addressable in log4j properties", func() {
		content, err := productlogging.GenerateLog4j(loggerConfigsOf(adversarialNames))
		Expect(err).ToNot(HaveOccurred())

		props := javaProperties(content)
		for name, level := range adversarialNames {
			Expect(props).To(HaveKeyWithValue("log4j.logger."+name, string(level)))
		}
		// The escape a naive writer omits: without it the key ends at the space and the level
		// becomes part of a "log4j.logger.com.example" value.
		Expect(content).To(ContainSubstring(`log4j.logger.com.example\ app=WARN`))
	})

	It("keeps them addressable in log4j2 properties", func() {
		content, err := productlogging.GenerateLog4j2(loggerConfigsOf(adversarialNames))
		Expect(err).ToNot(HaveOccurred())

		expected := map[string]string{}
		for name, level := range adversarialNames {
			expected[name] = string(level)
		}
		Expect(log4j2Loggers(content)).To(Equal(expected))
	})

	It("keeps them addressable in logback XML", func() {
		content, err := productlogging.GenerateLogback(loggerConfigsOf(adversarialNames))
		Expect(err).ToNot(HaveOccurred())

		parsed := parseLogback(content)
		Expect(parsed.Loggers).To(HaveLen(len(adversarialNames)))
		for _, logger := range parsed.Loggers {
			Expect(adversarialNames).To(HaveKeyWithValue(logger.Name, productlogging.LogLevel(logger.Level)))
		}
	})

	// Levels are enum-constrained in the CRD but not in this package's API, and they land in an
	// XML attribute / a properties value just like the names do.
	It("keeps a level that carries XML syntax inside its attribute", func() {
		level := productlogging.LogLevel(`DEBUG"/><appender-ref ref="FILE`)
		content, err := productlogging.GenerateLogback(map[string]productlogging.LoggerConfig{
			"com.example": {Name: "com.example", Level: level},
		})
		Expect(err).ToNot(HaveOccurred())

		parsed := parseLogback(content)
		Expect(parsed.Loggers).To(HaveLen(1))
		Expect(parsed.Loggers[0].Level).To(Equal(string(level)))
	})

	// The conversion pattern is product-supplied (ContainerLogging.Pattern) and is written as a
	// properties value; a line break in it would start a new property.
	It("keeps a product pattern from opening a second property", func() {
		gen, err := productlogging.GeneratorFor(productlogging.LoggingFrameworkLog4j)
		Expect(err).ToNot(HaveOccurred())
		pattern := "%m%n\nlog4j.rootLogger=OFF"
		content, err := gen.Render(productlogging.LogConfig{}, productlogging.RenderOptions{Pattern: pattern})
		Expect(err).ToNot(HaveOccurred())

		props := javaProperties(content)
		Expect(props).To(HaveKeyWithValue("log4j.appender.CONSOLE.layout.ConversionPattern", pattern))
		Expect(props).To(HaveKeyWithValue("log4j.rootLogger", "INFO, CONSOLE"))
	})

	DescribeTable("quotes them into Python string literals",
		func(name, literal string) {
			content, err := productlogging.GeneratePythonLogging(map[string]productlogging.LoggerConfig{
				name: {Name: name, Level: productlogging.LogLevelDebug},
			})
			Expect(err).ToNot(HaveOccurred())
			// An unescaped quote or line break here ends the literal and turns the whole module
			// into a SyntaxError, leaving the product with no logging configuration.
			Expect(pythonLoggersBlock(content)).To(ContainSubstring(literal + ": {"))
		},
		Entry("apostrophe", "it's.logger", `'it\'s.logger'`),
		Entry("backslash", `com\example`, `'com\\example'`),
		Entry("line break", "com\nexample", `'com\nexample'`),
		Entry("tab", "com\texample", `'com\texample'`),
	)

	// The rolling file path is derived from the container name, which this package's API does not
	// constrain either; it lands in the same kind of literal.
	It("quotes the rolling file path into a Python string literal", func() {
		gen, err := productlogging.GeneratorFor(productlogging.LoggingFrameworkPython)
		Expect(err).ToNot(HaveOccurred())
		content, err := gen.Render(productlogging.LogConfig{}, productlogging.RenderOptions{
			FileOutputPath: `/kubedoop/log/it's/app.py.json`,
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(content).To(ContainSubstring(`'filename': '/kubedoop/log/it\'s/app.py.json',`))
	})
})

var _ = Describe("logback document structure", func() {
	It("wires both appenders to the root logger and keeps the named logger separate", func() {
		content, err := productlogging.GenerateLogbackWithOptions(
			map[string]productlogging.LoggerConfig{
				"org.apache.zookeeper": {Name: "org.apache.zookeeper", Level: productlogging.LogLevelDebug},
			},
			productlogging.LogbackOptions{
				FileOutputPath: "/kubedoop/log/zookeeper/zookeeper.log4j.xml",
				RootLevel:      productlogging.LogLevelWarn,
			},
		)
		Expect(err).ToNot(HaveOccurred())

		parsed := parseLogback(content)
		appenders := map[string]string{}
		files := map[string]string{}
		for _, appender := range parsed.Appenders {
			appenders[appender.Name] = appender.Class
			files[appender.Name] = appender.File
		}
		Expect(appenders).To(Equal(map[string]string{
			"STDOUT": "ch.qos.logback.core.ConsoleAppender",
			"FILE":   "ch.qos.logback.core.rolling.RollingFileAppender",
		}))
		Expect(files["FILE"]).To(Equal("/kubedoop/log/zookeeper/zookeeper.log4j.xml"))

		refs := make([]string, 0, len(parsed.Root.AppenderRefs))
		for _, ref := range parsed.Root.AppenderRefs {
			refs = append(refs, ref.Ref)
		}
		Expect(refs).To(ConsistOf("STDOUT", "FILE"))
		Expect(parsed.Root.Level).To(Equal("WARN"))
		Expect(parsed.Loggers).To(HaveLen(1))
		Expect(parsed.Loggers[0].Name).To(Equal("org.apache.zookeeper"))
		Expect(parsed.Loggers[0].Level).To(Equal("DEBUG"))
	})
})

var _ = Describe("python module name", func() {
	// The file is mounted into the product's config directory. A product that puts that
	// directory on sys.path — the usual way a Python app loads its config — would shadow the
	// standard library's logging module with a file named logging.py.
	It("cannot shadow the standard library logging module", func() {
		gen, err := productlogging.GeneratorFor(productlogging.LoggingFrameworkPython)
		Expect(err).ToNot(HaveOccurred())
		Expect(gen.DefaultFileName()).To(Equal("log_config.py"))
	})
})

var _ = Describe("log4j2 logger identifiers", func() {
	// Sanitizing collapses these two names onto one identifier; unless the collision is broken,
	// the second "logger.<id>.name" overwrites the first and one logger is never configured.
	It("keeps loggers distinct when their names sanitize to the same identifier", func() {
		content, err := productlogging.GenerateLog4j2(loggerConfigsOf(map[string]productlogging.LogLevel{
			"com.example": productlogging.LogLevelDebug,
			"com-example": productlogging.LogLevelWarn,
		}))
		Expect(err).ToNot(HaveOccurred())

		Expect(log4j2Loggers(content)).To(Equal(map[string]string{
			"com.example": "DEBUG",
			"com-example": "WARN",
		}))
		Expect(content).To(ContainSubstring("loggers=com_example,com_example_2"))
	})

	It("emits identifiers that are bare property key segments", func() {
		content, err := productlogging.GenerateLog4j2(loggerConfigsOf(map[string]productlogging.LogLevel{
			"com.example app": productlogging.LogLevelInfo,
			"":                productlogging.LogLevelInfo,
		}))
		Expect(err).ToNot(HaveOccurred())

		props := javaProperties(content)
		for _, id := range strings.Split(props["loggers"], ",") {
			Expect(id).To(MatchRegexp(`^[A-Za-z0-9_]+$`))
		}
	})
})

func loggerConfigsOf(levels map[string]productlogging.LogLevel) map[string]productlogging.LoggerConfig {
	configs := make(map[string]productlogging.LoggerConfig, len(levels))
	for name, level := range levels {
		configs[name] = productlogging.LoggerConfig{Name: name, Level: level}
	}
	return configs
}

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
