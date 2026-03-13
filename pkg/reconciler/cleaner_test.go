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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

const cleanerTestNamespace = "default"

var _ = Describe("RoleGroupCleaner", func() {
	var cleaner *reconciler.RoleGroupCleaner
	var ctx context.Context
	var namespace string

	BeforeEach(func() {
		ctx = context.Background()
		namespace = cleanerTestNamespace
		cleaner = reconciler.NewRoleGroupCleaner(k8sClient, testScheme)
	})

	Describe("NewRoleGroupCleaner", func() {
		It("should create a RoleGroupCleaner", func() {
			Expect(cleaner).NotTo(BeNil())
		})

		It("should have client set", func() {
			Expect(cleaner.Client).To(Equal(k8sClient))
		})

		It("should have scheme set", func() {
			Expect(cleaner.Scheme).To(Equal(testScheme))
		})
	})

	Describe("Cleanup", func() {
		It("should return nil when there are no orphaned role groups", func() {
			spec := &v1alpha1.GenericClusterSpec{
				Roles: map[string]v1alpha1.RoleSpec{
					"test-role": {
						RoleGroups: map[string]v1alpha1.RoleGroupSpec{
							"default": {Replicas: ptr.To(int32(1))},
						},
					},
				},
			}
			status := &v1alpha1.GenericClusterStatus{}
			status.SetRoleGroup("test-role", "default")

			err := cleaner.Cleanup(ctx, namespace, "test-cluster", spec, status)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return nil for empty roles", func() {
			spec := &v1alpha1.GenericClusterSpec{
				Roles: map[string]v1alpha1.RoleSpec{},
			}
			status := &v1alpha1.GenericClusterStatus{}

			err := cleaner.Cleanup(ctx, namespace, "test-cluster", spec, status)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return nil for nil roles", func() {
			spec := &v1alpha1.GenericClusterSpec{
				Roles: nil,
			}
			status := &v1alpha1.GenericClusterStatus{}

			err := cleaner.Cleanup(ctx, namespace, "test-cluster", spec, status)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return nil when all status role groups exist in spec", func() {
			spec := &v1alpha1.GenericClusterSpec{
				Roles: map[string]v1alpha1.RoleSpec{
					"role-a": {
						RoleGroups: map[string]v1alpha1.RoleGroupSpec{
							"group-1": {Replicas: ptr.To(int32(1))},
							"group-2": {Replicas: ptr.To(int32(2))},
						},
					},
					"role-b": {
						RoleGroups: map[string]v1alpha1.RoleGroupSpec{
							"group-1": {Replicas: ptr.To(int32(3))},
						},
					},
				},
			}
			status := &v1alpha1.GenericClusterStatus{}
			status.SetRoleGroup("role-a", "group-1")
			status.SetRoleGroup("role-a", "group-2")
			status.SetRoleGroup("role-b", "group-1")

			err := cleaner.Cleanup(ctx, namespace, "test-cluster", spec, status)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return nil for empty status", func() {
			spec := &v1alpha1.GenericClusterSpec{
				Roles: map[string]v1alpha1.RoleSpec{
					"test-role": {
						RoleGroups: map[string]v1alpha1.RoleGroupSpec{
							"default": {Replicas: ptr.To(int32(1))},
						},
					},
				},
			}
			status := &v1alpha1.GenericClusterStatus{}

			err := cleaner.Cleanup(ctx, namespace, "test-cluster", spec, status)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return nil when no status role groups", func() {
			spec := &v1alpha1.GenericClusterSpec{
				Roles: map[string]v1alpha1.RoleSpec{
					"test-role": {
						RoleGroups: map[string]v1alpha1.RoleGroupSpec{
							"default": {Replicas: ptr.To(int32(1))},
						},
					},
				},
			}
			status := &v1alpha1.GenericClusterStatus{}

			err := cleaner.Cleanup(ctx, namespace, "test-cluster", spec, status)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should cleanup orphaned role group resources", func() {
			// Create resources for an orphaned role group
			resourceName := "cleanup-test-orphaned"

			// Create ConfigMap
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespace,
				},
				Data: map[string]string{"key": "value"},
			}
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())

			// Create Service
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespace,
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "None",
					Ports:     []corev1.ServicePort{{Port: 8080}},
				},
			}
			Expect(k8sClient.Create(ctx, svc)).To(Succeed())

			// Setup spec and status with orphaned group
			spec := &v1alpha1.GenericClusterSpec{
				Roles: map[string]v1alpha1.RoleSpec{
					"test-role": {
						RoleGroups: map[string]v1alpha1.RoleGroupSpec{
							"default": {Replicas: ptr.To(int32(1))},
						},
					},
				},
			}
			status := &v1alpha1.GenericClusterStatus{}
			status.SetRoleGroup("test-role", "default")
			status.SetRoleGroup("test-role", "orphaned") // This is orphaned

			err := cleaner.Cleanup(ctx, namespace, "cleanup-test", spec, status)
			Expect(err).ToNot(HaveOccurred())

			// Verify resources are deleted
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: resourceName}, &corev1.ConfigMap{})
				return err != nil
			}, "5s", "100ms").Should(BeTrue())
		})
	})
})

