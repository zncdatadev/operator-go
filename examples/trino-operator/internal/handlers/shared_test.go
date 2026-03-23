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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	trinov1alpha1 "github.com/zncdatadev/operator-go/examples/trino-operator/api/v1alpha1"
	"github.com/zncdatadev/operator-go/examples/trino-operator/internal/constants"
	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var _ = Describe("Shared Handlers", func() {
	var buildCtx *reconciler.RoleGroupBuildContext

	BeforeEach(func() {
		buildCtx = &reconciler.RoleGroupBuildContext{
			ClusterName:      "test-trino",
			ClusterNamespace: "default",
			ClusterLabels: map[string]string{
				"app.kubernetes.io/name":     "trino",
				"app.kubernetes.io/instance": "test-trino",
			},
			RoleName:      "coordinator",
			RoleGroupName: "default",
			ResourceName:  "test-trino-coordinator-default",
			RoleGroupSpec: v1alpha1.RoleGroupSpec{},
		}
	})

	Context("BuildHeadlessService", func() {
		It("Should build headless service with correct name format", func() {
			port := int32(8080)
			svc := BuildHeadlessService(buildCtx, port)

			Expect(svc).NotTo(BeNil())
			Expect(svc.Name).To(Equal("test-trino-coordinator-default-headless"))
			Expect(svc.Namespace).To(Equal("default"))
		})

		It("Should have correct labels", func() {
			port := int32(8080)
			svc := BuildHeadlessService(buildCtx, port)

			Expect(svc.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", "trino"))
			Expect(svc.Labels).To(HaveKeyWithValue("app.kubernetes.io/instance", "test-trino"))
		})

		It("Should have correct selector", func() {
			port := int32(8080)
			svc := BuildHeadlessService(buildCtx, port)

			Expect(svc.Spec.Selector).To(HaveKeyWithValue("app.kubernetes.io/name", "trino"))
			Expect(svc.Spec.Selector).To(HaveKeyWithValue("app.kubernetes.io/instance", "test-trino"))
		})

		It("Should have correct port configuration", func() {
			port := int32(9090)
			svc := BuildHeadlessService(buildCtx, port)

			Expect(svc.Spec.Ports).To(HaveLen(1))
			Expect(svc.Spec.Ports[0].Name).To(Equal("http"))
			Expect(svc.Spec.Ports[0].Port).To(Equal(int32(9090)))
			Expect(svc.Spec.Ports[0].Protocol).To(Equal(corev1.ProtocolTCP))
		})

		It("Should be headless (ClusterIP None)", func() {
			port := int32(8080)
			svc := BuildHeadlessService(buildCtx, port)

			Expect(svc.Spec.ClusterIP).To(Equal(corev1.ClusterIPNone))
		})
	})

	Context("BuildService", func() {
		It("Should build ClusterIP service with correct name", func() {
			port := int32(8080)
			svc := BuildService(buildCtx, port)

			Expect(svc).NotTo(BeNil())
			Expect(svc.Name).To(Equal("test-trino-coordinator-default"))
			Expect(svc.Namespace).To(Equal("default"))
		})

		It("Should have correct labels", func() {
			port := int32(8080)
			svc := BuildService(buildCtx, port)

			Expect(svc.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", "trino"))
			Expect(svc.Labels).To(HaveKeyWithValue("app.kubernetes.io/instance", "test-trino"))
		})

		It("Should have correct selector", func() {
			port := int32(8080)
			svc := BuildService(buildCtx, port)

			Expect(svc.Spec.Selector).To(HaveKeyWithValue("app.kubernetes.io/name", "trino"))
			Expect(svc.Spec.Selector).To(HaveKeyWithValue("app.kubernetes.io/instance", "test-trino"))
		})

		It("Should have correct port configuration", func() {
			port := int32(9090)
			svc := BuildService(buildCtx, port)

			Expect(svc.Spec.Ports).To(HaveLen(1))
			Expect(svc.Spec.Ports[0].Name).To(Equal("http"))
			Expect(svc.Spec.Ports[0].Port).To(Equal(int32(9090)))
			Expect(svc.Spec.Ports[0].Protocol).To(Equal(corev1.ProtocolTCP))
		})

		It("Should be ClusterIP type (not headless)", func() {
			port := int32(8080)
			svc := BuildService(buildCtx, port)

			Expect(svc.Spec.ClusterIP).NotTo(Equal(corev1.ClusterIPNone))
		})
	})

	Context("BuildConfigMap", func() {
		It("Should build ConfigMap with correct name format", func() {
			data := map[string]string{
				"config.properties": "key=value",
			}
			cm := BuildConfigMap(buildCtx, data)

			Expect(cm).NotTo(BeNil())
			Expect(cm.Name).To(Equal("test-trino-coordinator-default-config"))
			Expect(cm.Namespace).To(Equal("default"))
		})

		It("Should have correct labels", func() {
			data := map[string]string{
				"config.properties": "key=value",
			}
			cm := BuildConfigMap(buildCtx, data)

			Expect(cm.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", "trino"))
			Expect(cm.Labels).To(HaveKeyWithValue("app.kubernetes.io/instance", "test-trino"))
		})

		It("Should contain correct data", func() {
			data := map[string]string{
				"config.properties": "http-server.http.port=8080",
				"jvm.config":        "-Xmx2G",
			}
			cm := BuildConfigMap(buildCtx, data)

			Expect(cm.Data).To(HaveLen(2))
			Expect(cm.Data).To(HaveKeyWithValue("config.properties", "http-server.http.port=8080"))
			Expect(cm.Data).To(HaveKeyWithValue("jvm.config", "-Xmx2G"))
		})

		It("Should handle empty data", func() {
			data := map[string]string{}
			cm := BuildConfigMap(buildCtx, data)

			Expect(cm).NotTo(BeNil())
			Expect(cm.Data).To(BeEmpty())
		})
	})

	Context("BuildStatefulSet", func() {
		var cr *trinov1alpha1.TrinoCluster

		BeforeEach(func() {
			cr = &trinov1alpha1.TrinoCluster{
				Spec: trinov1alpha1.TrinoClusterSpec{
					Image: "trinodb/trino:435",
				},
			}
		})

		It("Should build StatefulSet with correct name and namespace", func() {
			sts := BuildStatefulSet(buildCtx, cr, "test-config", 8080, 1, "500m", "1", "2Gi")

			Expect(sts).NotTo(BeNil())
			Expect(sts.Name).To(Equal("test-trino-coordinator-default"))
			Expect(sts.Namespace).To(Equal("default"))
		})

		It("Should have correct replicas", func() {
			sts := BuildStatefulSet(buildCtx, cr, "test-config", 8080, 3, "500m", "1", "2Gi")

			Expect(*sts.Spec.Replicas).To(Equal(int32(3)))
		})

		It("Should have correct image", func() {
			sts := BuildStatefulSet(buildCtx, cr, "test-config", 8080, 1, "500m", "1", "2Gi")

			containers := sts.Spec.Template.Spec.Containers
			Expect(containers).To(HaveLen(1))
			Expect(containers[0].Image).To(Equal("trinodb/trino:435"))
		})

		It("Should have correct port configuration", func() {
			sts := BuildStatefulSet(buildCtx, cr, "test-config", 9090, 1, "500m", "1", "2Gi")

			containers := sts.Spec.Template.Spec.Containers
			Expect(containers[0].Ports).To(HaveLen(1))
			Expect(containers[0].Ports[0].Name).To(Equal("http"))
			Expect(containers[0].Ports[0].ContainerPort).To(Equal(int32(9090)))
		})

		It("Should have config volume", func() {
			sts := BuildStatefulSet(buildCtx, cr, "my-configmap", 8080, 1, "500m", "1", "2Gi")

			volumes := sts.Spec.Template.Spec.Volumes
			Expect(volumes).To(HaveLen(1))
			Expect(volumes[0].Name).To(Equal("config"))
			Expect(volumes[0].ConfigMap).NotTo(BeNil())
			Expect(volumes[0].ConfigMap.Name).To(Equal("my-configmap"))
		})

		It("Should have config volume mount", func() {
			sts := BuildStatefulSet(buildCtx, cr, "test-config", 8080, 1, "500m", "1", "2Gi")

			containers := sts.Spec.Template.Spec.Containers
			Expect(containers[0].VolumeMounts).To(HaveLen(1))
			Expect(containers[0].VolumeMounts[0].Name).To(Equal("config"))
			Expect(containers[0].VolumeMounts[0].MountPath).To(Equal("/etc/trino"))
		})

		It("Should have correct labels", func() {
			sts := BuildStatefulSet(buildCtx, cr, "test-config", 8080, 1, "500m", "1", "2Gi")

			Expect(sts.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", "trino"))
			Expect(sts.Labels).To(HaveKeyWithValue("app.kubernetes.io/instance", "test-trino"))
		})
	})

	Context("BuildPDB", func() {
		It("Should return nil when pdbSpec is nil", func() {
			pdb := BuildPDB(buildCtx, nil)

			Expect(pdb).To(BeNil())
		})

		It("Should return nil when PDB is disabled", func() {
			pdbSpec := &v1alpha1.PodDisruptionBudgetSpec{
				Enabled: false,
			}
			pdb := BuildPDB(buildCtx, pdbSpec)

			Expect(pdb).To(BeNil())
		})

		It("Should build PDB when enabled", func() {
			maxUnavailable := int32(1)
			pdbSpec := &v1alpha1.PodDisruptionBudgetSpec{
				Enabled:        true,
				MaxUnavailable: &maxUnavailable,
			}
			pdb := BuildPDB(buildCtx, pdbSpec)

			Expect(pdb).NotTo(BeNil())
			Expect(pdb.Name).To(Equal("test-trino-coordinator-default-pdb"))
			Expect(pdb.Namespace).To(Equal("default"))
		})

		It("Should have correct labels", func() {
			maxUnavailable := int32(1)
			pdbSpec := &v1alpha1.PodDisruptionBudgetSpec{
				Enabled:        true,
				MaxUnavailable: &maxUnavailable,
			}
			pdb := BuildPDB(buildCtx, pdbSpec)

			Expect(pdb.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", "trino"))
			Expect(pdb.Labels).To(HaveKeyWithValue("app.kubernetes.io/instance", "test-trino"))
		})

		It("Should have correct selector", func() {
			maxUnavailable := int32(1)
			pdbSpec := &v1alpha1.PodDisruptionBudgetSpec{
				Enabled:        true,
				MaxUnavailable: &maxUnavailable,
			}
			pdb := BuildPDB(buildCtx, pdbSpec)

			Expect(pdb.Spec.Selector.MatchLabels).To(HaveKeyWithValue("app.kubernetes.io/name", "trino"))
			Expect(pdb.Spec.Selector.MatchLabels).To(HaveKeyWithValue("app.kubernetes.io/instance", "test-trino"))
		})
	})

	Context("GetCoordinatorPort", func() {
		It("Should return default port when coordinators spec is nil", func() {
			cr := &trinov1alpha1.TrinoCluster{
				Spec: trinov1alpha1.TrinoClusterSpec{},
			}

			port := GetCoordinatorPort(cr)
			Expect(port).To(Equal(constants.DefaultHTTPPort))
		})

		It("Should return default port when HTTPPort is 0", func() {
			cr := &trinov1alpha1.TrinoCluster{
				Spec: trinov1alpha1.TrinoClusterSpec{
					Coordinators: &trinov1alpha1.CoordinatorsSpec{
						HTTPPort: 0,
					},
				},
			}

			port := GetCoordinatorPort(cr)
			Expect(port).To(Equal(constants.DefaultHTTPPort))
		})

		It("Should return custom port when specified", func() {
			cr := &trinov1alpha1.TrinoCluster{
				Spec: trinov1alpha1.TrinoClusterSpec{
					Coordinators: &trinov1alpha1.CoordinatorsSpec{
						HTTPPort: 9090,
					},
				},
			}

			port := GetCoordinatorPort(cr)
			Expect(port).To(Equal(int32(9090)))
		})
	})

	Context("GetWorkerPort", func() {
		It("Should return default port when workers spec is nil", func() {
			cr := &trinov1alpha1.TrinoCluster{
				Spec: trinov1alpha1.TrinoClusterSpec{},
			}

			port := GetWorkerPort(cr)
			Expect(port).To(Equal(constants.DefaultHTTPPort))
		})

		It("Should return default port when HTTPPort is 0", func() {
			cr := &trinov1alpha1.TrinoCluster{
				Spec: trinov1alpha1.TrinoClusterSpec{
					Workers: &trinov1alpha1.WorkersSpec{
						HTTPPort: 0,
					},
				},
			}

			port := GetWorkerPort(cr)
			Expect(port).To(Equal(constants.DefaultHTTPPort))
		})

		It("Should return custom port when specified", func() {
			cr := &trinov1alpha1.TrinoCluster{
				Spec: trinov1alpha1.TrinoClusterSpec{
					Workers: &trinov1alpha1.WorkersSpec{
						HTTPPort: 9091,
					},
				},
			}

			port := GetWorkerPort(cr)
			Expect(port).To(Equal(int32(9091)))
		})
	})

	Context("GetCoordinatorServiceName", func() {
		It("Should fall back to {name}-coordinator when no role groups defined", func() {
			cr := &trinov1alpha1.TrinoCluster{
				Spec: trinov1alpha1.TrinoClusterSpec{},
			}
			cr.Name = "my-trino-cluster"

			name := GetCoordinatorServiceName(cr)
			Expect(name).To(Equal("my-trino-cluster-coordinator"))
		})

		It("Should return {name}-{groupName} when coordinator role group is defined", func() {
			cr := &trinov1alpha1.TrinoCluster{
				Spec: trinov1alpha1.TrinoClusterSpec{
					Coordinators: &trinov1alpha1.CoordinatorsSpec{
						RoleSpec: v1alpha1.RoleSpec{
							RoleGroups: map[string]v1alpha1.RoleGroupSpec{
								"default": {},
							},
						},
					},
				},
			}
			cr.Name = "my-trino-cluster"

			name := GetCoordinatorServiceName(cr)
			Expect(name).To(Equal("my-trino-cluster-default"))
		})
	})

	Context("GetDiscoveryURI", func() {
		It("Should fall back to {name}-coordinator format when no role groups defined", func() {
			cr := &trinov1alpha1.TrinoCluster{
				Spec: trinov1alpha1.TrinoClusterSpec{},
			}
			cr.Name = "my-trino-cluster"

			uri := GetDiscoveryURI(cr, 8080)
			Expect(uri).To(Equal("http://my-trino-cluster-coordinator:8080"))
		})

		It("Should use actual coordinator service name when role group is defined", func() {
			cr := &trinov1alpha1.TrinoCluster{
				Spec: trinov1alpha1.TrinoClusterSpec{
					Coordinators: &trinov1alpha1.CoordinatorsSpec{
						RoleSpec: v1alpha1.RoleSpec{
							RoleGroups: map[string]v1alpha1.RoleGroupSpec{
								"default": {},
							},
						},
					},
				},
			}
			cr.Name = "test-cluster"

			uri := GetDiscoveryURI(cr, 9090)
			Expect(uri).To(Equal("http://test-cluster-default:9090"))
		})

		It("Should include custom port in URI", func() {
			cr := &trinov1alpha1.TrinoCluster{
				Spec: trinov1alpha1.TrinoClusterSpec{},
			}
			cr.Name = "test-cluster"

			uri := GetDiscoveryURI(cr, 9090)
			Expect(uri).To(Equal("http://test-cluster-coordinator:9090"))
		})
	})

	Context("GetReplicas", func() {
		It("Should return default replicas when RoleGroupSpec replicas is nil", func() {
			buildCtx.RoleGroupSpec = v1alpha1.RoleGroupSpec{
				Replicas: nil,
			}

			replicas := GetReplicas(buildCtx, 3)
			Expect(replicas).To(Equal(int32(3)))
		})

		It("Should return custom replicas when specified", func() {
			customReplicas := int32(5)
			buildCtx.RoleGroupSpec = v1alpha1.RoleGroupSpec{
				Replicas: &customReplicas,
			}

			replicas := GetReplicas(buildCtx, 3)
			Expect(replicas).To(Equal(int32(5)))
		})

		It("Should return zero replicas when specified", func() {
			zeroReplicas := int32(0)
			buildCtx.RoleGroupSpec = v1alpha1.RoleGroupSpec{
				Replicas: &zeroReplicas,
			}

			replicas := GetReplicas(buildCtx, 3)
			Expect(replicas).To(Equal(int32(0)))
		})
	})

	Context("ConvertStringToIntOrString", func() {
		It("Should convert string number to IntOrString", func() {
			result := ConvertStringToIntOrString("8080")
			Expect(result.Type).To(Equal(intstr.Int))
			Expect(result.IntVal).To(Equal(int32(8080)))
		})

		It("Should convert percentage string to IntOrString", func() {
			result := ConvertStringToIntOrString("50%")
			Expect(result.Type).To(Equal(intstr.String))
			Expect(result.StrVal).To(Equal("50%"))
		})

		It("Should handle empty string", func() {
			result := ConvertStringToIntOrString("")
			// intstr.Parse("") returns String type with empty string value
			Expect(result.Type).To(Equal(intstr.String))
			Expect(result.StrVal).To(Equal(""))
		})
	})
})
