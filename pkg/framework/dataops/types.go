// Package dataops implements explicitly authorized, durable operations on retained data.
package dataops

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

var GroupVersion = schema.GroupVersion{Group: "data.framework.kubedoop.dev", Version: "v1alpha1"}

const (
	RetryAnnotation   = "framework.kubedoop.dev/data-retry"
	SourceAnnotation  = "framework.kubedoop.dev/retained-data"
	BindingAnnotation = "framework.kubedoop.dev/retained-binding"
	AssetAnnotation   = "framework.kubedoop.dev/data-asset"
	LockAnnotation    = "framework.kubedoop.dev/data-operation"
)

type ClusterRef struct {
	// +kubebuilder:validation:MaxLength=4096
	APIVersion string `json:"apiVersion"`
	// +kubebuilder:validation:MaxLength=4096
	Kind string `json:"kind"`
	// +kubebuilder:validation:MaxLength=4096
	Name string `json:"name"`
	// +kubebuilder:validation:MaxLength=4096
	// +kubebuilder:validation:Type=string
	UID types.UID `json:"uid"`
}
type Source struct {
	Version int `json:"version"`
	// +kubebuilder:validation:MaxLength=4096
	// +kubebuilder:validation:Type=string
	CRUID types.UID `json:"crUID"`
	// +kubebuilder:validation:MaxLength=4096
	Role string `json:"role"`
	// +kubebuilder:validation:MaxLength=4096
	Group string `json:"group"`
	// +kubebuilder:validation:MaxLength=4096
	Slot string `json:"slot"`
	// +kubebuilder:validation:MaxLength=4096
	StorageClass string `json:"storageClass"`
	// +kubebuilder:validation:MaxLength=4096
	Capacity string `json:"capacity"`
}
type Binding struct {
	Version int `json:"version"`
	// +kubebuilder:validation:MaxLength=4096
	// +kubebuilder:validation:Type=string
	PVCUID types.UID `json:"pvcUID"`
	// +kubebuilder:validation:MaxLength=4096
	// +kubebuilder:validation:Type=string
	PVUID types.UID `json:"pvUID"`
	// +kubebuilder:validation:MaxLength=4096
	VolumeName string `json:"volumeName"`
}
type DataIdentity struct {
	Cluster ClusterRef `json:"cluster"`
	// +kubebuilder:validation:MaxLength=4096
	ClaimName string  `json:"claimName"`
	Binding   Binding `json:"binding"`
	Source    Source  `json:"source"`
}
type History struct {
	// +kubebuilder:validation:MaxLength=4096
	// +kubebuilder:validation:Type=string
	OperationUID types.UID `json:"operationUID"`
	// +kubebuilder:validation:MaxLength=4096
	Action    string        `json:"action"`
	From      DataIdentity  `json:"from"`
	To        *DataIdentity `json:"to,omitempty"`
	Completed metav1.Time   `json:"completed"`
	// +kubebuilder:validation:MaxLength=4096
	Verification string `json:"verification"`
}

// DataAsset is independent of a product CR's ownership and survives its deletion.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type DataAsset struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="initial data identity is immutable"
	Spec   DataIdentity `json:"spec"`
	Status AssetStatus  `json:"status,omitempty"`
}
type AssetStatus struct {
	// +kubebuilder:validation:MaxItems=1000
	RetiredCopies []DataIdentity `json:"retiredCopies,omitempty"`
	Current       *DataIdentity  `json:"current,omitempty"`
	Destroyed     bool           `json:"destroyed,omitempty"`
	// +kubebuilder:validation:MaxItems=1000
	History []History `json:"history,omitempty"`
}

// +kubebuilder:object:root=true
type DataAssetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DataAsset `json:"items"`
}
type Target struct {
	Cluster ClusterRef `json:"cluster"`
	// +kubebuilder:validation:MaxLength=4096
	ClaimName string `json:"claimName"`
	Source    Source `json:"source"`
}

type WorkerIdentity struct {
	UID int64 `json:"uid"`
	GID int64 `json:"gid"`
}

// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="operation intent is immutable"
type OperationSpec struct {
	// +kubebuilder:validation:MaxLength=4096
	Action string `json:"action"`
	// +kubebuilder:validation:MaxLength=4096
	AssetName string `json:"assetName"`
	// +kubebuilder:validation:MaxLength=4096
	// +kubebuilder:validation:Type=string
	AssetUID       types.UID      `json:"assetUID"`
	Source         DataIdentity   `json:"source"`
	SourceCluster  ClusterRef     `json:"sourceCluster"`
	WorkerIdentity WorkerIdentity `json:"workerIdentity"`
	Target         *Target        `json:"target,omitempty"`
	// +kubebuilder:validation:MaxLength=4096
	Approval string `json:"approval"`
}
type OperationStatus struct {
	// +kubebuilder:validation:MaxLength=4096
	WorkerReceipt string `json:"workerReceipt,omitempty"`
	Attempt       int32  `json:"attempt,omitempty"`
	// +kubebuilder:validation:MaxLength=4096
	Phase string `json:"phase,omitempty"`
	// +kubebuilder:validation:MaxLength=4096
	Message string `json:"message,omitempty"`
	// +kubebuilder:validation:MaxLength=4096
	SpecDigest string        `json:"specDigest,omitempty"`
	Target     *DataIdentity `json:"target,omitempty"`
	// +kubebuilder:validation:MaxLength=4096
	// +kubebuilder:validation:Type=string
	JobUID    types.UID    `json:"jobUID,omitempty"`
	Completed *metav1.Time `json:"completed,omitempty"`
}

// DataOperation's approval is an exact digest of the immutable operation intent.
// Creating operations is a separate RBAC capability from managing product CRs.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type DataOperation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              OperationSpec   `json:"spec"`
	Status            OperationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type DataOperationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DataOperation `json:"items"`
}

// +kubebuilder:validation:MaxLength=4096
func Approval(spec OperationSpec) string {
	spec.Approval = ""
	b, _ := json.Marshal(spec)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func AddToScheme(s *runtime.Scheme) error {
	s.AddKnownTypes(GroupVersion, &DataAsset{}, &DataAssetList{}, &DataOperation{}, &DataOperationList{})
	metav1.AddToGroupVersion(s, GroupVersion)
	return nil
}

const (
	ActionMigrate   = "migrate"
	ActionAdopt     = "adopt"
	ActionDestroy   = "destroy"
	phaseLocked     = "Locked"
	phaseRecord     = "Record"
	phaseBindTarget = "BindTarget"
)

const (
	phaseComplete   = "Complete"
	phaseErase      = "Erase"
	workerContainer = "data"
)

const phaseCopy = "Copy"

const phaseReclaimVolume = "ReclaimVolume"

const phaseDeleteVolume = "DeleteVolume"
