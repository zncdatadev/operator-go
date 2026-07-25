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
	stderrors "errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

			_, err := cleaner.Cleanup(ctx, namespace, "test-cluster", spec, status, "", nil)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return nil for empty roles", func() {
			spec := &v1alpha1.GenericClusterSpec{
				Roles: map[string]v1alpha1.RoleSpec{},
			}
			status := &v1alpha1.GenericClusterStatus{}

			_, err := cleaner.Cleanup(ctx, namespace, "test-cluster", spec, status, "", nil)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return nil for nil roles", func() {
			spec := &v1alpha1.GenericClusterSpec{
				Roles: nil,
			}
			status := &v1alpha1.GenericClusterStatus{}

			_, err := cleaner.Cleanup(ctx, namespace, "test-cluster", spec, status, "", nil)
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

			_, err := cleaner.Cleanup(ctx, namespace, "test-cluster", spec, status, "", nil)
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

			_, err := cleaner.Cleanup(ctx, namespace, "test-cluster", spec, status, "", nil)
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

			_, err := cleaner.Cleanup(ctx, namespace, "test-cluster", spec, status, "", nil)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should cleanup orphaned role group resources", func() {
			// Create resources for an orphaned role group
			resourceName := reconciler.RoleGroupResourceName("cleanup-test", "test-role", "orphaned")

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

			_, err := cleaner.Cleanup(ctx, namespace, "cleanup-test", spec, status, "", nil)
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

			_, err := cleaner.Cleanup(ctx, namespace, "delete", spec, status, "", nil)
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

			_, err := cleaner.Cleanup(ctx, namespace, "nonexistent", spec, status, "", nil)
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

			_, err := cleaner.Cleanup(ctx, namespace, "delete", spec, status, "", nil)
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

			_, err := cleaner.Cleanup(ctx, namespace, "delete", spec, status, "", nil)
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

			_, err := cleaner.Cleanup(ctx, namespace, "delete", spec, status, "", nil)
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

		_, err := cleaner.Cleanup(ctx, namespace, "multi-delete", spec, status, "", nil)
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

		_, err := cleaner.Cleanup(ctx, namespace, "full-cleanup", spec, status, "", nil)
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

		_, err := cleaner.Cleanup(ctx, namespace, "scale-to-zero", spec, status, "", nil)
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

		_, err := cleaner.Cleanup(ctx, namespace, "multi-orphan", spec, status, "", nil)
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
		_, _ = cleaner.Cleanup(canceledCtx, namespace, "ctx-cancel", spec, status, "", nil)
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

		_, err := cleaner.Cleanup(ctx, namespace, "scale-fail", spec, status, "", nil)
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

		_, err := cleaner.Cleanup(ctx, namespace, "zero-replicas", spec, status, "", nil)
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

		_, err := cleaner.Cleanup(ctx, namespace, "nil-replicas", spec, status, "", nil)
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

	// The cleaner constructs resource names as: reconciler.RoleGroupResourceName(clusterName, "role", groupName)
	// Tests must create resources using that naming convention.

	It("should skip deletion when StatefulSet is not owned by the cluster", func() {
		clusterName := "ownerref-skip"
		groupName := "grp1"
		resourceName := reconciler.RoleGroupResourceName(clusterName, "role", groupName) // "ownerref-skip-grp1"

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

		spec := &v1alpha1.GenericClusterSpec{
			Roles: map[string]v1alpha1.RoleSpec{"role": {RoleGroups: map[string]v1alpha1.RoleGroupSpec{}}},
		}
		status := &v1alpha1.GenericClusterStatus{}
		status.SetRoleGroup("role", groupName)

		_, cleanupErr := cleaner.Cleanup(ctx, namespace, clusterName, spec, status, "some-cluster-uid-that-does-not-match", nil)
		Expect(cleanupErr).To(Succeed())

		// StatefulSet should still exist (not owned → not deleted)
		existing := &appsv1.StatefulSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: resourceName}, existing)).To(Succeed())
		Expect(k8sClient.Delete(ctx, existing)).To(Succeed())
	})

	It("should delete StatefulSet when ownerUID matches", func() {
		clusterName := "ownerref-del"
		groupName := "grp2"
		resourceName := reconciler.RoleGroupResourceName(clusterName, "role", groupName) // "ownerref-del-grp2"
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

		spec := &v1alpha1.GenericClusterSpec{
			Roles: map[string]v1alpha1.RoleSpec{"role": {RoleGroups: map[string]v1alpha1.RoleGroupSpec{}}},
		}
		status := &v1alpha1.GenericClusterStatus{}
		status.SetRoleGroup("role", groupName)

		_, cleanupErr := cleaner.Cleanup(ctx, namespace, clusterName, spec, status, ownerUID, nil)
		Expect(cleanupErr).To(Succeed())

		// StatefulSet should be deleted
		existing := &appsv1.StatefulSet{}
		getErr := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: resourceName}, existing)
		Expect(getErr).To(HaveOccurred())
	})

	It("should skip deletion when ConfigMap is not owned by the cluster", func() {
		clusterName := "ownerref-cm-skip"
		groupName := "grp3"
		resourceName := reconciler.RoleGroupResourceName(clusterName, "role", groupName) // "ownerref-cm-skip-grp3"

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

		spec := &v1alpha1.GenericClusterSpec{
			Roles: map[string]v1alpha1.RoleSpec{"role": {RoleGroups: map[string]v1alpha1.RoleGroupSpec{}}},
		}
		status := &v1alpha1.GenericClusterStatus{}
		status.SetRoleGroup("role", groupName)

		_, cleanupErr := cleaner.Cleanup(ctx, namespace, clusterName, spec, status, "foreign-uid", nil)
		Expect(cleanupErr).To(Succeed())

		// ConfigMap should still exist
		existing := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: resourceName}, existing)).To(Succeed())
		Expect(k8sClient.Delete(ctx, existing)).To(Succeed())
	})

	It("should allow cleanup without ownerUID set (backward compatible)", func() {
		// When no ownerUID is set, all resources are treated as owned and deleted
		clusterName := "ownerref-nouid"
		groupName := "grp4"
		resourceName := reconciler.RoleGroupResourceName(clusterName, "role", groupName) // "ownerref-nouid-grp4"

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
		}
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())

		// cleaner without ownerUID — pass "" to treat all resources as owned (backward compatible)
		cleaner := reconciler.NewRoleGroupCleaner(k8sClient, testScheme)

		spec := &v1alpha1.GenericClusterSpec{
			Roles: map[string]v1alpha1.RoleSpec{"role": {RoleGroups: map[string]v1alpha1.RoleGroupSpec{}}},
		}
		status := &v1alpha1.GenericClusterStatus{}
		status.SetRoleGroup("role", groupName)

		_, cleanupErr := cleaner.Cleanup(ctx, namespace, clusterName, spec, status, "", nil)
		Expect(cleanupErr).To(Succeed())

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
		resourceName := reconciler.RoleGroupResourceName(clusterName, "role", groupName)

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
		_, cleanupErr := cleaner.Cleanup(ctx, cleanerTestNamespace, clusterName, spec, status, "", nil)
		Expect(cleanupErr).To(Succeed())

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
		resourceName := reconciler.RoleGroupResourceName(clusterName, "role", groupName)

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

		_, cleanupErr := cleaner.Cleanup(ctx, cleanerTestNamespace, clusterName, spec, status, "", nil)
		Expect(cleanupErr).To(Succeed())

		// ConfigMap should be deleted
		existing := &corev1.ConfigMap{}
		getErr := k8sClient.Get(ctx, types.NamespacedName{Namespace: cleanerTestNamespace, Name: resourceName}, existing)
		Expect(getErr).To(HaveOccurred())
	})

	It("should delete immediately when GrayDeleteGracePeriod is 0", func() {
		clusterName := "gray-immediate"
		groupName := "grp3"
		resourceName := reconciler.RoleGroupResourceName(clusterName, "role", groupName)

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

		_, cleanupErr := cleaner.Cleanup(ctx, cleanerTestNamespace, clusterName, spec, status, "", nil)
		Expect(cleanupErr).To(Succeed())

		// ConfigMap should be gone
		existing := &corev1.ConfigMap{}
		getErr := k8sClient.Get(ctx, types.NamespacedName{Namespace: cleanerTestNamespace, Name: resourceName}, existing)
		Expect(getErr).To(HaveOccurred())
	})

	It("reports the remaining grace period so the caller can requeue", func() {
		clusterName := "gray-requeue"
		groupName := "grp4"
		resourceName := reconciler.RoleGroupResourceName(clusterName, "role", groupName)

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: cleanerTestNamespace},
		}
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, cm)
		})

		gracePeriod := 10 * time.Minute
		cleaner := reconciler.NewRoleGroupCleaner(k8sClient, testScheme).
			WithGrayDeleteGracePeriod(gracePeriod)

		spec := &v1alpha1.GenericClusterSpec{
			Roles: map[string]v1alpha1.RoleSpec{"role": {RoleGroups: map[string]v1alpha1.RoleGroupSpec{}}},
		}
		status := &v1alpha1.GenericClusterStatus{}
		status.SetRoleGroup("role", groupName)

		// First pass marks the resource: the whole grace period is still ahead.
		requeue, err := cleaner.Cleanup(ctx, cleanerTestNamespace, clusterName, spec, status, "", nil)
		Expect(err).To(Succeed())
		Expect(requeue).To(Equal(gracePeriod))

		// Second pass sees the annotation and reports what is left of the grace period;
		// without it the deferred deletion would wait for an unrelated event.
		requeue, err = cleaner.Cleanup(ctx, cleanerTestNamespace, clusterName, spec, status, "", nil)
		Expect(err).To(Succeed())
		Expect(requeue).To(BeNumerically(">", 0))
		Expect(requeue).To(BeNumerically("<=", gracePeriod+time.Second))
	})
})

