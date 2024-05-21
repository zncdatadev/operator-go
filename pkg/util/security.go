package util

import (
	"encoding/base64"
	"github.com/zncdatadev/operator-go/pkg/errors"
	corev1 "k8s.io/api/core/v1"
)

// GetSecurityValueByKey get security value from secret by key
func GetSecurityValueByKey(secret *corev1.Secret, key string) (string, error) {
	if value, ok := secret.Data[key]; ok {
		base64Byte, err := DecodeBase64Byte(value)
		if err != nil {
			return "", err
		}
		return string(base64Byte), err
	}
	return "", errors.New(errors.SECURITY_KEY_NOT_FOUND)
}

func DecodeBase64Byte(value []byte) ([]byte, error) {

	dst := make([]byte, base64.StdEncoding.DecodedLen(len(value)))
	n, err := base64.StdEncoding.Decode(dst, value)
	if err != nil {
		return nil, err
	}
	dst = dst[:n]
	return dst, err
}
