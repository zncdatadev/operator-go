package framework

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// ProductDefinition declares product behavior once. Callbacks consume isolated
// effective values and return intent or checks; they do not perform cluster I/O.
type ProductDefinition[C, S, F any] struct {
	ClusterConfigDefaults S
	Name                  string
	ImageDefaults         ImageConfig
	Roles                 map[string]RoleDefinition[C]
	ValidateInput         func(EffectiveInput[C, S, F]) error
	GenerateGroup         func(EffectiveInput[C, S, F]) (RuntimeDescription, error)
	GenerateCluster       func(ClusterOutputInput[S, F]) (ClusterOutput, error)
	ValidateFinal         func(FinalView) []Check
}

// EffectiveInput carries resolved configuration and facts for one group. Input
// presence and the source projection remain outside this product-facing value.
type EffectiveInput[C, S, F any] struct {
	Platform      ClusterConfig
	ClusterConfig S
	Group         GroupIdentity
	Config        Config[C]
	Image         ResolvedImage
	Facts         F
	Topology      []ResolvedGroup[C]
}

// ResolvedGroup preserves declared topology when a sibling cannot resolve its
// configuration. Config is nil on failure; the declared identity remains present.
type ResolvedGroup[C any] struct {
	Group  GroupIdentity
	Config *Config[C]
	Error  string
}

type GroupOutcome struct {
	Platform           *PlatformObservation
	Group              GroupIdentity
	GeneratedEndpoints []Endpoint // declarations, not observed ready endpoints
	Error              string
	Facts              *FactDiagnostic
}

type ClusterOutputInput[S, F any] struct {
	ClusterConfig S
	Cluster       ClusterIdentity
	Shared        F
	Groups        []GroupOutcome
}

type ClusterOutputState string

const (
	ClusterOutputReady   ClusterOutputState = "Ready"
	ClusterOutputPending ClusterOutputState = "Pending"
)

// ClusterOutput distinguishes a complete shared-resource inventory from waiting
// for inputs. Ready with no ConfigMaps withdraws all previous shared outputs.
// Pending supplies a reason and no partial output. An error also preserves old
// outputs. Absence of the optional callback is a complete empty inventory.
type ClusterOutput struct {
	State      ClusterOutputState
	ConfigMaps []corev1.ConfigMap
	Reason     string
}

// ValidateClusterOutput checks the state/value contract only. Resource identity,
// ownership and application are checked by the framework's execution layers.
func ValidateClusterOutput(output ClusterOutput) error {
	switch output.State {
	case ClusterOutputReady:
		return nil
	case ClusterOutputPending:
		if strings.TrimSpace(output.Reason) == "" {
			return fmt.Errorf("pending cluster output requires a reason")
		}
		if len(output.ConfigMaps) != 0 {
			return fmt.Errorf("pending cluster output cannot include partial ConfigMaps")
		}
		return nil
	default:
		return fmt.Errorf("cluster output must explicitly be Ready or Pending")
	}
}

// FinalView is an isolated view after overrides and assembly. Product checks
// report relationships; they cannot repair or replace the final resources.
type FinalView struct {
	Generated RuntimeDescription
	Files     []File
	// These are structural premises, not proof of execution or log delivery.
	FilePreparationKnown bool
	LogCollectionKnown   bool
	Pod                  corev1.PodTemplateSpec
	Services             []corev1.Service
}
