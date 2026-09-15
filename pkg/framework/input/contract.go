package input

import (
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/zncdatadev/operator-go/pkg/framework"
)

// ContractVersion versions the generated binding, separately from SDK releases.
// Increment it when generated inputs and their runtime contract become incompatible.
const ContractVersion = 1

// CheckVersion rejects a binding from an incompatible generator contract.
func CheckVersion(version int) error {
	if version != ContractVersion {
		return fmt.Errorf("generated input contract version %d is incompatible with version %d; regenerate inputs", version, ContractVersion)
	}
	return nil
}

// Object is the Kubernetes object shape required by generated root bindings.
type Object interface {
	metav1.Object
	runtime.Object
}

// Binding is emitted by inputgen. It does not expose the reconciler or a staged
// resource pipeline. Operation is separate so pause does not depend on Project.
type Binding[CR Object] struct {
	Version     int
	Roles       []string
	AddToScheme func(*runtime.Scheme) error
	NewObject   func() CR
	Operation   func(CR) framework.ClusterOperation
	Project     func(CR) (Projection, error)
	Status      func(CR) *framework.ReconcileStatus
}

// Projection is an isolated raw CR snapshot. Replicas retain presence; neither
// role inheritance nor defaults are applied here. Empty roles are not discarded.
// Runtime facts and resolved values belong to the internal pipeline, not this ABI.
type Projection struct {
	Cluster       framework.ClusterIdentity
	Image         json.RawMessage
	ClusterConfig json.RawMessage
	Roles         []Role
}

// Role carries management and workload input in separate raw blocks.
type Role struct {
	Name       string
	Replicas   *int32
	Config     json.RawMessage
	RoleConfig json.RawMessage
	Overrides  *Overrides
	Groups     []Group
}

// Group contains exactly the user's group layer, without inherited role fields.
type Group struct {
	Name      string
	Replicas  *int32
	Config    json.RawMessage
	Overrides *Overrides
}

// ImageInput holds presence independently for image source and pull settings.
type ImageInput struct {
	Custom          *string            `json:"custom,omitempty"`
	Repo            *string            `json:"repo,omitempty"`
	ProductVersion  *string            `json:"productVersion,omitempty"`
	KubedoopVersion *string            `json:"kubedoopVersion,omitempty"`
	PullPolicy      *corev1.PullPolicy `json:"pullPolicy,omitempty"`
	PullSecretName  *string            `json:"pullSecretName,omitempty"`
}

type RoleConfigInput struct {
	PodDisruptionBudget *PodDisruptionBudgetInput `json:"podDisruptionBudget,omitempty"`
}

type PodDisruptionBudgetInput struct {
	Enabled        *bool  `json:"enabled,omitempty"`
	MaxUnavailable *int32 `json:"maxUnavailable,omitempty"`
}

// Overrides groups the wire's flat channels after projection. A nil channel is
// absent; CLIOverrides pointing to an empty slice explicitly clears arguments.
type Overrides struct {
	ConfigOverrides map[string]FileOverride `json:"configOverrides,omitempty"`
	EnvOverrides    map[string]string       `json:"envOverrides,omitempty"`
	CLIOverrides    *[]string               `json:"cliOverrides,omitempty"`
	PodOverrides    json.RawMessage         `json:"podOverrides,omitempty"`
}

// FileOverride expresses one file action. Execution and cross-file validation
// belong to the pipeline, not the generated input binding.
type FileOverride struct {
	Properties *PropertyOverride `json:"properties,omitempty"`
	Lines      *[]string         `json:"lines,omitempty"`
	Text       *string           `json:"text,omitempty"`
	Remove     *bool             `json:"remove,omitempty"`
}

type PropertyOverride struct {
	Set     *map[string]string `json:"set,omitempty"`
	Remove  *[]string          `json:"remove,omitempty"`
	Replace *map[string]string `json:"replace,omitempty"`
}
