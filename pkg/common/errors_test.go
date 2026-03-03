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

package common_test

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/common"
)

var _ = Describe("Errors", func() {
	Describe("ConfigValidationError", func() {
		It("should create error with field and wrapped error", func() {
			innerErr := errors.New("invalid value")
			err := common.ConfigValidationError("replicas", innerErr)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("configuration validation failed"))
			Expect(err.Error()).To(ContainSubstring("replicas"))
			Expect(err.Error()).To(ContainSubstring("invalid value"))
		})

		It("should wrap the original error", func() {
			innerErr := errors.New("inner error")
			err := common.ConfigValidationError("field", innerErr)

			Expect(errors.Unwrap(err)).To(Equal(innerErr))
		})
	})

	Describe("ResourceNotFoundError", func() {
		It("should create error with resource details", func() {
			innerErr := errors.New("not found")
			err := common.ResourceNotFoundError("StatefulSet", "default", "my-sts", innerErr)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("StatefulSet"))
			Expect(err.Error()).To(ContainSubstring("default/my-sts"))
			Expect(err.Error()).To(ContainSubstring("not found"))
		})

		It("should wrap the original error", func() {
			innerErr := errors.New("inner error")
			err := common.ResourceNotFoundError("Pod", "ns", "name", innerErr)

			Expect(errors.Unwrap(err)).To(Equal(innerErr))
		})
	})

	Describe("CreateResourceError", func() {
		It("should create error with resource details", func() {
			innerErr := errors.New("connection refused")
			err := common.CreateResourceError("ConfigMap", "kube-system", "my-config", innerErr)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create"))
			Expect(err.Error()).To(ContainSubstring("ConfigMap"))
			Expect(err.Error()).To(ContainSubstring("kube-system/my-config"))
			Expect(err.Error()).To(ContainSubstring("connection refused"))
		})

		It("should wrap the original error", func() {
			innerErr := errors.New("inner error")
			err := common.CreateResourceError("Service", "ns", "name", innerErr)

			Expect(errors.Unwrap(err)).To(Equal(innerErr))
		})
	})

	Describe("ConfigMergeError", func() {
		It("should create error with context", func() {
			innerErr := errors.New("type mismatch")
			err := common.ConfigMergeError("role-group-override", innerErr)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("configuration merge failed"))
			Expect(err.Error()).To(ContainSubstring("role-group-override"))
			Expect(err.Error()).To(ContainSubstring("type mismatch"))
		})

		It("should wrap the original error", func() {
			innerErr := errors.New("inner error")
			err := common.ConfigMergeError("context", innerErr)

			Expect(errors.Unwrap(err)).To(Equal(innerErr))
		})
	})

	Describe("ConfigParseError", func() {
		It("should create error with format", func() {
			innerErr := errors.New("unexpected character")
			err := common.ConfigParseError("yaml", innerErr)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse"))
			Expect(err.Error()).To(ContainSubstring("yaml"))
			Expect(err.Error()).To(ContainSubstring("unexpected character"))
		})

		It("should wrap the original error", func() {
			innerErr := errors.New("inner error")
			err := common.ConfigParseError("xml", innerErr)

			Expect(errors.Unwrap(err)).To(Equal(innerErr))
		})
	})

	Describe("Error composition", func() {
		It("should allow chaining errors", func() {
			baseErr := errors.New("base error")
			parseErr := common.ConfigParseError("json", baseErr)
			mergeErr := common.ConfigMergeError("config-merge", parseErr)

			// Should contain all error messages
			Expect(mergeErr.Error()).To(ContainSubstring("config-merge"))
			Expect(mergeErr.Error()).To(ContainSubstring("json"))

			// Should be able to unwrap to the parse error
			unwrapped := errors.Unwrap(mergeErr)
			Expect(unwrapped).To(Equal(parseErr))
		})

		It("should work with errors.Is and errors.As", func() {
			customErr := &customError{msg: "custom error"}
			wrappedErr := common.ConfigValidationError("field", customErr)

			var target *customError
			Expect(errors.As(wrappedErr, &target)).To(BeTrue())
		})
	})
})

// customError is a test error type for testing errors.As
type customError struct {
	msg string
}

func (e *customError) Error() string {
	return e.msg
}

var _ error = &customError{}
