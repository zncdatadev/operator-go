package pipeline

import (
	"fmt"
	"slices"

	"github.com/zncdatadev/operator-go/pkg/framework"
	"k8s.io/apimachinery/pkg/util/validation"
)

// BuildResources executes the pure fixed-order pipeline. Returned plans carry
// diagnostics and desired resources, never client operations or observed health.
func BuildResources[C, S, F any](definition framework.ProductDefinition[C, S, F], source SourceSnapshot[F],
	options AssemblyOptions,
) (ResourcePlan[C, S, F], error) {
	prepared, err := PrepareInputs(definition, source)
	if err != nil {
		return ResourcePlan[C, S, F]{}, err
	}
	return BuildPreparedResources(definition, prepared, nil, options)
}

// BuildPreparedResources consumes an explicit result for each valid group when
// facts is non-nil. A missing or unresolved result withholds only its group plan.
func BuildPreparedResources[C, S, F any](
	definition framework.ProductDefinition[C, S, F], prepared PreparedInputs[C, S, F],
	facts map[GroupKey]framework.FactResult[F], options AssemblyOptions,
) (ResourcePlan[C, S, F], error) {
	generated, err := GeneratePreparedGroups(definition, prepared, facts)
	if err != nil {
		return ResourcePlan[C, S, F]{}, err
	}
	return AssemblePreparedGroups(definition, prepared, generated, options)
}

// AssemblePreparedGroups consumes descriptions exactly once after runtime-dependent
// platform references have been resolved outside the pure pipeline. Unresolved
// groups retain their outcome but do not acquire a desired resource set.
func AssemblePreparedGroups[C, S, F any](
	definition framework.ProductDefinition[C, S, F], prepared PreparedInputs[C, S, F],
	generated []GeneratedGroup[C, S, F], options AssemblyOptions,
) (ResourcePlan[C, S, F], error) {
	prepared = CloneInput(prepared)
	plan := ResourcePlan[C, S, F]{Prepared: &prepared, ClusterOutput: ClusterOutput{State: ClusterOutputReady}}
	source := prepared.Source
	roles, err := BuildRoleResources(definition, source)
	if err != nil {
		return plan, err
	}
	plan.Roles = roles
	layers := make(map[string]GroupSource, len(source.Groups))
	for _, group := range source.Groups {
		layers[group.Role+"/"+group.Name] = group
	}
	for _, group := range generated {
		built := BuiltGroup[C, S, F]{Outcome: group.Outcome, Input: group.Input, Runtime: group.Runtime}
		if group.Outcome.Error == "" && group.Runtime != nil {
			if prepared.Platform.VectorAgentConfigMap != "" && group.Input.Config.Common.Logging.EnableVectorAgent &&
				len(group.Runtime.LogOutputs) > 0 && options.VectorDestination == nil {
				built.Outcome.Error = "vectorAgentConfigMap requires a resolved destination before assembly"
				built.Outcome.GeneratedEndpoints = nil
				plan.Groups = append(plan.Groups, built)
				continue
			}
			layer := layers[group.Outcome.Group.Role+"/"+group.Outcome.Group.Name]
			built.Resources, built.Files, built.Checks, err = buildGroup(group.Outcome.Group,
				group.Input.Config.Common, group.Input.Image, *group.Runtime, layer, options, definition.ValidateFinal)
			if err != nil {
				built.Outcome.Error = err.Error()
				built.Outcome.GeneratedEndpoints = nil
			} else if source.Operation.Stopped {
				zero := int32(0)
				built.Resources.StatefulSet.Spec.Replicas = &zero
			}
		}
		plan.Groups = append(plan.Groups, built)
	}
	if err := checkResourceInventory(plan); err != nil {
		return plan, err
	}
	if definition.GenerateCluster != nil {
		plan.ClusterOutput, err = generateClusterOutput(definition, prepared, plan.Groups)
		if err == nil {
			err = checkResourceInventory(plan)
		}
		if err != nil {
			plan.ClusterError = err.Error()
			plan.ClusterOutput = ClusterOutput{}
		}
	}
	return plan, nil
}

