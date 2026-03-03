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
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	databasev1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/database/v1alpha1"
	s3v1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/s3/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const dependencyTestNamespace = "default"

var _ = Describe("DependencyResolver", func() {
	var resolver *reconciler.DependencyResolver

	BeforeEach(func() {
		resolver = reconciler.NewDependencyResolver(k8sClient)
	})

	Describe("NewDependencyResolver", func() {
		It("should create a new DependencyResolver", func() {
			Expect(resolver).NotTo(BeNil())
			Expect(resolver.Client).To(Equal(k8sClient))
		})
	})

	Describe("Validate", func() {
		It("should return nil for nil spec", func() {
			err := resolver.Validate(context.Background(), nil)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return nil for valid spec without cluster operation", func() {
			spec := &commonsv1alpha1.GenericClusterSpec{}
			err := resolver.Validate(context.Background(), spec)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return nil for spec with nil cluster operation", func() {
			spec := &commonsv1alpha1.GenericClusterSpec{
				ClusterOperation: nil,
			}
			err := resolver.Validate(context.Background(), spec)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return ReconciliationPaused error when reconciliation is paused", func() {
			spec := &commonsv1alpha1.GenericClusterSpec{
				ClusterOperation: &commonsv1alpha1.ClusterOperationSpec{
					ReconciliationPaused: true,
				},
			}
			err := resolver.Validate(context.Background(), spec)
			Expect(err).To(HaveOccurred())

			var depErr *reconciler.DependencyError
			Expect(errors.As(err, &depErr)).To(BeTrue())
			Expect(depErr.Type).To(Equal("ReconciliationPaused"))
		})

		It("should return Stopped error when cluster is stopped", func() {
			spec := &commonsv1alpha1.GenericClusterSpec{
				ClusterOperation: &commonsv1alpha1.ClusterOperationSpec{
					Stopped: true,
				},
			}
			err := resolver.Validate(context.Background(), spec)
			Expect(err).To(HaveOccurred())

			var depErr *reconciler.DependencyError
			Expect(errors.As(err, &depErr)).To(BeTrue())
			Expect(depErr.Type).To(Equal("Stopped"))
		})

		It("should return nil when cluster operation is normal", func() {
			spec := &commonsv1alpha1.GenericClusterSpec{
				ClusterOperation: &commonsv1alpha1.ClusterOperationSpec{
					ReconciliationPaused: false,
					Stopped:              false,
				},
			}
			err := resolver.Validate(context.Background(), spec)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("ValidateConfigMap", func() {
		var ctx context.Context
		var namespace string

		BeforeEach(func() {
			ctx = context.Background()
			namespace = dependencyTestNamespace
		})

		It("should return error when ConfigMap does not exist", func() {
			err := resolver.ValidateConfigMap(ctx, namespace, "non-existent-configmap")
			Expect(err).To(HaveOccurred())

			var depErr *reconciler.DependencyError
			Expect(errors.As(err, &depErr)).To(BeTrue())
			Expect(depErr.Type).To(Equal("ConfigMapNotFound"))
		})

		It("should return nil when ConfigMap exists", func() {
			// Create a ConfigMap
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-configmap",
					Namespace: namespace,
				},
				Data: map[string]string{
					"key": "value",
				},
			}
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())

			err := resolver.ValidateConfigMap(ctx, namespace, "test-configmap")
			Expect(err).ToNot(HaveOccurred())

			// Cleanup
			Expect(k8sClient.Delete(ctx, cm)).To(Succeed())
		})
	})

	Describe("ValidateSecret", func() {
		var ctx context.Context
		var namespace string

		BeforeEach(func() {
			ctx = context.Background()
			namespace = dependencyTestNamespace
		})

		It("should return error when Secret does not exist", func() {
			err := resolver.ValidateSecret(ctx, namespace, "non-existent-secret")
			Expect(err).To(HaveOccurred())

			var depErr *reconciler.DependencyError
			Expect(errors.As(err, &depErr)).To(BeTrue())
			Expect(depErr.Type).To(Equal("SecretNotFound"))
		})

		It("should return nil when Secret exists", func() {
			// Create a Secret
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"password": []byte("secret-value"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			err := resolver.ValidateSecret(ctx, namespace, "test-secret")
			Expect(err).ToNot(HaveOccurred())

			// Cleanup
			Expect(k8sClient.Delete(ctx, secret)).To(Succeed())
		})
	})

	Describe("ValidateZKConfig", func() {
		It("should return nil for nil config", func() {
			err := resolver.ValidateZKConfig(context.Background(), nil)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return error for empty connection string", func() {
			err := resolver.ValidateZKConfig(context.Background(), "")
			Expect(err).To(HaveOccurred())

			var depErr *reconciler.DependencyError
			Expect(errors.As(err, &depErr)).To(BeTrue())
			Expect(depErr.Type).To(Equal("InvalidZKConfig"))
		})

		It("should return nil for valid connection string", func() {
			err := resolver.ValidateZKConfig(context.Background(), "zk1:2181,zk2:2181")
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return nil for non-string config", func() {
			err := resolver.ValidateZKConfig(context.Background(), 12345)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("ValidateS3Connection", func() {
		It("should return nil for nil config", func() {
			err := resolver.ValidateS3Connection(context.Background(), nil)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return error when host is empty", func() {
			s3Config := &s3v1alpha1.S3ConnectionSpec{
				Host: "",
			}
			err := resolver.ValidateS3Connection(context.Background(), s3Config)
			Expect(err).To(HaveOccurred())

			var depErr *reconciler.DependencyError
			Expect(errors.As(err, &depErr)).To(BeTrue())
			Expect(depErr.Type).To(Equal("InvalidS3Config"))
		})

		It("should return error when credentials is nil", func() {
			s3Config := &s3v1alpha1.S3ConnectionSpec{
				Host:        "s3.amazonaws.com",
				Credentials: nil,
			}
			err := resolver.ValidateS3Connection(context.Background(), s3Config)
			Expect(err).To(HaveOccurred())

			var depErr *reconciler.DependencyError
			Expect(errors.As(err, &depErr)).To(BeTrue())
			Expect(depErr.Type).To(Equal("InvalidS3Config"))
		})

		It("should return nil for valid S3 config", func() {
			s3Config := &s3v1alpha1.S3ConnectionSpec{
				Host: "s3.amazonaws.com",
				Credentials: &commonsv1alpha1.Credentials{
					SecretClass: "s3-credentials",
				},
			}
			err := resolver.ValidateS3Connection(context.Background(), s3Config)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return nil for non-S3ConnectionSpec config", func() {
			err := resolver.ValidateS3Connection(context.Background(), "not a valid config")
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("ValidateDatabaseConnection", func() {
		It("should return nil for nil config", func() {
			err := resolver.ValidateDatabaseConnection(context.Background(), nil)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return error when host is empty", func() {
			dbConfig := &databasev1alpha1.DatabaseConnectionSpec{
				Host: "",
			}
			err := resolver.ValidateDatabaseConnection(context.Background(), dbConfig)
			Expect(err).To(HaveOccurred())

			var depErr *reconciler.DependencyError
			Expect(errors.As(err, &depErr)).To(BeTrue())
			Expect(depErr.Type).To(Equal("InvalidDatabaseConfig"))
		})

		It("should return error when credentials secretClass is empty", func() {
			dbConfig := &databasev1alpha1.DatabaseConnectionSpec{
				Host: "mysql.default.svc.cluster.local",
				Credentials: &commonsv1alpha1.Credentials{
					SecretClass: "",
				},
			}
			err := resolver.ValidateDatabaseConnection(context.Background(), dbConfig)
			Expect(err).To(HaveOccurred())

			var depErr *reconciler.DependencyError
			Expect(errors.As(err, &depErr)).To(BeTrue())
			Expect(depErr.Type).To(Equal("InvalidDatabaseConfig"))
		})

		It("should return error when credentials is nil", func() {
			dbConfig := &databasev1alpha1.DatabaseConnectionSpec{
				Host:        "mysql.default.svc.cluster.local",
				Credentials: nil,
			}
			err := resolver.ValidateDatabaseConnection(context.Background(), dbConfig)
			Expect(err).To(HaveOccurred())

			var depErr *reconciler.DependencyError
			Expect(errors.As(err, &depErr)).To(BeTrue())
			Expect(depErr.Type).To(Equal("InvalidDatabaseConfig"))
		})

		It("should return nil for valid database config", func() {
			dbConfig := &databasev1alpha1.DatabaseConnectionSpec{
				Host: "mysql.default.svc.cluster.local",
				Credentials: &commonsv1alpha1.Credentials{
					SecretClass: "db-credentials",
				},
			}
			err := resolver.ValidateDatabaseConnection(context.Background(), dbConfig)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return nil for non-DatabaseConnectionSpec config", func() {
			err := resolver.ValidateDatabaseConnection(context.Background(), "not a valid config")
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("ValidateZKConnection", func() {
		It("should delegate to ValidateZKConfig", func() {
			err := resolver.ValidateZKConnection(context.Background(), "")
			Expect(err).To(HaveOccurred())

			err = resolver.ValidateZKConnection(context.Background(), "zk1:2181")
			Expect(err).ToNot(HaveOccurred())
		})
	})
})

var _ = Describe("DependencyError", func() {
	Describe("Error", func() {
		It("should return formatted message without cause", func() {
			err := &reconciler.DependencyError{
				Type:    "ConfigMapNotFound",
				Message: "ConfigMap default/test not found",
			}
			Expect(err.Error()).To(Equal("ConfigMapNotFound: ConfigMap default/test not found"))
		})

		It("should return formatted message with cause", func() {
			cause := errors.New("not found")
			err := &reconciler.DependencyError{
				Type:    "ConfigMapNotFound",
				Message: "ConfigMap default/test not found",
				Cause:   cause,
			}
			Expect(err.Error()).To(ContainSubstring("ConfigMapNotFound: ConfigMap default/test not found"))
			Expect(err.Error()).To(ContainSubstring("not found"))
		})
	})

	Describe("Unwrap", func() {
		It("should return nil when no cause", func() {
			err := &reconciler.DependencyError{
				Type:    "TestError",
				Message: "test message",
			}
			Expect(err.Unwrap()).To(Succeed())
		})

		It("should return the underlying cause", func() {
			cause := errors.New("underlying error")
			err := &reconciler.DependencyError{
				Type:    "TestError",
				Message: "test message",
				Cause:   cause,
			}
			Expect(err.Unwrap()).To(Equal(cause))
		})
	})
})

var _ = Describe("DependencyError helpers", func() {
	Describe("IsDependencyError", func() {
		It("should return true for DependencyError", func() {
			err := &reconciler.DependencyError{Type: "TestError"}
			Expect(reconciler.IsDependencyError(err)).To(BeTrue())
		})

		It("should return false for other errors", func() {
			err := errors.New("regular error")
			Expect(reconciler.IsDependencyError(err)).To(BeFalse())
		})
	})

	Describe("GetDependencyErrorType", func() {
		It("should return the type for DependencyError", func() {
			err := &reconciler.DependencyError{Type: "ConfigMapNotFound"}
			Expect(reconciler.GetDependencyErrorType(err)).To(Equal("ConfigMapNotFound"))
		})

		It("should return empty string for other errors", func() {
			err := errors.New("regular error")
			Expect(reconciler.GetDependencyErrorType(err)).To(Equal(""))
		})
	})

	Describe("LogDependencyError", func() {
		It("should log DependencyError with type", func() {
			err := &reconciler.DependencyError{
				Type:    "ConfigMapNotFound",
				Message: "ConfigMap not found",
			}
			reconciler.LogDependencyError(context.Background(), err)
		})

		It("should log regular error without type", func() {
			err := errors.New("regular error")
			reconciler.LogDependencyError(context.Background(), err)
		})
	})
})

var _ = Describe("ValidateEndpointFormat", func() {
	It("should return error for empty endpoint", func() {
		err := reconciler.ValidateEndpointFormat("", "testField")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("endpoint is empty"))
	})

	It("should return nil for bare hostname", func() {
		err := reconciler.ValidateEndpointFormat("localhost", "testField")
		Expect(err).ToNot(HaveOccurred())
	})

	It("should return nil for valid URL with scheme", func() {
		err := reconciler.ValidateEndpointFormat("http://localhost:8080", "testField")
		Expect(err).ToNot(HaveOccurred())
	})

	It("should return nil for valid HTTPS URL", func() {
		err := reconciler.ValidateEndpointFormat("https://example.com:443", "testField")
		Expect(err).ToNot(HaveOccurred())
	})

	It("should return nil for valid URL without port", func() {
		err := reconciler.ValidateEndpointFormat("http://example.com", "testField")
		Expect(err).ToNot(HaveOccurred())
	})

	It("should return error for invalid URL format", func() {
		err := reconciler.ValidateEndpointFormat("http://:8080", "testField")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("host is missing"))
	})
})

var _ = Describe("ParseConnectionStrings", func() {
	It("should return error for empty connection string", func() {
		_, err := reconciler.ParseConnectionStrings("")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("connection string is empty"))
	})

	It("should parse single host", func() {
		hosts, err := reconciler.ParseConnectionStrings("host1:2181")
		Expect(err).ToNot(HaveOccurred())
		Expect(hosts).To(Equal([]string{"host1:2181"}))
	})

	It("should parse multiple hosts", func() {
		hosts, err := reconciler.ParseConnectionStrings("host1:2181,host2:2181,host3:2181")
		Expect(err).ToNot(HaveOccurred())
		Expect(hosts).To(Equal([]string{"host1:2181", "host2:2181", "host3:2181"}))
	})

	It("should trim whitespace from hosts", func() {
		hosts, err := reconciler.ParseConnectionStrings("  host1:2181 , host2:2181  ,  host3:2181  ")
		Expect(err).ToNot(HaveOccurred())
		Expect(hosts).To(Equal([]string{"host1:2181", "host2:2181", "host3:2181"}))
	})

	It("should return error when no valid hosts found", func() {
		_, err := reconciler.ParseConnectionStrings("   ,  ,  ")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no valid hosts found"))
	})
})
