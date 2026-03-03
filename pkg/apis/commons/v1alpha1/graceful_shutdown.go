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

import (
	"time"
)

// GracefulShutdownSpec configures graceful shutdown behavior for pods.
// This controls how pods are terminated during scale down, rolling updates, or cluster stop.
type GracefulShutdownSpec struct {
	// Timeout is the termination grace period for pods.
	// This maps to terminationGracePeriodSeconds in the PodSpec.
	// +kubebuilder:validation:Pattern="^([0-9]+(\\.[0-9]+)?(ns|us|ms|s|m|h))+$"
	// +kubebuilder:default="30s"
	// +kubebuilder:validation:Optional
	Timeout string `json:"timeout,omitempty"`
}

// GetTimeout returns the timeout duration, defaulting to 30 seconds if not specified.
func (g *GracefulShutdownSpec) GetTimeout() time.Duration {
	if g == nil || g.Timeout == "" {
		return 30 * time.Second
	}
	d, err := time.ParseDuration(g.Timeout)
	if err != nil {
		return 30 * time.Second
	}
	return d
}

// GetTimeoutSeconds returns the timeout in seconds.
func (g *GracefulShutdownSpec) GetTimeoutSeconds() int64 {
	return int64(g.GetTimeout().Seconds())
}