var _ = Describe("RoleGroupCleaner status pruning", func() {
	var ctx context.Context
	var spec *v1alpha1.GenericClusterSpec

	BeforeEach(func() {
		ctx = context.Background()
		// "role" survives with group "keep"; every other tracked group is orphaned.
		spec = &v1alpha1.GenericClusterSpec{
			Roles: map[string]v1alpha1.RoleSpec{
				"role": {
					RoleGroups: map[string]v1alpha1.RoleGroupSpec{
						"keep": {Replicas: ptr.To(int32(1))},
					},
				},
			},
		}
	})

	It("removes a really-deleted role group from the status snapshot", func() {
		clusterName := "prune-deleted"
		resourceName := reconciler.RoleGroupResourceName(clusterName, "role", "gone")
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: cleanerTestNamespace},
		}
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())

		status := &v1alpha1.GenericClusterStatus{}
		status.SetRoleGroup("role", "keep")
		status.SetRoleGroup("role", "gone")

		cleaner := reconciler.NewRoleGroupCleaner(k8sClient, testScheme)
		_, err := cleaner.Cleanup(ctx, cleanerTestNamespace, clusterName, spec, status, "", nil)
		Expect(err).To(Succeed())

		// Status.RoleGroups must converge to the desired set, otherwise the orphan is
		// re-processed on every reconcile forever and the reported state stays wrong.
		Expect(status.GetRoleGroups()).To(Equal(map[string][]string{"role": {"keep"}}))
	})

	It("keeps a role group whose deletion is still deferred by the grace period", func() {
		clusterName := "prune-deferred"
		resourceName := reconciler.RoleGroupResourceName(clusterName, "role", "pending")
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: cleanerTestNamespace},
		}
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, cm)
		})

		status := &v1alpha1.GenericClusterStatus{}
		status.SetRoleGroup("role", "keep")
		status.SetRoleGroup("role", "pending")

		cleaner := reconciler.NewRoleGroupCleaner(k8sClient, testScheme).
			WithGrayDeleteGracePeriod(10 * time.Minute)
		_, err := cleaner.Cleanup(ctx, cleanerTestNamespace, clusterName, spec, status, "", nil)
		Expect(err).To(Succeed())

		// The resources are still there, so the group must stay tracked — dropping it would
		// make the next reconcile forget to finish the deletion.
		Expect(status.GetRoleGroups()).To(Equal(map[string][]string{"role": {"keep", "pending"}}))
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cleanerTestNamespace, Name: resourceName}, &corev1.ConfigMap{})).To(Succeed())
	})
})

