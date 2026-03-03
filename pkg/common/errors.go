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
	"fmt"
)

// ConfigValidationError creates an error for configuration validation failures.
func ConfigValidationError(field string, err error) error {
	return fmt.Errorf("configuration validation failed for %s: %w", field, err)
}

// ResourceNotFoundError creates an error indicating a resource was not found.
func ResourceNotFoundError(resourceType, namespace, name string, err error) error {
	return fmt.Errorf("%s %s/%s not found: %w", resourceType, namespace, name, err)
}

// CreateResourceError creates an error for resource creation failures.
func CreateResourceError(resourceType, namespace, name string, err error) error {
	return fmt.Errorf("failed to create %s %s/%s: %w", resourceType, namespace, name, err)
}

// ConfigMergeError creates an error for configuration merge failures.
func ConfigMergeError(context string, err error) error {
	return fmt.Errorf("configuration merge failed for %s: %w", context, err)
}

// ConfigParseError creates an error for configuration parsing failures.
func ConfigParseError(format string, err error) error {
	return fmt.Errorf("failed to parse %s configuration: %w", format, err)
}
