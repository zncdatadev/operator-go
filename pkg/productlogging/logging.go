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

// Package productlogging provides a product-agnostic logging engine: it converts a CRD
// LoggingConfigSpec into framework-specific logging config files (logback / log4j / log4j2 / python),
// deep-merges role/role-group logging, and exposes a generator registry. It depends only on
// the commons API types, so both pkg/config and pkg/reconciler (and product operators) can
// build on it without import cycles.
package productlogging

import (
	"fmt"
	"path"
	"strings"

	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/constant"
)

// ContainerLogging declares how one container's logging configuration file is generated.
// Products declare only the product-specific bits; the framework derives the rest from the
// (deep-merged) CRD logging spec.
type ContainerLogging struct {
	// Container is the container name; its merged logging spec (CRD logging.containers.<name>)
	// drives the generated file.
	Container string
	// Framework selects the output format (logback / log4j / log4j2 / python).
	Framework LoggingFramework
	// FileName overrides the ConfigMap key / file name. Empty uses the framework default
	// (e.g. "logback.xml").
	FileName string
	// Pattern overrides the encoder/layout pattern (product-specific, e.g. ZooKeeper's
	// "[myid:%X{myid}]" MDC). Empty uses the framework default.
	Pattern string
}

// LogFileSuffix returns the framework-owned rolling log-file suffix for a producer container.
// The suffix selects the Vector source that parses the file at the edge (the stable pipeline
// globs "<LogDir>*/*.<suffix>"):
//   - log4j and logback write log4j 1.x XMLLayout events -> ".log4j.xml" (files_log4j),
//   - log4j2 writes log4j2 XMLLayout events -> ".log4j2.xml" (files_log4j2),
//   - python writes JSON lines -> ".py.json" (files_py).
//
// Unknown frameworks return "" (RenderConfigFile rejects them via GeneratorFor first).
func LogFileSuffix(framework LoggingFramework) string {
	switch framework {
	case LoggingFrameworkLog4j, LoggingFrameworkLogback:
		return ".log4j.xml"
	case LoggingFrameworkLog4j2:
		return ".log4j2.xml"
	case LoggingFrameworkPython:
		return ".py.json"
	default:
		return ""
	}
}

// ContainerLogFileName returns the conventional rolling log-file name for a producer container
// (e.g. "<container>.log4j.xml"). It is the single source of the convention so the file
// appender (this package) and the Vector pipeline (pkg/vector) cannot drift.
// An unknown framework returns "" (not the bare container name), so direct callers fail
// fast on an invalid path instead of silently writing an ambiguous file.
func ContainerLogFileName(framework LoggingFramework, container string) string {
	suffix := LogFileSuffix(framework)
	if suffix == "" {
		return ""
	}
	return container + suffix
}

// ContainerLogDir returns the per-container log directory ("<KubedoopLogDir>/<lowercased
// container>") under which the container's rolling log file is written. The Vector sidecar
// pre-creates this directory (it starts first) and extracts the .container field from it.
func ContainerLogDir(container string) string {
	return path.Join(constant.KubedoopLogDir, strings.ToLower(container))
}

// RenderConfigFile generates the logging config file for a container declaration from its
// already-resolved (merged) logging spec, returning (fileName, content). The spec may be nil
// (no user config), in which case the generator falls back to its defaults.
//
// withFileAppender controls the rolling file appender: when true the config also writes to
// "<KubedoopLogDir>/<lowercased container>/<container>.<framework suffix>" so the Vector
// sidecar can collect and edge-parse it (see LogFileSuffix); when false the config is
// console-only. File logging is coupled to Vector — without a Vector consumer there is no
// shared log volume, so callers pass false to avoid an appender that writes to an unmounted
// path. The path convention is framework-owned (ContainerLogDir + ContainerLogFileName),
// not product-supplied.
func RenderConfigFile(spec *v1alpha1.LoggingConfigSpec, decl ContainerLogging, withFileAppender bool) (string, string, error) {
	gen, err := GeneratorFor(decl.Framework)
	if err != nil {
		return "", "", err
	}
	fileName := decl.FileName
	if fileName == "" {
		fileName = gen.DefaultFileName()
	}
	opts := RenderOptions{Pattern: decl.Pattern}
	if withFileAppender {
		// The stable path convention: "<KubedoopLogDir>/<lowercased container>/<file>". path.Join
		// collapses the trailing slash constant.KubedoopLogDir carries.
		opts.FileOutputPath = path.Join(ContainerLogDir(decl.Container), ContainerLogFileName(decl.Framework, decl.Container))
	}
	content, err := gen.Render(LogConfigFromSpec(spec), opts)
	if err != nil {
		return "", "", err
	}
	return fileName, content, nil
}

