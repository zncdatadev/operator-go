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

package security

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

const (
	// DefaultRunAsUser is the kubedoop org-standard non-root user ID. All kubedoop product
	// images run as uid 1001, so the framework default security context hardcodes this identity.
	DefaultRunAsUser int64 = 1001
	// DefaultRunAsGroup is the kubedoop org-standard primary group ID. It is intentionally 0
	// (the root group) for OpenShift compatibility: OpenShift assigns an arbitrary uid but keeps
	// gid 0, and kubedoop images make their files group-0 readable/writable, so running with
	// group 0 works under both standard Kubernetes and OpenShift's restricted SCC.
	DefaultRunAsGroup int64 = 0
	// DefaultFSGroup is the default filesystem group ID applied to mounted volumes so the
	// non-root process (uid 1001) can write to them.
	DefaultFSGroup int64 = 1001
)

// DefaultFSGroupChangePolicy is the ownership-recursion policy the framework applies alongside
// DefaultFSGroup.
//
// Setting fsGroup without it is a startup-time trap for exactly the workloads this SDK exists for.
// Kubernetes documents the field as: "Valid values are OnRootMismatch and Always. If not
// specified, Always is used." Always means the kubelet walks the ENTIRE volume on every mount,
// chown'ing and chmod'ing each file before the container starts. On an HDFS DataNode or a Kafka
// broker whose data PVC holds millions of files that is tens of minutes to hours — repeated on
// every restart, every rollout, every node drain — while the pod sits in ContainerCreating with
// nothing in its events explaining why.
//
// OnRootMismatch performs the same recursion only when the volume's ROOT directory does not
// already have the expected owner and mode, which is true exactly once: the first time a freshly
// provisioned volume is mounted. Every later mount is a single stat.
//
// The trade-off is real and deliberate: if something changes ownership deep inside the volume
// while the root stays correct, OnRootMismatch will not repair it. That is a repair the framework
// never promised, and paying for it on every start of every stateful pod is the wrong price.
//
// The policy has no effect on ephemeral volume types (secret, configMap, emptyDir), so the config
// mount and the shared log volume behave identically either way — the data PVC is what this is
// about.
const DefaultFSGroupChangePolicy = corev1.FSGroupChangeOnRootMismatch

// PodSecurityBuilder builds secure pod security contexts.
type PodSecurityBuilder struct {
	runAsUser                *int64
	runAsGroup               *int64
	fsGroup                  *int64
	fsGroupChangePolicy      *corev1.PodFSGroupChangePolicy
	runAsNonRoot             *bool
	seccompProfile           *corev1.SeccompProfile
	readOnlyRootFS           *bool
	allowPrivilegeEscalation *bool
	capabilities             *corev1.Capabilities
}

// NewPodSecurityBuilder creates a new PodSecurityBuilder.
func NewPodSecurityBuilder() *PodSecurityBuilder {
	return &PodSecurityBuilder{}
}

// WithRunAsUser sets the user ID to run as.
func (b *PodSecurityBuilder) WithRunAsUser(uid int64) *PodSecurityBuilder {
	b.runAsUser = &uid
	return b
}

// WithRunAsGroup sets the group ID to run as.
func (b *PodSecurityBuilder) WithRunAsGroup(gid int64) *PodSecurityBuilder {
	b.runAsGroup = &gid
	return b
}

// WithFSGroup sets the filesystem group ID.
func (b *PodSecurityBuilder) WithFSGroup(gid int64) *PodSecurityBuilder {
	b.fsGroup = &gid
	return b
}

// WithFSGroupChangePolicy sets how the kubelet applies fsGroup ownership to a volume: Always
// (recurse on every mount) or OnRootMismatch (recurse only when the volume root's owner and mode
// are not already correct). Leaving it unset means Kubernetes' own default, which is Always — see
// DefaultFSGroupChangePolicy for why that is the wrong default for a stateful workload.
func (b *PodSecurityBuilder) WithFSGroupChangePolicy(policy corev1.PodFSGroupChangePolicy) *PodSecurityBuilder {
	b.fsGroupChangePolicy = &policy
	return b
}

