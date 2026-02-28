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

package webhook_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/webhook"
)

var _ = Describe("ValidationError", func() {
	Describe("Error", func() {
		It("should format error without value", func() {
			err := webhook.NewValidationError("spec.replicas", "must be positive")
			Expect(err.Error()).To(Equal("spec.replicas: must be positive"))
		})

		It("should format error with value", func() {
			err := webhook.NewValidationErrorWithValue("spec.replicas", "must be positive", -1)
			Expect(err.Error()).To(ContainSubstring("spec.replicas"))
			Expect(err.Error()).To(ContainSubstring("must be positive"))
			Expect(err.Error()).To(ContainSubstring("-1"))
		})

		It("should handle different value types", func() {
			err := webhook.NewValidationErrorWithValue("spec.name", "invalid characters", "test@name")
			Expect(err.Error()).To(ContainSubstring("test@name"))
		})
	})

	Describe("NewValidationError", func() {
		It("should create error with field and message", func() {
			err := webhook.NewValidationError("field", "message")
			Expect(err.Field).To(Equal("field"))
			Expect(err.Message).To(Equal("message"))
			Expect(err.Value).To(BeNil())
		})
	})

	Describe("NewValidationErrorWithValue", func() {
		It("should create error with field, message and value", func() {
			err := webhook.NewValidationErrorWithValue("field", "message", "value")
			Expect(err.Field).To(Equal("field"))
			Expect(err.Message).To(Equal("message"))
			Expect(err.Value).To(Equal("value"))
		})
	})
})

var _ = Describe("ValidationErrors", func() {
	Describe("Error", func() {
		It("should return 'no validation errors' for empty list", func() {
			var errs webhook.ValidationErrors
			Expect(errs.Error()).To(Equal("no validation errors"))
		})

		It("should join multiple errors with semicolon", func() {
			errs := webhook.ValidationErrors{
				webhook.NewValidationError("field1", "error1"),
				webhook.NewValidationError("field2", "error2"),
			}
			Expect(errs.Error()).To(Equal("field1: error1; field2: error2"))
		})
	})

	Describe("Add", func() {
		It("should add error to list", func() {
			var errs webhook.ValidationErrors
			errs.Add("field", "message")
			Expect(errs).To(HaveLen(1))
			Expect(errs[0].Field).To(Equal("field"))
		})
	})

	Describe("AddWithValue", func() {
		It("should add error with value to list", func() {
			var errs webhook.ValidationErrors
			errs.AddWithValue("field", "message", 42)
			Expect(errs).To(HaveLen(1))
			Expect(errs[0].Value).To(Equal(42))
		})
	})

	Describe("HasErrors", func() {
		It("should return false for empty list", func() {
			var errs webhook.ValidationErrors
			Expect(errs.HasErrors()).To(BeFalse())
		})

		It("should return true when errors exist", func() {
			errs := webhook.ValidationErrors{webhook.NewValidationError("f", "m")}
			Expect(errs.HasErrors()).To(BeTrue())
		})
	})

	Describe("ToError", func() {
		It("should return nil for empty list", func() {
			var errs webhook.ValidationErrors
			Expect(errs.ToError()).To(BeNil())
		})

		It("should return errors when not empty", func() {
			errs := webhook.ValidationErrors{webhook.NewValidationError("f", "m")}
			Expect(errs.ToError()).To(Equal(errs))
		})
	})
})

var _ = Describe("Merge", func() {
	It("should merge multiple ValidationErrors", func() {
		errs1 := webhook.ValidationErrors{
			webhook.NewValidationError("field1", "error1"),
		}
		errs2 := webhook.ValidationErrors{
			webhook.NewValidationError("field2", "error2"),
			webhook.NewValidationError("field3", "error3"),
		}

		merged := webhook.Merge(errs1, errs2)
		Expect(merged).To(HaveLen(3))
		Expect(merged[0].Field).To(Equal("field1"))
		Expect(merged[1].Field).To(Equal("field2"))
		Expect(merged[2].Field).To(Equal("field3"))
	})

	It("should handle empty lists", func() {
		var empty webhook.ValidationErrors
		errs := webhook.ValidationErrors{
			webhook.NewValidationError("field", "error"),
		}

		merged := webhook.Merge(empty, errs, empty)
		Expect(merged).To(HaveLen(1))
	})

	It("should return empty list when all inputs are empty", func() {
		merged := webhook.Merge()
		Expect(merged).To(BeEmpty())
	})
})
