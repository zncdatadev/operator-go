package util

import "encoding/base64"

type Base64[T []byte | string] struct {
	data T
}

func (b Base64[T]) Encode() T {
	switch v := any(b.data).(type) {
	case []byte:
		str := base64.StdEncoding.EncodeToString(v)
		return T(str)
	case string:
		str := base64.StdEncoding.EncodeToString([]byte(v))
		return T(str)
	default:
		panic("unsupported data type")
	}
}

func (b Base64[T]) Decode() (T, error) {
	switch v := any(b.data).(type) {
	case []byte:
		bytes, err := base64.StdEncoding.DecodeString(string(v))
		if err != nil {
			var zero T
			return zero, err
		}
		return T(bytes), nil
	case string:
		bytes, err := base64.StdEncoding.DecodeString(v)
		if err != nil {
			var zero T
			return zero, err
		}
		return T(bytes), nil
	default:
		panic("unsupported data type")
	}
}
