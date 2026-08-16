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
	"regexp"
	"sort"
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
	// Framework selects the output format (logback / log4j / log4j2 / python), and by being set at
	// all it says the FRAMEWORK renders this container's config file.
	//
	// EMPTY means the product writes the file itself, and then LogFileName is required. The
	// container still joins the Vector pipeline in every other respect — the shared log volume, its
	// RW mount, the pre-created log directory, the source — but the framework renders nothing for
	// it, so there is no ConfigMap key to collide with the one the product writes.
	//
	// That is the seam for a config file which can never be a rendered template: Airflow's
	// log_config.py must be built on Airflow's own DEFAULT_LOGGING_CONFIG, and the python
	// generator's default file name is log_config.py, so a rendered file would collide with the
	// product's own key and fail the role group — while dropping the producer entirely would cost
	// the volume, the mount, the directory and the source.
	//
	// It is expressed by leaving this empty rather than by a "do not render" flag because the
	// product then has to STATE what it will write (LogFileName), which is what makes the
	// cross-producer collision check real and the suffix checkable. A flag only says what the
	// framework will not do, and leaves the product's half in prose.
	Framework LoggingFramework
	// FileName overrides the ConfigMap key / file name. Empty uses the framework default
	// (e.g. "logback.xml").
	FileName string
	// Pattern overrides the encoder/layout pattern (product-specific, e.g. ZooKeeper's
	// "[myid:%X{myid}]" MDC). Empty uses the framework default.
	Pattern string
	// LogFileName overrides the rolling log-file name (default ContainerLogFileName, e.g.
	// "node.log4j2.xml") for products with an established file-name contract (e.g. Spark's
	// "spark.log4j2.xml"). The name MUST keep the framework suffix (LogFileSuffix) — the
	// Vector pipeline selects its edge parser by that suffix, so RenderConfigFile rejects a
	// name that drops it. The per-container log directory is unaffected.
	LogFileName string
	// LogDirName overrides the log-directory segment this producer writes under — and therefore
	// the "container" field Vector tags its events with, since the collector extracts that field
	// from the directory segment and from nothing else. Empty keeps the default (the lowercased
	// Container), byte for byte.
	//
	// It exists because one string used to do two jobs: name the pod container, and identify the
	// log stream. A product whose container name is pinned by an existing contract could not keep
	// a different, equally pinned log tag. This changes NEITHER of the other jobs Container still
	// does — the pod container the shared log volume is mounted into is matched by Container, and
	// per-container logging is still configured under logging.containers.<Container> — nor the
	// default rolling log-file base name, which stays derived from Container so that two producers
	// sharing a directory cannot resolve to the same file.
	//
	// Products that compose their own log paths (a stdout redirect in the entrypoint, a
	// hand-written config file) must read the directory from LogDirFor rather than hardcoding it:
	// the framework pre-creates only the effective directory, so a stale hardcoded path either
	// fails to open or ships one container's streams under two different tags.
	//
	// Must be a single lowercase RFC 1123 label when set.
	LogDirName string
}

// LogDirSegment returns the log-directory segment a producer declaration writes under: the
// lowercased LogDirName when set, else the lowercased Container. It is the value Vector reports
// as the event's "container" field.
func LogDirSegment(decl ContainerLogging) string {
	if decl.LogDirName != "" {
		return strings.ToLower(decl.LogDirName)
	}
	return strings.ToLower(decl.Container)
}

// LogDirFor returns the absolute per-producer log directory
// ("<KubedoopLogDir>/<LogDirSegment(decl)>") under which the declaration's rolling log file is
// written.
//
// It is the single source of the convention: the file appender (this package) and the Vector
// sidecar's pre-creating mkdir (pkg/vector) both call it, so the directory that is created is
// always the directory the producer writes to. Two implementations would diverge on the
// lowercasing alone.
func LogDirFor(decl ContainerLogging) string {
	return path.Join(constant.KubedoopLogDir, LogDirSegment(decl))
}

// logDirNameRE is the shape an explicit LogDirName must have: one lowercase RFC 1123 label.
var logDirNameRE = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// validateLogDirName rejects an explicit LogDirName that is not a single lowercase RFC 1123
// label. The bound is not stylistic — the segment reaches two places that cannot defend
// themselves:
//
//   - It is concatenated unquoted into the Vector container's `/bin/sh -c` command, which
//     pre-creates the directory ("mkdir -p <dir> && exec vector ..."). Until this field existed
//     the value was always a pod container name, which the API server had already constrained to
//     a DNS-1123 label; a free-form string removes that implicit guard. Quoting instead of
//     rejecting is not an option: the command is part of the pod template, so changing it would
//     roll every pod of every product on upgrade, whereas rejecting an input nobody has yet
//     supplied changes nothing.
//   - It is one path segment under KubedoopLogDir and one capture of the collector's
//     `^<LogDir>(?P<container>.*?)/(?P<file>.*?)$`. An empty segment or "." collapses to the log
//     root, where the `<LogDir>*/*` glob no longer matches; ".." escapes it; and an embedded "/"
//     silently truncates the tag at the first separator, because that capture is non-greedy.
//     LogFileName already rejects "/" for the same reason.
//
// The default (an empty LogDirName, i.e. the container name) is deliberately NOT validated: it is
// already constrained upstream, and checking it here would newly fail declarations that render
// correctly today.
func validateLogDirName(decl ContainerLogging) error {
	if decl.LogDirName == "" {
		return nil
	}
	lowered := strings.ToLower(decl.LogDirName)
	if len(lowered) > 63 || !logDirNameRE.MatchString(lowered) {
		return fmt.Errorf(
			"logDirName %q for container %q must be a single lowercase RFC 1123 label (it becomes one path segment under %s, the Vector event's container tag, and part of the sidecar's mkdir command)",
			decl.LogDirName, decl.Container, constant.KubedoopLogDir)
	}
	return nil
}