// RootLoggerName is the reserved logger key in LoggingConfigSpec.Loggers that sets the
// root logger level. All other keys become named loggers.
const RootLoggerName = "ROOT"

// LogConfig is the framework-neutral logging model derived from a product CRD's
// LoggingConfigSpec. It is the single input to every LogFileGenerator, so the
// CRD-to-config-file mapping lives in one place instead of being re-implemented by each
// product operator.
//
// A zero LogConfig means "no user overrides": generators fall back to their defaults
// (root level INFO, no named loggers, no appender thresholds).
type LogConfig struct {
	// RootLevel overrides the root logger level. Empty means the generator default (INFO).
	RootLevel LogLevel
	// Loggers maps named loggers to their levels (excluding the reserved ROOT key).
	Loggers map[string]LogLevel
	// ConsoleLevel, when set, applies a threshold to the console appender so messages
	// below this level are dropped from stdout. Empty means no threshold.
	ConsoleLevel LogLevel
	// FileLevel, when set, applies a threshold to the file appender. Empty means no
	// threshold. Only meaningful when a file appender is generated (RenderOptions.FileOutputPath).
	FileLevel LogLevel
}

// LogConfigFromSpec converts a product CRD's per-container LoggingConfigSpec into the
// framework-neutral LogConfig. It centralizes the conventions every product shares:
//   - the reserved "ROOT" logger key sets the root level,
//   - all other Loggers entries become named loggers,
//   - Console / File levels become appender thresholds.
//
// A nil spec (no user logging config) yields a zero LogConfig.
func LogConfigFromSpec(spec *v1alpha1.LoggingConfigSpec) LogConfig {
	var lc LogConfig
	if spec == nil {
		return lc
	}
	for name, level := range spec.Loggers {
		if level == nil || level.Level == "" {
			continue
		}
		if name == RootLoggerName {
			lc.RootLevel = LogLevel(level.Level)
			continue
		}
		if lc.Loggers == nil {
			lc.Loggers = make(map[string]LogLevel)
		}
		lc.Loggers[name] = LogLevel(level.Level)
	}
	if spec.Console != nil && spec.Console.Level != "" {
		lc.ConsoleLevel = LogLevel(spec.Console.Level)
	}
	if spec.File != nil && spec.File.Level != "" {
		lc.FileLevel = LogLevel(spec.File.Level)
	}
	return lc
}

// RenderOptions carries product-specific knobs that are NOT expressible through the CRD
// logging spec: the encoder/layout pattern and the rolling file appender used for log
// aggregation. Products supply these; the framework supplies everything derivable from the
// CRD via LogConfig.
type RenderOptions struct {
	// Pattern overrides the encoder/layout pattern. Empty uses the framework default.
	Pattern string
	// FileOutputPath, when set, adds a bounded rolling file appender writing to this path.
	// The path must match the log consumer's glob (the Vector sidecar collects
	// "<LogDir>*/*.<framework suffix>"), so pass
	// "<LogDir>/<lowercased container>/<container>.<framework suffix>"
	// (ContainerLogDir + ContainerLogFileName).
	FileOutputPath string
	// MaxFileSize / MaxHistory bound the rolling file appender (total usage <=
	// MaxFileSize * (MaxHistory + 1)). Sensible defaults are applied when left zero.
	// TotalSizeCap is retained for API compatibility but unused by the stable appenders.
	MaxFileSize  string
	MaxHistory   int
	TotalSizeCap string
}

// LogFileGenerator renders a logging configuration file for one logging framework from the
// framework-neutral LogConfig plus product RenderOptions.
type LogFileGenerator interface {
	// Render produces the config file content.
	Render(cfg LogConfig, opts RenderOptions) (string, error)
	// DefaultFileName is the conventional config file name (e.g. "logback.xml"). Products
	// may override it when declaring container logging.
	DefaultFileName() string
}

