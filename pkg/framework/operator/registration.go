// Package operator connects generated input bindings to the internal controller.
// Products use their generated registration companion, not a mutable reconciler.
package operator

import (
	"context"
	"fmt"
	"slices"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/zncdatadev/operator-go/internal/framework/controller"
	"github.com/zncdatadev/operator-go/internal/framework/pipeline"
	"github.com/zncdatadev/operator-go/pkg/framework"
	"github.com/zncdatadev/operator-go/pkg/framework/dataops"
	"github.com/zncdatadev/operator-go/pkg/framework/input"
)

// Options supplies deployment facts, optional read-only resolution and platform
// assembly settings. Zero FactRefreshInterval selects the controller default.
type Options[C, S, F any] struct {
	Facts        F
	ResolveFacts func(context.Context, framework.FactsReader,
		framework.FactInput[C, S, F]) (framework.FactResult[F], error)
	Assembly            framework.AssemblyOptions
	FactRefreshInterval time.Duration
}

// Register is the generated companion's implementation seam. Registration adds
// types and a controller, but reads no CR, installs no CRD/RBAC and never starts
// the manager. Call it before manager.Start; registration is not a hot update.
func Register[CR input.Object, C, S, F any](manager ctrl.Manager,
	definition framework.ProductDefinition[C, S, F], options Options[C, S, F], binding input.Binding[CR],
) error {
	if manager == nil {
		return fmt.Errorf("registration requires a manager")
	}
	reconciler, err := prepareRegistration(definition, options, binding)
	if err != nil {
		return err
	}
	configuration, scheme := manager.GetConfig(), manager.GetScheme()
	if configuration == nil || scheme == nil {
		return fmt.Errorf("registration requires the manager configuration and scheme")
	}
	for _, add := range []func(*runtime.Scheme) error{
		corev1.AddToScheme, appsv1.AddToScheme, policyv1.AddToScheme, storagev1.AddToScheme, dataops.AddToScheme, binding.AddToScheme,
	} {
		if err := add(scheme); err != nil {
			return fmt.Errorf("register resource types: %w", err)
		}
	}
	// The cache only schedules observations. Intent, ownership and conflict
	// retries must use current API reads through this independent client.
	direct, err := client.New(configuration, client.Options{
		Scheme: scheme, HTTPClient: manager.GetHTTPClient(), Mapper: manager.GetRESTMapper(),
	})
	if err != nil {
		return fmt.Errorf("create direct controller client: %w", err)
	}
	reconciler.Client, reconciler.Scheme = direct, scheme
	if err := reconciler.SetupWithManager(manager); err != nil {
		return fmt.Errorf("register controller: %w", err)
	}
	return nil
}

func prepareRegistration[CR input.Object, C, S, F any](definition framework.ProductDefinition[C, S, F],
	options Options[C, S, F], binding input.Binding[CR],
) (*controller.Reconciler[CR, C, S, F], error) {
	if err := input.CheckVersion(binding.Version); err != nil {
		return nil, err
	}
	if binding.AddToScheme == nil || len(binding.Roles) == 0 || binding.NewObject == nil ||
		binding.Status == nil || binding.Operation == nil || binding.Project == nil {
		return nil, fmt.Errorf("complete generated input binding is required")
	}
	if err := pipeline.ValidateDefinition(definition); err != nil {
		return nil, fmt.Errorf("register product: %w", err)
	}
	if options.FactRefreshInterval < 0 {
		return nil, fmt.Errorf("fact refresh interval must be zero (default) or positive")
	}
	roles := make([]string, 0, len(definition.Roles))
	for role := range definition.Roles {
		roles = append(roles, role)
	}
	slices.Sort(roles)
	expected := slices.Sorted(slices.Values(binding.Roles))
	if !slices.Equal(roles, expected) {
		return nil, fmt.Errorf("product roles %v differ from generated input roles %v; regenerate input artifacts",
			roles, expected)
	}
	// Copy data only. Callback closures retain their identity and the author
	// remains responsible for any state captured outside these owned values.
	owned := definition
	owned.ClusterConfigDefaults = input.Clone(definition.ClusterConfigDefaults)
	owned.Roles = make(map[string]framework.RoleDefinition[C], len(definition.Roles))
	for name, role := range definition.Roles {
		owned.Roles[name] = input.Clone(role)
	}
	assembly := options.Assembly
	assembly.HelperIdentity = assembly.HelperIdentity.DeepCopy()
	assembly.VectorDestination = input.Clone(assembly.VectorDestination)
	binding.Roles = slices.Clone(binding.Roles)
	return &controller.Reconciler[CR, C, S, F]{Binding: binding, Definition: owned,
		Assembly: assembly, Facts: input.Clone(options.Facts), ResolveFacts: options.ResolveFacts,
		FactRefreshInterval: options.FactRefreshInterval}, nil
}
