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
under the License.
*/

package constant

import (
	"strings"
	"testing"
)

func TestKubedoopDomain(t *testing.T) {
	expected := "kubedoop.dev"
	if KubedoopDomain != expected {
		t.Errorf("KubedoopDomain = %s, want %s", KubedoopDomain, expected)
	}
	// Verify it's used correctly in derived constants
	if !strings.Contains(LabelEnrichmentEnable, KubedoopDomain) {
		t.Error("LabelEnrichmentEnable should contain KubedoopDomain")
	}
}

func TestKubernetesLabels(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected string
	}{
		{"LabelKubernetesComponent", LabelKubernetesComponent, "app.kubernetes.io/component"},
		{"LabelKubernetesInstance", LabelKubernetesInstance, "app.kubernetes.io/instance"},
		{"LabelKubernetesName", LabelKubernetesName, "app.kubernetes.io/name"},
		{"LabelKubernetesManagedBy", LabelKubernetesManagedBy, "app.kubernetes.io/managed-by"},
		{"LabelKubernetesRoleGroup", LabelKubernetesRoleGroup, "app.kubernetes.io/role-group"},
		{"LabelKubernetesVersion", LabelKubernetesVersion, "app.kubernetes.io/version"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != tt.expected {
				t.Errorf("%s = %s, want %s", tt.name, tt.value, tt.expected)
			}
			// All labels should start with the Kubernetes prefix
			if !strings.HasPrefix(tt.value, "app.kubernetes.io/") {
				t.Errorf("%s should start with 'app.kubernetes.io/'", tt.name)
			}
		})
	}
}

func TestMatchingLabelsNames(t *testing.T) {
	labels := MatchingLabelsNames()

	// Test 1: Should return exactly 5 labels
	if len(labels) != 5 {
		t.Fatalf("MatchingLabelsNames returned %d labels, want 5", len(labels))
	}

	// Test 2: Should contain exactly the expected labels in order
	expectedLabels := []string{
		LabelKubernetesName,
		LabelKubernetesInstance,
		LabelKubernetesRoleGroup,
		LabelKubernetesComponent,
		LabelKubernetesManagedBy,
	}

	for i, expected := range expectedLabels {
		if i >= len(labels) {
			t.Errorf("Missing label at position %d: %s", i, expected)
			continue
		}
		if labels[i] != expected {
			t.Errorf("labels[%d] = %s, want %s", i, labels[i], expected)
		}
	}

	// Test 3: Should not include LabelKubernetesVersion
	for _, label := range labels {
		if label == LabelKubernetesVersion {
			t.Error("MatchingLabelsNames should not include LabelKubernetesVersion")
		}
	}
}

func TestKubedoopPaths(t *testing.T) {
	tests := []struct {
		name          string
		value         string
		mustStartWith string
		mustEndWith   string
	}{
		{"KubedoopRoot", KubedoopRoot, "/kubedoop/", "/"},
		{"KubedoopKerberosDir", KubedoopKerberosDir, "/kubedoop/kerberos/", "/"},
		{"KubedoopTlsDir", KubedoopTlsDir, "/kubedoop/tls/", "/"},
		{"KubedoopListenerDir", KubedoopListenerDir, "/kubedoop/listener/", "/"},
		{"KubedoopJmxDir", KubedoopJmxDir, "/kubedoop/jmx/", "/"},
		{"KubedoopSecretDir", KubedoopSecretDir, "/kubedoop/secret/", "/"},
		{"KubedoopDataDir", KubedoopDataDir, "/kubedoop/data/", "/"},
		{"KubedoopConfigDir", KubedoopConfigDir, "/kubedoop/config/", "/"},
		{"KubedoopLogDir", KubedoopLogDir, "/kubedoop/log/", "/"},
		{"KubedoopConfigDirMount", KubedoopConfigDirMount, "/kubedoop/mount/config/", "/"},
		{"KubedoopLogDirMount", KubedoopLogDirMount, "/kubedoop/mount/log/", "/"},
		{"KubedoopMountDir", KubedoopMountDir, "/kubedoop/mount/", "/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test prefix
			if !strings.HasPrefix(tt.value, tt.mustStartWith) {
				t.Errorf("%s = %s, should start with %s", tt.name, tt.value, tt.mustStartWith)
			}
			// Test suffix
			if !strings.HasSuffix(tt.value, tt.mustEndWith) {
				t.Errorf("%s = %s, should end with %s", tt.name, tt.value, tt.mustEndWith)
			}
			// All paths should be absolute
			if !strings.HasPrefix(tt.value, "/") {
				t.Errorf("%s = %s, should be an absolute path", tt.name, tt.value)
			}
		})
	}
}

func TestRestarterLabels(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected string
	}{
		{"LabelRestarterEnable", LabelRestarterEnable, "restarter.kubedoop.dev/enable"},
		{"LabelRestarterEnableValue", LabelRestarterEnableValue, "true"},
		{"AnnotationSecretRestarterPrefix", AnnotationSecretRestarterPrefix, "secret.restarter.kubedoop.dev/"},
		{"AnnotationConfigMapRestarterPrefix", AnnotationConfigMapRestarterPrefix, "configmap.restarter.kubedoop.dev/"},
		{"LabelRestarterExpiresAtPrefix", LabelRestarterExpiresAtPrefix, "restarter.kubedoop.dev/expires-at."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != tt.expected {
				t.Errorf("%s = %s, want %s", tt.name, tt.value, tt.expected)
			}
			// Labels and annotations (except values) should contain the domain
			if tt.name != "LabelRestarterEnableValue" && !strings.Contains(tt.value, KubedoopDomain) {
				t.Errorf("%s should contain domain %s", tt.name, KubedoopDomain)
			}
		})
	}
}

func TestEnrichmentLabels(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected string
	}{
		{"LabelEnrichmentEnable", LabelEnrichmentEnable, "enrichment.kubedoop.dev/enable"},
		{"LabelEnrichmentEnableValue", LabelEnrichmentEnableValue, "true"},
		{"LabelEnrichmentNodeAddress", LabelEnrichmentNodeAddress, "enrichment.kubedoop.dev/node-address"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != tt.expected {
				t.Errorf("%s = %s, want %s", tt.name, tt.value, tt.expected)
			}
			// Labels (except values) should start with the enrichment prefix
			if tt.name != "LabelEnrichmentEnableValue" && !strings.HasPrefix(tt.value, "enrichment.") {
				t.Errorf("%s should start with 'enrichment.'", tt.name)
			}
		})
	}
}