var _ = Describe("RoleGroupCleaner metrics Service and events", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("deletes the metrics Service of an orphaned role group and emits Deleted events", func() {
		clusterName := "metrics-cleanup"
		resourceName := reconciler.RoleGroupResourceName(clusterName, "role", "gone")

		newService := func(name string) *corev1.Service {
			return &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: cleanerTestNamespace},
				Spec: corev1.ServiceSpec{
					ClusterIP: corev1.ClusterIPNone,
					Ports:     []corev1.ServicePort{{Port: 9090, Name: "metrics"}},
				},
			}
		}
		Expect(k8sClient.Create(ctx, newService(resourceName))).To(Succeed())
		Expect(k8sClient.Create(ctx, newService(resourceName+"-headless"))).To(Succeed())
		Expect(k8sClient.Create(ctx, newService(resourceName+"-metrics"))).To(Succeed())

		fakeRecorder := record.NewFakeRecorder(100)
		cleaner := reconciler.NewRoleGroupCleaner(k8sClient, testScheme).
			WithEventManager(reconciler.NewEventManager(fakeRecorder))

		spec := &v1alpha1.GenericClusterSpec{
			Roles: map[string]v1alpha1.RoleSpec{"role": {RoleGroups: map[string]v1alpha1.RoleGroupSpec{}}},
		}
		status := &v1alpha1.GenericClusterStatus{}
		status.SetRoleGroup("role", "gone")

		_, err := cleaner.Cleanup(ctx, cleanerTestNamespace, clusterName, spec, status, "", nil)
		Expect(err).To(Succeed())

		// The metrics Service is a framework slot like the other two; leaving it behind keeps a
		// Prometheus target for a role group that no longer exists.
		for _, name := range []string{resourceName, resourceName + "-headless", resourceName + "-metrics"} {
			getErr := k8sClient.Get(ctx, types.NamespacedName{Namespace: cleanerTestNamespace, Name: name}, &corev1.Service{})
			Expect(getErr).To(HaveOccurred(), "expected Service %s to be deleted", name)
		}

		var events []string
		for {
			select {
			case e := <-fakeRecorder.Events:
				events = append(events, e)
				continue
			default:
			}
			break
		}
		Expect(events).To(HaveLen(3))
		Expect(events).To(ContainElement(SatisfyAll(
			ContainSubstring("Deleted"),
			ContainSubstring(resourceName+"-metrics"),
			ContainSubstring(clusterName),
		)))
	})
})

