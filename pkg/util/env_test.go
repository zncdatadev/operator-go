package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestNewEnvVarsFromMap(t *testing.T) {
	envs := map[string]string{
		"KEY1": "VALUE1",
		"KEY2": "VALUE2",
	}

	expectedEnvVars := []corev1.EnvVar{
		{Name: "KEY2", Value: "VALUE2"},
		{Name: "KEY1", Value: "VALUE1"},
	}

	envVars := NewEnvVarsFromMap(envs)

	assert.Len(t, expectedEnvVars, len(envVars))

	assert.ElementsMatch(t, expectedEnvVars, envVars)
}
