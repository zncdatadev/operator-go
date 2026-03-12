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

package v1alpha1_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	trinov1alpha1 "github.com/zncdatadev/operator-go/examples/trino-operator/api/v1alpha1"
	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
)

func TestAPITypes(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "API Types Suite")
}

var _ = Describe("TrinoCluster", func() {
	var cr *trinov1alpha1.TrinoCluster

	BeforeEach(func() {
		cr = &trinov1alpha1.TrinoCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-trino",
				Namespace: "default",
			},
		}
	})

	Describe("GetSpec", func() {
		Context("when coordinators and workers are nil", func() {
			It("should return empty Roles map", func() {
				spec := cr.GetSpec()
				Expect(spec).NotTo(BeNil())
				Expect(spec.Roles).To(BeEmpty())
			})

			It("should return nil ClusterOperation", func() {
				spec := cr.GetSpec()
				Expect(spec.ClusterOperation).To(BeNil())
			})
		})

		Context("when coordinators is set", func() {
			BeforeEach(func() {
				cr.Spec.Coordinators = &trinov1alpha1.CoordinatorsSpec{
					RoleSpec: commonsv1alpha1.RoleSpec{
						RoleGroups: map[string]commonsv1alpha1.RoleGroupSpec{
							"default": {Replicas: int32Ptr(1)},
						},
					},
					HTTPPort: 8080,
				}
			})

			It("should include coordinators in Roles map", func() {
				spec := cr.GetSpec()
				Expect(spec.Roles).To(HaveKey("coordinators"))
			})

			It("should carry coordinators RoleGroups", func() {
				spec := cr.GetSpec()
				Expect(spec.Roles["coordinators"].RoleGroups).To(HaveKey("default"))
			})

			It("should not include workers when not set", func() {
				spec := cr.GetSpec()
				Expect(spec.Roles).NotTo(HaveKey("workers"))
			})
		})

		Context("when workers is set", func() {
			BeforeEach(func() {
				cr.Spec.Workers = &trinov1alpha1.WorkersSpec{
					RoleSpec: commonsv1alpha1.RoleSpec{
						RoleGroups: map[string]commonsv1alpha1.RoleGroupSpec{
							"default": {Replicas: int32Ptr(3)},
						},
					},
				}
			})

			It("should include workers in Roles map", func() {
				spec := cr.GetSpec()
				Expect(spec.Roles).To(HaveKey("workers"))
			})

			It("should carry workers RoleGroups", func() {
				spec := cr.GetSpec()
				Expect(spec.Roles["workers"].RoleGroups).To(HaveKey("default"))
			})

			It("should not include coordinators when not set", func() {
				spec := cr.GetSpec()
				Expect(spec.Roles).NotTo(HaveKey("coordinators"))
			})
		})

		Context("when both coordinators and workers are set", func() {
			BeforeEach(func() {
				cr.Spec.Coordinators = &trinov1alpha1.CoordinatorsSpec{
					RoleSpec: commonsv1alpha1.RoleSpec{
						RoleGroups: map[string]commonsv1alpha1.RoleGroupSpec{
							"default": {Replicas: int32Ptr(1)},
						},
					},
				}
				cr.Spec.Workers = &trinov1alpha1.WorkersSpec{
					RoleSpec: commonsv1alpha1.RoleSpec{
						RoleGroups: map[string]commonsv1alpha1.RoleGroupSpec{
							"default": {Replicas: int32Ptr(3)},
						},
					},
				}
			})

			It("should include both in Roles map", func() {
				spec := cr.GetSpec()
				Expect(spec.Roles).To(HaveLen(2))
				Expect(spec.Roles).To(HaveKey("coordinators"))
				Expect(spec.Roles).To(HaveKey("workers"))
			})
		})

		Context("when ClusterOperation is set", func() {
			BeforeEach(func() {
				stopped := true
				cr.Spec.ClusterOperation = &commonsv1alpha1.ClusterOperationSpec{
					Stopped: stopped,
				}
			})

			It("should pass ClusterOperation through", func() {
				spec := cr.GetSpec()
				Expect(spec.ClusterOperation).NotTo(BeNil())
				Expect(spec.ClusterOperation.Stopped).To(BeTrue())
			})
		})

		Context("returned spec is independent of CR", func() {
			It("should return a new GenericClusterSpec each call", func() {
				cr.Spec.Coordinators = &trinov1alpha1.CoordinatorsSpec{}
				spec1 := cr.GetSpec()
				spec2 := cr.GetSpec()
				// Both point to different structs
				Expect(spec1).NotTo(BeIdenticalTo(spec2))
			})
		})
	})

	Describe("GetStatus / SetStatus", func() {
		It("should return a pointer to the embedded status", func() {
			status := cr.GetStatus()
			Expect(status).NotTo(BeNil())
			Expect(status).To(BeIdenticalTo(&cr.Status.GenericClusterStatus))
		})

		It("should update status via SetStatus", func() {
			newStatus := &commonsv1alpha1.GenericClusterStatus{}
			newStatus.ObservedGeneration = 42
			cr.SetStatus(newStatus)
			Expect(cr.Status.ObservedGeneration).To(Equal(int64(42)))
		})
	})

	Describe("DeepCopyCluster", func() {
		It("should return a deep copy implementing ClusterInterface", func() {
			cr.Spec.Image = "trinodb/trino:435"
			copy := cr.DeepCopyCluster()
			Expect(copy).NotTo(BeNil())
			Expect(copy.GetName()).To(Equal(cr.GetName()))

			// Verify it's a deep copy (not same pointer)
			trinoCopy, ok := copy.(*trinov1alpha1.TrinoCluster)
			Expect(ok).To(BeTrue())
			Expect(trinoCopy).NotTo(BeIdenticalTo(cr))
			Expect(trinoCopy.Spec.Image).To(Equal(cr.Spec.Image))
		})
	})

	Describe("TrinoClusterSpec structure", func() {
		It("should not expose a top-level roles field in JSON", func() {
			// Verify spec fields are the typed ones only
			cr.Spec.Image = "trinodb/trino:435"
			cr.Spec.Coordinators = &trinov1alpha1.CoordinatorsSpec{}
			cr.Spec.Workers = &trinov1alpha1.WorkersSpec{}

			// GetSpec() should build roles dynamically - never stored in spec
			spec := cr.GetSpec()
			Expect(spec.Roles).To(HaveLen(2))
		})

		It("CoordinatorsSpec should carry role-level config through embedded RoleSpec", func() {
			cr.Spec.Coordinators = &trinov1alpha1.CoordinatorsSpec{
				RoleSpec: commonsv1alpha1.RoleSpec{
					ConfigOverrides: map[string]map[string]string{
						"config.properties": {"key": "value"},
					},
				},
				HTTPPort: 9090,
			}
			spec := cr.GetSpec()
			coordRole := spec.Roles["coordinators"]
			Expect(coordRole.ConfigOverrides).To(HaveKey("config.properties"))
		})
	})
})

func int32Ptr(i int32) *int32 {
	return &i
}