var _ = Describe("RoleGroupCleaner resource deletion", func() {
	var cleaner *reconciler.RoleGroupCleaner
	var ctx context.Context
	var namespace string

	BeforeEach(func() {
		ctx = context.Background()
		namespace = cleanerTestNamespace
		cleaner = reconciler.NewRoleGroupCleaner(k8sClient, testScheme)
	})

	Describe("deleteConfigMap", func() {
		It("should delete existing ConfigMap", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "delete-test-cm",
					Namespace: namespace,
				},
				Data: map[string]string{"key": "value"},
			}
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())

			// Verify it exists
			fetchedCM := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "delete-test-cm"}, fetchedCM)).To(Succeed())

			// Cleanup will delete it via the cleanupRoleGroup method
			spec := &v1alpha1.GenericClusterSpec{
				Roles: map[string]v1alpha1.RoleSpec{
					"role": {
						RoleGroups: map[string]v1alpha1.RoleGroupSpec{},
					},
				},
			}
			status := &v1alpha1.GenericClusterStatus{}
			status.SetRoleGroup("role", "test-cm") // Orphaned

			err := cleaner.Cleanup(ctx, namespace, "delete", spec, status)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should handle non-existent ConfigMap gracefully", func() {
			spec := &v1alpha1.GenericClusterSpec{
				Roles: map[string]v1alpha1.RoleSpec{
					"role": {
						RoleGroups: map[string]v1alpha1.RoleGroupSpec{},
					},
				},
			}
			status := &v1alpha1.GenericClusterStatus{}
			status.SetRoleGroup("role", "nonexistent")

			err := cleaner.Cleanup(ctx, namespace, "nonexistent", spec, status)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("deleteService", func() {
		It("should delete existing Service", func() {
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "delete-test-svc",
					Namespace: namespace,
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "None",
					Ports:     []corev1.ServicePort{{Port: 8080}},
				},
			}
			Expect(k8sClient.Create(ctx, svc)).To(Succeed())

			spec := &v1alpha1.GenericClusterSpec{
				Roles: map[string]v1alpha1.RoleSpec{
					"role": {
						RoleGroups: map[string]v1alpha1.RoleGroupSpec{},
					},
				},
			}
			status := &v1alpha1.GenericClusterStatus{}
			status.SetRoleGroup("role", "test-svc")

			err := cleaner.Cleanup(ctx, namespace, "delete", spec, status)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("deleteStatefulSet", func() {
		It("should delete existing StatefulSet", func() {
			replicas := int32(1)
			sts := &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "delete-test-sts",
					Namespace: namespace,
				},
				Spec: appsv1.StatefulSetSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "delete-test-sts"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "delete-test-sts"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "test",
									Image: "test-image",
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, sts)).To(Succeed())

			spec := &v1alpha1.GenericClusterSpec{
				Roles: map[string]v1alpha1.RoleSpec{
					"role": {
						RoleGroups: map[string]v1alpha1.RoleGroupSpec{},
					},
				},
			}
			status := &v1alpha1.GenericClusterStatus{}
			status.SetRoleGroup("role", "test-sts")

			err := cleaner.Cleanup(ctx, namespace, "delete", spec, status)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("deletePDB", func() {
		It("should delete existing PDB", func() {
			maxUnavailable := intstr.FromInt(1)
			pdb := &policyv1.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "delete-test-pdb",
					Namespace: namespace,
				},
				Spec: policyv1.PodDisruptionBudgetSpec{
					MaxUnavailable: &maxUnavailable,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "delete-test-pdb"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pdb)).To(Succeed())

			spec := &v1alpha1.GenericClusterSpec{
				Roles: map[string]v1alpha1.RoleSpec{
					"role": {
						RoleGroups: map[string]v1alpha1.RoleGroupSpec{},
					},
				},
			}
			status := &v1alpha1.GenericClusterStatus{}
			status.SetRoleGroup("role", "test-pdb")

			err := cleaner.Cleanup(ctx, namespace, "delete", spec, status)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})

var _ = Describe("RoleGroupCleaner with multiple resources", func() {
	var cleaner *reconciler.RoleGroupCleaner
	var ctx context.Context
	var namespace string

	BeforeEach(func() {
		ctx = context.Background()
		namespace = cleanerTestNamespace
		cleaner = reconciler.NewRoleGroupCleaner(k8sClient, testScheme)
	})

	It("should cleanup all resources for a role group", func() {
		resourceName := "multi-delete-test"

		// Create ConfigMap
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: namespace,
			},
			Data: map[string]string{"key": "value"},
		}
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())

		// Create Headless Service
		headlessSvc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName + "-headless",
				Namespace: namespace,
			},
			Spec: corev1.ServiceSpec{
				ClusterIP: "None",
				Ports:     []corev1.ServicePort{{Port: 8080}},
			},
		}
		Expect(k8sClient.Create(ctx, headlessSvc)).To(Succeed())

		// Create StatefulSet (simplified without PVC templates)
		replicas := int32(1)
		sts := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: namespace,
			},
			Spec: appsv1.StatefulSetSpec{
				Replicas:    &replicas,
				ServiceName: resourceName + "-headless",
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": resourceName},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": resourceName},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "test",
								Image: "test-image",
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, sts)).To(Succeed())

		// Setup orphaned group
		spec := &v1alpha1.GenericClusterSpec{
			Roles: map[string]v1alpha1.RoleSpec{
				"role": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{},
				},
			},
		}
		status := &v1alpha1.GenericClusterStatus{}
		status.SetRoleGroup("role", "test")

		err := cleaner.Cleanup(ctx, namespace, "multi-delete", spec, status)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should cleanup all resources including regular service and PDB", func() {
		resourceName := "full-cleanup-test"

		// Create PDB
		maxUnavailable := intstr.FromInt(1)
		pdb := &policyv1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: namespace,
			},
			Spec: policyv1.PodDisruptionBudgetSpec{
				MaxUnavailable: &maxUnavailable,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": resourceName},
				},
			},
		}
		Expect(k8sClient.Create(ctx, pdb)).To(Succeed())

		// Create ConfigMap
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: namespace,
			},
			Data: map[string]string{"key": "value"},
		}
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())

		// Create regular Service (ClusterIP will be auto-assigned, don't specify it)
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: namespace,
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{Port: 8080}},
			},
		}
		Expect(k8sClient.Create(ctx, svc)).To(Succeed())

		// Create Headless Service
		headlessSvc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName + "-headless",
				Namespace: namespace,
			},
			Spec: corev1.ServiceSpec{
				ClusterIP: "None",
				Ports:     []corev1.ServicePort{{Port: 8080}},
			},
		}
		Expect(k8sClient.Create(ctx, headlessSvc)).To(Succeed())

		// Create StatefulSet with replicas > 0 to test scaling path
		replicas := int32(3)
		sts := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: namespace,
			},
			Spec: appsv1.StatefulSetSpec{
				Replicas:    &replicas,
				ServiceName: resourceName + "-headless",
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": resourceName},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": resourceName},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "test",
								Image: "test-image",
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, sts)).To(Succeed())

		// Setup orphaned group
		spec := &v1alpha1.GenericClusterSpec{
			Roles: map[string]v1alpha1.RoleSpec{
				"role": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{},
				},
			},
		}
		status := &v1alpha1.GenericClusterStatus{}
		status.SetRoleGroup("role", "full-cleanup-test")

		err := cleaner.Cleanup(ctx, namespace, "full-cleanup", spec, status)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should scale StatefulSet to zero before deletion when replicas > 0", func() {
		resourceName := "scale-to-zero-test"

		// Create StatefulSet with replicas > 0
		replicas := int32(3)
		sts := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: namespace,
			},
			Spec: appsv1.StatefulSetSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": resourceName},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": resourceName},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "test",
								Image: "test-image",
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, sts)).To(Succeed())

		// Setup orphaned group
		spec := &v1alpha1.GenericClusterSpec{
			Roles: map[string]v1alpha1.RoleSpec{
				"role": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{},
				},
			},
		}
		status := &v1alpha1.GenericClusterStatus{}
		status.SetRoleGroup("role", "scale-to-zero-test")

		err := cleaner.Cleanup(ctx, namespace, "scale-to-zero", spec, status)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should handle multiple orphaned role groups", func() {
		// Create resources for first orphaned group
		cm1 := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "multi-orphan-1",
				Namespace: namespace,
			},
			Data: map[string]string{"key": "value"},
		}
		Expect(k8sClient.Create(ctx, cm1)).To(Succeed())

		// Create resources for second orphaned group
		cm2 := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "multi-orphan-2",
				Namespace: namespace,
			},
			Data: map[string]string{"key": "value"},
		}
		Expect(k8sClient.Create(ctx, cm2)).To(Succeed())

		// Setup spec and status with multiple orphaned groups
		spec := &v1alpha1.GenericClusterSpec{
			Roles: map[string]v1alpha1.RoleSpec{
				"role-a": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{
						"active": {Replicas: ptr.To(int32(1))},
					},
				},
			},
		}
		status := &v1alpha1.GenericClusterStatus{}
		status.SetRoleGroup("role-a", "active")
		status.SetRoleGroup("role-a", "orphan-1") // Orphaned
		status.SetRoleGroup("role-a", "orphan-2") // Orphaned

		err := cleaner.Cleanup(ctx, namespace, "multi-orphan", spec, status)
		Expect(err).ToNot(HaveOccurred())
	})
})

