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
)

func defaultConfigData() VectorConfigData {
	return VectorConfigData{
		LogDir:            "/var/log/app",
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
			contains: []string{"/var/log/app", "vector-aggregator:9000", "default", "test-cluster", "worker", "default"},
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
				d.LogDir = "/custom/logs"
				return d
			}(),
			contains: []string{"/custom/logs"},
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
}

func TestRenderVectorConfig_ContainsAllTransforms(t *testing.T) {
	data := defaultConfigData()
	result, err := RenderVectorConfig(data)
	if err != nil {
		t.Fatalf("RenderVectorConfig() error = %v", err)
	}

	expectedTransforms := []string{
		"parse_stdout",
		"parse_stderr",
		"parse_log4j",
		"parse_log4j2",
		"parse_py",
		"parse_airlift",
		"enrich_metadata",
	}

	for _, transform := range expectedTransforms {
		if !strings.Contains(result, transform+":") {
			t.Errorf("RenderVectorConfig() missing transform %q", transform)
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
	if !strings.Contains(result, `type: "vector"`) {
		t.Error("RenderVectorConfig() missing vector sink type")
	}
}

func TestRenderVectorConfig_APIDefaults(t *testing.T) {
	data := defaultConfigData()
	result, err := RenderVectorConfig(data)
	if err != nil {
		t.Fatalf("RenderVectorConfig() error = %v", err)
	}

	if !strings.Contains(result, "enabled: true") {
		t.Error("RenderVectorConfig() API should be enabled")
	}
	if !strings.Contains(result, "127.0.0.1:8686") {
		t.Error("RenderVectorConfig() API address should be 127.0.0.1:8686")
	}
}

func TestRenderVectorConfig_MetadataEnrichment(t *testing.T) {
	data := VectorConfigData{
		LogDir:            "/var/log/app",
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

	metadataChecks := []struct {
		key   string
		value string
	}{
		{`tags.namespace = "my-namespace"`, "namespace"},
		{`tags.cluster = "my-cluster"`, "cluster"},
		{`tags.role = "server"`, "role"},
		{`tags.role_group = "default"`, "role_group"},
	}

	for _, check := range metadataChecks {
		if !strings.Contains(result, check.key) {
			t.Errorf("RenderVectorConfig() missing metadata enrichment for %s: %q", check.value, check.key)
		}
	}
}