// orphanTestStatefulSet returns a minimal StatefulSet the cleaner can drive through its drain.
func orphanTestStatefulSet(name string, replicas int32) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: cleanerTestNamespace},
		Spec: appsv1.StatefulSetSpec{
			Replicas: ptr.To(replicas),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": name}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "img"}}},
			},
		},
	}
}

// orphanedGroupSpec is a spec whose single role declares no role group at all, so every group
// tracked in the status is orphaned.
func orphanedGroupSpec() *v1alpha1.GenericClusterSpec {
	return &v1alpha1.GenericClusterSpec{
		Roles: map[string]v1alpha1.RoleSpec{"role": {RoleGroups: map[string]v1alpha1.RoleGroupSpec{}}},
	}
}

var _ = Describe("RoleGroupCleaner ordered drain", func() {
	var ctx context.Context
	const pollInterval = 2 * time.Second

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("scales the StatefulSet to zero and defers its deletion to a later pass", func() {
		clusterName := "drain-scale"
		resourceName := reconciler.RoleGroupResourceName(clusterName, "role", "gone")
		sts := orphanTestStatefulSet(resourceName, 3)
		Expect(k8sClient.Create(ctx, sts)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, sts)
		})

		status := &v1alpha1.GenericClusterStatus{}
		status.SetRoleGroup("role", "gone")
		cleaner := reconciler.NewRoleGroupCleaner(k8sClient, testScheme).WithDrainPollInterval(pollInterval)

		requeue, err := cleaner.Cleanup(ctx, cleanerTestNamespace, clusterName, orphanedGroupSpec(), status, "", nil)
		Expect(err).To(Succeed())
		// Deleting in the same pass hands the pods to cascade garbage collection, which tears them
		// down in arbitrary order instead of the StatefulSet controller's ordered drain.
		Expect(requeue).To(Equal(pollInterval))

		live := &appsv1.StatefulSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cleanerTestNamespace, Name: resourceName}, live)).To(Succeed())
		Expect(live.Spec.Replicas).To(HaveValue(Equal(int32(0))))
		Expect(status.GetRoleGroups()).To(HaveKeyWithValue("role", ConsistOf("gone")))

		// Once the drain is done the next pass finishes the deletion and the group leaves the
		// status snapshot.
		requeue, err = cleaner.Cleanup(ctx, cleanerTestNamespace, clusterName, orphanedGroupSpec(), status, "", nil)
		Expect(err).To(Succeed())
		Expect(requeue).To(BeZero())
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cleanerTestNamespace, Name: resourceName}, live)).
			To(MatchError(k8serrors.IsNotFound, "IsNotFound"))
		Expect(status.GetRoleGroups()).To(BeEmpty())
	})

	It("waits for the pods to terminate before deleting the StatefulSet", func() {
		clusterName := "drain-wait"
		resourceName := reconciler.RoleGroupResourceName(clusterName, "role", "gone")
		sts := orphanTestStatefulSet(resourceName, 0)
		Expect(k8sClient.Create(ctx, sts)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, sts)
		})
		// envtest runs no StatefulSet controller, so the replica count that reports the drain is
		// written here instead: two pods are still on their way out.
		sts.Status.Replicas = 2
		Expect(k8sClient.Status().Update(ctx, sts)).To(Succeed())

		status := &v1alpha1.GenericClusterStatus{}
		status.SetRoleGroup("role", "gone")
		cleaner := reconciler.NewRoleGroupCleaner(k8sClient, testScheme).WithDrainPollInterval(pollInterval)

		requeue, err := cleaner.Cleanup(ctx, cleanerTestNamespace, clusterName, orphanedGroupSpec(), status, "", nil)
		Expect(err).To(Succeed())
		Expect(requeue).To(Equal(pollInterval))
		live := &appsv1.StatefulSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cleanerTestNamespace, Name: resourceName}, live)).To(Succeed())

		live.Status.Replicas = 0
		Expect(k8sClient.Status().Update(ctx, live)).To(Succeed())

		_, err = cleaner.Cleanup(ctx, cleanerTestNamespace, clusterName, orphanedGroupSpec(), status, "", nil)
		Expect(err).To(Succeed())
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cleanerTestNamespace, Name: resourceName}, live)).
			To(MatchError(k8serrors.IsNotFound, "IsNotFound"))
	})

	It("does not delete the next resource type until the previous one is really gone", func() {
		clusterName := "drain-confirm"
		resourceName := reconciler.RoleGroupResourceName(clusterName, "role", "gone")
		const holdFinalizer = "test.zncdata.dev/hold"

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:       resourceName,
				Namespace:  cleanerTestNamespace,
				Finalizers: []string{holdFinalizer},
			},
		}
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())
		releaseConfigMap := func() {
			live := &corev1.ConfigMap{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: cleanerTestNamespace, Name: resourceName}, live); err != nil {
				return
			}
			live.Finalizers = nil
			_ = k8sClient.Update(ctx, live)
		}
		DeferCleanup(func() {
			releaseConfigMap()
			_ = k8sClient.Delete(ctx, cm)
		})

		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: cleanerTestNamespace},
			Spec: corev1.ServiceSpec{
				ClusterIP: corev1.ClusterIPNone,
				Ports:     []corev1.ServicePort{{Port: 8080}},
			},
		}
		Expect(k8sClient.Create(ctx, svc)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, svc)
		})

		status := &v1alpha1.GenericClusterStatus{}
		status.SetRoleGroup("role", "gone")
		cleaner := reconciler.NewRoleGroupCleaner(k8sClient, testScheme).WithDrainPollInterval(pollInterval)

		requeue, err := cleaner.Cleanup(ctx, cleanerTestNamespace, clusterName, orphanedGroupSpec(), status, "", nil)
		Expect(err).To(Succeed())
		Expect(requeue).To(Equal(pollInterval))
		// The ConfigMap accepted the Delete but a finalizer still holds it, so the Service — which
		// the pods of this role group still resolve through — must not be dropped yet.
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cleanerTestNamespace, Name: resourceName}, &corev1.Service{})).To(Succeed())
		Expect(status.GetRoleGroups()).To(HaveKeyWithValue("role", ConsistOf("gone")))

		releaseConfigMap()
		_, err = cleaner.Cleanup(ctx, cleanerTestNamespace, clusterName, orphanedGroupSpec(), status, "", nil)
		Expect(err).To(Succeed())
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cleanerTestNamespace, Name: resourceName}, &corev1.Service{})).
			To(MatchError(k8serrors.IsNotFound, "IsNotFound"))
		Expect(status.GetRoleGroups()).To(BeEmpty())
	})
})

