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

package security_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/security"
	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("PodSecurityBuilder", func() {
	var builder *security.PodSecurityBuilder

	BeforeEach(func() {
		builder = security.NewPodSecurityBuilder()
	})

	Describe("NewPodSecurityBuilder", func() {
		It("should create a new builder", func() {
			Expect(builder).NotTo(BeNil())
		})
	})

	Describe("WithRunAsUser", func() {
		It("should set run as user", func() {
			builder.WithRunAsUser(1000)
			ctx := builder.BuildSecurityContext()
			Expect(ctx.RunAsUser).NotTo(BeNil())
			Expect(*ctx.RunAsUser).To(Equal(int64(1000)))
		})
	})

	Describe("WithRunAsGroup", func() {
		It("should set run as group", func() {
			builder.WithRunAsGroup(1000)
			ctx := builder.BuildSecurityContext()
			Expect(ctx.RunAsGroup).NotTo(BeNil())
			Expect(*ctx.RunAsGroup).To(Equal(int64(1000)))
		})
	})

	Describe("WithFSGroup", func() {
		It("should set fs group", func() {
			builder.WithFSGroup(1000)
			ctx := builder.BuildPodSecurityContext()
			Expect(ctx.FSGroup).NotTo(BeNil())
			Expect(*ctx.FSGroup).To(Equal(int64(1000)))
		})
	})

	Describe("WithRunAsNonRoot", func() {
		It("should set run as non-root", func() {
			builder.WithRunAsNonRoot(true)
			ctx := builder.BuildSecurityContext()
			Expect(ctx.RunAsNonRoot).NotTo(BeNil())
			Expect(*ctx.RunAsNonRoot).To(BeTrue())
		})
	})

	Describe("WithSeccompProfile", func() {
		It("should set seccomp profile", func() {
			profile := &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			}
			builder.WithSeccompProfile(profile)
			ctx := builder.BuildSecurityContext()
			Expect(ctx.SeccompProfile).NotTo(BeNil())
			Expect(ctx.SeccompProfile.Type).To(Equal(corev1.SeccompProfileTypeRuntimeDefault))
		})
	})

	Describe("WithReadOnlyRootFS", func() {
		It("should set read-only root filesystem", func() {
			builder.WithReadOnlyRootFS(true)
			ctx := builder.BuildSecurityContext()
			Expect(ctx.ReadOnlyRootFilesystem).NotTo(BeNil())
			Expect(*ctx.ReadOnlyRootFilesystem).To(BeTrue())
		})
	})

	Describe("WithAllowPrivilegeEscalation", func() {
		It("should set allow privilege escalation", func() {
			builder.WithAllowPrivilegeEscalation(false)
			ctx := builder.BuildSecurityContext()
			Expect(ctx.AllowPrivilegeEscalation).NotTo(BeNil())
			Expect(*ctx.AllowPrivilegeEscalation).To(BeFalse())
		})
	})

	Describe("WithDroppedCapabilities", func() {
		It("should set dropped capabilities", func() {
			builder.WithDroppedCapabilities("ALL")
			ctx := builder.BuildSecurityContext()
			Expect(ctx.Capabilities).NotTo(BeNil())
			Expect(ctx.Capabilities.Drop).To(ContainElements(corev1.Capability("ALL")))
		})
	})

	Describe("WithAddedCapabilities", func() {
		It("should set added capabilities", func() {
			builder.WithAddedCapabilities("NET_ADMIN")
			ctx := builder.BuildSecurityContext()
			Expect(ctx.Capabilities).NotTo(BeNil())
			Expect(ctx.Capabilities.Add).To(ContainElements(corev1.Capability("NET_ADMIN")))
		})
	})

	Describe("BuildSecurityContext", func() {
		It("should build a container security context", func() {
			builder.WithRunAsUser(1000).
				WithRunAsGroup(1000).
				WithRunAsNonRoot(true)

			ctx := builder.BuildSecurityContext()
			Expect(ctx).NotTo(BeNil())
			Expect(ctx.RunAsUser).NotTo(BeNil())
			Expect(*ctx.RunAsUser).To(Equal(int64(1000)))
		})
	})

	Describe("BuildPodSecurityContext", func() {
		It("should build a pod security context", func() {
			builder.WithRunAsUser(1000).
				WithRunAsGroup(1000).
				WithFSGroup(1000).
				WithRunAsNonRoot(true)

			ctx := builder.BuildPodSecurityContext()
			Expect(ctx).NotTo(BeNil())
			Expect(ctx.RunAsUser).NotTo(BeNil())
			Expect(ctx.FSGroup).NotTo(BeNil())
		})
	})

	Describe("BuildDefaultSecurityContext", func() {
		It("should build a default container security context", func() {
			ctx := builder.BuildDefaultSecurityContext()
			Expect(ctx).NotTo(BeNil())
			Expect(ctx.RunAsUser).NotTo(BeNil())
			Expect(*ctx.RunAsUser).To(Equal(security.DefaultRunAsUser))
			Expect(ctx.RunAsNonRoot).NotTo(BeNil())
			Expect(*ctx.RunAsNonRoot).To(BeTrue())
			Expect(ctx.AllowPrivilegeEscalation).NotTo(BeNil())
			Expect(*ctx.AllowPrivilegeEscalation).To(BeFalse())
			Expect(ctx.Capabilities).NotTo(BeNil())
			Expect(ctx.Capabilities.Drop).To(ContainElements(corev1.Capability("ALL")))
		})
	})

	Describe("BuildDefaultPodSecurityContext", func() {
		It("should build a default pod security context", func() {
			ctx := builder.BuildDefaultPodSecurityContext()
			Expect(ctx).NotTo(BeNil())
			Expect(ctx.RunAsUser).NotTo(BeNil())
			Expect(*ctx.RunAsUser).To(Equal(security.DefaultRunAsUser))
			Expect(ctx.FSGroup).NotTo(BeNil())
			Expect(*ctx.FSGroup).To(Equal(security.DefaultFSGroup))
			Expect(ctx.RunAsNonRoot).NotTo(BeNil())
			Expect(*ctx.RunAsNonRoot).To(BeTrue())
		})
	})

	Describe("DefaultPodSecurityBuilder", func() {
		It("should return a builder with secure defaults", func() {
			builder := security.DefaultPodSecurityBuilder()
			Expect(builder).NotTo(BeNil())

			ctx := builder.BuildSecurityContext()
			Expect(ctx.RunAsUser).NotTo(BeNil())
			Expect(*ctx.RunAsUser).To(Equal(security.DefaultRunAsUser))
		})
	})
})
