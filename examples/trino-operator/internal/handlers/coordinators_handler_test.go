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

package handlers

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	trinov1alpha1 "github.com/zncdatadev/operator-go/examples/trino-operator/api/v1alpha1"
	"github.com/zncdatadev/operator-go/examples/trino-operator/internal/constants"
	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("CoordinatorsHandler", func() {
	var (
		handler  *CoordinatorsHandler
		cr       *trinov1alpha1.TrinoCluster
		buildCtx *reconciler.RoleGroupBuildContext
		ctx      context.Context
	)

	BeforeEach(func() {
		handler = NewCoordinatorsHandler()
		cr = &trinov1alpha1.TrinoCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: trinov1alpha1.TrinoClusterSpec{
				Image: "trinodb/trino:435",
			},
		}
		buildCtx = &reconciler.RoleGroupBuildContext{
			ClusterName:      "test-cluster",
			ClusterNamespace: "default",
			RoleName:         "coordinators",
			RoleGroupName:    "default",
			ResourceName:     "test-cluster-coordinators-default",
			ClusterLabels:    map[string]string{"app": "trino"},
			RoleGroupSpec:    v1alpha1.RoleGroupSpec{},
		}
		ctx = context.Background()
	})

	Context("NewCoordinatorsHandler", func() {
		It("should create a new handler", func() {
			h := NewCoordinatorsHandler()
			Expect(h).NotTo(BeNil())
		})
	})

	Context("BuildResources", func() {
		It("should build all resources successfully", func() {
			resources, err := handler.BuildResources(ctx, nil, cr, buildCtx)
			Expect(err).NotTo(HaveOccurred())
			Expect(resources).NotTo(BeNil())
		})

		It("should build ConfigMap", func() {
			resources, err := handler.BuildResources(ctx, nil, cr, buildCtx)
			Expect(err).NotTo(HaveOccurred())
			Expect(resources.ConfigMap).NotTo(BeNil())
			Expect(resources.ConfigMap.Name).To(Equal("test-cluster-coordinators-default-config"))
		})

		It("should build HeadlessService", func() {
			resources, err := handler.BuildResources(ctx, nil, cr, buildCtx)
			Expect(err).NotTo(HaveOccurred())
			Expect(resources.HeadlessService).NotTo(BeNil())
			Expect(resources.HeadlessService.Name).To(Equal("test-cluster-coordinators-default-headless"))
		})

		It("should build Service", func() {
			resources, err := handler.BuildResources(ctx, nil, cr, buildCtx)
			Expect(err).NotTo(HaveOccurred())
			Expect(resources.Service).NotTo(BeNil())
			Expect(resources.Service.Name).To(Equal("test-cluster-coordinators-default"))
		})

		It("should build StatefulSet", func() {
			resources, err := handler.BuildResources(ctx, nil, cr, buildCtx)
			Expect(err).NotTo(HaveOccurred())
			Expect(resources.StatefulSet).NotTo(BeNil())
			Expect(resources.StatefulSet.Name).To(Equal("test-cluster-coordinators-default"))
		})

		It("should use default port when not specified", func() {
			resources, err := handler.BuildResources(ctx, nil, cr, buildCtx)
			Expect(err).NotTo(HaveOccurred())
			Expect(resources.Service.Spec.Ports[0].Port).To(Equal(constants.DefaultHTTPPort))
		})

		It("should use custom port when specified", func() {
			cr.Spec.Coordinators = &trinov1alpha1.CoordinatorsSpec{
				HTTPPort: 9090,
			}
			resources, err := handler.BuildResources(ctx, nil, cr, buildCtx)
			Expect(err).NotTo(HaveOccurred())
			Expect(resources.Service.Spec.Ports[0].Port).To(Equal(int32(9090)))
		})

		It("should build PDB when configured", func() {
			enabled := true
			cr.Spec.Coordinators = &trinov1alpha1.CoordinatorsSpec{
				RoleSpec: v1alpha1.RoleSpec{
					RoleConfig: &v1alpha1.RoleConfigSpec{
						PodDisruptionBudget: &v1alpha1.PodDisruptionBudgetSpec{
							Enabled: enabled,
						},
					},
				},
			}
			resources, err := handler.BuildResources(ctx, nil, cr, buildCtx)
			Expect(err).NotTo(HaveOccurred())
			Expect(resources.PodDisruptionBudget).NotTo(BeNil())
		})

		It("should not build PDB when not configured", func() {
			resources, err := handler.BuildResources(ctx, nil, cr, buildCtx)
			Expect(err).NotTo(HaveOccurred())
			Expect(resources.PodDisruptionBudget).To(BeNil())
		})

		It("should include catalogs in ConfigMap when specified", func() {
			cr.Spec.Catalogs = []trinov1alpha1.CatalogSpec{
				{Name: "hive", Type: "hive"},
			}
			resources, err := handler.BuildResources(ctx, nil, cr, buildCtx)
			Expect(err).NotTo(HaveOccurred())
			Expect(resources.ConfigMap.Data).To(HaveKey("catalog/hive.properties"))
		})
	})

	Context("getRoleConfigPDB", func() {
		It("should return nil when spec is nil", func() {
			result := getRoleConfigPDB(nil)
			Expect(result).To(BeNil())
		})

		It("should return nil when RoleConfig is nil", func() {
			result := getRoleConfigPDB(&trinov1alpha1.CoordinatorsSpec{})
			Expect(result).To(BeNil())
		})

		It("should return nil when PodDisruptionBudget is nil", func() {
			result := getRoleConfigPDB(&trinov1alpha1.CoordinatorsSpec{
				RoleSpec: v1alpha1.RoleSpec{
					RoleConfig: &v1alpha1.RoleConfigSpec{},
				},
			})
			Expect(result).To(BeNil())
		})

		It("should return PDB spec when configured", func() {
			pdbSpec := &v1alpha1.PodDisruptionBudgetSpec{Enabled: true}
			result := getRoleConfigPDB(&trinov1alpha1.CoordinatorsSpec{
				RoleSpec: v1alpha1.RoleSpec{
					RoleConfig: &v1alpha1.RoleConfigSpec{
						PodDisruptionBudget: pdbSpec,
					},
				},
			})
			Expect(result).To(Equal(pdbSpec))
		})
	})
})
