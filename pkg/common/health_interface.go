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
)

// HealthCheckResult represents the result of a health check.
type HealthCheckResult struct {
	// Healthy indicates whether the health check passed.
	Healthy bool

	// Message provides additional information about the health check result.
	Message string

	// Error contains any error that occurred during the health check.
	Error error
}

// ServiceHealthCheck defines the contract for business-level health checks.
// Products implement this interface to provide custom health verification logic.
//
// Unlike basic Pod readiness, ServiceHealthCheck verifies that the application
// is fully operational (e.g., HDFS SafeMode off, HBase RegionServers registered).
type ServiceHealthCheck interface {
	// CheckHealthy performs a health check and returns the result.
	// The implementation should verify that the service is ready to accept traffic
	// and perform its intended function.
	//
	// Parameters:
	// - ctx: Context for cancellation and timeout
	// - client: Kubernetes client for resource lookup
	// - namespace: The namespace of the cluster
	// - name: The name of the cluster
	//
	// Returns:
	// - bool: true if healthy, false otherwise
	// - error: any error that occurred during the check
	CheckHealthy(ctx context.Context, client client.Client, namespace, name string) (bool, error)
}

// ServiceHealthCheckFunc is a function type that implements ServiceHealthCheck.
type ServiceHealthCheckFunc func(ctx context.Context, client client.Client, namespace, name string) (bool, error)

// CheckHealthy implements ServiceHealthCheck.
func (f ServiceHealthCheckFunc) CheckHealthy(ctx context.Context, client client.Client, namespace, name string) (bool, error) {
	return f(ctx, client, namespace, name)
}

// AlwaysHealthy is a health check that always returns healthy.
var AlwaysHealthy ServiceHealthCheck = ServiceHealthCheckFunc(func(ctx context.Context, client client.Client, namespace, name string) (bool, error) {
	return true, nil
})

// AlwaysUnhealthy is a health check that always returns unhealthy.
var AlwaysUnhealthy ServiceHealthCheck = ServiceHealthCheckFunc(func(ctx context.Context, client client.Client, namespace, name string) (bool, error) {
	return false, nil
})

// CompositeHealthCheck combines multiple health checks.
// All checks must pass for the composite check to be healthy.
type CompositeHealthCheck struct {
	checks []ServiceHealthCheck
}

// NewCompositeHealthCheck creates a new composite health check.
func NewCompositeHealthCheck(checks ...ServiceHealthCheck) *CompositeHealthCheck {
	return &CompositeHealthCheck{checks: checks}
}

// CheckHealthy implements ServiceHealthCheck.
func (c *CompositeHealthCheck) CheckHealthy(ctx context.Context, client client.Client, namespace, name string) (bool, error) {
	for _, check := range c.checks {
		healthy, err := check.CheckHealthy(ctx, client, namespace, name)
		if err != nil {
			return false, err
		}
		if !healthy {
			return false, nil
		}
	}
	return true, nil
}

// AddCheck adds a health check to the composite.
func (c *CompositeHealthCheck) AddCheck(check ServiceHealthCheck) {
	c.checks = append(c.checks, check)
}
