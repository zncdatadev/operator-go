package pipeline

import (
	"fmt"
	"path"
	"strconv"
	"strings"
)

// validateTrinoFinal checks only modeled relations under the declared launch
// premise. Unknown content or execution is not rewritten or called consistent.
func validateTrinoFinal(view FinalView) []Check {
	if !view.FilePreparationKnown {
		return []Check{{Subject: trinoExecutionCheck, State: Unknown, Reason: "file preparation changed"}}
	}
	main := findContainer(view.Pod, view.Generated.Main.Name)
	if !processPremise(view.Generated.Main, main) {
		return []Check{{Subject: trinoExecutionCheck, State: Unknown,
			Reason: "image, command, args, environment or working directory changed"}}
	}
	for _, access := range view.Generated.Main.Access {
		if access.Directory != view.Generated.ConfigDirectory {
			continue
		}
		found := false
		for _, mount := range main.VolumeMounts {
			if strings.HasPrefix(mount.MountPath, access.MountPath+"/") {
				return []Check{{Subject: trinoExecutionCheck, State: Unknown,
					Reason: "a nested mount may replace generated configuration"}}
			}
			if mount.Name == access.Directory && mount.MountPath == access.MountPath &&
				mount.SubPath == "" && mount.SubPathExpr == "" {
				found = true
			}
		}
		if !found {
			return []Check{{Subject: trinoExecutionCheck, State: Unknown, Reason: "configuration mount changed"}}
		}
	}
	checks := make([]Check, 0, 6)
	for _, name := range []string{"config.properties", trinoNodeFile, trinoJVMFile, "log.properties"} {
		if findFile(view.Files, view.Generated.ConfigDirectory, name) == nil {
			checks = append(checks, Check{Subject: name, State: Conflict, Reason: "required by the declared Trino launcher"})
		}
	}
	file := findFile(view.Files, view.Generated.ConfigDirectory, "config.properties")
	port, known := literalProperty(file, "http-server.http.port")
	if known {
		number, err := strconv.ParseInt(port, 10, 32)
		if err != nil || number < 1 || number > 65535 {
			checks = append(checks, Check{Subject: trinoHTTPCheck, State: Conflict, Reason: "invalid explicit HTTP port"})
		} else {
			matches := false
			for _, declared := range main.Ports {
				if declared.Name == trinoHTTPEndpoint && declared.ContainerPort == int32(number) {
					matches = true
				}
			}
			state, reason := Consistent, "file port matches the final named container port"
			if !matches {
				state, reason = Conflict, "file port does not match the final named container port"
			}
			checks = append(checks, Check{Subject: trinoHTTPCheck, State: state, Reason: reason})
		}
	} else {
		checks = append(checks, Check{Subject: trinoHTTPCheck, State: Unknown,
			Reason: "HTTP property is absent or not structured"})
	}
	logPath, known := literalProperty(file, "log.path")
	for _, output := range view.Generated.LogOutputs {
		if !view.LogCollectionKnown {
			continue
		}
		expected := ""
		for _, access := range view.Generated.Main.Access {
			if access.Directory == output.Directory {
				expected = path.Join(access.MountPath, output.RelativePath)
			}
		}
		state, reason := Unknown, "native log path is not known"
		if known {
			state, reason = Consistent, "native log path matches declared collection source"
			if logPath != expected {
				state, reason = Conflict, fmt.Sprintf("log.path %q differs from collected path %q", logPath, expected)
			}
		}
		checks = append(checks, Check{Subject: "trino.logging", State: state, Reason: reason})
	}
	return checks
}

func literalProperty(file *File, key string) (string, bool) {
	if file == nil {
		return "", false
	}
	properties, ok := file.Content.(KeyValues)
	if !ok {
		return "", false
	}
	value, ok := properties.Values[key].(Literal)
	return string(value), ok
}
