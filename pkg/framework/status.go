package framework

import (
	"maps"
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ReconcileStatus reports controller observations separately from configuration
// inputs. Workload readiness does not assert application health or data durability.
type ReconcileStatus struct {
	ObservedGeneration int64                  `json:"observedGeneration,omitempty"`
	Conditions         []metav1.Condition     `json:"conditions,omitempty"`
	Groups             []GroupReconcileStatus `json:"groups,omitempty"`
	Roles              []RoleReconcileStatus  `json:"roles,omitempty"`
}

type RoleReconcileStatus struct {
	Name    string `json:"name"`
	Applied bool   `json:"applied"`
	Message string `json:"message,omitempty"`
}

type GroupReconcileStatus struct {
	Platform          *PlatformObservation `json:"platform,omitempty"`
	Role              string               `json:"role"`
	Name              string               `json:"name"`
	ExecutionReplicas *int32               `json:"executionReplicas,omitempty"`
	DesiredReplicas   int32                `json:"desiredReplicas"`
	ReadyReplicas     int32                `json:"readyReplicas"`
	Applied           bool                 `json:"applied"`
	Message           string               `json:"message,omitempty"`
	Checks            []Check              `json:"checks,omitempty"`
	Facts             *FactDiagnostic      `json:"facts,omitempty"`
}

// DeepCopyInto copies only the fixed status data model. It does not depend on
// the generated-input copier or introduce an inverse package dependency.
func (in *ReconcileStatus) DeepCopyInto(out *ReconcileStatus) {
	*out = *in
	out.Roles = slices.Clone(in.Roles)
	if in.Conditions != nil {
		out.Conditions = make([]metav1.Condition, len(in.Conditions))
		for index := range in.Conditions {
			in.Conditions[index].DeepCopyInto(&out.Conditions[index])
		}
	}
	if in.Groups != nil {
		out.Groups = make([]GroupReconcileStatus, len(in.Groups))
		for index := range in.Groups {
			group := in.Groups[index]
			out.Groups[index] = group
			out.Groups[index].Checks = slices.Clone(group.Checks)
			if group.ExecutionReplicas != nil {
				value := *group.ExecutionReplicas
				out.Groups[index].ExecutionReplicas = &value
			}
			if group.Platform != nil {
				p := *group.Platform
				p.Diagnostic.Observed = slices.Clone(group.Platform.Diagnostic.Observed)
				p.Listeners = slices.Clone(group.Platform.Listeners)
				for i := range p.Listeners {
					p.Listeners[i].Ports = maps.Clone(p.Listeners[i].Ports)
				}
				out.Groups[index].Platform = &p
			}
			if group.Facts != nil {
				value := *group.Facts
				value.Observed = slices.Clone(group.Facts.Observed)
				out.Groups[index].Facts = &value
			}
		}
	}
}

func (in *ReconcileStatus) DeepCopy() *ReconcileStatus {
	if in == nil {
		return nil
	}
	out := new(ReconcileStatus)
	in.DeepCopyInto(out)
	return out
}
