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

package reconciler_test

import (
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
)

var _ = Describe("Errors", func() {
	Describe("ConfigError", func() {
		Describe("NewConfigError", func() {
			It("should create a ConfigError with field and message", func() {
				err := reconciler.NewConfigError("spec.replicas", "must be positive")
				Expect(err).NotTo(BeNil())
				Expect(err.Field).To(Equal("spec.replicas"))
				Expect(err.Message).To(Equal("must be positive"))
			})
		})

		Describe("Error", func() {
			It("should return formatted error message", func() {
				err := reconciler.NewConfigError("spec.replicas", "must be positive")
				Expect(err.Error()).To(Equal(`config error in field "spec.replicas": must be positive`))
			})
		})
	})

	Describe("ReconcileError", func() {
		Describe("NewReconcileError", func() {
			It("should create a ReconcileError without cause", func() {
				err := reconciler.NewReconcileError("PreReconcile", "extension hook failed", nil)
				Expect(err).NotTo(BeNil())
				Expect(err.Phase).To(Equal("PreReconcile"))
				Expect(err.Message).To(Equal("extension hook failed"))
				Expect(err.Cause).To(BeNil())
			})

			It("should create a ReconcileError with cause", func() {
				cause := errors.New("underlying error")
				err := reconciler.NewReconcileError("DependencyValidation", "dependency validation failed", cause)
				Expect(err).NotTo(BeNil())
				Expect(err.Phase).To(Equal("DependencyValidation"))
				Expect(err.Message).To(Equal("dependency validation failed"))
				Expect(err.Cause).To(Equal(cause))
			})
		})

		Describe("Error", func() {
			It("should return formatted error message without cause", func() {
				err := reconciler.NewReconcileError("PreReconcile", "extension hook failed", nil)
				Expect(err.Error()).To(Equal(`reconcile error in phase "PreReconcile": extension hook failed`))
			})

			It("should return formatted error message with cause", func() {
				cause := errors.New("underlying error")
				err := reconciler.NewReconcileError("DependencyValidation", "dependency validation failed", cause)
				Expect(err.Error()).To(ContainSubstring("reconcile error in phase \"DependencyValidation\""))
				Expect(err.Error()).To(ContainSubstring("dependency validation failed"))
				Expect(err.Error()).To(ContainSubstring("underlying error"))
			})
		})

		Describe("Unwrap", func() {
			It("should return nil when no cause", func() {
				err := reconciler.NewReconcileError("PreReconcile", "extension hook failed", nil)
				Expect(err.Unwrap()).To(BeNil())
			})

			It("should return the underlying cause", func() {
				cause := errors.New("underlying error")
				err := reconciler.NewReconcileError("DependencyValidation", "dependency validation failed", cause)
				Expect(err.Unwrap()).To(Equal(cause))
			})
		})
	})

	Describe("ResourceBuildError", func() {
		Describe("NewResourceBuildError", func() {
			It("should create a ResourceBuildError without cause", func() {
				err := reconciler.NewResourceBuildError("StatefulSet", "namenode", "default", "failed to build", nil)
				Expect(err).NotTo(BeNil())
				Expect(err.ResourceType).To(Equal("StatefulSet"))
				Expect(err.RoleName).To(Equal("namenode"))
				Expect(err.GroupName).To(Equal("default"))
				Expect(err.Message).To(Equal("failed to build"))
				Expect(err.Cause).To(BeNil())
			})

			It("should create a ResourceBuildError with cause", func() {
				cause := errors.New("invalid configuration")
				err := reconciler.NewResourceBuildError("ConfigMap", "datanode", "worker", "invalid config", cause)
				Expect(err).NotTo(BeNil())
				Expect(err.Cause).To(Equal(cause))
			})
		})

		Describe("Error", func() {
			It("should return formatted error message without cause", func() {
				err := reconciler.NewResourceBuildError("StatefulSet", "namenode", "default", "failed to build", nil)
				Expect(err.Error()).To(Equal(`failed to build StatefulSet for role "namenode" group "default": failed to build`))
			})

			It("should return formatted error message with cause", func() {
				cause := errors.New("invalid configuration")
				err := reconciler.NewResourceBuildError("ConfigMap", "datanode", "worker", "invalid config", cause)
				Expect(err.Error()).To(ContainSubstring("failed to build ConfigMap"))
				Expect(err.Error()).To(ContainSubstring("datanode"))
				Expect(err.Error()).To(ContainSubstring("worker"))
				Expect(err.Error()).To(ContainSubstring("invalid config"))
				Expect(err.Error()).To(ContainSubstring("invalid configuration"))
			})
		})

		Describe("Unwrap", func() {
			It("should return nil when no cause", func() {
				err := reconciler.NewResourceBuildError("StatefulSet", "namenode", "default", "failed to build", nil)
				Expect(err.Unwrap()).To(BeNil())
			})

			It("should return the underlying cause", func() {
				cause := errors.New("invalid configuration")
				err := reconciler.NewResourceBuildError("ConfigMap", "datanode", "worker", "invalid config", cause)
				Expect(err.Unwrap()).To(Equal(cause))
			})
		})
	})

	Describe("ResourceApplyError", func() {
		Describe("NewResourceApplyError", func() {
			It("should create a ResourceApplyError without cause", func() {
				err := reconciler.NewResourceApplyError("StatefulSet", "default", "test-cluster", "failed to apply", nil)
				Expect(err).NotTo(BeNil())
				Expect(err.ResourceType).To(Equal("StatefulSet"))
				Expect(err.Namespace).To(Equal("default"))
				Expect(err.ResourceName).To(Equal("test-cluster"))
				Expect(err.Message).To(Equal("failed to apply"))
				Expect(err.Cause).To(BeNil())
			})

			It("should create a ResourceApplyError with cause", func() {
				cause := errors.New("connection refused")
				err := reconciler.NewResourceApplyError("Service", "production", "my-service", "API server error", cause)
				Expect(err).NotTo(BeNil())
				Expect(err.Cause).To(Equal(cause))
			})
		})

		Describe("Error", func() {
			It("should return formatted error message without cause", func() {
				err := reconciler.NewResourceApplyError("StatefulSet", "default", "test-cluster", "failed to apply", nil)
				Expect(err.Error()).To(Equal("failed to apply StatefulSet default/test-cluster: failed to apply"))
			})

			It("should return formatted error message with cause", func() {
				cause := errors.New("connection refused")
				err := reconciler.NewResourceApplyError("Service", "production", "my-service", "API server error", cause)
				Expect(err.Error()).To(ContainSubstring("failed to apply Service production/my-service"))
				Expect(err.Error()).To(ContainSubstring("API server error"))
				Expect(err.Error()).To(ContainSubstring("connection refused"))
			})
		})

		Describe("Unwrap", func() {
			It("should return nil when no cause", func() {
				err := reconciler.NewResourceApplyError("StatefulSet", "default", "test-cluster", "failed to apply", nil)
				Expect(err.Unwrap()).To(BeNil())
			})

			It("should return the underlying cause", func() {
				cause := errors.New("connection refused")
				err := reconciler.NewResourceApplyError("Service", "production", "my-service", "API server error", cause)
				Expect(err.Unwrap()).To(Equal(cause))
			})
		})
	})

	Describe("Error type checking functions", func() {
		Describe("IsReconcileError", func() {
			It("should return true for ReconcileError", func() {
				err := reconciler.NewReconcileError("PreReconcile", "test", nil)
				Expect(reconciler.IsReconcileError(err)).To(BeTrue())
			})

			It("should return false for other errors", func() {
				err := errors.New("regular error")
				Expect(reconciler.IsReconcileError(err)).To(BeFalse())
			})

			It("should return false for ConfigError", func() {
				err := reconciler.NewConfigError("field", "message")
				Expect(reconciler.IsReconcileError(err)).To(BeFalse())
			})
		})

		Describe("IsConfigError", func() {
			It("should return true for ConfigError", func() {
				err := reconciler.NewConfigError("field", "message")
				Expect(reconciler.IsConfigError(err)).To(BeTrue())
			})

			It("should return false for other errors", func() {
				err := errors.New("regular error")
				Expect(reconciler.IsConfigError(err)).To(BeFalse())
			})

			It("should return false for ReconcileError", func() {
				err := reconciler.NewReconcileError("phase", "message", nil)
				Expect(reconciler.IsConfigError(err)).To(BeFalse())
			})
		})

		Describe("IsResourceBuildError", func() {
			It("should return true for ResourceBuildError", func() {
				err := reconciler.NewResourceBuildError("StatefulSet", "role", "group", "message", nil)
				Expect(reconciler.IsResourceBuildError(err)).To(BeTrue())
			})

			It("should return false for other errors", func() {
				err := errors.New("regular error")
				Expect(reconciler.IsResourceBuildError(err)).To(BeFalse())
			})

			It("should return false for ResourceApplyError", func() {
				err := reconciler.NewResourceApplyError("StatefulSet", "ns", "name", "message", nil)
				Expect(reconciler.IsResourceBuildError(err)).To(BeFalse())
			})
		})

		Describe("IsResourceApplyError", func() {
			It("should return true for ResourceApplyError", func() {
				err := reconciler.NewResourceApplyError("StatefulSet", "ns", "name", "message", nil)
				Expect(reconciler.IsResourceApplyError(err)).To(BeTrue())
			})

			It("should return false for other errors", func() {
				err := errors.New("regular error")
				Expect(reconciler.IsResourceApplyError(err)).To(BeFalse())
			})

			It("should return false for ResourceBuildError", func() {
				err := reconciler.NewResourceBuildError("StatefulSet", "role", "group", "message", nil)
				Expect(reconciler.IsResourceApplyError(err)).To(BeFalse())
			})
		})
	})

	Describe("Error wrapping with errors.Is and errors.As", func() {
		It("should support errors.As for ReconcileError", func() {
			cause := errors.New("underlying error")
			err := reconciler.NewReconcileError("phase", "message", cause)

			var reconcileErr *reconciler.ReconcileError
			Expect(errors.As(err, &reconcileErr)).To(BeTrue())
			Expect(reconcileErr.Phase).To(Equal("phase"))
		})

		It("should support errors.As for ResourceBuildError", func() {
			cause := errors.New("underlying error")
			err := reconciler.NewResourceBuildError("StatefulSet", "role", "group", "message", cause)

			var buildErr *reconciler.ResourceBuildError
			Expect(errors.As(err, &buildErr)).To(BeTrue())
			Expect(buildErr.ResourceType).To(Equal("StatefulSet"))
		})

		It("should support errors.As for ResourceApplyError", func() {
			cause := errors.New("underlying error")
			err := reconciler.NewResourceApplyError("Service", "ns", "name", "message", cause)

			var applyErr *reconciler.ResourceApplyError
			Expect(errors.As(err, &applyErr)).To(BeTrue())
			Expect(applyErr.ResourceType).To(Equal("Service"))
		})

		It("should support errors.Is for wrapped errors", func() {
			cause := errors.New("underlying error")
			err := reconciler.NewReconcileError("phase", "message", cause)
			Expect(errors.Is(err, cause)).To(BeTrue())
		})

		It("should support fmt.Errorf wrapping", func() {
			originalErr := reconciler.NewConfigError("field", "invalid value")
			wrappedErr := fmt.Errorf("failed to validate: %w", originalErr)

			var configErr *reconciler.ConfigError
			Expect(errors.As(wrappedErr, &configErr)).To(BeTrue())
			Expect(configErr.Field).To(Equal("field"))
		})
	})
})
