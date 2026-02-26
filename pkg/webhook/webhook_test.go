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
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/webhook"
)

// Test CR type for testing
type TestCR struct {
	Name  string
	Value int
}

var _ = Describe("WebhookManager", func() {
	var manager *webhook.WebhookManager[*TestCR]

	BeforeEach(func() {
		manager = webhook.NewWebhookManager[*TestCR]()
	})

	Describe("NewWebhookManager", func() {
		It("should create a new manager", func() {
			Expect(manager).NotTo(BeNil())
		})
	})

	Describe("WithDefaulter", func() {
		It("should set a defaulter", func() {
			defaulter := webhook.NewFuncDefaulter[*TestCR](func(ctx context.Context, cr *TestCR) error {
				cr.Name = "default"
				return nil
			})
			manager.WithDefaulter(defaulter)
			Expect(manager.HasDefaulter()).To(BeTrue())
		})

		It("should not set nil defaulter", func() {
			manager.WithDefaulter(nil)
			Expect(manager.HasDefaulter()).To(BeFalse())
		})
	})

	Describe("WithValidator", func() {
		It("should set a validator", func() {
			validator := webhook.NewFuncValidator[*TestCR](func(ctx context.Context, cr *TestCR) error {
				return nil
			})
			manager.WithValidator(validator)
			Expect(manager.HasValidator()).To(BeTrue())
		})

		It("should not set nil validator", func() {
			manager.WithValidator(nil)
			Expect(manager.HasValidator()).To(BeFalse())
		})
	})

	Describe("ApplyDefaults", func() {
		It("should apply defaults to CR", func() {
			defaulter := webhook.NewFuncDefaulter[*TestCR](func(ctx context.Context, cr *TestCR) error {
				if cr.Name == "" {
					cr.Name = "default-name"
				}
				return nil
			})
			manager.WithDefaulter(defaulter)

			cr := &TestCR{}
			err := manager.ApplyDefaults(context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(cr.Name).To(Equal("default-name"))
		})

		It("should return error when defaulter fails", func() {
			defaulter := webhook.NewFuncDefaulter[*TestCR](func(ctx context.Context, cr *TestCR) error {
				return errors.New("default error")
			})
			manager.WithDefaulter(defaulter)

			cr := &TestCR{}
			err := manager.ApplyDefaults(context.Background(), cr)
			Expect(err).To(HaveOccurred())
		})

		It("should not fail with nil defaulter", func() {
			cr := &TestCR{}
			err := manager.ApplyDefaults(context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Validate", func() {
		It("should validate CR successfully", func() {
			validator := webhook.NewFuncValidator[*TestCR](func(ctx context.Context, cr *TestCR) error {
				if cr.Value < 0 {
					return errors.New("value must be non-negative")
				}
				return nil
			})
			manager.WithValidator(validator)

			cr := &TestCR{Value: 10}
			err := manager.Validate(context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error when validation fails", func() {
			validator := webhook.NewFuncValidator[*TestCR](func(ctx context.Context, cr *TestCR) error {
				if cr.Value < 0 {
					return errors.New("value must be non-negative")
				}
				return nil
			})
			manager.WithValidator(validator)

			cr := &TestCR{Value: -1}
			err := manager.Validate(context.Background(), cr)
			Expect(err).To(HaveOccurred())
		})

		It("should not fail with nil validator", func() {
			cr := &TestCR{}
			err := manager.Validate(context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("HasDefaulter", func() {
		It("should return false for NoOpDefaulter", func() {
			Expect(manager.HasDefaulter()).To(BeFalse())
		})

		It("should return true for custom defaulter", func() {
			manager.WithDefaulter(webhook.NewFuncDefaulter[*TestCR](nil))
			Expect(manager.HasDefaulter()).To(BeTrue())
		})
	})

	Describe("HasValidator", func() {
		It("should return false for NoOpValidator", func() {
			Expect(manager.HasValidator()).To(BeFalse())
		})

		It("should return true for custom validator", func() {
			manager.WithValidator(webhook.NewFuncValidator[*TestCR](nil))
			Expect(manager.HasValidator()).To(BeTrue())
		})
	})
})

var _ = Describe("NoOpDefaulter", func() {
	Describe("SetDefaults", func() {
		It("should do nothing and return nil", func() {
			defaulter := webhook.NewNoOpDefaulter[*TestCR]()
			cr := &TestCR{Name: "test"}
			err := defaulter.SetDefaults(context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(cr.Name).To(Equal("test"))
		})
	})
})

var _ = Describe("FuncDefaulter", func() {
	Describe("SetDefaults", func() {
		It("should call the wrapped function", func() {
			called := false
			defaulter := webhook.NewFuncDefaulter[*TestCR](func(ctx context.Context, cr *TestCR) error {
				called = true
				return nil
			})
			cr := &TestCR{}
			err := defaulter.SetDefaults(context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(called).To(BeTrue())
		})

		It("should return nil when function is nil", func() {
			defaulter := webhook.NewFuncDefaulter[*TestCR](nil)
			cr := &TestCR{}
			err := defaulter.SetDefaults(context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

var _ = Describe("NoOpValidator", func() {
	Describe("Validate", func() {
		It("should do nothing and return nil", func() {
			validator := webhook.NewNoOpValidator[*TestCR]()
			cr := &TestCR{Value: -1}
			err := validator.Validate(context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

var _ = Describe("FuncValidator", func() {
	Describe("Validate", func() {
		It("should call the wrapped function", func() {
			called := false
			validator := webhook.NewFuncValidator[*TestCR](func(ctx context.Context, cr *TestCR) error {
				called = true
				return nil
			})
			cr := &TestCR{}
			err := validator.Validate(context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(called).To(BeTrue())
		})

		It("should return nil when function is nil", func() {
			validator := webhook.NewFuncValidator[*TestCR](nil)
			cr := &TestCR{}
			err := validator.Validate(context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

var _ = Describe("ValidateFieldLength", func() {
	It("should pass for valid length", func() {
		err := webhook.ValidateFieldLength("test", "name", 1, 10)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should fail for too short", func() {
		err := webhook.ValidateFieldLength("", "name", 1, 10)
		Expect(err).To(HaveOccurred())
	})

	It("should fail for too long", func() {
		err := webhook.ValidateFieldLength("this is too long", "name", 1, 5)
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("ValidateNonEmptyMap", func() {
	It("should pass for non-empty map", func() {
		m := map[string]string{"key": "value"}
		err := webhook.ValidateNonEmptyMap(m, "labels")
		Expect(err).NotTo(HaveOccurred())
	})

	It("should fail for empty map", func() {
		m := map[string]string{}
		err := webhook.ValidateNonEmptyMap(m, "labels")
		Expect(err).To(HaveOccurred())
	})
})
