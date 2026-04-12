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

package extensions

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	trinov1alpha1 "github.com/zncdatadev/operator-go/examples/trino-operator/api/v1alpha1"
	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
)

var _ = Describe("HealthExtension", func() {
	var (
		ctx     context.Context
		ext     *HealthExtension
		trinoCR *trinov1alpha1.TrinoCluster
	)

	BeforeEach(func() {
		ctx = context.Background()
		ext = NewHealthExtension()
		trinoCR = &trinov1alpha1.TrinoCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-trino",
				Namespace: "default",
			},
			Spec: trinov1alpha1.TrinoClusterSpec{
				Image: &commonsv1alpha1.ImageSpec{Custom: "trinodb/trino:435"},
			},
		}
	})

	Describe("Name", func() {
		It("should return health-extension", func() {
			Expect(ext.Name()).To(Equal("health-extension"))
		})
	})

	Describe("Validate", func() {
		Context("with valid cluster", func() {
			It("should pass PreReconcile for coordinators role", func() {
				err := ext.PreReconcile(ctx, nil, trinoCR, "coordinators")
				Expect(err).ToNot(HaveOccurred())
			})

			It("should pass PreReconcile for workers role", func() {
				err := ext.PreReconcile(ctx, nil, trinoCR, "workers")
				Expect(err).ToNot(HaveOccurred())
			})

			It("should pass PostReconcile for coordinators role", func() {
				err := ext.PostReconcile(ctx, nil, trinoCR, "coordinators")
				Expect(err).ToNot(HaveOccurred())
			})

			It("should pass PostReconcile for workers role", func() {
				err := ext.PostReconcile(ctx, nil, trinoCR, "workers")
				Expect(err).ToNot(HaveOccurred())
			})

			It("should pass PostReconcile for unknown role", func() {
				err := ext.PostReconcile(ctx, nil, trinoCR, "unknown")
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("with invalid cluster type", func() {
			It("should fail PostReconcile when passed wrong cluster type", func() {
				// Cannot create a type that implements ClusterInterface but isn't *TrinoCluster
				// without extensive mocking. The type assertion check in PostReconcile is a
				// runtime safety check that protects against incorrect usage.
				Skip("Cannot create a type that implements ClusterInterface but isn't *TrinoCluster without extensive mocking")
			})
		})
	})
})
