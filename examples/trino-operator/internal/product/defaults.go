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
	"fmt"
	"slices"

	trinov1alpha1 "github.com/zncdatadev/operator-go/examples/trino-operator/api/v1alpha1"
	"github.com/zncdatadev/operator-go/examples/trino-operator/internal/constants"
	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
)

// Role names. These must match the keys returned by TrinoCluster.GetSpec().Roles.
const (
	RoleCoordinators = "coordinators"
	RoleWorkers      = "workers"
)

// ConfigDefaults is the Trino ProductDefaults hook. It returns the product's intrinsic
// config.properties for a given role group as an *OverridesSpec — the same shape users write
// in the CRD. The SDK merges it as the LOWEST layer (product < role < role group), so any
// value a user sets via configOverrides always wins.
//
// This is where role-specific product knowledge lives (coordinator vs worker) and where
// values are derived from the CR (the discovery URI points at the coordinator Service the
// framework will create). It contains no imperative resource construction — purely data.
func ConfigDefaults(cr *trinov1alpha1.TrinoCluster, roleName, _ string) *commonsv1alpha1.OverridesSpec {
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

	return &commonsv1alpha1.OverridesSpec{
		ConfigOverrides: map[string]map[string]string{
			"config.properties": props,
		},
	}
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
