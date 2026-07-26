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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zncdatadev/operator-go/pkg/productlogging"
)

func defaultConfigData() VectorConfigData {
	return VectorConfigData{
		LogDir:            "/kubedoop/log/",
		AggregatorAddress: "vector-aggregator:9000",
		Namespace:         "default",
		ClusterName:       "test-cluster",
		RoleName:          "worker",
		RoleGroupName:     "default",
	}
}

func TestRenderVectorConfig(t *testing.T) {
	tests := []struct {
		name     string
		data     VectorConfigData
		contains []string
	}{
		{
			name:     "default config",
			data:     defaultConfigData(),
			contains: []string{"/kubedoop/log/", "vector-aggregator:9000", "default", "test-cluster", "worker", "default"},
		},
		{
			name: "custom aggregator address",
			data: func() VectorConfigData {
				d := defaultConfigData()
				d.AggregatorAddress = "custom-aggregator.example.com:9001"
				return d
			}(),
			contains: []string{"custom-aggregator.example.com:9001"},
		},
		{
			name: "custom log directory",
			data: func() VectorConfigData {
				d := defaultConfigData()
				d.LogDir = "/custom/logs/"
				return d
			}(),
			contains: []string{"/custom/logs/"},
		},
		{
			name: "custom namespace and cluster",
			data: func() VectorConfigData {
				d := defaultConfigData()
				d.Namespace = "production"
				d.ClusterName = "prod-cluster-1"
				d.RoleName = "coordinator"
				d.RoleGroupName = "large"
				return d
			}(),
			contains: []string{"production", "prod-cluster-1", "coordinator", "large"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := RenderVectorConfig(tt.data)
			if err != nil {
				t.Fatalf("RenderVectorConfig() error = %v", err)
			}

			for _, substr := range tt.contains {
				if !strings.Contains(result, substr) {
					t.Errorf("RenderVectorConfig() result missing expected substring %q", substr)
				}
			}
		})
	}
}

// TestRenderVectorConfig_LogDirTrailingSlash asserts that a LogDir without a trailing slash
// is normalized: the stable per-container globs are composed as "<LogDir>*/*.<suffix>".
func TestRenderVectorConfig_LogDirTrailingSlash(t *testing.T) {
	data := defaultConfigData()
	data.LogDir = "/var/log/app" // no trailing slash
	result, err := RenderVectorConfig(data)
	if err != nil {
		t.Fatalf("RenderVectorConfig() error = %v", err)
	}
	if !strings.Contains(result, "/var/log/app/*/*.stdout.log") {
		t.Errorf("RenderVectorConfig() missing normalized per-container glob, got:\n%s", result)
	}
	if strings.Contains(result, "/var/log/app*/") {
		t.Errorf("RenderVectorConfig() rendered an unnormalized glob (missing slash)")
	}
}

// TestRenderVectorConfig_EscapesInterpolatedValues asserts that a value carrying a quote or a
// line break stays inside its scalar: unescaped, it would close the YAML string (or the VRL
// statement) and emit a config Vector cannot load.
func TestRenderVectorConfig_EscapesInterpolatedValues(t *testing.T) {
	data := defaultConfigData()
	data.AggregatorAddress = "agg\":9000\nplayground: true"
	data.Namespace = `ns"; .injected = "yes`
	data.ClusterName = "cluster\nname"
	data.RoleName = `role\path`

	result, err := RenderVectorConfig(data)
	if err != nil {
		t.Fatalf("RenderVectorConfig() error = %v", err)
	}

	checks := []string{
		`address: "agg\":9000\nplayground: true"`,
		`.namespace = "ns\"; .injected = \"yes"`,
		`.cluster = "cluster\nname"`,
		`.role = "role\\path"`,
	}
	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("RenderVectorConfig() missing escaped value %q, got:\n%s", check, result)
		}
	}

	// Every rendered line must still belong to the document: an unescaped break would have
	// produced a top-level "playground: true" line of its own.
	for _, line := range strings.Split(result, "\n") {
		if line == "playground: true" {
			t.Error("RenderVectorConfig() let an interpolated value escape into a config line")
		}
	}
}

// TestRenderVectorConfig_RejectsUnquotableLogDir asserts LogDir is refused rather than emitted
// into the source globs and the VRL raw-string regex, neither of which can escape these.
func TestRenderVectorConfig_RejectsUnquotableLogDir(t *testing.T) {
	for _, logDir := range []string{"/var/log\n", "/var/'log'/", `/var/"log"/`} {
		data := defaultConfigData()
		data.LogDir = logDir
		if _, err := RenderVectorConfig(data); err == nil {
			t.Errorf("RenderVectorConfig() expected error for LogDir %q, got nil", logDir)
		}
	}
}

