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

package util

import corev1 "k8s.io/api/core/v1"

func NewEnvVarsFromMap(envs map[string]string) []corev1.EnvVar {
	envVars := make([]corev1.EnvVar, 0, len(envs))
	for k, v := range envs {
		envVars = append(envVars, corev1.EnvVar{Name: k, Value: v})
	}
	return envVars
}
