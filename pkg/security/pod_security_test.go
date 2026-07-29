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
		It("should build the canonical default container security context (1001 identity + hardening)", func() {
			ctx := builder.BuildDefaultSecurityContext()
			Expect(ctx).NotTo(BeNil())
			Expect(ctx.RunAsUser).NotTo(BeNil())
			Expect(*ctx.RunAsUser).To(Equal(int64(1001)))
			Expect(*ctx.RunAsUser).To(Equal(security.DefaultRunAsUser))
			Expect(ctx.RunAsGroup).NotTo(BeNil())
			Expect(*ctx.RunAsGroup).To(Equal(int64(0)))
			Expect(*ctx.RunAsGroup).To(Equal(security.DefaultRunAsGroup))
			Expect(ctx.RunAsNonRoot).NotTo(BeNil())
			Expect(*ctx.RunAsNonRoot).To(BeTrue())
			Expect(ctx.AllowPrivilegeEscalation).NotTo(BeNil())
			Expect(*ctx.AllowPrivilegeEscalation).To(BeFalse())
			Expect(ctx.Capabilities).NotTo(BeNil())
			Expect(ctx.Capabilities.Drop).To(ContainElements(corev1.Capability("ALL")))
			Expect(ctx.SeccompProfile).NotTo(BeNil())
			Expect(ctx.SeccompProfile.Type).To(Equal(corev1.SeccompProfileTypeRuntimeDefault))
		})
	})

	Describe("BuildDefaultPodSecurityContext", func() {
		It("should build the canonical default pod security context (1001 identity + hardening)", func() {
			ctx := builder.BuildDefaultPodSecurityContext()
			Expect(ctx).NotTo(BeNil())
			Expect(ctx.RunAsUser).NotTo(BeNil())
			Expect(*ctx.RunAsUser).To(Equal(int64(1001)))
			Expect(*ctx.RunAsUser).To(Equal(security.DefaultRunAsUser))
			Expect(ctx.RunAsGroup).NotTo(BeNil())
			Expect(*ctx.RunAsGroup).To(Equal(int64(0)))
			Expect(*ctx.RunAsGroup).To(Equal(security.DefaultRunAsGroup))
			Expect(ctx.FSGroup).NotTo(BeNil())
			Expect(*ctx.FSGroup).To(Equal(int64(1001)))
			Expect(*ctx.FSGroup).To(Equal(security.DefaultFSGroup))
			Expect(ctx.RunAsNonRoot).NotTo(BeNil())
			Expect(*ctx.RunAsNonRoot).To(BeTrue())
			Expect(ctx.SeccompProfile).NotTo(BeNil())
			Expect(ctx.SeccompProfile.Type).To(Equal(corev1.SeccompProfileTypeRuntimeDefault))
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

var _ = Describe("fsGroup ownership recursion policy", func() {
	It("keeps the two paths to 'the framework default' identical", func() {
		// DefaultPodSecurityBuilder().BuildPodSecurityContext() and
		// BuildDefaultPodSecurityContext() are two separate expressions of the same thing. A field
		// added to one and forgotten in the other is a difference nobody chose — which is exactly
		// how fsGroup came to be set without a change policy on both paths at once.
		Expect(security.DefaultPodSecurityBuilder().BuildPodSecurityContext()).
			To(Equal(security.NewPodSecurityBuilder().BuildDefaultPodSecurityContext()))
	})

	It("pairs fsGroup with a change policy, because unset means Always", func() {
		// Kubernetes: "Valid values are OnRootMismatch and Always. If not specified, Always is
		// used." Always makes the kubelet chown every file on the volume before the container
		// starts, on EVERY start — minutes to hours for a data PVC with millions of files.
		ctx := security.NewPodSecurityBuilder().BuildDefaultPodSecurityContext()
		Expect(ctx.FSGroup).NotTo(BeNil(), "the policy only matters because fsGroup is set")
		Expect(ctx.FSGroupChangePolicy).NotTo(BeNil())
		Expect(*ctx.FSGroupChangePolicy).To(Equal(corev1.FSGroupChangeOnRootMismatch))
	})

	It("leaves the policy unset unless a caller asks for one", func() {
		// The generic builder stays opt-in like its siblings: only the framework default pairs the
		// two, so a caller assembling a context by hand is not given an opinion it did not state.
		Expect(security.NewPodSecurityBuilder().WithFSGroup(1000).
			BuildPodSecurityContext().FSGroupChangePolicy).To(BeNil())
		Expect(*security.NewPodSecurityBuilder().
			WithFSGroupChangePolicy(corev1.FSGroupChangeAlways).
			BuildPodSecurityContext().FSGroupChangePolicy).To(Equal(corev1.FSGroupChangeAlways))
	})
})
