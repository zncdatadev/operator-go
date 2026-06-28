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
		out, err := g.Render(cfg, productlogging.RenderOptions{FileOutputPath: "/kubedoop/log/zk.stdout.log"})
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring(`<root level="WARN">`))
		Expect(out).To(ContainSubstring(`<logger name="org.apache.zookeeper" level="DEBUG" />`))
		Expect(out).To(ContainSubstring(`<filter class="ch.qos.logback.classic.filter.ThresholdFilter">`))
		Expect(out).To(ContainSubstring("<level>INFO</level>"))  // console threshold
		Expect(out).To(ContainSubstring("<level>ERROR</level>")) // file threshold
		Expect(out).To(ContainSubstring("RollingFileAppender"))
	})

	It("renders log4j2 root, loggers and threshold filters", func() {
		g, _ := productlogging.GeneratorFor(productlogging.LoggingFrameworkLog4j2)
		out, err := g.Render(cfg, productlogging.RenderOptions{FileOutputPath: "/kubedoop/log/zk.stdout.log"})
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring("rootLogger.level=WARN"))
		Expect(out).To(ContainSubstring("appenders=console,file"))
		Expect(out).To(ContainSubstring("appender.console.filter.threshold.level=INFO"))
		Expect(out).To(ContainSubstring("appender.file.filter.threshold.level=ERROR"))
		Expect(out).To(ContainSubstring("logger.org_apache_zookeeper.level=DEBUG"))
	})

	It("renders python root, loggers and a file handler", func() {
		g, _ := productlogging.GeneratorFor(productlogging.LoggingFrameworkPython)
		out, err := g.Render(cfg, productlogging.RenderOptions{FileOutputPath: "/kubedoop/log/zk.stdout.log"})
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring("'level': 'WARNING'")) // root WARN -> WARNING
		Expect(out).To(ContainSubstring("'org.apache.zookeeper'"))
		Expect(out).To(ContainSubstring("RotatingFileHandler"))
	})
})