// GeneratorFor returns the LogFileGenerator for a logging framework.
func GeneratorFor(framework LoggingFramework) (LogFileGenerator, error) {
	switch framework {
	case LoggingFrameworkLogback:
		return logbackGenerator{}, nil
	case LoggingFrameworkLog4j:
		return log4jGenerator{}, nil
	case LoggingFrameworkLog4j2:
		return log4j2Generator{}, nil
	case LoggingFrameworkPython:
		return pythonGenerator{}, nil
	default:
		return nil, fmt.Errorf("unsupported logging framework: %s", framework)
	}
}

type logbackGenerator struct{}

func (logbackGenerator) DefaultFileName() string { return "logback.xml" }
func (logbackGenerator) Render(cfg LogConfig, opts RenderOptions) (string, error) {
	return renderLogback(cfg, opts)
}

type log4jGenerator struct{}

func (log4jGenerator) DefaultFileName() string { return "log4j.properties" }
func (log4jGenerator) Render(cfg LogConfig, opts RenderOptions) (string, error) {
	return renderLog4j(cfg, opts)
}

type log4j2Generator struct{}

func (log4j2Generator) DefaultFileName() string { return "log4j2.properties" }
func (log4j2Generator) Render(cfg LogConfig, opts RenderOptions) (string, error) {
	return renderLog4j2(cfg, opts)
}

type pythonGenerator struct{}

func (pythonGenerator) DefaultFileName() string { return "logging.py" }
func (pythonGenerator) Render(cfg LogConfig, opts RenderOptions) (string, error) {
	return renderPython(cfg, opts)
}

// MergeLoggingSpec deep-merges role-level and roleGroup-level logging specs. RoleGroup values
// win at the leaf: containers are unioned by name, loggers within a container are unioned by
// name (group overrides per key), and Console / File / EnableVectorAgent override only when
// set at the group level. This is a field-level merge, not whole-object replacement, so a
// role group that sets one logger does not wipe the role's other logging settings.
func MergeLoggingSpec(role, group *v1alpha1.LoggingSpec) *v1alpha1.LoggingSpec {
	if role == nil {
		return group
	}
	if group == nil {
		return role
	}

	merged := &v1alpha1.LoggingSpec{}
	merged.EnableVectorAgent = role.EnableVectorAgent
	if group.EnableVectorAgent != nil {
		merged.EnableVectorAgent = group.EnableVectorAgent
	}

	if role.Containers != nil || group.Containers != nil {
		merged.Containers = make(map[string]v1alpha1.LoggingConfigSpec)
		for name, rc := range role.Containers {
			merged.Containers[name] = rc
		}
		for name, gc := range group.Containers {
			if rc, ok := merged.Containers[name]; ok {
				merged.Containers[name] = mergeContainerLogging(rc, gc)
			} else {
				merged.Containers[name] = gc
			}
		}
	}
	return merged
}

// mergeContainerLogging deep-merges one container's logging config (group wins at the leaf).
func mergeContainerLogging(role, group v1alpha1.LoggingConfigSpec) v1alpha1.LoggingConfigSpec {
	merged := v1alpha1.LoggingConfigSpec{
		Console: role.Console,
		File:    role.File,
	}
	// Only override when the group actually sets a level, so a role group supplying an empty
	// console/file (e.g. `console: {}`) does not silently wipe the role-level threshold.
	if group.Console != nil && group.Console.Level != "" {
		merged.Console = group.Console
	}
	if group.File != nil && group.File.Level != "" {
		merged.File = group.File
	}
	if role.Loggers != nil || group.Loggers != nil {
		merged.Loggers = make(map[string]*v1alpha1.LogLevelSpec)
		for k, v := range role.Loggers {
			merged.Loggers[k] = v
		}
		for k, v := range group.Loggers {
			merged.Loggers[k] = v
		}
	}
	return merged
}

// loggersToLoggerConfigs adapts the LogConfig logger map to the legacy []LoggerConfig form
// consumed by the underlying renderers.
func loggersToLoggerConfigs(loggers map[string]LogLevel) map[string]LoggerConfig {
	if len(loggers) == 0 {
		return nil
	}
	out := make(map[string]LoggerConfig, len(loggers))
	for name, level := range loggers {
		out[name] = LoggerConfig{Name: name, Level: level}
	}
	return out
}
