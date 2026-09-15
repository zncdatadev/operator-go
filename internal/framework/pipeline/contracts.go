// Package pipeline owns the SDK's pure config-to-resource execution. Its plans
// and stages are internal; products declare framework values, not this pipeline.
package pipeline

import (
	"encoding/json"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/zncdatadev/operator-go/pkg/framework"
	"github.com/zncdatadev/operator-go/pkg/framework/input"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
)

// Aliases keep one domain model shared with the public author contract. No
// product-facing types are redefined by the implementation package.
type CommonConfig = framework.CommonConfig
type Resources = framework.Resources
type CPU = framework.CPU
type Memory = framework.Memory
type Logging = framework.Logging
type ContainerLogging = framework.ContainerLogging
type Logger = framework.Logger
type RoleConfig = framework.RoleConfig
type PodDisruptionBudgetConfig = framework.PodDisruptionBudgetConfig
type ImageConfig = framework.ImageConfig
type ResolvedImage = framework.ResolvedImage
type ClusterOperation = framework.ClusterOperation
type AssemblyOptions = framework.AssemblyOptions
type ClusterIdentity = framework.ClusterIdentity
type GroupIdentity = framework.GroupIdentity
type RoleIdentity = framework.RoleIdentity
type GroupOutcome = framework.GroupOutcome
type ClusterOutputState = framework.ClusterOutputState
type ClusterOutput = framework.ClusterOutput
type FinalView = framework.FinalView
type FactResource = framework.FactResource
type FactsReader = framework.FactsReader
type FactState = framework.FactState
type FactObject = framework.FactObject
type FactDiagnostic = framework.FactDiagnostic
type RuntimeDescription = framework.RuntimeDescription
type Process = framework.Process
type Directory = framework.Directory

// RetainedData is the resolved physical request, owned by assembly.
type RetainedData struct {
	StorageClassName string
	Capacity         resource.Quantity
}
type DirectoryAccess = framework.DirectoryAccess
type File = framework.File
type FileContent = framework.FileContent
type KeyValues = framework.KeyValues
type Lines = framework.Lines
type Text = framework.Text
type PropertyCodec = framework.PropertyCodec
type PropertiesCodec = framework.PropertiesCodec
type PropertyValue = framework.PropertyValue
type Literal = framework.Literal
type PodNameBinding = framework.PodNameBinding
type Endpoint = framework.Endpoint
type LogOutput = framework.LogOutput
type CheckState = framework.CheckState
type Check = framework.Check
type ImageInput = input.ImageInput
type RoleConfigInput = input.RoleConfigInput
type PodDisruptionBudgetInput = input.PodDisruptionBudgetInput
type Overrides = input.Overrides
type FileOverride = input.FileOverride
type PropertyOverride = input.PropertyOverride

const (
	Consistent                = framework.Consistent
	Conflict                  = framework.Conflict
	Unknown                   = framework.Unknown
	FactsResolved             = framework.FactsResolved
	FactsPending              = framework.FactsPending
	FactsInvalid              = framework.FactsInvalid
	FactsReadError            = framework.FactsReadError
	ClusterOutputReady        = framework.ClusterOutputReady
	ClusterOutputPending      = framework.ClusterOutputPending
	materializerContainerName = "prepare-files"
	materializationPlanVolume = "config-plan"
	materializationPlanFile   = "materialization.json"
	materializationRoot       = "/materialized"
	materializationPlanPath   = "/plan/materialization.json"
	vectorContainerName       = "vector"
	vectorConfigFile          = "vector.yaml"
	vectorConfigPath          = "/etc/vector"
)

// SourceSnapshot is built from a raw Projection plus independently supplied
// facts and execution intent. It is never emitted by external generated code.
type SourceSnapshot[F any] struct {
	Operation     ClusterOperation
	ClusterConfig json.RawMessage
	Cluster       ClusterIdentity
	Image         json.RawMessage
	Shared        F
	Groups        []GroupSource
	Roles         []RoleSource
}

type RoleSource struct {
	Name   string
	Config json.RawMessage
}
type GroupSource struct {
	Role, Name               string
	Replicas                 int32
	RoleConfigLayer, Config  json.RawMessage
	RoleOverrides, Overrides *Overrides
}
type GroupKey struct{ Role, Name string }

type PreparedInputs[C, S, F any] struct {
	Platform      framework.ClusterConfig
	ClusterConfig S
	Source        SourceSnapshot[F]
	Image         ResolvedImage
	Topology      []framework.ResolvedGroup[C]
}

type GeneratedGroup[C, S, F any] struct {
	Outcome GroupOutcome
	Input   *framework.EffectiveInput[C, S, F]
	Runtime *RuntimeDescription
}

type GroupResources struct {
	Coordination    *framework.WorkloadCoordination
	ConfigMap       corev1.ConfigMap
	StatefulSet     appsv1.StatefulSet
	Service         corev1.Service
	HeadlessService corev1.Service
	RetainedData    *RetainedDataSlot
}
type RetainedDataSlot struct {
	Name string
	RetainedData
}
type BuiltRole struct {
	Role                RoleIdentity
	Config              *RoleConfig
	PodDisruptionBudget *policyv1.PodDisruptionBudget
	Error               string
}
type BuiltGroup[C, S, F any] struct {
	Outcome   GroupOutcome
	Input     *framework.EffectiveInput[C, S, F]
	Runtime   *RuntimeDescription
	Files     []File
	Resources *GroupResources
	Checks    []Check
}

type ResourcePlan[C, S, F any] struct {
	Prepared      *PreparedInputs[C, S, F]
	Groups        []BuiltGroup[C, S, F]
	Roles         []BuiltRole
	ClusterOutput ClusterOutput
	ClusterError  string
}

// CloneInput is restricted to generated/config data, never arbitrary resources,
// callbacks or runtime descriptions containing codecs.
func CloneInput[T any](value T) T { return input.Clone(value) }
