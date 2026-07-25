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

package common

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// The registry stores extensions under the ClusterInterface instantiation, so an extension
// written against a concrete product CR cannot be registered directly. The adapters below
// bridge the two: they hold the single conversion from ClusterInterface to the product CR
// type, which keeps the conversion out of every product hook, and they skip CRs of a foreign
// type instead of failing the hook — a registry shared by several CR types would otherwise
// break reconciliation of every CR but one.

// AsClusterExtension adapts a ClusterExtension written for a concrete CR type to the erased
// instantiation accepted by ExtensionRegistry. Hooks are skipped for CRs of another type.
func AsClusterExtension[CR ClusterInterface](extension ClusterExtension[CR]) ClusterExtension[ClusterInterface] {
	return &clusterExtensionAdapter[CR]{extension: extension}
}

// AsRoleExtension adapts a RoleExtension written for a concrete CR type to the erased
// instantiation accepted by ExtensionRegistry. Hooks are skipped for CRs of another type.
func AsRoleExtension[CR ClusterInterface](extension RoleExtension[CR]) RoleExtension[ClusterInterface] {
	return &roleExtensionAdapter[CR]{extension: extension}
}

// AsRoleGroupExtension adapts a RoleGroupExtension written for a concrete CR type to the
// erased instantiation accepted by ExtensionRegistry. Hooks are skipped for CRs of another
// type.
func AsRoleGroupExtension[CR ClusterInterface](extension RoleGroupExtension[CR]) RoleGroupExtension[ClusterInterface] {
	return &roleGroupExtensionAdapter[CR]{extension: extension}
}

// typedCR converts cr to the extension's CR type, reporting whether the extension applies.
func typedCR[CR ClusterInterface](ctx context.Context, name string, cr ClusterInterface) (CR, bool) {
	typed, ok := cr.(CR)
	if !ok {
		log.FromContext(ctx).V(1).Info("Skipping extension registered for another CR type",
			"extension", name)
	}
	return typed, ok
}

type clusterExtensionAdapter[CR ClusterInterface] struct {
	extension ClusterExtension[CR]
}

func (a *clusterExtensionAdapter[CR]) Name() string {
	return a.extension.Name()
}

func (a *clusterExtensionAdapter[CR]) PreReconcile(ctx context.Context, client client.Client, cr ClusterInterface) error {
	typed, ok := typedCR[CR](ctx, a.Name(), cr)
	if !ok {
		return nil
	}
	return a.extension.PreReconcile(ctx, client, typed)
}

func (a *clusterExtensionAdapter[CR]) PostReconcile(ctx context.Context, client client.Client, cr ClusterInterface) error {
	typed, ok := typedCR[CR](ctx, a.Name(), cr)
	if !ok {
		return nil
	}
	return a.extension.PostReconcile(ctx, client, typed)
}

func (a *clusterExtensionAdapter[CR]) OnReconcileError(ctx context.Context, client client.Client, cr ClusterInterface, err error) error {
	typed, ok := typedCR[CR](ctx, a.Name(), cr)
	if !ok {
		return nil
	}
	return a.extension.OnReconcileError(ctx, client, typed, err)
}

type roleExtensionAdapter[CR ClusterInterface] struct {
	extension RoleExtension[CR]
}

func (a *roleExtensionAdapter[CR]) Name() string {
	return a.extension.Name()
}

func (a *roleExtensionAdapter[CR]) PreReconcile(ctx context.Context, client client.Client, cr ClusterInterface, roleName string) error {
	typed, ok := typedCR[CR](ctx, a.Name(), cr)
	if !ok {
		return nil
	}
	return a.extension.PreReconcile(ctx, client, typed, roleName)
}

func (a *roleExtensionAdapter[CR]) PostReconcile(ctx context.Context, client client.Client, cr ClusterInterface, roleName string) error {
	typed, ok := typedCR[CR](ctx, a.Name(), cr)
	if !ok {
		return nil
	}
	return a.extension.PostReconcile(ctx, client, typed, roleName)
}

type roleGroupExtensionAdapter[CR ClusterInterface] struct {
	extension RoleGroupExtension[CR]
}

func (a *roleGroupExtensionAdapter[CR]) Name() string {
	return a.extension.Name()
}

func (a *roleGroupExtensionAdapter[CR]) PreReconcile(ctx context.Context, client client.Client, cr ClusterInterface, roleName, roleGroupName string) error {
	typed, ok := typedCR[CR](ctx, a.Name(), cr)
	if !ok {
		return nil
	}
	return a.extension.PreReconcile(ctx, client, typed, roleName, roleGroupName)
}

func (a *roleGroupExtensionAdapter[CR]) PostReconcile(ctx context.Context, client client.Client, cr ClusterInterface, roleName, roleGroupName string) error {
	typed, ok := typedCR[CR](ctx, a.Name(), cr)
	if !ok {
		return nil
	}
	return a.extension.PostReconcile(ctx, client, typed, roleName, roleGroupName)
}