// ValidateProducers checks a producer declaration list as a whole, before anything is built.
//
// Two rules. Each declaration's LogDirName must be a legal segment (validateLogDirName), and no
// two declarations may resolve to the same absolute log file — two writers with two independent
// rotation policies on one file in one emptyDir, which neither Kubernetes nor the products
// notice. The collision is narrow by construction: the default log-file base name follows the
// pod container name, which is unique within a pod, so it is only reachable when a product also
// pins LogFileName on two producers that share a directory. Sharing a directory is otherwise
// legal and coherent — one product tag, several containers, distinct file names.
func ValidateProducers(decls []ContainerLogging) error {
	seen := make(map[string]string, len(decls))
	for _, decl := range decls {
		if err := validateLogDirName(decl); err != nil {
			return err
		}
		// A producer whose file the PRODUCT writes must say what it will write. Without a name the
		// framework has nothing real to check the cross-producer collision against, and no way to
		// tell whether the file will be collectible at all.
		if decl.Framework == "" && decl.LogFileName == "" {
			return fmt.Errorf(
				"container %q declares no logging framework, so the product writes its own config file — "+
					"then logFileName is required, naming the rolling log file the product will write. "+
					"Set framework to have the framework render the file instead",
				decl.Container)
		}
		// The Vector pipeline globs "<logDir>*/*<suffix>" once per framework, so the SUFFIX is what
		// decides whether a file is collected and which edge parser reads it. When the framework
		// renders the file it owns that; when the product does, this is the only place it is
		// checkable — and skipping it is how a product could mount the volume, create the directory,
		// write its logs and have Vector collect nothing, with every signal green.
		if decl.Framework == "" && !hasKnownLogFileSuffix(decl.LogFileName) {
			return fmt.Errorf(
				"log file name %q for container %q ends in no known framework suffix (%s): the Vector "+
					"source globs on that suffix, so nothing would collect the file",
				decl.LogFileName, decl.Container, strings.Join(KnownLogFileSuffixes(), ", "))
		}
		logFileName := ContainerLogFileName(decl.Framework, decl.Container)
		if decl.LogFileName != "" {
			logFileName = decl.LogFileName
		}
		full := path.Join(LogDirFor(decl), logFileName)
		if other, clash := seen[full]; clash {
			return fmt.Errorf(
				"containers %q and %q both write their log file to %q: two appenders rotating one file lose entries silently. Give them different logDirName values, or different logFileName values",
				other, decl.Container, full)
		}
		seen[full] = decl.Container
	}
	return nil
}

// KnownLogFileSuffixes returns every rolling log-file suffix the Vector pipeline globs on, sorted
// so the set is stable in an error message. A product writing its own log file must use one of
// them, or nothing collects it.
func KnownLogFileSuffixes() []string {
	seen := make(map[string]struct{}, len(loggingFrameworks))
	out := make([]string, 0, len(loggingFrameworks))
	for _, spec := range loggingFrameworks {
		if _, dup := seen[spec.logFileSuffix]; dup {
			continue
		}
		seen[spec.logFileSuffix] = struct{}{}
		out = append(out, spec.logFileSuffix)
	}
	sort.Strings(out)
	return out
}

