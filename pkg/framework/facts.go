package framework

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// FactResource is a Kubernetes object accepted by the exact-read facts seam.
// It adds no write operations or dependency on a controller implementation.
type FactResource interface {
	metav1.Object
	runtime.Object
}

// FactsReader permits exact object reads only. The framework records provenance
// and schedules refresh; a product resolver cannot write, list or add watches here.
type FactsReader interface {
	Get(context.Context, types.NamespacedName, FactResource) error
}

type FactState string

const (
	FactsResolved  FactState = "resolved"
	FactsPending   FactState = "pending"
	FactsInvalid   FactState = "invalid"
	FactsReadError FactState = "readError"
)

// FactObject records observed identity and freshness, never object contents.
type FactObject struct {
	APIVersion      string `json:"apiVersion"`
	Kind            string `json:"kind"`
	Namespace       string `json:"namespace"`
	Name            string `json:"name"`
	UID             string `json:"uid,omitempty"`
	ResourceVersion string `json:"resourceVersion,omitempty"`
}

type FactDiagnostic struct {
	State    FactState    `json:"state"`
	Reason   string       `json:"reason,omitempty"`
	Message  string       `json:"message,omitempty"`
	Observed []FactObject `json:"observed,omitempty"` // supplied by the framework's tracked reader
}

// FactResult contains a value only when Resolved. The framework owns Observed
// provenance and replaces any observations returned by a product resolver.
type FactResult[F any] struct {
	Value      *F
	Diagnostic FactDiagnostic
}

// FactInput carries folded intent and declared topology, not observed workloads.
// Shared is registration-level base data; the resolver supplies this group's Facts.
type FactInput[C, S, F any] struct {
	Platform      ClusterConfig
	ClusterConfig S
	Group         GroupIdentity
	Config        Config[C]
	Image         ResolvedImage
	Shared        F
	Topology      []ResolvedGroup[C]
}
