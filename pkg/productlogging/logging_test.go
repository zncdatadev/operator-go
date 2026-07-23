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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/productlogging"
	"k8s.io/utils/ptr"
)

var _ = Describe("LogConfigFromSpec", func() {
	It("returns a zero LogConfig for a nil spec", func() {
		lc := productlogging.LogConfigFromSpec(nil)
		Expect(lc.RootLevel).To(BeEmpty())
		Expect(lc.Loggers).To(BeNil())
		Expect(lc.ConsoleLevel).To(BeEmpty())
		Expect(lc.FileLevel).To(BeEmpty())
	})

	It("maps the ROOT key to the root level and others to named loggers", func() {
		lc := productlogging.LogConfigFromSpec(&v1alpha1.LoggingConfigSpec{
			Loggers: map[string]*v1alpha1.LogLevelSpec{
				productlogging.RootLoggerName: {Level: "WARN"},
				"org.apache.zookeeper":        {Level: "DEBUG"},
				"empty":                       {Level: ""},
			},
			Console: &v1alpha1.LogLevelSpec{Level: "INFO"},
			File:    &v1alpha1.LogLevelSpec{Level: "ERROR"},
		})
		Expect(string(lc.RootLevel)).To(Equal("WARN"))
		Expect(lc.Loggers).To(HaveLen(1))
		Expect(string(lc.Loggers["org.apache.zookeeper"])).To(Equal("DEBUG"))
		Expect(string(lc.ConsoleLevel)).To(Equal("INFO"))
		Expect(string(lc.FileLevel)).To(Equal("ERROR"))
	})
})