// conflictingUpdateClient answers the first n Update calls with a 409, reproducing another writer
// touching the StatefulSet between the cleaner's Get and its scale-down.
type conflictingUpdateClient struct {
	client.Client
	conflicts int
}

func (c *conflictingUpdateClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if c.conflicts > 0 {
		c.conflicts--
		return k8serrors.NewConflict(schema.GroupResource{Resource: "statefulsets"}, obj.GetName(),
			fmt.Errorf("simulated conflict"))
	}
	return c.Client.Update(ctx, obj, opts...)
}

// throttlingDeleteClient answers every Delete with a 429, as the API server does when the operator
// exceeds its request budget.
type throttlingDeleteClient struct {
	client.Client
}

func (c *throttlingDeleteClient) Delete(_ context.Context, _ client.Object, _ ...client.DeleteOption) error {
	return k8serrors.NewTooManyRequests("slow down", 1)
}

// failingDeleteClient rejects the deletion of one specific object name, so a single role group can
// be wedged while the others still make progress.
type failingDeleteClient struct {
	client.Client
	name string
}

func (c *failingDeleteClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if obj.GetName() == c.name {
		return k8serrors.NewInternalError(fmt.Errorf("simulated delete failure"))
	}
	return c.Client.Delete(ctx, obj, opts...)
}

