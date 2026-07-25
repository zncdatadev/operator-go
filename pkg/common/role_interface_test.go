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

package common_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/common"
	corev1 "k8s.io/api/core/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
)

var _ = Describe("RoleGroupInfo", func() {
	Describe("GetAffinity", func() {
		newRoleGroupInfo := func(config *v1alpha1.RoleGroupConfigSpec) *common.RoleGroupInfo {
			return &common.RoleGroupInfo{
				RoleName:      "namenode",
				RoleGroupName: "default",
				Spec:          v1alpha1.RoleGroupSpec{Config: config},
			}
		}

		It("should return nil when the role group has no config", func() {
			affinity, err := newRoleGroupInfo(nil).GetAffinity()
			Expect(err).ToNot(HaveOccurred())
			Expect(affinity).To(BeNil())
		})

		It("should return nil when the config declares no affinity", func() {
			affinity, err := newRoleGroupInfo(&v1alpha1.RoleGroupConfigSpec{}).GetAffinity()
			Expect(err).ToNot(HaveOccurred())
			Expect(affinity).To(BeNil())
		})

		It("should decode the affinity declared in the role group config", func() {
			declared := &corev1.Affinity{
				PodAntiAffinity: &corev1.PodAntiAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
						{TopologyKey: "kubernetes.io/hostname"},
					},
				},
			}
			raw, err := json.Marshal(declared)
			Expect(err).ToNot(HaveOccurred())

			affinity, err := newRoleGroupInfo(&v1alpha1.RoleGroupConfigSpec{
				Affinity: &k8sruntime.RawExtension{Raw: raw},
			}).GetAffinity()

			Expect(err).ToNot(HaveOccurred())
			Expect(affinity).To(Equal(declared))
		})

		It("should report invalid affinity instead of dropping it", func() {
			affinity, err := newRoleGroupInfo(&v1alpha1.RoleGroupConfigSpec{
				Affinity: &k8sruntime.RawExtension{Raw: []byte(`{"podAntiAffinity": [`)},
			}).GetAffinity()

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`role "namenode"`))
			Expect(err.Error()).To(ContainSubstring(`group "default"`))
			Expect(affinity).To(BeNil())
		})
	})
})
