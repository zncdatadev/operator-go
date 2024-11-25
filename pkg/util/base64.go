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

package util

import "encoding/base64"

type Base64[T []byte | string] struct {
	Data T
}

func (b Base64[T]) Encode() T {
	switch v := any(b.Data).(type) {
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
	switch v := any(b.Data).(type) {
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