var _ = Describe("RoleGroupCleaner API failure handling", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("retries the scale-down on conflict instead of failing the pass", func() {
		clusterName := "drain-conflict"
		resourceName := reconciler.RoleGroupResourceName(clusterName, "role", "gone")
		sts := orphanTestStatefulSet(resourceName, 2)
		Expect(k8sClient.Create(ctx, sts)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, sts)
		})

		status := &v1alpha1.GenericClusterStatus{}
		status.SetRoleGroup("role", "gone")
		conflicting := &conflictingUpdateClient{Client: k8sClient, conflicts: 2}
		cleaner := reconciler.NewRoleGroupCleaner(conflicting, testScheme)

		_, err := cleaner.Cleanup(ctx, cleanerTestNamespace, clusterName, orphanedGroupSpec(), status, "", nil)
		// A concurrent write to the StatefulSet is routine; it must not leave the role group
		// half-deleted with its pods still running.
		Expect(err).To(Succeed())
		Expect(conflicting.conflicts).To(BeZero())

		live := &appsv1.StatefulSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cleanerTestNamespace, Name: resourceName}, live)).To(Succeed())
		Expect(live.Spec.Replicas).To(HaveValue(Equal(int32(0))))
	})

	It("reports a 429 as a RateLimitError so the reconcile backs off", func() {
		clusterName := "cleanup-throttled"
		resourceName := reconciler.RoleGroupResourceName(clusterName, "role", "gone")
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: cleanerTestNamespace},
		}
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, cm)
		})

		status := &v1alpha1.GenericClusterStatus{}
		status.SetRoleGroup("role", "gone")
		cleaner := reconciler.NewRoleGroupCleaner(&throttlingDeleteClient{Client: k8sClient}, testScheme).
			WithRateLimitRetryAfter(7 * time.Second)

		_, err := cleaner.Cleanup(ctx, cleanerTestNamespace, clusterName, orphanedGroupSpec(), status, "", nil)
		Expect(err).To(HaveOccurred())
		// Throttling says nothing about the cluster's state: reporting it as a cleanup failure
		// marks a healthy cluster Degraded and sends the next pass into the same rejected request.
		var rateLimitErr *reconciler.RateLimitError
		Expect(stderrors.As(err, &rateLimitErr)).To(BeTrue())
		Expect(rateLimitErr.RetryAfter).To(Equal(7 * time.Second))
		Expect(status.GetRoleGroups()).To(HaveKeyWithValue("role", ConsistOf("gone")))
	})

	It("keeps cleaning the other role groups when one of them fails", func() {
		clusterName := "cleanup-isolated"
		badName := reconciler.RoleGroupResourceName(clusterName, "role", "bad")
		goodName := reconciler.RoleGroupResourceName(clusterName, "role", "good")

		for _, name := range []string{badName, goodName} {
			cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: cleanerTestNamespace}}
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, cm)
			})
		}

		status := &v1alpha1.GenericClusterStatus{}
		status.SetRoleGroup("role", "bad")
		status.SetRoleGroup("role", "good")
		cleaner := reconciler.NewRoleGroupCleaner(&failingDeleteClient{Client: k8sClient, name: badName}, testScheme)

		_, err := cleaner.Cleanup(ctx, cleanerTestNamespace, clusterName, orphanedGroupSpec(), status, "", nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("role/bad"))

		// "bad" is processed first: aborting the pass on the first failure would keep the healthy
		// orphan — and its status entry — alive for as long as the broken one exists.
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cleanerTestNamespace, Name: goodName}, &corev1.ConfigMap{})).
			To(MatchError(k8serrors.IsNotFound, "IsNotFound"))
		Expect(status.GetRoleGroups()).To(HaveKeyWithValue("role", ConsistOf("bad")))
	})
})

