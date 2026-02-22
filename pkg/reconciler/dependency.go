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

package reconciler

import (
	"context"
	"fmt"

	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// DependencyResolver validates external dependencies.
type DependencyResolver struct {
	Client client.Client
}

// NewDependencyResolver creates a new DependencyResolver.
func NewDependencyResolver(client client.Client) *DependencyResolver {
	return &DependencyResolver{Client: client}
}

// Validate checks if all dependencies are available.
func (d *DependencyResolver) Validate(ctx context.Context, spec *v1alpha1.GenericClusterSpec) error {
	// Check cluster operation
	if spec.ClusterOperation != nil {
		if spec.ClusterOperation.ReconciliationPaused {
			return &DependencyError{
				Type:    "ReconciliationPaused",
				Message: "Reconciliation is paused",
			}
		}
		if spec.ClusterOperation.Stopped {
			return &DependencyError{
				Type:    "Stopped",
				Message: "Cluster is stopped",
			}
		}
	}

	return nil
}

// ValidateConfigMap validates that a ConfigMap exists.
func (d *DependencyResolver) ValidateConfigMap(ctx context.Context, namespace, name string) error {
	cm := &corev1.ConfigMap{}
	key := types.NamespacedName{Namespace: namespace, Name: name}

	if err := d.Client.Get(ctx, key, cm); err != nil {
		return &DependencyError{
			Type:    "ConfigMapNotFound",
			Message: fmt.Sprintf("ConfigMap %s/%s not found", namespace, name),
			Cause:   err,
		}
	}

	return nil
}

// ValidateSecret validates that a Secret exists.
func (d *DependencyResolver) ValidateSecret(ctx context.Context, namespace, name string) error {
	secret := &corev1.Secret{}
	key := types.NamespacedName{Namespace: namespace, Name: name}

	if err := d.Client.Get(ctx, key, secret); err != nil {
		return &DependencyError{
			Type:    "SecretNotFound",
			Message: fmt.Sprintf("Secret %s/%s not found", namespace, name),
			Cause:   err,
		}
	}

	return nil
}

// ValidateZKConfig validates Zookeeper configuration.
// The context parameter is reserved for future use (e.g., ZK connectivity validation).
func (d *DependencyResolver) ValidateZKConfig(_ context.Context, zkConfig *v1alpha1.ZKConfig) error {
	if zkConfig == nil {
		return nil
	}

	if zkConfig.ConnectionString == "" {
		return &DependencyError{
			Type:    "InvalidZKConfig",
			Message: "Zookeeper connection string is empty",
		}
	}

	return nil
}

// DependencyError represents a dependency validation error.
type DependencyError struct {
	Type    string
	Message string
	Cause   error
}

// Error implements error.
func (e *DependencyError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (cause: %v)", e.Type, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

// Unwrap returns the underlying error.
func (e *DependencyError) Unwrap() error {
	return e.Cause
}

// IsDependencyError checks if an error is a DependencyError.
func IsDependencyError(err error) bool {
	_, ok := err.(*DependencyError)
	return ok
}

// GetDependencyErrorType returns the type of a DependencyError.
func GetDependencyErrorType(err error) string {
	if depErr, ok := err.(*DependencyError); ok {
		return depErr.Type
	}
	return ""
}

// LogDependencyError logs a dependency error.
func LogDependencyError(ctx context.Context, err error) {
	logger := log.FromContext(ctx)
	if depErr, ok := err.(*DependencyError); ok {
		logger.Error(err, "Dependency validation failed", "type", depErr.Type)
	} else {
		logger.Error(err, "Dependency validation failed")
	}
}
