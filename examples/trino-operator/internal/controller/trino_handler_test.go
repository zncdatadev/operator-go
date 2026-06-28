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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	trinov1alpha1 "github.com/zncdatadev/operator-go/examples/trino-operator/api/v1alpha1"
	"github.com/zncdatadev/operator-go/examples/trino-operator/internal/constants"
	"github.com/zncdatadev/operator-go/examples/trino-operator/internal/product"
	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/config"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
)

// buildCtxFor assembles a RoleGroupBuildContext the way the GenericReconciler would: it merges
// the product defaults (lowest layer) with the given CRD overrides (highest layer) through the
// SDK ConfigMerger, so the handler sees exactly what it would at runtime.
func buildCtxFor(cr *trinov1alpha1.TrinoCluster, role string, crdOverrides *commonsv1alpha1.OverridesSpec) *reconciler.RoleGroupBuildContext {
	const group = "default"
	merger := config.NewConfigMerger()
	merged := merger.Merge(product.ComputeConfig(cr, role, group), crdOverrides)

	return &reconciler.RoleGroupBuildContext{
		ClusterName:      cr.Name,
		ClusterNamespace: "default",
		ClusterLabels:    map[string]string{"app": "trino"},
		ClusterSpec:      cr.GetSpec(),
		RoleName:         role,
		RoleSpec:         &commonsv1alpha1.RoleSpec{},
		RoleGroupName:    group,
		RoleGroupSpec:    commonsv1alpha1.RoleGroupSpec{Replicas: ptr.To(int32(1))},
		MergedConfig:     merged,
		ResourceName:     reconciler.RoleGroupResourceName(cr.Name, role, group),
	}
}

func newTrinoCR() *trinov1alpha1.TrinoCluster {
	cr := &trinov1alpha1.TrinoCluster{}
	cr.Name = "test-trino"
	cr.Namespace = "default"
	cr.Spec = trinov1alpha1.TrinoClusterSpec{
		Image: &commonsv1alpha1.ImageSpec{Custom: "trinodb/trino:435"},
		Coordinators: &trinov1alpha1.CoordinatorsSpec{
			RoleSpec: commonsv1alpha1.RoleSpec{
				RoleGroups: map[string]commonsv1alpha1.RoleGroupSpec{"default": {}},
			},
		},
		Workers: &trinov1alpha1.WorkersSpec{
			RoleSpec: commonsv1alpha1.RoleSpec{
				RoleGroups: map[string]commonsv1alpha1.RoleGroupSpec{"default": {}},
			},
		},
		Catalogs: []trinov1alpha1.CatalogSpec{
			{Name: "tpch", Type: "tpch"},
		},
	}
	return cr
}