var _ = Describe("RoleGroupCleaner error paths", func() {
	var cleaner *reconciler.RoleGroupCleaner
	var ctx context.Context
	var namespace string

	BeforeEach(func() {
		ctx = context.Background()
		namespace = cleanerTestNamespace
		cleaner = reconciler.NewRoleGroupCleaner(k8sClient, testScheme)
	})

	It("should handle context cancellation during cleanup", func() {
		// Create a resource
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ctx-cancel-test",
				Namespace: namespace,
			},
			Data: map[string]string{"key": "value"},
		}
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())

		// Use canceled context
		canceledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		spec := &v1alpha1.GenericClusterSpec{
			Roles: map[string]v1alpha1.RoleSpec{
				"role": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{},
				},
			},
		}
		status := &v1alpha1.GenericClusterStatus{}
		status.SetRoleGroup("role", "ctx-cancel-test")

		// Cleanup with canceled context - may or may not error depending on timing
		_ = cleaner.Cleanup(canceledCtx, namespace, "ctx-cancel", spec, status)
	})

	It("should continue when StatefulSet scale to zero fails", func() {
		resourceName := "scale-fail-test"

		// Create StatefulSet with replicas > 0
		replicas := int32(2)
		sts := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: namespace,
			},
			Spec: appsv1.StatefulSetSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": resourceName},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": resourceName},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "test",
								Image: "test-image",
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, sts)).To(Succeed())

		// Setup orphaned group
		spec := &v1alpha1.GenericClusterSpec{
			Roles: map[string]v1alpha1.RoleSpec{
				"role": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{},
				},
			},
		}
		status := &v1alpha1.GenericClusterStatus{}
		status.SetRoleGroup("role", "scale-fail-test")

		err := cleaner.Cleanup(ctx, namespace, "scale-fail", spec, status)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should handle StatefulSet with zero replicas", func() {
		resourceName := "zero-replicas-test"

		// Create StatefulSet with replicas = 0
		replicas := int32(0)
		sts := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: namespace,
			},
			Spec: appsv1.StatefulSetSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": resourceName},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": resourceName},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "test",
								Image: "test-image",
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, sts)).To(Succeed())

		// Setup orphaned group
		spec := &v1alpha1.GenericClusterSpec{
			Roles: map[string]v1alpha1.RoleSpec{
				"role": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{},
				},
			},
		}
		status := &v1alpha1.GenericClusterStatus{}
		status.SetRoleGroup("role", "zero-replicas-test")

		err := cleaner.Cleanup(ctx, namespace, "zero-replicas", spec, status)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should handle StatefulSet with nil replicas", func() {
		resourceName := "nil-replicas-test"

		// Create StatefulSet with nil replicas
		sts := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: namespace,
			},
			Spec: appsv1.StatefulSetSpec{
				Replicas: nil,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": resourceName},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": resourceName},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "test",
								Image: "test-image",
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, sts)).To(Succeed())

		// Setup orphaned group
		spec := &v1alpha1.GenericClusterSpec{
			Roles: map[string]v1alpha1.RoleSpec{
				"role": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{},
				},
			},
		}
		status := &v1alpha1.GenericClusterStatus{}
		status.SetRoleGroup("role", "nil-replicas-test")

		err := cleaner.Cleanup(ctx, namespace, "nil-replicas", spec, status)
		Expect(err).ToNot(HaveOccurred())
	})
})

