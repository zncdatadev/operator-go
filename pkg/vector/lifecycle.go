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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// defaultReadinessProbe returns a readiness probe for the Vector container.
// It performs an HTTP GET on the Vector API health endpoint.
func defaultReadinessProbe() *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: VectorHealthEndpoint,
				Port: intstr.FromInt(VectorAPIPort),
			},
		},
		InitialDelaySeconds: VectorReadinessInitialDelaySeconds,
		PeriodSeconds:       VectorReadinessPeriodSeconds,
	}
}