var _ = Describe("TrinoRoleGroupHandler", func() {
	var handler *TrinoRoleGroupHandler

	BeforeEach(func() {
		handler = NewTrinoRoleGroupHandler(scheme.Scheme)
	})

	It("implements the SDK RoleGroupHandler interface", func() {
		var _ reconciler.RoleGroupHandler[*trinov1alpha1.TrinoCluster] = handler
		Expect(handler).NotTo(BeNil())
	})

	Describe("BuildResources (framework owns orchestration)", func() {
		It("builds the coordinator role group from the merged config", func() {
			cr := newTrinoCR()
			buildCtx := buildCtxFor(cr, product.RoleCoordinators, nil)

			res, err := handler.BuildResources(ctx, k8sClient, cr, buildCtx)
			Expect(err).NotTo(HaveOccurred())

			// Framework-built resources.
			Expect(res.ConfigMap).NotTo(BeNil())
			Expect(res.HeadlessService).NotTo(BeNil())
			Expect(res.Service).NotTo(BeNil())
			Expect(res.StatefulSet).NotTo(BeNil())

			// config.properties comes from product defaults via the merge pipeline, rendered
			// by the framework's properties adapter.
			cp := res.ConfigMap.Data["config.properties"]
			Expect(cp).To(ContainSubstring("coordinator=true"))
			Expect(cp).To(ContainSubstring("discovery-server.enabled=true"))

			// Product-specific files appended by the handler.
			Expect(res.ConfigMap.Data).To(HaveKey("jvm.config"))
			Expect(res.ConfigMap.Data["jvm.config"]).To(ContainSubstring("-Xmx" + constants.DefaultCoordinatorMaxMemory))
			Expect(res.ConfigMap.Data).To(HaveKey("catalog/tpch.properties"))

			// Logging file rendered declaratively by the framework (LoggingContainers).
			Expect(res.ConfigMap.Data).To(HaveKey("log4j2.properties"))

			// Primary container named "trino", config mounted at /etc/trino, CR-driven image.
			container := res.StatefulSet.Spec.Template.Spec.Containers[0]
			Expect(container.Name).To(Equal(constants.MainContainerName))
			Expect(container.Image).To(Equal("trinodb/trino:435"))
			var mountPath string
			for _, vm := range container.VolumeMounts {
				if vm.Name == "config" {
					mountPath = vm.MountPath
				}
			}
			Expect(mountPath).To(Equal("/etc/trino"))
		})

		It("builds the worker role group without catalog files", func() {
			cr := newTrinoCR()
			buildCtx := buildCtxFor(cr, product.RoleWorkers, nil)

			res, err := handler.BuildResources(ctx, k8sClient, cr, buildCtx)
			Expect(err).NotTo(HaveOccurred())

			Expect(res.ConfigMap.Data["config.properties"]).To(ContainSubstring("coordinator=false"))
			Expect(res.ConfigMap.Data["jvm.config"]).To(ContainSubstring("-Xmx" + constants.DefaultWorkerMaxMemory))
			Expect(res.ConfigMap.Data).NotTo(HaveKey("catalog/tpch.properties"))
		})

		It("lets a CRD configOverride win over a product default for the same key", func() {
			cr := newTrinoCR()
			crdOverrides := &commonsv1alpha1.OverridesSpec{
				ConfigOverrides: map[string]map[string]string{
					// Override a key the product default sets (coordinator=true) and add a new one.
					"config.properties": {"coordinator": "false", "query.max-memory": "8GB"},
				},
			}
			buildCtx := buildCtxFor(cr, product.RoleCoordinators, crdOverrides)

			res, err := handler.BuildResources(ctx, k8sClient, cr, buildCtx)
			Expect(err).NotTo(HaveOccurred())

			cp := res.ConfigMap.Data["config.properties"]
			// CRD override wins over the product default.
			Expect(cp).To(MatchRegexp(`(?m)^coordinator=false$`))
			Expect(cp).NotTo(MatchRegexp(`(?m)^coordinator=true$`))
			// New user key coexists with product defaults.
			Expect(cp).To(ContainSubstring("query.max-memory=8GB"))
			Expect(cp).To(ContainSubstring("discovery-server.enabled=true"))
		})

		It("does not clobber a user-provided catalog file (setIfAbsent; CRD wins)", func() {
			// Catalog files are .properties, so configOverrides expresses them cleanly — a good
			// test of the handler's setIfAbsent guard against overwriting pipeline-produced keys.
			cr := newTrinoCR() // declares catalog "tpch" of type tpch
			crdOverrides := &commonsv1alpha1.OverridesSpec{
				ConfigOverrides: map[string]map[string]string{
					"catalog/tpch.properties": {"connector.name": "blackhole"},
				},
			}
			buildCtx := buildCtxFor(cr, product.RoleCoordinators, crdOverrides)

			res, err := handler.BuildResources(ctx, k8sClient, cr, buildCtx)
			Expect(err).NotTo(HaveOccurred())

			// The user-provided catalog wins; the product-generated tpch connector is not applied.
			cat := res.ConfigMap.Data["catalog/tpch.properties"]
			Expect(cat).To(ContainSubstring("connector.name=blackhole"))
			Expect(cat).NotTo(ContainSubstring("connector.name=tpch"))
		})
	})
})