var _ = Describe("GeneratorFor", func() {
	It("returns a generator for each supported framework", func() {
		for _, fw := range []productlogging.LoggingFramework{
			productlogging.LoggingFrameworkLogback,
			productlogging.LoggingFrameworkLog4j,
			productlogging.LoggingFrameworkLog4j2,
			productlogging.LoggingFrameworkPython,
		} {
			g, err := productlogging.GeneratorFor(fw)
			Expect(err).NotTo(HaveOccurred())
			Expect(g).NotTo(BeNil())
			Expect(g.DefaultFileName()).NotTo(BeEmpty())
		}
	})

	It("errors for an unsupported framework", func() {
		_, err := productlogging.GeneratorFor(productlogging.LoggingFramework("nope"))
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("RenderConfigFile file path", func() {
	// constant.KubedoopLogDir carries a trailing slash; the framework-derived stable file path
	// ("<LogDir>/<lowercased container>/<container>.<framework suffix>") must collapse to a
	// single slash (no "/kubedoop/log//main/...").
	It("renders the framework-derived per-container file appender path with a single slash", func() {
		for _, fw := range []productlogging.LoggingFramework{
			productlogging.LoggingFrameworkLogback,
			productlogging.LoggingFrameworkLog4j,
			productlogging.LoggingFrameworkLog4j2,
			productlogging.LoggingFrameworkPython,
		} {
			_, content, err := productlogging.RenderConfigFile(nil, productlogging.ContainerLogging{
				Framework: fw,
				Container: "Main", // mixed case: the directory is lowercased, the file keeps the name
			}, true)
			Expect(err).NotTo(HaveOccurred(), "framework %s", fw)
			expected := "/kubedoop/log/main/Main" + productlogging.LogFileSuffix(fw)
			Expect(content).To(ContainSubstring(expected), "framework %s", fw)
			Expect(content).NotTo(ContainSubstring("//main"), "framework %s", fw)
		}
	})

	It("derives the framework-owned file names (stable per-framework suffixes)", func() {
		Expect(productlogging.ContainerLogFileName(productlogging.LoggingFrameworkLog4j, "kafka")).To(Equal("kafka.log4j.xml"))
		Expect(productlogging.ContainerLogFileName(productlogging.LoggingFrameworkLogback, "zookeeper")).To(Equal("zookeeper.log4j.xml"))
		Expect(productlogging.ContainerLogFileName(productlogging.LoggingFrameworkLog4j2, "hive")).To(Equal("hive.log4j2.xml"))
		Expect(productlogging.ContainerLogFileName(productlogging.LoggingFrameworkPython, "superset")).To(Equal("superset.py.json"))
		Expect(productlogging.ContainerLogDir("Main")).To(Equal("/kubedoop/log/main"))
	})

	It("honors a LogFileName override that keeps the framework suffix", func() {
		_, content, err := productlogging.RenderConfigFile(nil, productlogging.ContainerLogging{
			Framework:   productlogging.LoggingFrameworkLog4j2,
			Container:   "node",
			LogFileName: "spark.log4j2.xml",
		}, true)
		Expect(err).NotTo(HaveOccurred())
		Expect(content).To(ContainSubstring("/kubedoop/log/node/spark.log4j2.xml"))
		Expect(content).NotTo(ContainSubstring("node.log4j2.xml"))
	})

	It("rejects a LogFileName override that drops the framework suffix", func() {
		_, _, err := productlogging.RenderConfigFile(nil, productlogging.ContainerLogging{
			Framework:   productlogging.LoggingFrameworkLog4j2,
			Container:   "node",
			LogFileName: "spark.log",
		}, true)
		Expect(err).To(MatchError(ContainSubstring("must keep the framework suffix")))
	})

	It("omits the file appender when withFileAppender is false (console-only)", func() {
		_, content, err := productlogging.RenderConfigFile(nil, productlogging.ContainerLogging{
			Framework: productlogging.LoggingFrameworkLogback,
			Container: "main",
		}, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(content).NotTo(ContainSubstring("main.log4j.xml"))
		Expect(content).NotTo(ContainSubstring("RollingFileAppender"))
	})

	It("omits the log4j FILE appender when withFileAppender is false (console-only)", func() {
		fileName, content, err := productlogging.RenderConfigFile(nil, productlogging.ContainerLogging{
			Framework: productlogging.LoggingFrameworkLog4j,
			Container: "kafka",
		}, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(fileName).To(Equal("log4j.properties"))
		Expect(content).To(ContainSubstring("log4j.rootLogger=INFO, CONSOLE\n"))
		Expect(content).NotTo(ContainSubstring("FILE"))
		Expect(content).NotTo(ContainSubstring("kafka.log4j.xml"))
	})

	It("wires the log4j FILE appender (XMLLayout) to the framework-derived path when withFileAppender is true", func() {
		fileName, content, err := productlogging.RenderConfigFile(nil, productlogging.ContainerLogging{
			Framework: productlogging.LoggingFrameworkLog4j,
			Container: "kafka",
		}, true)
		Expect(err).NotTo(HaveOccurred())
		Expect(fileName).To(Equal("log4j.properties"))
		Expect(content).To(ContainSubstring("log4j.rootLogger=INFO, CONSOLE, FILE"))
		Expect(content).To(ContainSubstring("log4j.appender.FILE=org.apache.log4j.RollingFileAppender"))
		Expect(content).To(ContainSubstring("log4j.appender.FILE.File=/kubedoop/log/kafka/kafka.log4j.xml"))
		Expect(content).To(ContainSubstring("log4j.appender.FILE.MaxFileSize=5MB"))
		Expect(content).To(ContainSubstring("log4j.appender.FILE.MaxBackupIndex=1"))
		// The stable FILE layout is the log4j XMLLayout, which the Vector files_log4j source
		// edge-parses; no plain-text pattern on the FILE appender.
		Expect(content).To(ContainSubstring("log4j.appender.FILE.layout=org.apache.log4j.xml.XMLLayout"))
		Expect(content).NotTo(ContainSubstring("log4j.appender.FILE.layout.ConversionPattern"))
	})

	It("renders a product-supplied log4j conversion pattern verbatim on the console appender (Kafka style)", func() {
		_, content, err := productlogging.RenderConfigFile(nil, productlogging.ContainerLogging{
			Framework: productlogging.LoggingFrameworkLog4j,
			Container: "kafka",
			Pattern:   "[%d] %p %m (%c)%n",
		}, true)
		Expect(err).NotTo(HaveOccurred())
		Expect(content).To(ContainSubstring("log4j.appender.CONSOLE.layout.ConversionPattern=[%d] %p %m (%c)%n"))
		// The FILE appender uses XMLLayout; the pattern only applies to the console appender.
		Expect(content).To(ContainSubstring("log4j.appender.FILE.layout=org.apache.log4j.xml.XMLLayout"))
	})
})

var _ = Describe("MergeLoggingSpec", func() {
	It("returns the other side when one is nil", func() {
		g := &v1alpha1.LoggingSpec{EnableVectorAgent: ptr.To(true)}
		Expect(productlogging.MergeLoggingSpec(nil, g)).To(Equal(g))
		Expect(productlogging.MergeLoggingSpec(g, nil)).To(Equal(g))
		Expect(productlogging.MergeLoggingSpec(nil, nil)).To(BeNil())
	})

	It("deep-merges containers and loggers at the leaf, with role group winning", func() {
		role := &v1alpha1.LoggingSpec{
			EnableVectorAgent: ptr.To(true),
			Containers: map[string]v1alpha1.LoggingConfigSpec{
				"main": {
					Loggers: map[string]*v1alpha1.LogLevelSpec{
						"ROOT":  {Level: "INFO"},
						"a.b.c": {Level: "DEBUG"},
					},
					Console: &v1alpha1.LogLevelSpec{Level: "INFO"},
				},
			},
		}
		group := &v1alpha1.LoggingSpec{
			Containers: map[string]v1alpha1.LoggingConfigSpec{
				"main": {
					Loggers: map[string]*v1alpha1.LogLevelSpec{
						"ROOT":  {Level: "WARN"},  // overrides role
						"x.y.z": {Level: "ERROR"}, // adds to role
					},
				},
			},
		}

		merged := productlogging.MergeLoggingSpec(role, group)
		Expect(merged).NotTo(BeNil())
		// role-level EnableVectorAgent survives because the group did not set it.
		Expect(merged.EnableVectorAgent).To(Equal(ptr.To(true)))
		c := merged.Containers["main"]
		Expect(c.Loggers["ROOT"].Level).To(Equal("WARN"))   // group override
		Expect(c.Loggers["a.b.c"].Level).To(Equal("DEBUG")) // role survives
		Expect(c.Loggers["x.y.z"].Level).To(Equal("ERROR")) // group addition
		Expect(c.Console.Level).To(Equal("INFO"))           // role survives (group did not set)
	})

	It("does not let an empty group console/file wipe the role-level threshold", func() {
		role := &v1alpha1.LoggingSpec{
			Containers: map[string]v1alpha1.LoggingConfigSpec{
				"main": {
					Console: &v1alpha1.LogLevelSpec{Level: "INFO"},
					File:    &v1alpha1.LogLevelSpec{Level: "ERROR"},
				},
			},
		}
		// Group supplies console/file with no level (e.g. `console: {}`).
		group := &v1alpha1.LoggingSpec{
			Containers: map[string]v1alpha1.LoggingConfigSpec{
				"main": {
					Console: &v1alpha1.LogLevelSpec{},
					File:    &v1alpha1.LogLevelSpec{},
				},
			},
		}
		c := productlogging.MergeLoggingSpec(role, group).Containers["main"]
		Expect(c.Console.Level).To(Equal("INFO")) // role threshold preserved
		Expect(c.File.Level).To(Equal("ERROR"))   // role threshold preserved
	})
})

var _ = Describe("Render with appender thresholds", func() {
	cfg := productlogging.LogConfig{
		RootLevel:    productlogging.LogLevelWarn,
		Loggers:      map[string]productlogging.LogLevel{"org.apache.zookeeper": productlogging.LogLevelDebug},
		ConsoleLevel: productlogging.LogLevelInfo,
		FileLevel:    productlogging.LogLevelError,
	}

	It("renders logback root, loggers and console/file ThresholdFilters", func() {
		g, _ := productlogging.GeneratorFor(productlogging.LoggingFrameworkLogback)
		out, err := g.Render(cfg, productlogging.RenderOptions{FileOutputPath: "/kubedoop/log/zk/zk.log4j.xml"})
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring(`<root level="WARN">`))
		Expect(out).To(ContainSubstring(`<logger name="org.apache.zookeeper" level="DEBUG" />`))
		Expect(out).To(ContainSubstring(`<filter class="ch.qos.logback.classic.filter.ThresholdFilter">`))
		Expect(out).To(ContainSubstring("<level>INFO</level>"))  // console threshold
		Expect(out).To(ContainSubstring("<level>ERROR</level>")) // file threshold
		Expect(out).To(ContainSubstring("RollingFileAppender"))
	})

	It("renders log4j root, loggers and appender Thresholds", func() {
		g, _ := productlogging.GeneratorFor(productlogging.LoggingFrameworkLog4j)
		out, err := g.Render(cfg, productlogging.RenderOptions{FileOutputPath: "/kubedoop/log/zk/zk.log4j.xml"})
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring("log4j.rootLogger=WARN, CONSOLE, FILE"))
		Expect(out).To(ContainSubstring("log4j.appender.CONSOLE.Threshold=INFO"))
		Expect(out).To(ContainSubstring("log4j.appender.FILE.Threshold=ERROR"))
		Expect(out).To(ContainSubstring("log4j.logger.org.apache.zookeeper=DEBUG"))
		// File appender must be bounded (MaxFileSize + MaxBackupIndex), not unbounded.
		Expect(out).To(ContainSubstring("log4j.appender.FILE.MaxFileSize=5MB"))
		Expect(out).To(ContainSubstring("log4j.appender.FILE.MaxBackupIndex=1"))
	})

	It("renders log4j2 root, loggers and threshold filters", func() {
		g, _ := productlogging.GeneratorFor(productlogging.LoggingFrameworkLog4j2)
		out, err := g.Render(cfg, productlogging.RenderOptions{FileOutputPath: "/kubedoop/log/zk/zk.log4j2.xml"})
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring("rootLogger.level=WARN"))
		Expect(out).To(ContainSubstring("appenders=console,file"))
		Expect(out).To(ContainSubstring("appender.console.filter.threshold.level=INFO"))
		Expect(out).To(ContainSubstring("appender.file.filter.threshold.level=ERROR"))
		Expect(out).To(ContainSubstring("logger.org_apache_zookeeper.level=DEBUG"))
		// File appender must be bounded (rollover policy + strategy), not unbounded.
		Expect(out).To(ContainSubstring("SizeBasedTriggeringPolicy"))
		Expect(out).To(ContainSubstring("appender.file.policies.size.size=5MB"))
		Expect(out).To(ContainSubstring("appender.file.strategy.max=1"))
	})

	It("renders python root, loggers and a file handler", func() {
		g, _ := productlogging.GeneratorFor(productlogging.LoggingFrameworkPython)
		out, err := g.Render(cfg, productlogging.RenderOptions{FileOutputPath: "/kubedoop/log/zk/zk.py.json"})
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring("'level': 'WARNING'")) // root WARN -> WARNING
		Expect(out).To(ContainSubstring("'org.apache.zookeeper'"))
		Expect(out).To(ContainSubstring("RotatingFileHandler"))
		// RotatingFileHandler must set maxBytes/backupCount, else rotation is disabled.
		Expect(out).To(ContainSubstring("'maxBytes': 5242880"))
		Expect(out).To(ContainSubstring("'backupCount': 1"))
	})
})