// TestRenderVectorConfig_RegexQuotesLogDir asserts the container/file extraction regex treats
// LogDir literally: an unquoted metacharacter would widen the match beyond the log directory.
func TestRenderVectorConfig_RegexQuotesLogDir(t *testing.T) {
	data := defaultConfigData()
	data.LogDir = "/var/log.d/"

	result, err := RenderVectorConfig(data)
	if err != nil {
		t.Fatalf("RenderVectorConfig() error = %v", err)
	}

	want := `parse_regex!(.file, r'^/var/log\.d/(?P<container>.*?)/(?P<file>.*?)$')`
	if !strings.Contains(result, want) {
		t.Errorf("RenderVectorConfig() missing regex-quoted LogDir %q, got:\n%s", want, result)
	}
}

// TestRenderVectorConfig_SourceGlobsMatchLogFileSuffixes is the guard that keeps the collector
// and the producers in sync: the file appenders name their files from
// productlogging.LogFileSuffix, so every framework productlogging can render must have a source
// glob for that exact suffix. The list is productlogging's own enumeration rather than a copy of
// it, so a framework registered there without a source here fails this test instead of silently
// shipping no logs for it.
func TestRenderVectorConfig_SourceGlobsMatchLogFileSuffixes(t *testing.T) {
	result, err := RenderVectorConfig(defaultConfigData())
	if err != nil {
		t.Fatalf("RenderVectorConfig() error = %v", err)
	}

	frameworks := productlogging.SupportedLoggingFrameworks()
	if len(frameworks) == 0 {
		t.Fatal("productlogging.SupportedLoggingFrameworks() is empty")
	}
	for _, framework := range frameworks {
		suffix := productlogging.LogFileSuffix(framework)
		if suffix == "" {
			t.Fatalf("productlogging.LogFileSuffix(%q) is empty", framework)
		}
		glob := "/kubedoop/log/*/*" + suffix
		if !strings.Contains(result, glob) {
			t.Errorf("RenderVectorConfig() has no source glob %q for framework %q", glob, framework)
		}
	}
}

func TestRenderVectorConfig_GoldenFile(t *testing.T) {
	data := defaultConfigData()
	result, err := RenderVectorConfig(data)
	if err != nil {
		t.Fatalf("RenderVectorConfig() error = %v", err)
	}

	goldenPath := filepath.Join("testdata", "default_vector.yaml.golden")
	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("Failed to read golden file: %v", err)
	}

	expected := strings.TrimSpace(string(golden))
	actual := strings.TrimSpace(result)

	if actual != expected {
		t.Errorf("RenderVectorConfig() output does not match golden file.\nExpected:\n%s\n\nGot:\n%s", expected, actual)
	}
}

func TestRenderVectorConfig_ContainsAllSources(t *testing.T) {
	data := defaultConfigData()
	result, err := RenderVectorConfig(data)
	if err != nil {
		t.Fatalf("RenderVectorConfig() error = %v", err)
	}

	expectedSources := []string{
		"files_stdout",
		"files_stderr",
		"files_log4j",
		"files_log4j2",
		"files_py",
		"files_airlift",
		"vector",
	}

	for _, source := range expectedSources {
		if !strings.Contains(result, source+":") {
			t.Errorf("RenderVectorConfig() missing source %q", source)
		}
	}

	// The per-container globs of the stable pipeline ("<LogDir>*/*.<suffix>").
	expectedGlobs := []string{
		"/kubedoop/log/*/*.stdout.log",
		"/kubedoop/log/*/*.stderr.log",
		"/kubedoop/log/*/*.log4j.xml",
		"/kubedoop/log/*/*.log4j2.xml",
		"/kubedoop/log/*/*.py.json",
		"/kubedoop/log/*/*.airlift.json",
	}
	for _, glob := range expectedGlobs {
		if !strings.Contains(result, glob) {
			t.Errorf("RenderVectorConfig() missing per-container glob %q", glob)
		}
	}
}

func TestRenderVectorConfig_ContainsAllTransforms(t *testing.T) {
	data := defaultConfigData()
	result, err := RenderVectorConfig(data)
	if err != nil {
		t.Fatalf("RenderVectorConfig() error = %v", err)
	}

	expectedTransforms := []string{
		"processed_files_stdout",
		"processed_files_stderr",
		"processed_files_log4j",
		"processed_files_log4j2",
		"processed_files_py",
		"processed_files_airlift",
		"extended_logs_files",
		"extended_logs",
	}

	for _, transform := range expectedTransforms {
		if !strings.Contains(result, transform+":") {
			t.Errorf("RenderVectorConfig() missing transform %q", transform)
		}
	}
}

