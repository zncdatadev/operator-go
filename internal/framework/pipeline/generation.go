package pipeline

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/zncdatadev/operator-go/pkg/framework"
)

func copyJSON[T any](value T) (T, error) {
	var out T
	data, err := json.Marshal(value)
	if err != nil {
		return out, err
	}
	err = json.Unmarshal(data, &out)
	return out, err
}

func resolveTopology[C, S, F any](
	definition framework.ProductDefinition[C, S, F], source SourceSnapshot[F],
) ([]framework.ResolvedGroup[C], error) {
	identities, err := SourceGroupIdentities(source)
	if err != nil {
		return nil, err
	}
	layers := make(map[string]GroupSource, len(source.Groups))
	for _, group := range source.Groups {
		layers[group.Role+"/"+group.Name] = group
	}
	topology := make([]framework.ResolvedGroup[C], 0, len(identities))
	for _, identity := range identities {
		group := layers[identity.Role+"/"+identity.Name]
		resolved := framework.ResolvedGroup[C]{Group: identity}
		defaults, ok := definition.Roles[group.Role]
		if !ok {
			resolved.Error = "role is not declared by the product"
		} else if config, err := ResolveConfig(defaults.Config, group.RoleConfigLayer, group.Config); err != nil {
			resolved.Error = err.Error()
		} else {
			resolved.Config = &config
		}
		topology = append(topology, resolved)
	}
	return topology, nil
}

func snapshotInput[C, S, F any](
	group framework.ResolvedGroup[C], clusterConfig S, image ResolvedImage, shared F,
	topology []framework.ResolvedGroup[C],
) (framework.EffectiveInput[C, S, F], error) {
	return copyJSON(framework.EffectiveInput[C, S, F]{Group: group.Group, Config: *group.Config,
		ClusterConfig: clusterConfig, Image: image, Facts: shared, Topology: topology})
}

// Input validators all see the same resolved snapshot, not one another's partial
// validation results. Generators then see all input failures. This is not a
// dependency scheduler: a later generation failure is only in GroupOutcome.
func validateTopology[C, S, F any](
	definition framework.ProductDefinition[C, S, F], platform framework.ClusterConfig, clusterConfig S,
	image ResolvedImage, shared F,
	resolved []framework.ResolvedGroup[C], facts map[GroupKey]framework.FactResult[F],
) ([]framework.ResolvedGroup[C], error) {
	if definition.ValidateInput == nil {
		return resolved, nil
	}
	validated := slices.Clone(resolved)
	for i, group := range resolved {
		if group.Error != "" {
			continue
		}
		value := shared
		if fact, ok := facts[GroupKey{Role: group.Group.Role, Name: group.Group.Name}]; ok {
			if fact.Diagnostic.State != FactsResolved {
				continue // unavailable facts do not invalidate declared configuration
			}
			value = *fact.Value
		}
		input, err := snapshotInput(group, clusterConfig, image, value, resolved)
		if err != nil {
			return nil, fmt.Errorf("validation snapshot: %w", err)
		}
		input.Platform = CloneInput(platform)
		if err := definition.ValidateInput(input); err != nil {
			validated[i].Config = nil
			validated[i].Error = err.Error()
		}
	}
	return validated, nil
}

func generateOne[C, S, F any](
	definition framework.ProductDefinition[C, S, F], input framework.EffectiveInput[C, S, F],
) (*RuntimeDescription, error) {
	generationInput, err := copyJSON(input)
	if err != nil {
		return nil, err
	}
	runtime, err := definition.GenerateGroup(generationInput)
	if err != nil {
		return nil, err
	}
	runtime = CloneRuntime(runtime)
	if runtime.Main.Image == "" {
		runtime.Main.Image = input.Image.Reference
	} else if runtime.Main.Image != input.Image.Reference {
		return nil, fmt.Errorf("Main.Image %q conflicts with resolved spec.image %q",
			runtime.Main.Image, input.Image.Reference)
	}
	for i := range runtime.Initializers {
		if runtime.Initializers[i].Image == "" {
			runtime.Initializers[i].Image = runtime.Main.Image
		}
	}
	if err := ValidateRuntime(runtime); err != nil {
		return nil, err
	}
	return &runtime, nil
}
