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
// Default log level is INFO
type LogLevelSpec struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:="INFO"
	// +kubebuilder:validation:Enum=FATAL;ERROR;WARN;INFO;DEBUG;TRACE
	Level string `json:"level,omitempty"`
}
