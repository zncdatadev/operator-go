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
)

const (
	// DefaultRunAsUser is the default non-root user ID.
	DefaultRunAsUser int64 = 1001
	// DefaultRunAsGroup is the default non-root group ID.
	DefaultRunAsGroup int64 = 1001
	// DefaultFSGroup is the default filesystem group ID.
	DefaultFSGroup int64 = 1001
)

// PodSecurityBuilder builds secure pod security contexts.
type PodSecurityBuilder struct {
	runAsUser                *int64
	runAsGroup               *int64
	fsGroup                  *int64
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
	if b.seccompProfile != nil {
		ctx.SeccompProfile = b.seccompProfile
	}

	return ctx
}

// BuildDefaultSecurityContext creates a container security context with secure defaults.
func (b *PodSecurityBuilder) BuildDefaultSecurityContext() *corev1.SecurityContext {
	nonRoot := true
	allowEscalation := false

	return &corev1.SecurityContext{
		RunAsUser:                ptrInt64(DefaultRunAsUser),
		RunAsGroup:               ptrInt64(DefaultRunAsGroup),
		RunAsNonRoot:             &nonRoot,
		AllowPrivilegeEscalation: &allowEscalation,
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
}

// BuildDefaultPodSecurityContext creates a pod security context with secure defaults.
func (b *PodSecurityBuilder) BuildDefaultPodSecurityContext() *corev1.PodSecurityContext {
	nonRoot := true

	return &corev1.PodSecurityContext{
		RunAsUser:    ptrInt64(DefaultRunAsUser),
		RunAsGroup:   ptrInt64(DefaultRunAsGroup),
		RunAsNonRoot: &nonRoot,
		FSGroup:      ptrInt64(DefaultFSGroup),
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
}

// ptrInt64 returns a pointer to an int64.
func ptrInt64(v int64) *int64 {
	return &v
}

// DefaultPodSecurityBuilder returns a builder with secure defaults.
func DefaultPodSecurityBuilder() *PodSecurityBuilder {
	return NewPodSecurityBuilder().
		WithRunAsUser(DefaultRunAsUser).
		WithRunAsGroup(DefaultRunAsGroup).
		WithFSGroup(DefaultFSGroup).
		WithRunAsNonRoot(true).
		WithDroppedCapabilities("ALL").
		WithAllowPrivilegeEscalation(false).
		WithSeccompProfile(&corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		})
}
