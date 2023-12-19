package status

import (
	"fmt"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ZncdataStatus struct {
	// +kubebuilder:validation:Optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// +kubebuilder:validation:Optional
	URLs       []URL  `json:"urls,omitempty"`
	Generation int64  `json:"generation,omitempty"`
	Name       string `json:"name,omitempty"`
	Type       string `json:"type,omitempty"`
}

func (status ZncdataStatus) IsAvailable() bool {
	return apimeta.IsStatusConditionTrue(status.Conditions, ConditionTypeAvailable)
}

func (status *ZncdataStatus) SetStatusCondition(condition metav1.Condition) (updated bool) {
	// if the condition already exists, update it
	existingCondition := apimeta.FindStatusCondition(status.Conditions, condition.Type)
	if existingCondition == nil {
		condition.ObservedGeneration = status.GetGeneration()
		condition.LastTransitionTime = metav1.Now()
		conditions := status.Conditions
		status.Conditions = append(conditions, condition)
		updated = true
	} else if existingCondition.Status != condition.Status || existingCondition.Reason != condition.Reason || existingCondition.Message != condition.Message {
		existingCondition.Status = condition.Status
		existingCondition.Reason = condition.Reason
		existingCondition.Message = condition.Message
		existingCondition.ObservedGeneration = status.GetGeneration()
		existingCondition.LastTransitionTime = metav1.Now()
		updated = true
	}
	return
}

// InitStatusConditions initializes the status conditions to the provided conditions.
func (status ZncdataStatus) InitStatusConditions() {
	status.Conditions = []metav1.Condition{}
	status.SetStatusCondition(metav1.Condition{
		Type:               ConditionTypeProgressing,
		Status:             metav1.ConditionTrue,
		Reason:             ConditionReasonPreparing,
		Message:            fmt.Sprintf("%s is preparing", status.Type),
		ObservedGeneration: status.GetGeneration(),
		LastTransitionTime: metav1.Now(),
	})
	status.SetStatusCondition(metav1.Condition{
		Type:               ConditionTypeAvailable,
		Status:             metav1.ConditionFalse,
		Reason:             ConditionReasonPreparing,
		Message:            fmt.Sprintf("%s is preparing", status.Type),
		ObservedGeneration: status.GetGeneration(),
		LastTransitionTime: metav1.Now(),
	})
}

func (status *ZncdataStatus) InitStatus(object client.Object) {
	generation := object.GetGeneration()
	name := object.GetName()
	kind := object.GetObjectKind().GroupVersionKind().Kind
	status.Generation = generation
	status.Name = name
	status.Type = kind
}

func (status ZncdataStatus) GetConditions() []metav1.Condition {
	return status.Conditions
}

func (status *ZncdataStatus) SetConditions(conditions []metav1.Condition) {
	status.Conditions = conditions
}

func (status ZncdataStatus) GetGeneration() int64 {
	return status.Generation
}

// URL is a URL with a name
type URL struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}