var _ = Describe("RoleGroupCleaner role PodDisruptionBudget reclaim", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	newRolePDB := func(name, roleName string, ownerUID types.UID, labelled bool) *policyv1.PodDisruptionBudget {
		labels := map[string]string{}
		if labelled {
			labels[reconciler.LabelRolePodDisruptionBudget] = roleName
		}
		return &policyv1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: cleanerTestNamespace,
				Labels:    labels,
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: "test.zncdata.dev/v1alpha1",
					Kind:       "MockCluster",
					Name:       "owner",
					UID:        ownerUID,
					Controller: ptr.To(true),
				}},
			},
			Spec: policyv1.PodDisruptionBudgetSpec{
				MaxUnavailable: ptr.To(intstr.FromInt32(1)),
				Selector:       &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			},
		}
	}

	It("deletes the PDB of a role that disappeared from the spec and keeps the others", func() {
		clusterName := "role-pdb"
		ownerUID := types.UID("role-pdb-owner-uid")

		removed := newRolePDB(reconciler.RoleResourceName(clusterName, "removed"), "removed", ownerUID, true)
		kept := newRolePDB(reconciler.RoleResourceName(clusterName, "kept"), "kept", ownerUID, true)
		// A product's own PDB carries the same controller owner reference, so only the framework's
		// role slot label can tell them apart.
		custom := newRolePDB(clusterName+"-custom", "removed", ownerUID, false)
		for _, pdb := range []*policyv1.PodDisruptionBudget{removed, kept, custom} {
			Expect(k8sClient.Create(ctx, pdb)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, pdb)
			})
		}

		spec := &v1alpha1.GenericClusterSpec{
			Roles: map[string]v1alpha1.RoleSpec{
				"kept": {RoleGroups: map[string]v1alpha1.RoleGroupSpec{"default": {Replicas: ptr.To(int32(1))}}},
			},
		}
		status := &v1alpha1.GenericClusterStatus{}
		status.SetRoleGroup("kept", "default")

		cleaner := reconciler.NewRoleGroupCleaner(k8sClient, testScheme)
		_, err := cleaner.Cleanup(ctx, cleanerTestNamespace, clusterName, spec, status, ownerUID, nil)
		Expect(err).To(Succeed())

		// Nothing else enumerates a role that is gone: its groups leave Status.RoleGroups as they
		// are cleaned, and the apply path only ever writes the PDB of a declared role.
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(removed), &policyv1.PodDisruptionBudget{})).
			To(MatchError(k8serrors.IsNotFound, "IsNotFound"))
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(kept), &policyv1.PodDisruptionBudget{})).To(Succeed())
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(custom), &policyv1.PodDisruptionBudget{})).To(Succeed())
	})

	It("leaves the role PDB of another cluster alone", func() {
		clusterName := "role-pdb-foreign"
		foreign := newRolePDB(reconciler.RoleResourceName(clusterName, "removed"), "removed", "some-other-cluster-uid", true)
		Expect(k8sClient.Create(ctx, foreign)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, foreign)
		})

		cleaner := reconciler.NewRoleGroupCleaner(k8sClient, testScheme)
		_, err := cleaner.Cleanup(ctx, cleanerTestNamespace, clusterName,
			&v1alpha1.GenericClusterSpec{Roles: map[string]v1alpha1.RoleSpec{}},
			&v1alpha1.GenericClusterStatus{}, types.UID("role-pdb-foreign-uid"), nil)
		Expect(err).To(Succeed())

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(foreign), &policyv1.PodDisruptionBudget{})).To(Succeed())
	})
})