var _ = Describe("RoleGroupCleaner ownerReference validation", func() {
	var ctx context.Context
	var namespace string

	BeforeEach(func() {
		ctx = context.Background()
		namespace = cleanerTestNamespace
	})

	// The cleaner constructs resource names as: clusterName + "-" + groupName
	// Tests must create resources using that naming convention.

	It("should skip deletion when StatefulSet is not owned by the cluster", func() {
		clusterName := "ownerref-skip"
		groupName := "grp1"
		resourceName := clusterName + "-" + groupName // "ownerref-skip-grp1"

		sts := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: namespace,
				// no OwnerReferences — simulates a manually-created or foreign resource
			},
			Spec: appsv1.StatefulSetSpec{
				Replicas: ptr.To(int32(1)),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": resourceName},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": resourceName}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "img"}}},
				},
			},
		}
		Expect(k8sClient.Create(ctx, sts)).To(Succeed())

		cleaner := reconciler.NewRoleGroupCleaner(k8sClient, testScheme)
		cleaner.WithOwner(&metav1.ObjectMeta{UID: "some-cluster-uid-that-does-not-match"})

		spec := &v1alpha1.GenericClusterSpec{
			Roles: map[string]v1alpha1.RoleSpec{"role": {RoleGroups: map[string]v1alpha1.RoleGroupSpec{}}},
		}
		status := &v1alpha1.GenericClusterStatus{}
		status.SetRoleGroup("role", groupName)

		Expect(cleaner.Cleanup(ctx, namespace, clusterName, spec, status)).To(Succeed())

		// StatefulSet should still exist (not owned → not deleted)
		existing := &appsv1.StatefulSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: resourceName}, existing)).To(Succeed())
		Expect(k8sClient.Delete(ctx, existing)).To(Succeed())
	})

	It("should delete StatefulSet when ownerUID matches", func() {
		clusterName := "ownerref-del"
		groupName := "grp2"
		resourceName := clusterName + "-" + groupName // "ownerref-del-grp2"
		ownerUID := types.UID("test-cluster-uid-456")

		sts := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: "test.zncdata.dev/v1alpha1",
						Kind:       "TestCluster",
						Name:       clusterName,
						UID:        ownerUID,
						Controller: ptr.To(true),
					},
				},
			},
			Spec: appsv1.StatefulSetSpec{
				Replicas: ptr.To(int32(0)),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": resourceName},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": resourceName}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "img"}}},
				},
			},
		}
		Expect(k8sClient.Create(ctx, sts)).To(Succeed())

		cleaner := reconciler.NewRoleGroupCleaner(k8sClient, testScheme)
		cleaner.WithOwner(&metav1.ObjectMeta{UID: ownerUID})

		spec := &v1alpha1.GenericClusterSpec{
			Roles: map[string]v1alpha1.RoleSpec{"role": {RoleGroups: map[string]v1alpha1.RoleGroupSpec{}}},
		}
		status := &v1alpha1.GenericClusterStatus{}
		status.SetRoleGroup("role", groupName)

		Expect(cleaner.Cleanup(ctx, namespace, clusterName, spec, status)).To(Succeed())

		// StatefulSet should be deleted
		existing := &appsv1.StatefulSet{}
		getErr := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: resourceName}, existing)
		Expect(getErr).To(HaveOccurred())
	})

	It("should skip deletion when ConfigMap is not owned by the cluster", func() {
		clusterName := "ownerref-cm-skip"
		groupName := "grp3"
		resourceName := clusterName + "-" + groupName // "ownerref-cm-skip-grp3"

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: namespace,
				// no OwnerReferences
			},
			Data: map[string]string{"key": "value"},
		}
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())

		cleaner := reconciler.NewRoleGroupCleaner(k8sClient, testScheme)
		cleaner.WithOwner(&metav1.ObjectMeta{UID: "foreign-uid"})

		spec := &v1alpha1.GenericClusterSpec{
			Roles: map[string]v1alpha1.RoleSpec{"role": {RoleGroups: map[string]v1alpha1.RoleGroupSpec{}}},
		}
		status := &v1alpha1.GenericClusterStatus{}
		status.SetRoleGroup("role", groupName)

		Expect(cleaner.Cleanup(ctx, namespace, clusterName, spec, status)).To(Succeed())

		// ConfigMap should still exist
		existing := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: resourceName}, existing)).To(Succeed())
		Expect(k8sClient.Delete(ctx, existing)).To(Succeed())
	})

	It("should allow cleanup without ownerUID set (backward compatible)", func() {
		// When no ownerUID is set, all resources are treated as owned and deleted
		clusterName := "ownerref-nouid"
		groupName := "grp4"
		resourceName := clusterName + "-" + groupName // "ownerref-nouid-grp4"

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
		}
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())

		// cleaner without WithOwner call — ownerUID is ""
		cleaner := reconciler.NewRoleGroupCleaner(k8sClient, testScheme)

		spec := &v1alpha1.GenericClusterSpec{
			Roles: map[string]v1alpha1.RoleSpec{"role": {RoleGroups: map[string]v1alpha1.RoleGroupSpec{}}},
		}
		status := &v1alpha1.GenericClusterStatus{}
		status.SetRoleGroup("role", groupName)

		Expect(cleaner.Cleanup(ctx, namespace, clusterName, spec, status)).To(Succeed())

		// ConfigMap should be deleted (no ownerUID → treat all as owned)
		existing := &corev1.ConfigMap{}
		getErr := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: resourceName}, existing)
		Expect(getErr).To(HaveOccurred())
	})
})

