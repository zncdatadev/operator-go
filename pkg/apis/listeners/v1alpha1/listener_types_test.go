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
	"k8s.io/utils/ptr"

	"github.com/zncdatadev/operator-go/pkg/apis/listeners/v1alpha1"
)

var _ = Describe("ListenerSpec", func() {

	Describe("PublishNotReadyAddresses", func() {
		It("serializes an explicit false so the CRD default cannot restore true", func() {
			spec := v1alpha1.ListenerSpec{
				ClassName:                "cluster-internal",
				PublishNotReadyAddresses: ptr.To(false),
			}

			data, err := json.Marshal(spec)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(ContainSubstring(`"publishNotReadyAddresses":false`))
		})

		It("is omitted when unset, leaving the CRD default in charge", func() {
			data, err := json.Marshal(v1alpha1.ListenerSpec{ClassName: "cluster-internal"})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).NotTo(ContainSubstring("publishNotReadyAddresses"))
		})
	})

	Describe("GetPublishNotReadyAddresses", func() {
		It("returns the documented default when unset", func() {
			spec := &v1alpha1.ListenerSpec{}
			Expect(spec.GetPublishNotReadyAddresses()).To(BeTrue())
		})

		It("returns the documented default on a nil spec", func() {
			var spec *v1alpha1.ListenerSpec
			Expect(spec.GetPublishNotReadyAddresses()).To(BeTrue())
		})

		It("returns the explicit value when set", func() {
			Expect((&v1alpha1.ListenerSpec{PublishNotReadyAddresses: ptr.To(false)}).GetPublishNotReadyAddresses()).To(BeFalse())
			Expect((&v1alpha1.ListenerSpec{PublishNotReadyAddresses: ptr.To(true)}).GetPublishNotReadyAddresses()).To(BeTrue())
		})
	})

	Describe("required fields", func() {
		It("always serializes className", func() {
			data, err := json.Marshal(v1alpha1.ListenerSpec{})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(ContainSubstring(`"className":""`))
		})
	})
})
