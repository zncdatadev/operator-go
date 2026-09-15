package pipeline

import (
	"fmt"
	"reflect"

	"github.com/zncdatadev/operator-go/pkg/framework"
)

const factMissingResultReason = "MissingResult"

func PrepareInputs[C, S, F any](definition framework.ProductDefinition[C, S, F], source SourceSnapshot[F]) (
	PreparedInputs[C, S, F], error,
) {
	var out PreparedInputs[C, S, F]
	if err := ValidateDefinition(definition); err != nil {
		return out, err
	}
	if err := checkProfile(reflect.TypeFor[F](), make(map[reflect.Type]bool)); err != nil {
		return out, fmt.Errorf("shared facts: %w", err)
	}
	// Preserve omitted raw config layers: JSON roundtrip would turn nil into literal null.
	snapshot := CloneInput(source)
	clusterConfig, err := ResolveClusterConfig(definition.ClusterConfigDefaults, snapshot.ClusterConfig)
	if err != nil {
		return out, err
	}
	platform, _, err := splitPlatformConfig(snapshot.ClusterConfig)
	if err != nil {
		return out, err
	}
	image, err := ResolveImage(definition.Name, definition.ImageDefaults, snapshot.Image)
	if err != nil {
		return out, err
	}
	if _, err := SourceRoleIdentities(snapshot); err != nil {
		return out, err
	}
	topology, err := resolveTopology(definition, snapshot)
	if err != nil {
		return out, err
	}
	return PreparedInputs[C, S, F]{Platform: platform, Source: snapshot, ClusterConfig: clusterConfig,
		Image: image, Topology: topology}, nil
}

func normalizeFacts[C, F any](topology []framework.ResolvedGroup[C], facts map[GroupKey]framework.FactResult[F]) (
	map[GroupKey]framework.FactResult[F], error,
) {
	if facts == nil {
		return nil, nil
	}
	known := make(map[GroupKey]bool, len(topology))
	out := make(map[GroupKey]framework.FactResult[F], len(topology))
	for _, group := range topology {
		key := GroupKey{Role: group.Group.Role, Name: group.Group.Name}
		known[key] = true
		if group.Config == nil {
			continue
		}
		result, ok := facts[key]
		if !ok {
			result.Diagnostic = FactDiagnostic{State: FactsInvalid, Reason: factMissingResultReason,
				Message: "facts resolver did not provide a result for this group"}
		} else {
			switch result.Diagnostic.State {
			case FactsResolved:
				if result.Value == nil {
					result = invalidFactResult(result)
				}
			case FactsPending, FactsInvalid, FactsReadError:
				if result.Value != nil {
					result = invalidFactResult(result)
				}
			default:
				result = invalidFactResult(result)
			}
		}
		cloned, err := copyJSON(result)
		if err != nil {
			// A supported type can still contain a value outside the JSON data
			// profile (for example NaN). That invalidates only its consumer.
			cloned = framework.FactResult[F]{Diagnostic: FactDiagnostic{State: FactsInvalid, Reason: "InvalidFactValue",
				Message:  "resolved facts cannot be represented by the supported data profile",
				Observed: CloneInput(result.Diagnostic.Observed)}}
		}
		out[key] = cloned
	}
	for key := range facts {
		if !known[key] {
			return nil, fmt.Errorf("facts result references unknown group %s/%s", key.Role, key.Name)
		}
	}
	return out, nil
}

func invalidFactResult[F any](in framework.FactResult[F]) framework.FactResult[F] {
	return framework.FactResult[F]{Diagnostic: FactDiagnostic{State: FactsInvalid, Reason: "InvalidResult",
		Message: "facts result must have a known state and a value only when resolved", Observed: in.Diagnostic.Observed}}
}
