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
		{Name: "KEY1", Value: "VALUE1"},
		{Name: "KEY2", Value: "VALUE2"},
	}

	envVars := NewEnvVarsFromMap(envs)

	if len(envVars) != len(expectedEnvVars) {
		t.Errorf("Expected %d env vars, but got %d", len(expectedEnvVars), len(envVars))
	}

	assert.Equal(t, expectedEnvVars, envVars)
}
