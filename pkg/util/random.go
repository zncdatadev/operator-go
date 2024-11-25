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

import (
	"math/rand"
	"regexp"
)

const (
	letterBytes  = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	specialBytes = "!@#$%^&*()_+-=[]{}\\|;':\",.<>/?`~"
	numBytes     = "0123456789"
)

// GenerateRandomStr generates a random string of the specified length.
// The string can contain letters, special characters, and numbers based on the flags provided.
func GenerateRandomStr(length int, useLetters bool, useSpecial bool, useNum bool) string {
	bytes := make([]byte, length)
	for i := range bytes {
		if useLetters {
			bytes[i] = letterBytes[rand.Intn(len(letterBytes))]
		} else if useSpecial {
			bytes[i] = specialBytes[rand.Intn(len(specialBytes))]
		} else if useNum {
			bytes[i] = numBytes[rand.Intn(len(numBytes))]
		}
	}
	return string(bytes)
}

// GenerateSimplePassword generates a simple password of the specified length.
// The password will contain only letters and numbers.
func GenerateSimplePassword(length int) string {
	return GenerateRandomStr(length, true, false, true)
}

// RemoveSpecialCharacter - remove special character
func RemoveSpecialCharacter(str string) string {
	regex := regexp.MustCompile("[^a-zA-Z0-9]+")
	result := regex.ReplaceAllString(str, "")
	return result
}
