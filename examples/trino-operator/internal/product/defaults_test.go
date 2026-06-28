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

package product_test

import (
	"strings"
	"testing"

	trinov1alpha1 "github.com/zncdatadev/operator-go/examples/trino-operator/api/v1alpha1"
	"github.com/zncdatadev/operator-go/examples/trino-operator/internal/product"
	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
)

func testCR() *trinov1alpha1.TrinoCluster {
	cr := &trinov1alpha1.TrinoCluster{}
	cr.Name = "test-trino"
	cr.Spec.Coordinators = &trinov1alpha1.CoordinatorsSpec{
		RoleSpec: commonsv1alpha1.RoleSpec{
			RoleGroups: map[string]commonsv1alpha1.RoleGroupSpec{"default": {}},
		},
	}
	return cr
}

func configProps(t *testing.T, cr *trinov1alpha1.TrinoCluster, role string) map[string]string {
	t.Helper()
	ov := product.ConfigDefaults(cr, role, "default")
	if ov == nil || ov.ConfigOverrides["config.properties"] == nil {
		t.Fatalf("ConfigDefaults returned no config.properties for role %q", role)
	}
	return ov.ConfigOverrides["config.properties"]
}

func TestConfigDefaultsCoordinator(t *testing.T) {
	props := configProps(t, testCR(), product.RoleCoordinators)

	if got := props["coordinator"]; got != "true" {
		t.Errorf("coordinator = %q, want true", got)
	}
	if got := props["discovery-server.enabled"]; got != "true" {
		t.Errorf("discovery-server.enabled = %q, want true", got)
	}
	// The discovery URI must point at the Service the SDK actually creates
	// ({cluster}-{role}-{group}).
	if uri := props["discovery.uri"]; !strings.Contains(uri, "test-trino-coordinators-default") {
		t.Errorf("discovery.uri = %q, want it to reference test-trino-coordinators-default", uri)
	}
}

func TestConfigDefaultsWorker(t *testing.T) {
	props := configProps(t, testCR(), product.RoleWorkers)

	if got := props["coordinator"]; got != "false" {
		t.Errorf("coordinator = %q, want false", got)
	}
	if _, ok := props["discovery-server.enabled"]; ok {
		t.Errorf("workers must not set discovery-server.enabled")
	}
	// Workers still discover the coordinator.
	if uri := props["discovery.uri"]; !strings.Contains(uri, "test-trino-coordinators-default") {
		t.Errorf("discovery.uri = %q, want it to reference the coordinator service", uri)
	}
}

func TestCoordinatorPortDefaultAndOverride(t *testing.T) {
	cr := testCR()
	if got := product.CoordinatorPort(cr); got != 8080 {
		t.Errorf("default CoordinatorPort = %d, want 8080", got)
	}

	cr.Spec.Coordinators.HTTPPort = 9090
	if got := product.CoordinatorPort(cr); got != 9090 {
		t.Errorf("overridden CoordinatorPort = %d, want 9090", got)
	}
	props := configProps(t, cr, product.RoleCoordinators)
	if got := props["http-server.http.port"]; got != "9090" {
		t.Errorf("http-server.http.port = %q, want 9090", got)
	}
}
