/*
Copyright 2024 ZNCDataDev.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package builder

import (
	"maps"

	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// PDBBuilder constructs PodDisruptionBudget resources.
type PDBBuilder struct {
	Name           string
	Namespace      string
	Labels         map[string]string
	Annotations    map[string]string
	Selector       map[string]string
	MaxUnavailable *intstr.IntOrString
	Enabled        bool
}

// NewPDBBuilder creates a new PDBBuilder.
func NewPDBBuilder(name, namespace string) *PDBBuilder {
	return &PDBBuilder{
		Name:        name,
		Namespace:   namespace,
		Labels:      make(map[string]string),
		Annotations: make(map[string]string),
		Selector:    make(map[string]string),
		Enabled:     true,
	}
}

// WithLabels sets the labels.
func (b *PDBBuilder) WithLabels(labels map[string]string) *PDBBuilder {
	for k, v := range labels {
		b.Labels[k] = v
	}
	return b
}

// WithAnnotations sets the annotations.
func (b *PDBBuilder) WithAnnotations(annotations map[string]string) *PDBBuilder {
	for k, v := range annotations {
		b.Annotations[k] = v
	}
	return b
}

// WithSelector sets the selector.
func (b *PDBBuilder) WithSelector(selector map[string]string) *PDBBuilder {
	for k, v := range selector {
		b.Selector[k] = v
	}
	return b
}

// WithSpec sets the PDB spec from v1alpha1.PodDisruptionBudgetSpec.
func (b *PDBBuilder) WithSpec(spec *v1alpha1.PodDisruptionBudgetSpec) *PDBBuilder {
	if spec == nil {
		return b
	}

	b.Enabled = spec.Enabled

	if spec.MaxUnavailable != nil {
		b.MaxUnavailable = &intstr.IntOrString{
			Type:   intstr.Int,
			IntVal: *spec.MaxUnavailable,
		}
	}

	return b
}

// WithMaxUnavailable sets the max unavailable.
func (b *PDBBuilder) WithMaxUnavailable(value intstr.IntOrString) *PDBBuilder {
	b.MaxUnavailable = &value
	return b
}

// WithEnabled sets whether PDB is enabled.
func (b *PDBBuilder) WithEnabled(enabled bool) *PDBBuilder {
	b.Enabled = enabled
	return b
}

// Build creates the PodDisruptionBudget. Like the other builders, the returned object shares no
// map or pointer with the builder, so mutating it (or building twice) cannot corrupt the
// builder's state.
//
// It panics when no selector was set. A PodDisruptionBudget with an empty selector is accepted by
// the API server and selects every pod in the namespace, so it would silently start blocking
// voluntary evictions (node drains) for workloads the caller does not own — the one invalid
// object in this package that no API server error would ever reveal.
func (b *PDBBuilder) Build() *policyv1.PodDisruptionBudget {
	if len(b.Selector) == 0 {
		panic("PDBBuilder: " + b.Name + " has no selector; a PodDisruptionBudget with an empty selector matches every pod in the namespace")
	}

	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:        b.Name,
			Namespace:   b.Namespace,
			Labels:      maps.Clone(b.Labels),
			Annotations: maps.Clone(b.Annotations),
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: maps.Clone(b.Selector),
			},
		},
	}

	if b.MaxUnavailable != nil {
		pdb.Spec.MaxUnavailable = clonePtr(b.MaxUnavailable)
	} else {
		// Default to MaxUnavailable=1
		maxUnavailable := intstr.FromInt(1)
		pdb.Spec.MaxUnavailable = &maxUnavailable
	}

	return pdb
}

// IsEnabled returns whether the PDB is enabled.
func (b *PDBBuilder) IsEnabled() bool {
	return b.Enabled
}

// NamespacedName returns the NamespacedName for the PDB.
func (b *PDBBuilder) NamespacedName() types.NamespacedName {
	return types.NamespacedName{
		Name:      b.Name,
		Namespace: b.Namespace,
	}
}