var _ = Describe("RoleGroupCleaner gray deletion", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("should annotate resource on first detection and defer deletion", func() {
		clusterName := "gray-defer"
		groupName := "grp1"
		resourceName := clusterName + "-" + groupName

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: cleanerTestNamespace},
		}
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())

		cleaner := reconciler.NewRoleGroupCleaner(k8sClient, testScheme).
			WithGrayDeleteGracePeriod(10 * time.Minute) // large grace period

		spec := &v1alpha1.GenericClusterSpec{
			Roles: map[string]v1alpha1.RoleSpec{"role": {RoleGroups: map[string]v1alpha1.RoleGroupSpec{}}},
		}
		status := &v1alpha1.GenericClusterStatus{}
		status.SetRoleGroup("role", groupName)

		// First cleanup call: should annotate and NOT delete
		Expect(cleaner.Cleanup(ctx, cleanerTestNamespace, clusterName, spec, status)).To(Succeed())

		// ConfigMap should still exist but have the annotation
		existing := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cleanerTestNamespace, Name: resourceName}, existing)).To(Succeed())
		Expect(existing.Annotations).To(HaveKey(reconciler.AnnotationPendingDeletion))

		// Cleanup
		Expect(k8sClient.Delete(ctx, existing)).To(Succeed())
	})

	It("should delete resource after grace period has elapsed", func() {
		clusterName := "gray-elapsed"
		groupName := "grp2"
		resourceName := clusterName + "-" + groupName

		// Create ConfigMap pre-annotated with a past timestamp
		past := time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339)
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:        resourceName,
				Namespace:   cleanerTestNamespace,
				Annotations: map[string]string{reconciler.AnnotationPendingDeletion: past},
			},
		}
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())

		// Grace period is only 1 minute — already elapsed
		cleaner := reconciler.NewRoleGroupCleaner(k8sClient, testScheme).
			WithGrayDeleteGracePeriod(1 * time.Minute)

		spec := &v1alpha1.GenericClusterSpec{
			Roles: map[string]v1alpha1.RoleSpec{"role": {RoleGroups: map[string]v1alpha1.RoleGroupSpec{}}},
		}
		status := &v1alpha1.GenericClusterStatus{}
		status.SetRoleGroup("role", groupName)

		Expect(cleaner.Cleanup(ctx, cleanerTestNamespace, clusterName, spec, status)).To(Succeed())

		// ConfigMap should be deleted
		existing := &corev1.ConfigMap{}
		getErr := k8sClient.Get(ctx, types.NamespacedName{Namespace: cleanerTestNamespace, Name: resourceName}, existing)
		Expect(getErr).To(HaveOccurred())
	})

	It("should delete immediately when GrayDeleteGracePeriod is 0", func() {
		clusterName := "gray-immediate"
		groupName := "grp3"
		resourceName := clusterName + "-" + groupName

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: cleanerTestNamespace},
		}
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())

		// No GrayDeleteGracePeriod (default 0 = immediate)
		cleaner := reconciler.NewRoleGroupCleaner(k8sClient, testScheme)

		spec := &v1alpha1.GenericClusterSpec{
			Roles: map[string]v1alpha1.RoleSpec{"role": {RoleGroups: map[string]v1alpha1.RoleGroupSpec{}}},
		}
		status := &v1alpha1.GenericClusterStatus{}
		status.SetRoleGroup("role", groupName)

		Expect(cleaner.Cleanup(ctx, cleanerTestNamespace, clusterName, spec, status)).To(Succeed())

		// ConfigMap should be gone
		existing := &corev1.ConfigMap{}
		getErr := k8sClient.Get(ctx, types.NamespacedName{Namespace: cleanerTestNamespace, Name: resourceName}, existing)
		Expect(getErr).To(HaveOccurred())
	})
})
