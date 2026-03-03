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

package v1alpha1

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	trinov1alpha1 "github.com/zncdatadev/operator-go/examples/trino-operator/api/v1alpha1"
	"github.com/zncdatadev/operator-go/examples/trino-operator/internal/constants"
	"github.com/zncdatadev/operator-go/pkg/webhook"
)

// nolint:unused
// log is for logging in this package.
var trinoclusterlog = logf.Log.WithName("trinocluster-resource")

// SetupTrinoClusterWebhookWithManager registers the webhook for TrinoCluster in the manager.
func SetupTrinoClusterWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &trinov1alpha1.TrinoCluster{}).
		WithValidator(&TrinoClusterCustomValidator{}).
		WithDefaulter(&TrinoClusterCustomDefaulter{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-trino-kubedoop-dev-v1alpha1-trinocluster,mutating=true,failurePolicy=fail,sideEffects=None,groups=trino.kubedoop.dev,resources=trinoclusters,verbs=create;update,versions=v1alpha1,name=mtrinocluster-v1alpha1.kb.io,admissionReviewVersions=v1

// TrinoClusterCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind TrinoCluster when those are created or updated.
type TrinoClusterCustomDefaulter struct{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind TrinoCluster.
func (d *TrinoClusterCustomDefaulter) Default(_ context.Context, obj *trinov1alpha1.TrinoCluster) error {
	trinoclusterlog.Info("Defaulting for TrinoCluster", "name", obj.GetName())

	// Set default image if not specified
	if obj.Spec.Image == "" {
		obj.Spec.Image = constants.DefaultImage
		trinoclusterlog.Info("Set default image", "image", constants.DefaultImage)
	}

	// Initialize coordinators if not specified
	if obj.Spec.Coordinators == nil {
		obj.Spec.Coordinators = &trinov1alpha1.CoordinatorsSpec{}
	}

	// Set default coordinator port if not specified
	if obj.Spec.Coordinators.HTTPPort == 0 {
		obj.Spec.Coordinators.HTTPPort = constants.DefaultHTTPPort
		trinoclusterlog.Info("Set default coordinator HTTP port", "port", constants.DefaultHTTPPort)
	}

	// Initialize workers if not specified
	if obj.Spec.Workers == nil {
		obj.Spec.Workers = &trinov1alpha1.WorkersSpec{}
	}

	// Set default worker port if not specified
	if obj.Spec.Workers.HTTPPort == 0 {
		obj.Spec.Workers.HTTPPort = constants.DefaultHTTPPort
		trinoclusterlog.Info("Set default worker HTTP port", "port", constants.DefaultHTTPPort)
	}

	return nil
}

// +kubebuilder:webhook:path=/validate-trino-kubedoop-dev-v1alpha1-trinocluster,mutating=false,failurePolicy=fail,sideEffects=None,groups=trino.kubedoop.dev,resources=trinoclusters,verbs=create;update,versions=v1alpha1,name=vtrinocluster-v1alpha1.kb.io,admissionReviewVersions=v1

// TrinoClusterCustomValidator struct is responsible for validating the TrinoCluster resource
// when it is created, updated, or deleted.
type TrinoClusterCustomValidator struct{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type TrinoCluster.
func (v *TrinoClusterCustomValidator) ValidateCreate(_ context.Context, obj *trinov1alpha1.TrinoCluster) (admission.Warnings, error) {
	trinoclusterlog.Info("Validation for TrinoCluster upon creation", "name", obj.GetName())

	errs := v.validateTrinoCluster(obj)
	if errs.HasErrors() {
		return nil, errs
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type TrinoCluster.
func (v *TrinoClusterCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj *trinov1alpha1.TrinoCluster) (admission.Warnings, error) {
	trinoclusterlog.Info("Validation for TrinoCluster upon update", "name", newObj.GetName())

	errs := v.validateTrinoCluster(newObj)

	// Validate immutable fields
	if oldObj.Spec.Image != "" && oldObj.Spec.Image != newObj.Spec.Image {
		errs.Add("spec.image", "image cannot be changed after creation")
	}

	if errs.HasErrors() {
		return nil, errs
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type TrinoCluster.
func (v *TrinoClusterCustomValidator) ValidateDelete(_ context.Context, obj *trinov1alpha1.TrinoCluster) (admission.Warnings, error) {
	trinoclusterlog.Info("Validation for TrinoCluster upon deletion", "name", obj.GetName())

	// No validation needed for deletion
	return nil, nil
}

// validateTrinoCluster validates all fields of the TrinoCluster
func (v *TrinoClusterCustomValidator) validateTrinoCluster(obj *trinov1alpha1.TrinoCluster) webhook.ValidationErrors {
	errs := webhook.ValidationErrors{}

	// Validate image
	if obj.Spec.Image != "" {
		if err := validateImage(obj.Spec.Image); err != nil {
			errs.AddWithValue("spec.image", err.Error(), obj.Spec.Image)
		}
	}

	// Validate coordinators
	if obj.Spec.Coordinators != nil {
		v.validateCoordinators(obj.Spec.Coordinators, &errs)
	}

	// Validate workers
	if obj.Spec.Workers != nil {
		v.validateWorkers(obj.Spec.Workers, &errs)
	}

	// Validate catalogs
	for i, catalog := range obj.Spec.Catalogs {
		v.validateCatalog(&catalog, i, &errs)
	}

	return errs
}

// validateCoordinators validates coordinator configuration
func (v *TrinoClusterCustomValidator) validateCoordinators(spec *trinov1alpha1.CoordinatorsSpec, errs *webhook.ValidationErrors) {
	// Validate HTTP port
	if spec.HTTPPort != 0 {
		if err := validatePort(spec.HTTPPort, "spec.coordinators.httpPort"); err != nil {
			errs.AddWithValue("spec.coordinators.httpPort", err.Error(), spec.HTTPPort)
		}
	}
}

// validateWorkers validates worker configuration
func (v *TrinoClusterCustomValidator) validateWorkers(spec *trinov1alpha1.WorkersSpec, errs *webhook.ValidationErrors) {
	// Validate HTTP port
	if spec.HTTPPort != 0 {
		if err := validatePort(spec.HTTPPort, "spec.workers.httpPort"); err != nil {
			errs.AddWithValue("spec.workers.httpPort", err.Error(), spec.HTTPPort)
		}
	}
}

// validateCatalog validates catalog configuration
func (v *TrinoClusterCustomValidator) validateCatalog(spec *trinov1alpha1.CatalogSpec, index int, errs *webhook.ValidationErrors) {
	fieldPrefix := fmt.Sprintf("spec.catalogs[%d]", index)

	// Validate catalog name
	if spec.Name == "" {
		errs.Add(fieldPrefix+".name", "catalog name is required")
	} else if err := validateCatalogName(spec.Name); err != nil {
		errs.AddWithValue(fieldPrefix+".name", err.Error(), spec.Name)
	}

	// Validate catalog type
	if spec.Type == "" {
		errs.Add(fieldPrefix+".type", "catalog type is required")
	} else if err := validateCatalogType(spec.Type); err != nil {
		errs.AddWithValue(fieldPrefix+".type", err.Error(), spec.Type)
	}
}

// validateImage validates container image format
func validateImage(image string) error {
	// Image format: [registry/]repository[:tag]
	// Examples: trinodb/trino:435, ghcr.io/trinodb/trino:435, trino:latest
	imagePattern := `^([a-z0-9-]+(\.[a-z0-9-]+)*(:[0-9]+)?/)?[a-z0-9_.-]+(/[a-z0-9_.-]+)*(:[a-zA-Z0-9_.-]+)?$`
	matched, err := regexp.MatchString(imagePattern, image)
	if err != nil {
		return fmt.Errorf("failed to validate image: %w", err)
	}
	if !matched {
		return fmt.Errorf("invalid image format")
	}
	return nil
}

// validatePort validates port number range
func validatePort(port int32, field string) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("must be between 1 and 65535")
	}
	return nil
}

// validateCatalogName validates catalog name format
func validateCatalogName(name string) error {
	// Catalog name must be lowercase alphanumeric with underscores
	// and cannot start with a number
	if len(name) == 0 || len(name) > 64 {
		return fmt.Errorf("must be between 1 and 64 characters")
	}

	namePattern := `^[a-z][a-z0-9_]*$`
	matched, err := regexp.MatchString(namePattern, name)
	if err != nil {
		return fmt.Errorf("failed to validate catalog name: %w", err)
	}
	if !matched {
		return fmt.Errorf("must start with a lowercase letter and contain only lowercase letters, numbers, and underscores")
	}

	return nil
}

// validateCatalogType validates catalog type against allowed values
func validateCatalogType(catalogType string) error {
	validTypes := map[string]bool{
		"hive":       true,
		"iceberg":    true,
		"kafka":      true,
		"mysql":      true,
		"postgresql": true,
		"delta":      true,
		"tpch":       true,
		"tpcds":      true,
	}

	normalizedType := strings.ToLower(catalogType)
	if !validTypes[normalizedType] {
		validList := make([]string, 0, len(validTypes))
		for t := range validTypes {
			validList = append(validList, t)
		}
		return fmt.Errorf("must be one of: %s", strings.Join(validList, ", "))
	}

	return nil
}
