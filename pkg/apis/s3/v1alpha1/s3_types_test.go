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
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/zncdatadev/operator-go/pkg/apis/s3/v1alpha1"
)

var _ = Describe("S3 status", func() {

	// kubectl, kstatus and the commons GenericClusterStatus all key on `.status.conditions`.
	It("serializes S3ConnectionStatus conditions under the plural key", func() {
		status := v1alpha1.S3ConnectionStatus{
			Conditions: []metav1.Condition{{Type: "Available", Status: metav1.ConditionTrue}},
		}

		data, err := json.Marshal(status)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring(`"conditions"`))
		Expect(string(data)).NotTo(ContainSubstring(`"condition"`))
	})

	It("serializes S3BucketStatus conditions under the plural key", func() {
		status := v1alpha1.S3BucketStatus{
			Conditions: []metav1.Condition{{Type: "Available", Status: metav1.ConditionTrue}},
		}

		data, err := json.Marshal(status)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring(`"conditions"`))
		Expect(string(data)).NotTo(ContainSubstring(`"condition"`))
	})
})

var _ = Describe("S3 required fields", func() {

	It("always serializes the connection host", func() {
		data, err := json.Marshal(v1alpha1.S3ConnectionSpec{})
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring(`"host":""`))
	})

	It("always serializes the bucket name", func() {
		data, err := json.Marshal(v1alpha1.S3BucketSpec{})
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring(`"bucketName":""`))
	})
})
