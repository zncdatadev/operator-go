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

package util_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ = Describe("K8sUtil", func() {
	var (
		k8sUtil *util.K8sUtil
	)

	BeforeEach(func() {
		k8sUtil = util.NewK8sUtil(k8sClient, testScheme)
	})

	Describe("NewK8sUtil", func() {
		It("should create a new K8sUtil", func() {
			Expect(k8sUtil).NotTo(BeNil())
		})
	})

	Describe("CreateOrUpdate", func() {
		It("should create a new resource", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cm",
					Namespace: "default",
				},
				Data: map[string]string{"key": "value"},
			}

			result, err := k8sUtil.CreateOrUpdate(ctx, cm, func() error {
				return nil
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(controllerutil.OperationResultCreated))
		})

		It("should update an existing resource", func() {
			// Create first
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cm-update",
					Namespace: "default",
				},
				Data: map[string]string{"key": "value"},
			}
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())

			// Update
			result, err := k8sUtil.CreateOrUpdate(ctx, cm, func() error {
				cm.Data["key"] = "updated"
				return nil
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(controllerutil.OperationResultUpdated))
		})
	})

	Describe("DeleteIfExists", func() {
		It("should delete an existing resource", func() {
			// Create first
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cm-delete",
					Namespace: "default",
				},
			}
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())

			// Delete
			err := k8sUtil.DeleteIfExists(ctx, cm)
			Expect(err).NotTo(HaveOccurred())

			// Verify deleted
			err = k8sClient.Get(ctx, types.NamespacedName{Name: "test-cm-delete", Namespace: "default"}, &corev1.ConfigMap{})
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})

		It("should return nil if resource doesn't exist", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "non-existent",
					Namespace: "default",
				},
			}
			err := k8sUtil.DeleteIfExists(ctx, cm)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Get", func() {
		It("should retrieve a resource", func() {
			// Create first
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cm-get",
					Namespace: "default",
				},
				Data: map[string]string{"key": "value"},
			}
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())

			// Get
			result := &corev1.ConfigMap{}
			err := k8sUtil.Get(ctx, types.NamespacedName{Name: "test-cm-get", Namespace: "default"}, result)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Data["key"]).To(Equal("value"))
		})

		It("should return error if resource doesn't exist", func() {
			result := &corev1.ConfigMap{}
			err := k8sUtil.Get(ctx, types.NamespacedName{Name: "non-existent", Namespace: "default"}, result)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("List", func() {
		It("should list resources", func() {
			// Create some resources
			for i := range 3 {
				cm := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      string(rune('a' + i)),
						Namespace: "default",
					},
				}
				Expect(k8sClient.Create(ctx, cm)).To(Succeed())
			}

			// List
			list := &corev1.ConfigMapList{}
			err := k8sUtil.List(ctx, list, client.InNamespace("default"))
			Expect(err).NotTo(HaveOccurred())
			Expect(len(list.Items)).To(BeNumerically(">=", 3))
		})
	})

	Describe("ResourceExists", func() {
		It("should return true if resource exists", func() {
			// Create first
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cm-exists",
					Namespace: "default",
				},
			}
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())

			// Check
			exists, err := k8sUtil.ResourceExists(ctx, types.NamespacedName{Name: "test-cm-exists", Namespace: "default"}, &corev1.ConfigMap{})
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeTrue())
		})

		It("should return false if resource doesn't exist", func() {
			exists, err := k8sUtil.ResourceExists(ctx, types.NamespacedName{Name: "non-existent", Namespace: "default"}, &corev1.ConfigMap{})
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeFalse())
		})
	})

	Describe("Create", func() {
		It("should create a new resource", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cm-create",
					Namespace: "default",
				},
				Data: map[string]string{"key": "value"},
			}

			err := k8sUtil.Create(ctx, cm)
			Expect(err).NotTo(HaveOccurred())

			// Verify created
			result := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "test-cm-create", Namespace: "default"}, result)).To(Succeed())
			Expect(result.Data["key"]).To(Equal("value"))
		})
	})

	Describe("ApplyOwnership", func() {
		It("should set owner reference", func() {
			// Create owner
			owner := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "owner-cm",
					Namespace: "default",
				},
			}
			Expect(k8sClient.Create(ctx, owner)).To(Succeed())

			// Create owned resource
			owned := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "owned-cm",
					Namespace: "default",
				},
			}

			err := k8sUtil.ApplyOwnership(owner, owned)
			Expect(err).NotTo(HaveOccurred())
			Expect(owned.OwnerReferences).To(HaveLen(1))
		})
	})

	Describe("SetOwnerReference", func() {
		It("should set owner reference", func() {
			// Create owner
			owner := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "owner-ref-cm",
					Namespace: "default",
				},
			}
			Expect(k8sClient.Create(ctx, owner)).To(Succeed())

			// Create owned resource
			owned := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "owned-ref-cm",
					Namespace: "default",
				},
			}

			err := k8sUtil.SetOwnerReference(owner, owned)
			Expect(err).NotTo(HaveOccurred())
			Expect(owned.OwnerReferences).To(HaveLen(1))
		})

		It("should not add duplicate owner reference", func() {
			// Create owner
			owner := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "owner-dup-cm",
					Namespace: "default",
				},
			}
			Expect(k8sClient.Create(ctx, owner)).To(Succeed())

			// Create owned resource
			owned := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "owned-dup-cm",
					Namespace: "default",
				},
			}

			// Set twice
			Expect(k8sUtil.SetOwnerReference(owner, owned)).To(Succeed())
			Expect(k8sUtil.SetOwnerReference(owner, owned)).To(Succeed())
			Expect(owned.OwnerReferences).To(HaveLen(1))
		})
	})

	Describe("Update", func() {
		It("should update a resource", func() {
			// Create first
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cm-update-2",
					Namespace: "default",
				},
				Data: map[string]string{"key": "value"},
			}
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())

			// Update
			cm.Data["key"] = "updated"
			err := k8sUtil.Update(ctx, cm)
			Expect(err).NotTo(HaveOccurred())

			// Verify
			result := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "test-cm-update-2", Namespace: "default"}, result)).To(Succeed())
			Expect(result.Data["key"]).To(Equal("updated"))
		})
	})
})
