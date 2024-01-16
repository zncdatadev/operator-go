package util

import (
	"testing"
)

func TestDecodeBase64Byte(t *testing.T) {
	value := `aGVsbG8gd29ybGQK`
	base64Byte, err := DecodeBase64Byte([]byte(value))
	if err != nil {
		t.Errorf("DecodeBase64Byte() error = %v", err)
	}
	t.Logf("%s decode to %s", value, string(base64Byte))
}