// GeneratePreparedGroups validates effective input and invokes each eligible
// product generator once. Actual LogOutputs then decide platform dependencies.
func GeneratePreparedGroups[C, S, F any](definition framework.ProductDefinition[C, S, F],
	prepared PreparedInputs[C, S, F], facts map[GroupKey]framework.FactResult[F],
) ([]GeneratedGroup[C, S, F], error) {
	prepared = CloneInput(prepared)
	facts, err := normalizeFacts(prepared.Topology, facts)
	if err != nil {
		return nil, err
	}
	source := prepared.Source
	topology, err := validateTopology(
		definition, prepared.Platform, prepared.ClusterConfig, prepared.Image, source.Shared, prepared.Topology, facts)
	if err != nil {
		return nil, err
	}
	groups := make([]GeneratedGroup[C, S, F], 0, len(topology))
	for _, group := range topology {
		generated := GeneratedGroup[C, S, F]{Outcome: GroupOutcome{Group: group.Group, Error: group.Error}}
		fact, external := facts[GroupKey{Role: group.Group.Role, Name: group.Group.Name}]
		if external {
			generated.Outcome.Facts = &fact.Diagnostic
		}
		if group.Error == "" && (!external || fact.Diagnostic.State == FactsResolved) {
			value := source.Shared
			if external {
				value = *fact.Value
			}
			in, err := snapshotInput(group, prepared.ClusterConfig, prepared.Image, value, topology)
			if err != nil {
				return nil, fmt.Errorf("input snapshot: %w", err)
			}
			in.Platform = CloneInput(prepared.Platform)
			generated.Input = &in
			generated.Runtime, err = generateOne(definition, in)
			if err != nil {
				generated.Outcome.Error = err.Error()
			} else {
				generated.Outcome.GeneratedEndpoints = slices.Clone(generated.Runtime.Endpoints)
				for _, directory := range generated.Runtime.Directories {
					if directory.Secret != nil || directory.Listener != nil {
						generated.Outcome.Platform = &framework.PlatformObservation{Phase: "Preparing",
							Diagnostic: framework.FactDiagnostic{State: framework.FactsPending, Reason: "PlatformNotObserved",
								Message: "Platform producer has not been observed"}}
						break
					}
				}
			}
		}
		groups = append(groups, generated)
	}
	return groups, nil
}

func generateClusterOutput[C, S, F any](
	definition framework.ProductDefinition[C, S, F], prepared PreparedInputs[C, S, F],
	groups []BuiltGroup[C, S, F],
) (ClusterOutput, error) {
	outcomes := make([]GroupOutcome, 0, len(groups))
	for _, group := range groups {
		outcomes = append(outcomes, group.Outcome)
	}
	in, err := copyJSON(framework.ClusterOutputInput[S, F]{Cluster: prepared.Source.Cluster,
		ClusterConfig: prepared.ClusterConfig, Shared: prepared.Source.Shared, Groups: outcomes})
	if err != nil {
		return ClusterOutput{}, err
	}
	output, err := definition.GenerateCluster(in)
	if err != nil {
		return ClusterOutput{}, err
	}
	if err := framework.ValidateClusterOutput(output); err != nil {
		return ClusterOutput{}, err
	}
	// A callback's retained map must not mutate the returned resource plan.
	output.ConfigMaps = slices.Clone(output.ConfigMaps)
	for index := range output.ConfigMaps {
		cm := output.ConfigMaps[index].DeepCopy()
		if cm.Namespace != in.Cluster.Namespace || len(validation.IsDNS1123Subdomain(cm.Name)) != 0 {
			return ClusterOutput{}, fmt.Errorf("cluster ConfigMap must have a valid name in namespace %q", in.Cluster.Namespace)
		}
		if err := checkSharedConfigMapSlot(prepared.Topology, cm.Namespace, cm.Name); err != nil {
			return ClusterOutput{}, err
		}
		cm.Labels = resourceLabels(prepared.Source.Cluster, cm.Labels)
		output.ConfigMaps[index] = *cm
	}
	return output, nil
}

// A group's fixed ConfigMap slot remains reserved while its configuration or
// facts are unavailable. Shared outputs cannot occupy a temporarily absent slot.
func checkSharedConfigMapSlot[C any](topology []framework.ResolvedGroup[C], namespace, name string) error {
	for _, group := range topology {
		identity := group.Group
		if identity.Namespace == namespace && identity.ServiceName() == name {
			return fmt.Errorf("cluster ConfigMap %s/%s conflicts with reserved ConfigMap slot for role group %s/%s",
				namespace, name, identity.Role, identity.Name)
		}
	}
	return nil
}

// RefreshClusterOutput uses post-apply platform observations while preserving
// the same pure generation and reserved-slot validation as the original plan.
func RefreshClusterOutput[C, S, F any](definition framework.ProductDefinition[C, S, F], plan *ResourcePlan[C, S, F]) {
	if definition.GenerateCluster == nil || plan.Prepared == nil {
		return
	}
	output, err := generateClusterOutput(definition, *plan.Prepared, plan.Groups)
	if err != nil {
		plan.ClusterError = err.Error()
		plan.ClusterOutput = ClusterOutput{}
		return
	}
	plan.ClusterOutput = output
	plan.ClusterError = ""
	if err = checkResourceInventory(*plan); err != nil {
		plan.ClusterError = err.Error()
		plan.ClusterOutput = ClusterOutput{}
	}
}
