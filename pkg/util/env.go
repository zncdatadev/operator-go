package util

import corev1 "k8s.io/api/core/v1"

func EnvsToEnvVars(envs map[string]string) []corev1.EnvVar {
	envVars := make([]corev1.EnvVar, 0, len(envs))
	for k, v := range envs {
		envVars = append(envVars, corev1.EnvVar{Name: k, Value: v})
	}
	return envVars
}
