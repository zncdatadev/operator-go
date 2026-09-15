// Package logging contains pure native logging adapters. Products still declare
// which process reads each configuration and which files it actually produces.
package logging

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/zncdatadev/operator-go/pkg/framework"
)

const (
	pythonOffLevel      = "OFF"
	pythonDebugLevel    = "DEBUG"
	pythonLevelKey      = "level"
	pythonHandlersKey   = "handlers"
	pythonFormatterName = "standard"
)

// Python returns a stdlib logging.config.dictConfig JSON document. Console and
// file thresholds are independent. ROOT names Python's root logger; other keys
// name native hierarchical loggers. An enabled file sink requires an absolute
// filename whose parent the product declares as a writable runtime directory.
// The product loads this document before emitting its events and declares the
// file as a LogOutput only when File.Level is not OFF.
func Python(config framework.ContainerLogging, filename string) (framework.Text, error) {
	console, err := pythonLevel(config.Console.Level)
	if err != nil {
		return "", fmt.Errorf("console.level: %w", err)
	}
	file, err := pythonLevel(config.File.Level)
	if err != nil {
		return "", fmt.Errorf("file.level: %w", err)
	}
	root, exists := config.Loggers["ROOT"]
	if !exists {
		return "", fmt.Errorf("loggers.ROOT is required")
	}
	rootLevel, err := pythonLevel(root.Level)
	if err != nil {
		return "", fmt.Errorf("loggers.ROOT.level: %w", err)
	}
	handlers, selected := map[string]any{}, []string{}
	if config.Console.Level != pythonOffLevel {
		handlers["console"] = map[string]any{"class": "logging.StreamHandler", pythonLevelKey: console,
			"formatter": pythonFormatterName, "stream": "ext://sys.stdout"}
		selected = append(selected, "console")
	}
	if config.File.Level != pythonOffLevel {
		if !path.IsAbs(filename) || filename == "/" || path.Clean(filename) != filename || strings.ContainsAny(filename, "\x00\r\n") {
			return "", fmt.Errorf("enabled file logging requires a clean absolute filename")
		}
		handlers["file"] = map[string]any{"class": "logging.handlers.RotatingFileHandler", pythonLevelKey: file,
			"formatter": pythonFormatterName, "filename": filename, "encoding": "utf-8", "maxBytes": 10 * 1024 * 1024,
			"backupCount": 3}
		selected = append(selected, "file")
	}
	loggers := map[string]any{}
	for name, logger := range config.Loggers {
		if name == "ROOT" {
			continue
		}
		if strings.TrimSpace(name) != name || name == "" || strings.ContainsAny(name, "\x00\r\n") {
			return "", fmt.Errorf("logger name must be nonempty and single-line; use ROOT for the root logger")
		}
		level, err := pythonLevel(logger.Level)
		if err != nil {
			return "", fmt.Errorf("logger %q: %w", name, err)
		}
		loggers[name] = map[string]any{pythonLevelKey: level, pythonHandlersKey: []string{}, "propagate": true}
	}
	data, err := json.MarshalIndent(map[string]any{"version": 1, "disable_existing_loggers": false,
		"formatters":      map[string]any{pythonFormatterName: map[string]any{"format": "%(asctime)s %(levelname)s %(name)s %(message)s"}},
		pythonHandlersKey: handlers, "loggers": loggers, "root": map[string]any{pythonLevelKey: rootLevel, pythonHandlersKey: selected}}, "", "  ")
	return framework.Text(data), err
}

func pythonLevel(level string) (int, error) {
	levels := map[string]int{"TRACE": 5, pythonDebugLevel: 10, "INFO": 20, "WARN": 30, "ERROR": 40, "FATAL": 50, pythonOffLevel: 2147483647}
	value, ok := levels[level]
	if !ok {
		return 0, fmt.Errorf("unsupported level %q", level)
	}
	return value, nil
}
