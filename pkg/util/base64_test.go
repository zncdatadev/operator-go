package util

import (
	"bytes"
	"testing"
)

func TestBase64EncodeWithBytes(t *testing.T) {
	value := []byte("hello world")
	base64 := Base64[[]byte]{value}
	encoded := base64.Encode()
	expected := []byte("aGVsbG8gd29ybGQ=")
	if !bytes.Equal(encoded, expected) {
		t.Errorf("Base64.Encode() = %v, want %v", encoded, expected)
	}
}

func TestBase64EncodeWithString(t *testing.T) {
	value := "hello world"
	base64 := Base64[string]{value}
	encoded := base64.Encode()
	expected := "aGVsbG8gd29ybGQ="
	if encoded != expected {
		t.Errorf("Base64.Encode() = %v, want %v", encoded, expected)
	}
}

func TestBase64DecodeWithBytes(t *testing.T) {
	value := []byte("aGVsbG8gd29ybGQ=")
	base64 := Base64[[]byte]{value}
	decoded, err := base64.Decode()
	if err != nil {
		t.Errorf("Base64.Decode() error = %v", err)
	}
	expected := []byte("hello world")
	if !bytes.Equal(decoded, expected) {
		t.Errorf("Base64.Decode() = %v, want %v", string(decoded), expected)
	}
}

func TestBase64DecodeWithString(t *testing.T) {
	value := "aGVsbG8gd29ybGQ="
	base64 := Base64[string]{value}
	decoded, err := base64.Decode()
	if err != nil {
		t.Errorf("Base64.Decode() error = %v", err)
	}
	expected := "hello world"
	if decoded != expected {
		t.Errorf("Base64.Decode() = %v, want %v", decoded, expected)
	}
}

func TestBase64DecodeWithInvalidString(t *testing.T) {
	value := "aGVsbG8gd29ybGQ"
	base64 := Base64[string]{value}
	_, err := base64.Decode()
	if err == nil {
		t.Errorf("Base64.Decode() error = %v, want %v", err, "invalid base64")
	}
}

func TestBase64DecodeWithInvalidBytes(t *testing.T) {
	value := []byte("aGVsbG8gd29ybGQ")
	base64 := Base64[[]byte]{value}
	_, err := base64.Decode()
	if err == nil {
		t.Errorf("Base64.Decode() error = %v, want %v", err, "invalid base64")
	}
}
