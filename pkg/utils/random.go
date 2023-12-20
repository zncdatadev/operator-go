package utils

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	mathRand "math/rand"
	"regexp"
)

const (
	minioSecretIDLength = 40
)

// GenerateSecretAccessKey - generate random base64 numeric value from a random seed.
func GenerateSecretAccessKey() (string, error) {
	rb := make([]byte, minioSecretIDLength)
	if _, e := rand.Read(rb); e != nil {
		return "", errors.New("could not generate Secret Key")
	}

	return string(Encode(rb)), nil
}

// Encode queues message
func Encode(value []byte) []byte {
	length := len(value)
	encoded := make([]byte, base64.URLEncoding.EncodedLen(length))
	base64.URLEncoding.Encode(encoded, value)
	return encoded
}

// GenerateRandomStr - generate random string for given length
func GenerateRandomStr(length int) string {
	letterBytes := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	bytes := make([]byte, length)
	for i := range bytes {
		bytes[i] = letterBytes[mathRand.Intn(len(letterBytes))]
	}

	return string(bytes)
}

// RemoveSpecialCharacter - remove special character
func RemoveSpecialCharacter(str string) string {
	regex := regexp.MustCompile("[^a-zA-Z0-9]+")
	result := regex.ReplaceAllString(str, "")
	return result
}
