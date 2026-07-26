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

package vector

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"text/template"

	"github.com/zncdatadev/operator-go/pkg/productlogging"
)

// VectorConfigData contains parameters for vector.yaml generation.
type VectorConfigData struct {
	// LogDir is the directory under which each producer container writes its log files
	// ("<LogDir>/<container>/<file>", see productlogging.ContainerLogFileName). A trailing
	// slash is appended when missing: the stable pipeline's source globs and container/file
	// extraction are rendered as "<LogDir>*/*.<suffix>" and "^<LogDir>(?P<container>...)".
	LogDir string

	// AggregatorAddress is the address of the Vector aggregator.
	AggregatorAddress string

	// Namespace is the Kubernetes namespace of the workload.
	Namespace string

	// ClusterName is the name of the cluster.
	ClusterName string

	// RoleName is the name of the role within the cluster.
	RoleName string

	// RoleGroupName is the name of the role group within the role.
	RoleGroupName string
}

// RenderVectorConfig renders a vector.yaml config from the given data.
// This is a pure function with no K8s dependencies.
//
// Every interpolated value is escaped for the context it lands in (see the quote / regexQuote
// template helpers), so a hostile or merely malformed input — an aggregator address carrying a
// quote or a newline — cannot break out of its scalar and emit a config Vector rejects.
func RenderVectorConfig(data VectorConfigData) (string, error) {
	if data.AggregatorAddress == "" {
		return "", fmt.Errorf("AggregatorAddress is required")
	}
	if data.LogDir == "" {
		return "", fmt.Errorf("LogDir is required")
	}
	// LogDir is emitted unquoted (source globs) as well as inside a VRL raw-string regex, and
	// neither context has an escape for these characters.
	if strings.ContainsAny(data.LogDir, "\n\r\"'") {
		return "", fmt.Errorf("LogDir %q must not contain line breaks or quotes", data.LogDir)
	}
	// The template composes per-container globs as "<LogDir>*/*<suffix>", so LogDir must end
	// with exactly one slash (constant.KubedoopLogDir already carries one).
	data.LogDir = strings.TrimRight(data.LogDir, "/") + "/"

	tmpl, err := template.New(VectorSidecarName).Funcs(template.FuncMap{
		"APIPort":     func() int { return VectorAPIPort },
		"MetricsPort": func() int { return VectorMetricsPort },
		"logSuffix":   logFileSuffix,
		"quote":       quoteDoubleQuoted,
		// The container/file extraction regex embeds LogDir literally; without this a path
		// metacharacter (e.g. a "." in the directory name) would silently widen the match.
		"regexQuote": regexp.QuoteMeta,
	}).Parse(vectorConfigTemplate)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// logFileSuffix resolves a producer framework to the rolling log-file suffix its file appender
// writes, so the source globs below are derived from the same constant the appender uses
// (productlogging.LogFileSuffix) instead of repeating the literals. An unknown framework fails
// the render rather than emitting a source that matches nothing.
func logFileSuffix(framework string) (string, error) {
	suffix := productlogging.LogFileSuffix(productlogging.LoggingFramework(framework))
	if suffix == "" {
		return "", fmt.Errorf("no log file suffix is defined for logging framework %q", framework)
	}
	return suffix, nil
}

// quoteDoubleQuoted renders s as a double-quoted scalar. It serves the template's two string
// contexts — YAML double-quoted scalars (the sink address) and VRL string literals (the
// metadata assignments inside the "source: |" blocks) — which share the same backslash escapes.
// Control characters are emitted as escapes because a raw line break would end the YAML scalar
// or the VRL statement.
func quoteDoubleQuoted(s string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"\n", `\n`,
		"\r", `\r`,
		"\t", `\t`,
	)
	return `"` + replacer.Replace(s) + `"`
}
