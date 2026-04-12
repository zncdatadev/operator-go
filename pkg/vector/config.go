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
	"text/template"
)

// VectorConfigData contains parameters for vector.yaml generation.
type VectorConfigData struct {
	// LogDir is the directory where application log files are written.
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
func RenderVectorConfig(data VectorConfigData) (string, error) {
	if data.AggregatorAddress == "" {
		return "", fmt.Errorf("AggregatorAddress is required")
	}
	if data.LogDir == "" {
		return "", fmt.Errorf("LogDir is required")
	}

	tmpl, err := template.New("vector").Funcs(template.FuncMap{
		"APIPort": func() int { return VectorAPIPort },
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
