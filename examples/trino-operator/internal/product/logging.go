package product

import (
	"fmt"
	"strings"

	"github.com/zncdatadev/operator-go/pkg/framework"
)

const (
	trinoRootLogger = "ROOT"
	trinoOffLevel   = "OFF"
	trinoTraceLevel = "TRACE"
	trinoDebugLevel = "DEBUG"
	trinoErrorLevel = "ERROR"
	trinoWarnLevel  = "WARN"
	trinoLoggerName = "io.trino"
)

type trinoLoggingPlan struct {
	Console, File bool
	Levels        map[string]framework.PropertyValue
}

// Airlift 336 routes both handlers through JUL logger levels. A single enabled
// sink, or two sinks with the same threshold, can be represented by clamping
// each explicit logger (including ROOT). Unequal enabled thresholds cannot.
// ROOT is the framework spelling; Airlift's native root property key is empty.
func resolveTrinoLogging(logging framework.Logging) (trinoLoggingPlan, error) {
	var plan trinoLoggingPlan
	for _, container := range sortedKeys(logging.Containers) {
		if container != trinoName {
			return plan, fmt.Errorf("unsupported log producer %q", container)
		}
	}
	config, exists := logging.Containers[trinoName]
	if !exists {
		return plan, fmt.Errorf("config.logging.containers.trino is required")
	}
	console, err := trinoLogRank(config.Console.Level)
	if err != nil {
		return plan, fmt.Errorf("config.logging.containers.trino.console.level: %w", err)
	}
	file, err := trinoLogRank(config.File.Level)
	if err != nil {
		return plan, fmt.Errorf("config.logging.containers.trino.file.level: %w", err)
	}
	plan.Console, plan.File = config.Console.Level != trinoOffLevel, config.File.Level != trinoOffLevel
	if plan.Console && plan.File && console != file {
		return plan, fmt.Errorf("trino cannot represent different active console.level and file.level thresholds; " +
			"use equal thresholds or set one sink to OFF")
	}
	threshold := file
	if plan.Console {
		threshold = console
	}
	plan.Levels = make(map[string]framework.PropertyValue, len(config.Loggers))
	for _, name := range sortedKeys(config.Loggers) {
		if strings.TrimSpace(name) == "" {
			return trinoLoggingPlan{}, fmt.Errorf("trino logger name is empty; use ROOT for the root logger")
		}
		level := config.Loggers[name].Level
		rank, err := trinoLogRank(level)
		if err != nil {
			return trinoLoggingPlan{}, fmt.Errorf("unsupported Trino logger level for %q: %w", name, err)
		}
		if rank < threshold {
			level = config.File.Level
			if plan.Console {
				level = config.Console.Level
			}
		}
		key := name
		if key == trinoRootLogger {
			key = ""
		}
		plan.Levels[key] = framework.Literal(level)
	}
	if _, exists := plan.Levels[""]; !exists {
		return trinoLoggingPlan{}, fmt.Errorf("config.logging.containers.trino.loggers.ROOT is required")
	}
	return plan, nil
}

func trinoLogRank(level string) (int, error) {
	levels := []string{trinoTraceLevel, trinoDebugLevel, trinoInfoLevel, trinoWarnLevel, trinoErrorLevel, trinoOffLevel}
	for rank, candidate := range levels {
		if level == candidate {
			return rank, nil
		}
	}
	return 0, fmt.Errorf("unsupported level %q", level)
}
