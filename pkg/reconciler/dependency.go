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
	"net/url"
	"strings"

	databasev1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/database/v1alpha1"
	s3v1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/s3/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// DependencyKind identifies the kind of an external object a cluster CR depends on.
type DependencyKind string

const (
	// DependencyConfigMap is a ConfigMap the CR references but does not create.
	DependencyConfigMap DependencyKind = "ConfigMap"
	// DependencySecret is a Secret the CR references but does not create.
	DependencySecret DependencyKind = "Secret"
)

// Dependency identifies one external object whose existence GenericReconciler verifies before
// reconciling any role. Products declare them through GenericReconcilerConfig.Dependencies,
// which is the only place the traversal of a product-specific spec can live: the generic spec
// has no field describing where a product hides its references.
//
// An empty Namespace means the cluster CR's own namespace.
type Dependency struct {
	// Kind selects the object type to look up (ConfigMap or Secret).
	Kind DependencyKind
	// Namespace of the object; empty means the CR's namespace.
	Namespace string
	// Name of the object; must not be empty.
	Name string
}

// DependencyResolver validates external dependencies.
type DependencyResolver struct {
	Client client.Client
}

// NewDependencyResolver creates a new DependencyResolver.
func NewDependencyResolver(client client.Client) *DependencyResolver {
	return &DependencyResolver{Client: client}
}

// Validate is a no-op kept for backward compatibility with operators that call it directly.
//
// The reconcile flow no longer routes through it: dependency checking is declarative and
// opt-in through GenericReconcilerConfig.Dependencies, which yields []Dependency and is
// dispatched to ValidateConfigMap / ValidateSecret. Richer, product-shaped checks
// (ValidateS3Connection, ValidateDatabaseConnection, ValidateZKConfig) remain explicit calls,
// because only the product knows where those specs live.
//
// It also does NOT evaluate ClusterOperation pause/stop: those flags are handled at the very top
// of the reconcile loop (GenericReconciler.reconcile), before any resource mutation.
func (d *DependencyResolver) Validate(_ context.Context, _ interface{}) error {
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
func (d *DependencyResolver) ValidateZKConfig(_ context.Context, zkConfig interface{}) error {
	if zkConfig == nil {
		return nil
	}

	if connStr, ok := zkConfig.(string); ok {
		if connStr == "" {
			return &DependencyError{
				Type:    "InvalidZKConfig",
				Message: "Zookeeper connection string is empty",
			}
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

// ValidateEndpointFormat validates endpoint format.
func ValidateEndpointFormat(endpoint, fieldName string) error {
	if endpoint == "" {
		return fmt.Errorf("%s: endpoint is empty", fieldName)
	}

	// If it has a scheme, parse it as URL
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("%s: invalid URL format: %w", fieldName, err)
	}

	// Only treat as URL if it has a valid scheme (http, https, etc.)
	// and the URL is properly formed
	if parsed.Scheme != "" && parsed.Hostname() != "" && parsed.Host != parsed.Hostname() {
		// Valid URL with scheme, hostname, and port (or at least non-empty host)
		return nil
	}

	// Not a valid URL or bare hostname
	// Check for bare hostname:port or just hostname
	if parsed.Scheme == "" {
		// No scheme - bare hostname (already validated endpoint != "" above)
		return nil
	}

	// Has a scheme but not properly formed URL
	if parsed.Hostname() == "" {
		return fmt.Errorf("%s: host is missing in endpoint", fieldName)
	}

	return nil
}

// ParseConnectionStrings parses connection strings and returns host list.
func ParseConnectionStrings(connStr string) ([]string, error) {
	if connStr == "" {
		return nil, fmt.Errorf("connection string is empty")
	}

	parts := strings.Split(connStr, ",")
	hosts := make([]string, 0, len(parts))

	for _, part := range parts {
		host := strings.TrimSpace(part)
		if host != "" {
			hosts = append(hosts, host)
		}
	}

	if len(hosts) == 0 {
		return nil, fmt.Errorf("no valid hosts found in connection string")
	}

	return hosts, nil
}

// ValidateZKConnection validates Zookeeper connection string (deprecated, use ValidateZKConfig instead).
func (d *DependencyResolver) ValidateZKConnection(_ context.Context, connStr string) error {
	return d.ValidateZKConfig(context.Background(), connStr)
}

// ValidateS3Connection validates S3 configuration.
func (d *DependencyResolver) ValidateS3Connection(_ context.Context, s3Config interface{}) error {
	if s3Config == nil {
		return nil
	}

	if spec, ok := s3Config.(*s3v1alpha1.S3ConnectionSpec); ok {
		if spec.Host == "" {
			return &DependencyError{
				Type:    "InvalidS3Config",
				Message: "S3 host is empty",
			}
		}

		if spec.Credentials == nil || spec.Credentials.SecretClass == "" {
			return &DependencyError{
				Type:    "InvalidS3Config",
				Message: "S3 secretClass is empty",
			}
		}
	}

	return nil
}

// ValidateDatabaseConnection validates database configuration.
func (d *DependencyResolver) ValidateDatabaseConnection(_ context.Context, dbConfig interface{}) error {
	if dbConfig == nil {
		return nil
	}

	if spec, ok := dbConfig.(*databasev1alpha1.DatabaseConnectionSpec); ok {
		if spec.Host == "" {
			return &DependencyError{
				Type:    "InvalidDatabaseConfig",
				Message: "Database endpoint is empty",
			}
		}

		if spec.Credentials == nil || spec.Credentials.SecretClass == "" {
			return &DependencyError{
				Type:    "InvalidDatabaseConfig",
				Message: "Database secretClass is empty",
			}
		}
	}

	return nil
}