// WithRunAsNonRoot sets whether to run as non-root.
func (b *PodSecurityBuilder) WithRunAsNonRoot(nonRoot bool) *PodSecurityBuilder {
	b.runAsNonRoot = &nonRoot
	return b
}

// WithSeccompProfile sets the seccomp profile.
func (b *PodSecurityBuilder) WithSeccompProfile(profile *corev1.SeccompProfile) *PodSecurityBuilder {
	b.seccompProfile = profile
	return b
}

// WithReadOnlyRootFS sets whether the root filesystem should be read-only.
func (b *PodSecurityBuilder) WithReadOnlyRootFS(readOnly bool) *PodSecurityBuilder {
	b.readOnlyRootFS = &readOnly
	return b
}

// WithAllowPrivilegeEscalation sets whether privilege escalation is allowed.
func (b *PodSecurityBuilder) WithAllowPrivilegeEscalation(allow bool) *PodSecurityBuilder {
	b.allowPrivilegeEscalation = &allow
	return b
}

// WithCapabilities sets the Linux capabilities.
func (b *PodSecurityBuilder) WithCapabilities(capabilities *corev1.Capabilities) *PodSecurityBuilder {
	b.capabilities = capabilities
	return b
}

// WithDroppedCapabilities sets the capabilities to drop.
func (b *PodSecurityBuilder) WithDroppedCapabilities(caps ...corev1.Capability) *PodSecurityBuilder {
	if b.capabilities == nil {
		b.capabilities = &corev1.Capabilities{}
	}
	b.capabilities.Drop = caps
	return b
}

// WithAddedCapabilities sets the capabilities to add.
func (b *PodSecurityBuilder) WithAddedCapabilities(caps ...corev1.Capability) *PodSecurityBuilder {
	if b.capabilities == nil {
		b.capabilities = &corev1.Capabilities{}
	}
	b.capabilities.Add = caps
	return b
}

// BuildSecurityContext creates a container security context.
func (b *PodSecurityBuilder) BuildSecurityContext() *corev1.SecurityContext {
	ctx := &corev1.SecurityContext{}

	if b.runAsUser != nil {
		ctx.RunAsUser = b.runAsUser
	}
	if b.runAsGroup != nil {
		ctx.RunAsGroup = b.runAsGroup
	}
	if b.runAsNonRoot != nil {
		ctx.RunAsNonRoot = b.runAsNonRoot
	}
	if b.seccompProfile != nil {
		ctx.SeccompProfile = b.seccompProfile
	}
	if b.readOnlyRootFS != nil {
		ctx.ReadOnlyRootFilesystem = b.readOnlyRootFS
	}
	if b.allowPrivilegeEscalation != nil {
		ctx.AllowPrivilegeEscalation = b.allowPrivilegeEscalation
	}
	if b.capabilities != nil {
		ctx.Capabilities = b.capabilities
	}

	return ctx
}

// BuildPodSecurityContext creates a pod security context.
func (b *PodSecurityBuilder) BuildPodSecurityContext() *corev1.PodSecurityContext {
	ctx := &corev1.PodSecurityContext{}

	if b.runAsUser != nil {
		ctx.RunAsUser = b.runAsUser
	}
	if b.runAsGroup != nil {
		ctx.RunAsGroup = b.runAsGroup
	}
	if b.runAsNonRoot != nil {
		ctx.RunAsNonRoot = b.runAsNonRoot
	}
	if b.fsGroup != nil {
		ctx.FSGroup = b.fsGroup
	}
	if b.fsGroupChangePolicy != nil {
		ctx.FSGroupChangePolicy = b.fsGroupChangePolicy
	}
	if b.seccompProfile != nil {
		ctx.SeccompProfile = b.seccompProfile
	}

	return ctx
}

