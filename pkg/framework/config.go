package framework

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Config is the complete effective configuration passed to product code. Common
// and Product share one flat config object in the CR; these fields are not wire nesting.
type Config[C any] struct {
	Common  CommonConfig
	Product C
}

type CommonConfig struct {
	Resources               Resources       `json:"resources"`
	Logging                 Logging         `json:"logging"`
	Affinity                corev1.Affinity `json:"affinity"`
	GracefulShutdownTimeout metav1.Duration `json:"gracefulShutdownTimeout"`
}

type Resources struct {
	CPU     CPU     `json:"cpu"`
	Memory  Memory  `json:"memory"`
	Storage Storage `json:"storage"`
}

// Storage is the standard data-directory storage domain. An empty product
// default selects ephemeral storage. User input must name a supported type.
// A type change discards the inherited branch; it never migrates live data.
type Storage struct {
	Type             StorageType       `json:"type"`
	StorageClassName string            `json:"storageClassName"`
	Capacity         resource.Quantity `json:"capacity"`
}

type StorageType string

const (
	StorageEphemeral  StorageType = "ephemeral"
	StoragePersistent StorageType = "persistent"
)

type CPU struct {
	Min resource.Quantity `json:"min"`
	Max resource.Quantity `json:"max"`
}

type Memory struct {
	Limit resource.Quantity `json:"limit"`
}

type Logging struct {
	EnableVectorAgent bool                        `json:"enableVectorAgent"`
	Containers        map[string]ContainerLogging `json:"containers,omitempty"`
}

type ContainerLogging struct {
	Console Logger            `json:"console"`
	File    Logger            `json:"file"`
	Loggers map[string]Logger `json:"loggers,omitempty"`
}

type Logger struct {
	Level string `json:"level"`
}

// RoleDefinition separates inherited workload defaults from management defaults
// consumed once for the entire role, including when it has no groups.
type RoleDefinition[C any] struct {
	Config     Config[C]
	RoleConfig RoleConfig
}

type RoleConfig struct {
	PodDisruptionBudget PodDisruptionBudgetConfig `json:"podDisruptionBudget"`
}

type PodDisruptionBudgetConfig struct {
	Enabled        bool  `json:"enabled"`
	MaxUnavailable int32 `json:"maxUnavailable"`
}

// ImageConfig contains product defaults. Input presence and image resolution
// belong to the input boundary and internal pipeline, respectively.
type ImageConfig struct {
	Custom          string            `json:"custom"`
	Repo            string            `json:"repo"`
	ProductVersion  string            `json:"productVersion"`
	KubedoopVersion string            `json:"kubedoopVersion"`
	PullPolicy      corev1.PullPolicy `json:"pullPolicy"`
	PullSecretName  string            `json:"pullSecretName"`
}

type ResolvedImage struct {
	Reference      string            `json:"reference"`
	PullPolicy     corev1.PullPolicy `json:"pullPolicy"`
	PullSecretName string            `json:"pullSecretName"`
}

// ClusterOperation is separate from product cluster configuration. Generated
// bindings read these fixed controls before full input projection or validation.
type ClusterOperation struct {
	Stopped              bool `json:"stopped"`
	ReconciliationPaused bool `json:"reconciliationPaused"`
}

// AssemblyOptions supplies deployment-owned helper images and identity. It is
// neither a capability registry nor an image builder or runtime-user detector.
type AssemblyOptions struct {
	MaterializerImage string
	VectorImage       string
	HelperIdentity    *corev1.SecurityContext
	// VectorDestination is supplied by the controller after resolving the
	// standard cluster reference. Nil selects the local stdout JSON sink.
	VectorDestination *VectorDestination
}
