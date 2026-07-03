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

package controller

import (
	"context"
	"fmt"

	trinov1alpha1 "github.com/zncdatadev/operator-go/examples/trino-operator/api/v1alpha1"
	trinoconfig "github.com/zncdatadev/operator-go/examples/trino-operator/internal/config"
	"github.com/zncdatadev/operator-go/examples/trino-operator/internal/constants"
	"github.com/zncdatadev/operator-go/examples/trino-operator/internal/product"
	"github.com/zncdatadev/operator-go/pkg/config"
	"github.com/zncdatadev/operator-go/pkg/productlogging"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Compile-time proof that TrinoCluster wires framework-owned vector.yaml generation: when a role
// group enables the Vector agent, the GenericReconciler reads this ConfigMap name, resolves the
// aggregator address, and generates vector.yaml into the role group ConfigMap.
var _ reconciler.VectorAggregatorProvider = (*trinov1alpha1.TrinoCluster)(nil)

// TrinoRoleGroupHandler builds Trino role group resources. It embeds the SDK's
// BaseRoleGroupHandler so the framework owns the bulk of resource orchestration — ConfigMap
// (rendered from the merged config, including the product config computed by
// product.ComputeConfig), Services, the StatefulSet (with sidecars and podOverrides applied
// by the framework), and the PDB. The override below only adds the product-specific bits the
// merge pipeline cannot model declaratively.
type TrinoRoleGroupHandler struct {
	*reconciler.BaseRoleGroupHandler[*trinov1alpha1.TrinoCluster]
}

// NewTrinoRoleGroupHandler creates the handler and configures the framework defaults.
func NewTrinoRoleGroupHandler(scheme *runtime.Scheme) *TrinoRoleGroupHandler {
	base := reconciler.NewBaseRoleGroupHandler[*trinov1alpha1.TrinoCluster](defaultImage(), scheme)

	// config.properties (provided as a map via ProductConfig / CRD overrides) is rendered
	// with the properties format adapter.
	base.ConfigGenerator = config.NewMultiFormatConfigGenerator()
	base.ConfigGenerator.RegisterDefaultFormats()

	// Trino reads config from /etc/trino; name the main container "trino" so it matches the
	// per-container logging key below.
	base.ConfigMountPath = "/etc/trino"
	base.MainContainerName = constants.MainContainerName

	// Declarative logging: the framework renders the Log4j2 config file into the ConfigMap
	// from the deep-merged CRD logging spec.
	base.LoggingContainers = []productlogging.ContainerLogging{
		{Container: constants.MainContainerName, Framework: productlogging.LoggingFrameworkLog4j2},
	}

	// Ports are the same for both roles.
	containerPorts := []corev1.ContainerPort{
		{Name: "http", ContainerPort: constants.DefaultHTTPPort, Protocol: corev1.ProtocolTCP},
	}
	servicePorts := []corev1.ServicePort{
		{Name: "http", Port: constants.DefaultHTTPPort, Protocol: corev1.ProtocolTCP},
	}
	for _, role := range []string{product.RoleCoordinators, product.RoleWorkers} {
		base.SetRoleContainerPorts(role, containerPorts)
		base.SetRoleServicePorts(role, servicePorts)
	}

	return &TrinoRoleGroupHandler{BaseRoleGroupHandler: base}
}

// BuildResources delegates the 90% to the framework, then appends the product-specific pieces
// the merge pipeline cannot express:
//   - the CR-driven container image (resolved with the product name),
//   - jvm.config (a newline-delimited flag list, not key=value),
//   - the coordinator-only catalog files.
func (h *TrinoRoleGroupHandler) BuildResources(
	ctx context.Context,
	k8sClient client.Client,
	cr *trinov1alpha1.TrinoCluster,
	buildCtx *reconciler.RoleGroupBuildContext,
) (*reconciler.RoleGroupResources, error) {
	resources, err := h.BaseRoleGroupHandler.BuildResources(ctx, k8sClient, cr, buildCtx)
	if err != nil {
		return nil, err
	}

	// The image is declared in the CR spec (defaulted by the webhook) and resolved with the
	// product name. The framework built the container with the operator's default image; set
	// the CR-driven image on the primary container, looked up by name (it is renamed via
	// base.MainContainerName) rather than by index so a future prepended sidecar can't shift it.
	if cr.Spec.Image != nil && resources.StatefulSet != nil {
		containers := resources.StatefulSet.Spec.Template.Spec.Containers
		for i := range containers {
			if containers[i].Name == constants.MainContainerName {
				containers[i].Image = cr.Spec.Image.GetImage(constants.ProductName)
				containers[i].ImagePullPolicy = cr.Spec.Image.GetPullPolicy()
				break
			}
		}
	}

	if resources.ConfigMap != nil {
		if resources.ConfigMap.Data == nil {
			resources.ConfigMap.Data = make(map[string]string)
		}

		// jvm.config is a flag list, not key=value, so it is generated here as a whole file
		// (like the logging file) rather than flowing through the merge pipeline. setIfAbsent
		// only avoids clobbering a jvm.config the pipeline already placed under this key; note
		// that configOverrides renders as key=value, so it is NOT a suitable channel for tuning
		// JVM flags — a product needing user-tunable JVM options would expose a typed field/env.
		setIfAbsent(resources.ConfigMap.Data, "jvm.config", func() string { return jvmConfig(buildCtx.RoleName) })

		// Catalog connector files live only on the coordinator.
		if buildCtx.RoleName == product.RoleCoordinators {
			catalogs := trinoconfig.NewCatalogConfigBuilder().WithCatalogs(cr.Spec.Catalogs).Build()
			for name, content := range catalogs {
				key := fmt.Sprintf("catalog/%s.properties", name)
				setIfAbsent(resources.ConfigMap.Data, key, func() string { return content })
			}
		}
	}

	return resources, nil
}

// setIfAbsent writes value() into data[key] only when the key is not already present, so
// product config never overwrites config the merge pipeline produced (CRD always wins).
func setIfAbsent(data map[string]string, key string, value func() string) {
	if _, exists := data[key]; !exists {
		data[key] = value()
	}
}

// jvmConfig renders the role-specific JVM options.
func jvmConfig(roleName string) string {
	b := trinoconfig.NewJVMConfigBuilder()
	if roleName == product.RoleWorkers {
		b.ForWorker()
	} else {
		b.ForCoordinator()
	}
	return b.Build()
}

// defaultImage is the operator's default Trino image. The CR's spec.image (defaulted by the
// webhook) overrides it per reconcile in BuildResources.
func defaultImage() string {
	return fmt.Sprintf("%s/trino:%s-%s",
		constants.DefaultImageRepo,
		constants.DefaultImageProductVersion,
		constants.DefaultImageKubedoopVersion,
	)
}

// Ensure interface implementation.
var _ reconciler.RoleGroupHandler[*trinov1alpha1.TrinoCluster] = &TrinoRoleGroupHandler{}
