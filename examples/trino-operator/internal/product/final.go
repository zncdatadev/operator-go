package product

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/zncdatadev/operator-go/pkg/framework"
)

// validateTrinoFinal checks only modeled relations under the declared launch
// premise. framework.Unknown content or execution is not rewritten or called consistent.
func validateTrinoFinal(view framework.FinalView) []framework.Check {
	if !view.FilePreparationKnown {
		return []framework.Check{{Subject: trinoExecutionCheck, State: framework.Unknown, Reason: "file preparation changed"}}
	}
	main := findContainer(view.Pod, view.Generated.Main.Name)
	if !processPremise(view.Generated.Main, main) {
		return []framework.Check{{Subject: trinoExecutionCheck, State: framework.Unknown,
			Reason: "image, command, args, environment or working directory changed"}}
	}
	for _, access := range view.Generated.Main.Access {
		if access.Directory != view.Generated.ConfigDirectory {
			continue
		}
		found := false
		for _, mount := range main.VolumeMounts {
			if strings.HasPrefix(mount.MountPath, access.MountPath+"/") {
				return []framework.Check{{Subject: trinoExecutionCheck, State: framework.Unknown,
					Reason: "a nested mount may replace generated configuration"}}
			}
			if mount.Name == access.Directory && mount.MountPath == access.MountPath &&
				mount.SubPath == "" && mount.SubPathExpr == "" {
				found = true
			}
		}
		if !found {
			return []framework.Check{{Subject: trinoExecutionCheck, State: framework.Unknown, Reason: "configuration mount changed"}}
		}
	}
	checks := make([]framework.Check, 0, 6)
	for _, name := range []string{"config.properties", trinoNodeFile, trinoJVMFile, "log.properties"} {
		if findFile(view.Files, view.Generated.ConfigDirectory, name) == nil {
			checks = append(checks, framework.Check{Subject: name, State: framework.Conflict, Reason: "required by the declared Trino launcher"})
		}
	}
	file := findFile(view.Files, view.Generated.ConfigDirectory, "config.properties")
	port, known := literalProperty(file, "http-server.http.port")
	if known {
		number, err := strconv.ParseInt(port, 10, 32)
		if err != nil || number < 1 || number > 65535 {
			checks = append(checks, framework.Check{Subject: trinoHTTPCheck, State: framework.Conflict, Reason: "invalid explicit HTTP port"})
		} else {
			matches := false
			for _, declared := range main.Ports {
				if declared.Name == trinoHTTPEndpoint && declared.ContainerPort == int32(number) {
					matches = true
				}
			}
			state, reason := framework.Consistent, "file port matches the final named container port"
			if !matches {
				state, reason = framework.Conflict, "file port does not match the final named container port"
			}
			checks = append(checks, framework.Check{Subject: trinoHTTPCheck, State: state, Reason: reason})
		}
	} else {
		checks = append(checks, framework.Check{Subject: trinoHTTPCheck, State: framework.Unknown,
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
		state, reason := framework.Unknown, "native log path is not known"
		if known {
			state, reason = framework.Consistent, "native log path matches declared collection source"
			if logPath != expected {
				state, reason = framework.Conflict, fmt.Sprintf("log.path %q differs from collected path %q", logPath, expected)
			}
		}
		checks = append(checks, framework.Check{Subject: "trino.logging", State: state, Reason: reason})
	}
	return checks
}

func literalProperty(file *framework.File, key string) (string, bool) {
	if file == nil {
		return "", false
	}
	properties, ok := file.Content.(framework.KeyValues)
	if !ok {
		return "", false
	}
	value, ok := properties.Values[key].(framework.Literal)
	return string(value), ok
}