// TestRenderVectorConfig_EdgeParsing asserts the stable edge-parsing semantics survive
// rendering: structured parsing of log4j/log4j2/py events, container/file extraction, and
// the normalized event schema fields.
func TestRenderVectorConfig_EdgeParsing(t *testing.T) {
	data := defaultConfigData()
	result, err := RenderVectorConfig(data)
	if err != nil {
		t.Fatalf("RenderVectorConfig() error = %v", err)
	}

	checks := []string{
		// log4j XML edge parsing (namespace wrapper + XML parse).
		`xmlns:log4j=\"http://jakarta.apache.org/log4j/\"`,
		"parse_xml(wrapped_xml_event)",
		// log4j2 Instant/timeMillis handling.
		"instant.@epochSecond",
		"event.@timeMillis",
		// python JSON parsing.
		"parse_json(raw_message)",
		`parse_timestamp(asctime, "%F %T,%3f")`,
		// container/file extraction from the source path.
		"parse_regex!(.file, r'^/kubedoop/log/(?P<container>.*?)/(?P<file>.*?)$')",
		"del(.source_type)",
		// stable host key.
		`host_key: "pod"`,
		// data dir matches the sidecar data volume mount.
		"data_dir: /kubedoop/vector/var",
	}
	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("RenderVectorConfig() missing stable pipeline fragment %q", check)
		}
	}
}

func TestRenderVectorConfig_ContainsSink(t *testing.T) {
	data := defaultConfigData()
	result, err := RenderVectorConfig(data)
	if err != nil {
		t.Fatalf("RenderVectorConfig() error = %v", err)
	}

	if !strings.Contains(result, "aggregator:") {
		t.Error("RenderVectorConfig() missing aggregator sink")
	}
	if !strings.Contains(result, "type: vector") {
		t.Error("RenderVectorConfig() missing vector sink type")
	}
	if !strings.Contains(result, "- extended_logs") {
		t.Error("RenderVectorConfig() aggregator sink should consume extended_logs")
	}
}

func TestRenderVectorConfig_APIDefaults(t *testing.T) {
	data := defaultConfigData()
	result, err := RenderVectorConfig(data)
	if err != nil {
		t.Fatalf("RenderVectorConfig() error = %v", err)
	}

	// Enabled, so `kubectl exec` + `vector top` still works for in-pod debugging.
	if !strings.Contains(result, "enabled: true") {
		t.Error("RenderVectorConfig() API should be enabled")
	}
	// ...but bound to loopback. Vector's API is unauthenticated GraphQL, and `vector tap` over
	// it streams the log events flowing through the pipeline — application logs, for these
	// products. On 0.0.0.0 that endpoint was reachable from anywhere in the pod network.
	if !strings.Contains(result, "address: 127.0.0.1:8686") {
		t.Error("RenderVectorConfig() API address should be 127.0.0.1:8686")
	}
	if strings.Contains(result, "0.0.0.0") {
		t.Error("RenderVectorConfig() must not bind any wildcard address")
	}
	if !strings.Contains(result, "playground: false") {
		t.Error("RenderVectorConfig() API playground should be disabled")
	}
}

func TestRenderVectorConfig_MetadataEnrichment(t *testing.T) {
	data := VectorConfigData{
		LogDir:            "/kubedoop/log/",
		AggregatorAddress: "vector-aggregator:9000",
		Namespace:         "my-namespace",
		ClusterName:       "my-cluster",
		RoleName:          "server",
		RoleGroupName:     "default",
	}
	result, err := RenderVectorConfig(data)
	if err != nil {
		t.Fatalf("RenderVectorConfig() error = %v", err)
	}

	// The stable schema stamps FLAT metadata fields (not nested under .tags).
	metadataChecks := []struct {
		key   string
		value string
	}{
		{`.namespace = "my-namespace"`, "namespace"},
		{`.cluster = "my-cluster"`, "cluster"},
		{`.role = "server"`, "role"},
		{`.roleGroup = "default"`, "roleGroup"},
	}

	for _, check := range metadataChecks {
		if !strings.Contains(result, check.key) {
			t.Errorf("RenderVectorConfig() missing metadata enrichment for %s: %q", check.value, check.key)
		}
	}
	if strings.Contains(result, ".tags.") {
		t.Error("RenderVectorConfig() must not nest metadata under .tags (stable schema is flat)")
	}
}
