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

package controller

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	trinov1alpha1 "github.com/zncdatadev/operator-go/examples/trino-operator/api/v1alpha1"
	"github.com/zncdatadev/operator-go/examples/trino-operator/internal/constants"
	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
)

var _ = Describe("TrinoRoleGroupHandler", func() {
	var (
		handler *TrinoRoleGroupHandler
		ctx     context.Context
	)

	BeforeEach(func() {
		handler = NewTrinoRoleGroupHandler()
		ctx = context.Background()
	})

	Describe("NewTrinoRoleGroupHandler", func() {
		It("should create a new handler successfully", func() {
			handler := NewTrinoRoleGroupHandler()
			Expect(handler).NotTo(BeNil())
			Expect(handler.coordinatorsHandler).NotTo(BeNil())
			Expect(handler.workersHandler).NotTo(BeNil())
		})

		It("should return a new instance each time", func() {
			handler1 := NewTrinoRoleGroupHandler()
			handler2 := NewTrinoRoleGroupHandler()
			Expect(handler1).NotTo(BeIdenticalTo(handler2))
		})
	})

	Describe("BuildResources", func() {
		var (
			cr       *trinov1alpha1.TrinoCluster
			buildCtx *reconciler.RoleGroupBuildContext
		)

		BeforeEach(func() {
			cr = &trinov1alpha1.TrinoCluster{
				Spec: trinov1alpha1.TrinoClusterSpec{
					Image: &commonsv1alpha1.ImageSpec{Custom: "trinodb/trino:435"},
					Coordinators: &trinov1alpha1.CoordinatorsSpec{
						RoleSpec: commonsv1alpha1.RoleSpec{
							RoleGroups: map[string]commonsv1alpha1.RoleGroupSpec{
								"default": {},
							},
						},
					},
					Workers: &trinov1alpha1.WorkersSpec{
						RoleSpec: commonsv1alpha1.RoleSpec{
							RoleGroups: map[string]commonsv1alpha1.RoleGroupSpec{
								"default": {},
							},
						},
					},
				},
			}

			buildCtx = &reconciler.RoleGroupBuildContext{
				ClusterName:      "test-trino",
				ClusterNamespace: "default",
				ClusterLabels:    map[string]string{"app": "trino"},
				ResourceName:     "test-trino-coordinators-default",
			}
		})

		Context("when role is coordinators", func() {
			BeforeEach(func() {
				buildCtx.RoleName = RoleCoordinators
				buildCtx.RoleGroupName = constants.DefaultRoleGroupName
			})

			It("should route to coordinators handler and return resources", func() {
				resources, err := handler.BuildResources(ctx, k8sClient, cr, buildCtx)

				Expect(err).NotTo(HaveOccurred())
				Expect(resources).NotTo(BeNil())
				Expect(resources.ConfigMap).NotTo(BeNil())
				Expect(resources.HeadlessService).NotTo(BeNil())
				Expect(resources.Service).NotTo(BeNil())
				Expect(resources.StatefulSet).NotTo(BeNil())
			})
		})

		Context("when role is workers", func() {
			BeforeEach(func() {
				buildCtx.RoleName = RoleWorkers
				buildCtx.RoleGroupName = constants.DefaultRoleGroupName
				buildCtx.ResourceName = "test-trino-workers-default"
			})

			It("should route to workers handler and return resources", func() {
				resources, err := handler.BuildResources(ctx, k8sClient, cr, buildCtx)

				Expect(err).NotTo(HaveOccurred())
				Expect(resources).NotTo(BeNil())
				Expect(resources.ConfigMap).NotTo(BeNil())
				Expect(resources.HeadlessService).NotTo(BeNil())
				Expect(resources.StatefulSet).NotTo(BeNil())
			})
		})

		Context("when role is unknown", func() {
			BeforeEach(func() {
				buildCtx.RoleName = "unknown-role"
				buildCtx.RoleGroupName = constants.DefaultRoleGroupName
			})

			It("should return an error for unknown role", func() {
				resources, err := handler.BuildResources(ctx, k8sClient, cr, buildCtx)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unknown role"))
				Expect(err.Error()).To(ContainSubstring("unknown-role"))
				Expect(resources).To(BeNil())
			})
		})

		DescribeTable("routing based on role name",
			func(roleName string, expectError bool) {
				buildCtx.RoleName = roleName
				buildCtx.RoleGroupName = constants.DefaultRoleGroupName

				resources, err := handler.BuildResources(ctx, k8sClient, cr, buildCtx)

				if expectError {
					Expect(err).To(HaveOccurred())
					Expect(resources).To(BeNil())
				} else {
					Expect(err).NotTo(HaveOccurred())
					Expect(resources).NotTo(BeNil())
				}
			},
			Entry("coordinators role", RoleCoordinators, false),
			Entry("workers role", RoleWorkers, false),
			Entry("unknown role", "unknown", true),
			Entry("empty role", "", true),
			Entry("invalid role", "invalid-role", true),
		)
	})

})

var _ = Describe("TrinoRoleGroupHandler Interface Compliance", func() {
	It("should implement the RoleGroupHandler interface", func() {
		handler := NewTrinoRoleGroupHandler()

		// This test verifies compile-time interface compliance
		// If the handler doesn't implement the interface, this won't compile
		var _ reconciler.RoleGroupHandler[*trinov1alpha1.TrinoCluster] = handler
		Expect(handler).NotTo(BeNil())
	})
})

var _ = Describe("Role Constants", func() {
	It("should have correct role name constants", func() {
		Expect(RoleCoordinators).To(Equal("coordinators"))
		Expect(RoleWorkers).To(Equal("workers"))
	})

	It("should have different values for different roles", func() {
		Expect(RoleCoordinators).NotTo(Equal(RoleWorkers))
	})
})

// Error verification tests
var _ = Describe("TrinoRoleGroupHandler Error Handling", func() {
	var (
		handler  *TrinoRoleGroupHandler
		buildCtx *reconciler.RoleGroupBuildContext
		cr       *trinov1alpha1.TrinoCluster
	)

	BeforeEach(func() {
		handler = NewTrinoRoleGroupHandler()
		buildCtx = &reconciler.RoleGroupBuildContext{
			ClusterName:      "test-trino",
			ClusterNamespace: "default",
			RoleName:         "invalid-role",
			RoleGroupName:    "default",
			ResourceName:     "test-trino-invalid-default",
		}
		cr = &trinov1alpha1.TrinoCluster{}
	})

	It("should return a wrapped error with role name for unknown roles", func() {
		_, err := handler.BuildResources(context.Background(), k8sClient, cr, buildCtx)

		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, errors.New("unknown role"))).To(BeFalse()) // Error is not wrapped with errors.Is
		Expect(err.Error()).To(ContainSubstring("unknown role"))
		Expect(err.Error()).To(ContainSubstring("invalid-role"))
	})
})