// BuildDefaultSecurityContext returns the framework's canonical default container security
// context. It carries BOTH the kubedoop org-standard identity AND the security-hardening fields:
//   - RunAsUser: 1001              — kubedoop images run as uid 1001
//   - RunAsGroup: 0                — group 0, OpenShift-compatible (arbitrary uid + gid 0)
//   - RunAsNonRoot: true           — refuse to start as root
//   - AllowPrivilegeEscalation: false
//   - Capabilities: drop ALL
//   - SeccompProfile: RuntimeDefault
//
// This is the single default the framework applies by default in the base role-group handler.
// MergedConfig.PodOverrides customizes it and DEEP-MERGES PER FIELD (strategic merge patch), so an
// override stating only one field keeps the rest of the hardening — and must therefore explicitly
// restate any default it wants to change. An image that has to run as root sets both
// `runAsUser: 0` and `runAsNonRoot: false`; setting only the first inherits `runAsNonRoot: true`
// and the kubelet refuses to start the container. See the base handler's WithSecurityContext docs
// and docs/security.md §3.3.2.
//
// Wholesale replacement is a different mechanism: SidecarConfig.SecurityContext replaces a
// sidecar's context outright, and BaseRoleGroupHandler.WithSecurityContext replaces this default
// for every role group.
func (b *PodSecurityBuilder) BuildDefaultSecurityContext() *corev1.SecurityContext {
	return &corev1.SecurityContext{
		RunAsUser:                ptr.To(DefaultRunAsUser),
		RunAsGroup:               ptr.To(DefaultRunAsGroup),
		RunAsNonRoot:             ptr.To(true),
		AllowPrivilegeEscalation: ptr.To(false),
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
}

// BuildDefaultPodSecurityContext returns the framework's canonical default pod security context.
// It carries BOTH the kubedoop org-standard identity AND hardening:
//   - RunAsUser: 1001
//   - RunAsGroup: 0                — OpenShift-compatible group 0
//   - FSGroup: 1001               — so mounted volumes are writable by uid 1001
//   - FSGroupChangePolicy: OnRootMismatch — so applying that group does not walk the whole
//     volume on every start (see DefaultFSGroupChangePolicy)
//   - RunAsNonRoot: true
//   - SeccompProfile: RuntimeDefault
//
// This is the single default the framework applies by default in the base role-group handler.
// MergedConfig.PodOverrides customizes it and DEEP-MERGES PER FIELD (strategic merge patch): an
// override stating only `fsGroupChangePolicy: Always` gets the recursion back while keeping
// fsGroup, the identity and the seccomp profile. That field-level merge is what makes the escape
// hatch on DefaultFSGroupChangePolicy usable at all. See docs/security.md §3.3.2.
func (b *PodSecurityBuilder) BuildDefaultPodSecurityContext() *corev1.PodSecurityContext {
	return &corev1.PodSecurityContext{
		RunAsUser:           ptr.To(DefaultRunAsUser),
		RunAsGroup:          ptr.To(DefaultRunAsGroup),
		RunAsNonRoot:        ptr.To(true),
		FSGroup:             ptr.To(DefaultFSGroup),
		FSGroupChangePolicy: ptr.To(DefaultFSGroupChangePolicy),
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
}

// DefaultPodSecurityBuilder returns a builder with secure defaults. It must stay in step with
// BuildDefaultSecurityContext / BuildDefaultPodSecurityContext: the two are separate paths to the
// same "framework default", and a field present in one but not the other is a difference nobody
// chose.
func DefaultPodSecurityBuilder() *PodSecurityBuilder {
	return NewPodSecurityBuilder().
		WithRunAsUser(DefaultRunAsUser).
		WithRunAsGroup(DefaultRunAsGroup).
		WithFSGroup(DefaultFSGroup).
		WithFSGroupChangePolicy(DefaultFSGroupChangePolicy).
		WithRunAsNonRoot(true).
		WithDroppedCapabilities("ALL").
		WithAllowPrivilegeEscalation(false).
		WithSeccompProfile(&corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		})
}
