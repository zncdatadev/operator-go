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

package client_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/zncdatadev/operator-go/pkg/client"
)

var _ = Describe("Client", Serial, func() {
	resource := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-configmap",
			Namespace: "default",
		},
	}

	AfterEach(func() {
		obj := &corev1.ConfigMap{}
		if err := k8sClient.Get(ctx, ctrlclient.ObjectKeyFromObject(resource), obj); err == nil {
			Expect(k8sClient.Delete(ctx, obj)).To(Succeed())
		}
	})

	Describe("CreateDoesNotExist", func() {

		Context("CreateDoesNotExist", func() {
			It("should create a resource if it does not exist", func() {
				err := k8sClient.Get(ctx, ctrlclient.ObjectKeyFromObject(resource), resource.DeepCopy())
				Expect(apierrors.IsNotFound(err)).To(BeTrue())

				err = client.CreateDoesNotExist(ctx, k8sClient, resource.DeepCopy())
				Expect(err).NotTo(HaveOccurred())

				obj := resource.DeepCopy()
				err = k8sClient.Get(ctx, ctrlclient.ObjectKeyFromObject(resource), obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj.Name).To(Equal(resource.Name))
			})
		})
	})

	Describe("CreateOrUpdate", Serial, func() {

		Context("create a resource if it does not exist", func() {
			It("should create", func() {
				fmt.Println(resource.Name)
				mutant, err := client.CreateOrUpdate(ctx, k8sClient, resource.DeepCopy())
				Expect(err).NotTo(HaveOccurred())
				Expect(mutant).To(BeTrue())

				obj := &corev1.ConfigMap{}
				err = k8sClient.Get(ctx, ctrlclient.ObjectKeyFromObject(resource), obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj.Name).To(Equal(resource.Name))
			})
		})
		Context("update a resource", Serial, func() {
			It("should first create", func() {
				mutant, err := client.CreateOrUpdate(ctx, k8sClient, resource.DeepCopy())
				Expect(err).NotTo(HaveOccurred())
				Expect(mutant).To(BeTrue())
			})

			It("should update a resource if it already exists", func() {
				mutant, err := client.CreateOrUpdate(ctx, k8sClient, resource.DeepCopy())
				Expect(err).NotTo(HaveOccurred())
				Expect(mutant).To(BeTrue())

				resource.Data = map[string]string{
					"key": "value",
				}

				mutant, err = client.CreateOrUpdate(ctx, k8sClient, resource.DeepCopy())
				Expect(err).NotTo(HaveOccurred())
				Expect(mutant).To(BeTrue())

				obj := &corev1.ConfigMap{}
				err = k8sClient.Get(ctx, ctrlclient.ObjectKeyFromObject(resource), obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj.Name).To(Equal(resource.Name))
				Expect(obj.Data).To(HaveKeyWithValue("key", "value"))
			})
			It("should not update a resource if it has not changed", func() {
				mutant, err := client.CreateOrUpdate(ctx, k8sClient, resource.DeepCopy())
				Expect(err).NotTo(HaveOccurred())
				Expect(mutant).To(BeTrue())

				mutant, err = client.CreateOrUpdate(ctx, k8sClient, resource.DeepCopy())
				Expect(err).NotTo(HaveOccurred())
				Expect(mutant).To(BeFalse())
			})
		})
	})
})