// hasKnownLogFileSuffix reports whether name ends in a suffix the Vector pipeline globs on.
func hasKnownLogFileSuffix(name string) bool {
	for _, suffix := range KnownLogFileSuffixes() {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

// LoggingFramework defines the logging framework type.
type LoggingFramework string

const (
	LoggingFrameworkLog4j   LoggingFramework = "log4j"
	LoggingFrameworkLog4j2  LoggingFramework = "log4j2"
	LoggingFrameworkLogback LoggingFramework = "logback"
	LoggingFrameworkPython  LoggingFramework = "python"
)

// loggingFrameworkSpec holds everything the framework layer derives from one logging framework.
// A single table entry is the only registration a new framework needs, which is what lets
// SupportedLoggingFrameworks act as the enumeration the drift guards range over.
type loggingFrameworkSpec struct {
	// logFileSuffix is the rolling log-file suffix the file appender writes and the Vector
	// source globs on (see LogFileSuffix).
	logFileSuffix string
	// generator renders the framework's config file.
	generator LogFileGenerator
}

var loggingFrameworks = map[LoggingFramework]loggingFrameworkSpec{
	LoggingFrameworkLog4j:   {logFileSuffix: ".log4j.xml", generator: log4jGenerator{}},
	LoggingFrameworkLogback: {logFileSuffix: ".log4j.xml", generator: logbackGenerator{}},
	LoggingFrameworkLog4j2:  {logFileSuffix: ".log4j2.xml", generator: log4j2Generator{}},
	LoggingFrameworkPython:  {logFileSuffix: ".py.json", generator: pythonGenerator{}},
}

// SupportedLoggingFrameworks returns every framework this package can generate a config file for,
// in a stable order. It exists so the log-collection contract can be checked mechanically: the
// Vector pipeline must carry a source glob for each framework's LogFileSuffix, and ranging over
// this enumeration is what makes a newly registered framework fail those tests instead of
// silently shipping no logs for it.
func SupportedLoggingFrameworks() []LoggingFramework {
	frameworks := make([]LoggingFramework, 0, len(loggingFrameworks))
	for framework := range loggingFrameworks {
		frameworks = append(frameworks, framework)
	}
	sort.Slice(frameworks, func(i, j int) bool { return frameworks[i] < frameworks[j] })
	return frameworks
}

// LogFileSuffix returns the framework-owned rolling log-file suffix for a producer container.
// The suffix selects the Vector source that parses the file at the edge: pkg/vector renders its
// source globs ("<LogDir>*/*<suffix>") from this function, so adding a framework here without a
// matching source fails the vector render.
//
//   - log4j and logback write log4j 1.x XMLLayout events -> ".log4j.xml" (files_log4j),
//   - log4j2 writes log4j2 XMLLayout events -> ".log4j2.xml" (files_log4j2),
//   - python writes JSON lines -> ".py.json" (files_py).
//
// Unknown frameworks return "" (RenderConfigFile rejects them via GeneratorFor first).
func LogFileSuffix(framework LoggingFramework) string {
	return loggingFrameworks[framework].logFileSuffix
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
// calls it too, to pre-create the directory (it starts first), and extracts the .container
// field from the same path.
//
// It is the convention for a producer that declares no LogDirName. A declaration that may carry
// one must go through LogDirFor instead, which is what this delegates to.
func ContainerLogDir(container string) string {
	return LogDirFor(ContainerLogging{Container: container})
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
	// Outside the withFileAppender branch on purpose: an unusable logDirName is a mistake in the
	// declaration, and whether it is caught should not depend on whether Vector is on this cycle.
	if err := validateLogDirName(decl); err != nil {
		return "", "", err
	}
	fileName := decl.FileName
	if fileName == "" {
		fileName = gen.DefaultFileName()
	}
	opts := RenderOptions{Pattern: decl.Pattern}
	if withFileAppender {
		logFileName := ContainerLogFileName(decl.Framework, decl.Container)
		if decl.LogFileName != "" {
			if !strings.HasSuffix(decl.LogFileName, LogFileSuffix(decl.Framework)) {
				return "", "", fmt.Errorf(
					"log file name %q for container %q must keep the framework suffix %q (it selects the Vector edge parser)",
					decl.LogFileName, decl.Container, LogFileSuffix(decl.Framework))
			}
			if strings.Contains(decl.LogFileName, "/") {
				return "", "", fmt.Errorf(
					"log file name %q for container %q must be a bare file name: a path separator would escape the per-container log directory and the Vector collection glob",
					decl.LogFileName, decl.Container)
			}
			logFileName = decl.LogFileName
		}
		// The stable path convention: "<KubedoopLogDir>/<log dir segment>/<file>". path.Join
		// collapses the trailing slash constant.KubedoopLogDir carries.
		opts.FileOutputPath = path.Join(LogDirFor(decl), logFileName)
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
	spec, ok := loggingFrameworks[framework]
	if !ok {
		return nil, fmt.Errorf("unsupported logging framework: %s", framework)
	}
	return spec.generator, nil
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

// The module must not be called "logging.py": the generated file is mounted into the product's
// config directory, and a product that puts that directory on sys.path (the usual way a Python
// app loads its config) would shadow the standard library's logging module with it, breaking
// every "import logging" in the process.
func (pythonGenerator) DefaultFileName() string { return "log_config.py" }
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
		// One rule for both sides: an entry that states no level is not information. From the role
		// group it means "inherit" — the same as console/file above — so it must not overwrite the
		// role's entry; and where there is nothing to inherit it is dropped rather than carried.
		//
		// Overriding unconditionally made `loggers: {ROOT: {}}` erase the role's ROOT level: the
		// renderers skip a level-less entry, so the logger silently reverted to the product's
		// built-in default instead of the role's. Dropping instead of carrying also keeps the
		// merged map free of nil values — `loggers: {foo: null}` is a legal spelling of the same
		// empty entry, and a product reading merged.Loggers[k].Level should not have to know that.
		for _, loggers := range []map[string]*v1alpha1.LogLevelSpec{role.Loggers, group.Loggers} {
			for k, v := range loggers {
				if v == nil || v.Level == "" {
					continue
				}
				merged.Loggers[k] = v
			}
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
