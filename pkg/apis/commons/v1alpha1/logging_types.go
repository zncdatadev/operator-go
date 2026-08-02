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

package v1alpha1

type LoggingSpec struct {
	// +kubebuilder:validation:Optional
	Containers map[string]LoggingConfigSpec `json:"containers,omitempty"`
	// +kubebuilder:validation:Optional
	EnableVectorAgent *bool `json:"enableVectorAgent,omitempty"`
}

type LoggingConfigSpec struct {
	// +kubebuilder:validation:Optional
	Loggers map[string]*LogLevelSpec `json:"loggers,omitempty"`

	// +kubebuilder:validation:Optional
	Console *LogLevelSpec `json:"console,omitempty"`

	// +kubebuilder:validation:Optional
	File *LogLevelSpec `json:"file,omitempty"`
}

// LogLevelSpec
// level mapping if app log level is not standard
//   - FATAL -> CRITICAL
//   - ERROR -> ERROR
//   - WARN -> WARNING
//   - INFO -> INFO
//   - DEBUG -> DEBUG
//   - TRACE -> DEBUG
//
// The effective default is INFO, applied at consumption time by the renderers rather than by the
// CRD — see Level below.
type LogLevelSpec struct {
	// Level is the log threshold. Unset means "inherit": the role's value when a role group leaves
	// it out, and the product's own default (root logger INFO, no appender threshold) when nobody
	// sets it.
	//
	// It carries NO +kubebuilder:default, and must not gain one. This type sits inside `config`,
	// the block folded Role -> RoleGroup, where structural defaulting fills a leaf as soon as its
	// ENCLOSING OBJECT exists — so a default here lands in any role group that wrote `console: {}`,
	// making "unset here" indistinguishable from "explicitly INFO" and stopping the role's value
	// from ever winning. That is not hypothetical: with `default:="INFO"` a role asking for DEBUG
	// and a role group writing an empty `console: {}` produced INFO, and the guard in
	// mergeContainerLogging written to prevent exactly that could never fire, because the API
	// server had already filled the field before the merge saw it.
	//
	// Same rule and same reason as StorageResource.Capacity and RoleGroupConfigSpec's other folded
	// leaves; defaults for these live at consumption time.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=FATAL;ERROR;WARN;INFO;DEBUG;TRACE
	Level string `json:"level,omitempty"`
}
