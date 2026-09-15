package pipeline

import (
	"fmt"
	"reflect"

	"github.com/zncdatadev/operator-go/pkg/framework"
	"github.com/zncdatadev/operator-go/pkg/framework/input"
)

// SourceFromProjection combines raw generated input with deployment facts and
// independently read execution intent. Only this internal boundary folds replica
// presence. It never invokes product callbacks or loses groups on config errors.
func SourceFromProjection[F any](
	projection input.Projection, facts F, operation ClusterOperation,
) (SourceSnapshot[F], error) {
	if err := input.ValidateProductType(reflect.TypeFor[F]()); err != nil {
		return SourceSnapshot[F]{}, fmt.Errorf("shared facts: %w", err)
	}
	raw := CloneInput(projection)
	source := SourceSnapshot[F]{Cluster: raw.Cluster, Image: raw.Image,
		ClusterConfig: raw.ClusterConfig, Shared: CloneInput(facts), Operation: operation}
	for _, role := range raw.Roles {
		source.Roles = append(source.Roles, RoleSource{Name: role.Name, Config: role.RoleConfig})
		replicas := int32(1)
		if role.Replicas != nil {
			replicas = *role.Replicas
		}
		if replicas < 0 {
			return source, fmt.Errorf("%s.replicas must not be negative", role.Name)
		}
		for _, group := range role.Groups {
			count := replicas
			if group.Replicas != nil {
				count = *group.Replicas
			}
			if count < 0 {
				return source, fmt.Errorf("%s/%s.replicas must not be negative", role.Name, group.Name)
			}
			source.Groups = append(source.Groups, GroupSource{Role: role.Name, Name: group.Name, Replicas: count,
				RoleConfigLayer: CloneInput(role.Config), Config: group.Config,
				RoleOverrides: CloneInput(role.Overrides), Overrides: group.Overrides})
		}
	}
	if _, err := SourceRoleIdentities(source); err != nil {
		return SourceSnapshot[F]{}, err
	}
	return source, nil
}

// Build is a pure complete build from raw input and resolved base facts. Runtime
// controllers prepare inputs first and supply per-group fact outcomes separately.
func Build[C, S, F any](definition framework.ProductDefinition[C, S, F], projection input.Projection,
	facts F, options AssemblyOptions,
) (ResourcePlan[C, S, F], error) {
	source, err := SourceFromProjection(projection, facts, ClusterOperation{})
	if err != nil {
		return ResourcePlan[C, S, F]{}, err
	}
	return BuildResources(definition, source, options)
}
