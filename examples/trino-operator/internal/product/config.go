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

// Package product holds Trino's product-intrinsic logic — the knowledge that is neither the
// SDK framework's nor the user's, expressed as data that flows through the SDK merge pipeline.
package product

import (
	"context"
	"fmt"
	"slices"

	"sigs.k8s.io/controller-runtime/pkg/client"

	trinov1alpha1 "github.com/zncdatadev/operator-go/examples/trino-operator/api/v1alpha1"
	"github.com/zncdatadev/operator-go/examples/trino-operator/internal/constants"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
)

// Role names. These must match the keys returned by TrinoCluster.GetSpec().Roles.
const (
	RoleCoordinators = "coordinators"
	RoleWorkers      = "workers"
)

// ComputeConfig is Trino's RoleGroupResolver. It computes the product's config.properties for a
// given role group and returns it as a *reconciler.Contribution, which the SDK renders into the
// same override shape users write in the CRD. The SDK merges it as the LOWEST layer
// (product < role < role group), so any value a user sets via configOverrides always wins.
//
// It runs AFTER the typed config block is folded (product defaults < role < role group), so a
// value derived from the effective config — a JVM heap sized from the memory limit the user raised
// — is reachable here and reaches the ConfigMap. That was impossible before: the effective config
// was not computed until after the role group's ConfigMap had already been written.
//
// This is config generation, not defaulting: it runs every reconcile and is where
// role-specific product knowledge lives (coordinator vs worker) and where values are derived
// from live cluster state (the discovery URI is built from the coordinator Service the
// framework will create). It contains no imperative resource construction — purely data.
// The ctx and client are unused here — trino's configuration is a pure function of the CR — but
// they are what the seam exists for: a product resolving an S3Connection reference or a ZooKeeper
// address does that lookup here and reports a failure through the error return, rather than
// swallowing it and rendering a silently wrong config.
func ComputeConfig(
	_ context.Context, _ client.Client, cr *trinov1alpha1.TrinoCluster,
	rg *reconciler.RoleGroupBuildContext,
) (*reconciler.Contribution, error) {
	roleName := rg.RoleName
	port := CoordinatorPort(cr)

	props := map[string]string{
		"http-server.http.port": fmt.Sprintf("%d", port),
		"discovery.uri":         discoveryURI(cr, port),
	}

	switch roleName {
	case RoleCoordinators:
		props["coordinator"] = "true"
		props["node-scheduler.include-coordinator"] = "false"
		props["discovery-server.enabled"] = "true"
	case RoleWorkers:
		props["coordinator"] = "false"
	}

	return &reconciler.Contribution{
		ConfigOverrides: map[string]map[string]string{
			"config.properties": props,
		},
	}, nil
}

// CoordinatorPort returns the coordinator HTTP port from the CR or the product default.
func CoordinatorPort(cr *trinov1alpha1.TrinoCluster) int32 {
	if cr.Spec.Coordinators != nil && cr.Spec.Coordinators.HTTPPort != 0 {
		return cr.Spec.Coordinators.HTTPPort
	}
	return constants.DefaultHTTPPort
}

// coordinatorServiceName returns the client-facing coordinator Service name. The SDK names
// role group resources as {cluster}-{role}-{group}, so we derive the name from a coordinator
// role group, matching the Service the framework actually creates. Group names are sorted so
// the choice is deterministic across reconciles (map iteration order is randomized) — without
// this, the discovery URI could change between reconciles and churn the config in a deployment
// with multiple coordinator role groups.
func coordinatorServiceName(cr *trinov1alpha1.TrinoCluster) string {
	groupName := constants.DefaultRoleGroupName
	if cr.Spec.Coordinators != nil && len(cr.Spec.Coordinators.RoleGroups) > 0 {
		names := make([]string, 0, len(cr.Spec.Coordinators.RoleGroups))
		for g := range cr.Spec.Coordinators.RoleGroups {
			names = append(names, g)
		}
		slices.Sort(names)
		groupName = names[0]
	}
	return reconciler.RoleGroupResourceName(cr.Name, RoleCoordinators, groupName)
}

// discoveryURI builds the Trino discovery URI from the coordinator Service name and port.
func discoveryURI(cr *trinov1alpha1.TrinoCluster, port int32) string {
	return fmt.Sprintf("http://%s:%d", coordinatorServiceName(cr), port)
}

// DiscoveryURI returns the client-facing coordinator URI. It backs both the workers'
// discovery.uri in config.properties and the cluster discovery ConfigMap published by
// extensions.DiscoveryExtension.
func DiscoveryURI(cr *trinov1alpha1.TrinoCluster) string {
	return discoveryURI(cr, CoordinatorPort(cr))
}
