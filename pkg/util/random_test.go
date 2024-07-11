package util

import (
	"testing"
)

func TestGenerateRandomStr(t *testing.T) {
	length := 10
	useLetters := true
	useSpecial := true
	useNum := true

	randomStr := GenerateRandomStr(length, useLetters, useSpecial, useNum)

	if len(randomStr) != length {
		t.Errorf("Generated random string length is incorrect. Expected: %d, Got: %d", length, len(randomStr))
	}

	// Add additional assertions here if needed

	t.Logf("Generated random string: %s", randomStr)
}

func TestGenerateSimplePassword(t *testing.T) {
	length := 8
	password := GenerateSimplePassword(length)

	if len(password) != length {
		t.Errorf("Generated password length is incorrect. Expected: %d, Got: %d", length, len(password))
	}

	// Add additional assertions here if needed

	t.Logf("Generated password: %s", password)
}
