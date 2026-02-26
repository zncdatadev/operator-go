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

package testutil

import (
	"fmt"
	"reflect"

	"github.com/onsi/gomega/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// HaveCondition returns a matcher that checks if a condition exists with the given type and reason.
func HaveCondition(conditionType, reason string) types.GomegaMatcher {
	return &haveConditionMatcher{
		conditionType: conditionType,
		reason:        reason,
	}
}

type haveConditionMatcher struct {
	conditionType string
	reason        string
}

func (m *haveConditionMatcher) Match(actual interface{}) (bool, error) {
	conditions, ok := actual.([]metav1.Condition)
	if !ok {
		return false, fmt.Errorf("expected []metav1.Condition, got %T", actual)
	}

	for _, cond := range conditions {
		if string(cond.Type) == m.conditionType && cond.Reason == m.reason {
			return true, nil
		}
	}
	return false, nil
}

func (m *haveConditionMatcher) FailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected condition with type=%s and reason=%s to exist in %v", m.conditionType, m.reason, actual)
}

func (m *haveConditionMatcher) NegatedFailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected condition with type=%s and reason=%s NOT to exist in %v", m.conditionType, m.reason, actual)
}

// HaveOwnerReference returns a matcher that checks if an owner reference exists with the given name and kind.
func HaveOwnerReference(name, kind string) types.GomegaMatcher {
	return &haveOwnerReferenceMatcher{
		name: name,
		kind: kind,
	}
}

type haveOwnerReferenceMatcher struct {
	name string
	kind string
}

func (m *haveOwnerReferenceMatcher) Match(actual interface{}) (bool, error) {
	ownerRefs, ok := actual.([]metav1.OwnerReference)
	if !ok {
		obj, ok := actual.(runtime.Object)
		if !ok {
			return false, fmt.Errorf("expected []metav1.OwnerReference or runtime.Object with ObjectMeta, got %T", actual)
		}

		// Try to get OwnerReferences from the object using reflection with panic recovery
		func() {
			defer func() {
				if r := recover(); r != nil {
					// Reflection failed, ownerRefs will remain nil
				}
			}()
			val := reflect.ValueOf(obj)
			if val.Kind() == reflect.Ptr {
				val = val.Elem()
			}

			metaField := val.FieldByName("ObjectMeta")
			if !metaField.IsValid() {
				return
			}

			meta, ok := metaField.Interface().(metav1.ObjectMeta)
			if !ok {
				return
			}
			ownerRefs = meta.OwnerReferences
		}()

		if ownerRefs == nil {
			return false, fmt.Errorf("could not extract OwnerReferences from object of type %T", actual)
		}
	}

	for _, ref := range ownerRefs {
		if ref.Name == m.name && ref.Kind == m.kind {
			return true, nil
		}
	}
	return false, nil
}

func (m *haveOwnerReferenceMatcher) FailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected OwnerReference with name=%s and kind=%s to exist", m.name, m.kind)
}

func (m *haveOwnerReferenceMatcher) NegatedFailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected OwnerReference with name=%s and kind=%s NOT to exist", m.name, m.kind)
}

// BeCreatedSuccessfully returns a matcher that checks if an object was created successfully.
func BeCreatedSuccessfully() types.GomegaMatcher {
	return &beCreatedSuccessfullyMatcher{}
}

type beCreatedSuccessfullyMatcher struct{}

func (m *beCreatedSuccessfullyMatcher) Match(actual interface{}) (bool, error) {
	err, ok := actual.(error)
	if !ok {
		return actual == nil, nil
	}
	return err == nil, nil
}

func (m *beCreatedSuccessfullyMatcher) FailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected creation to succeed, but got error: %v", actual)
}

func (m *beCreatedSuccessfullyMatcher) NegatedFailureMessage(actual interface{}) string {
	return "Expected creation to fail, but it succeeded"
}

// HaveName returns a matcher that checks if an object has the expected name.
func HaveName(expectedName string) types.GomegaMatcher {
	return &haveNameMatcher{expectedName: expectedName}
}

type haveNameMatcher struct {
	expectedName string
}

func (m *haveNameMatcher) Match(actual interface{}) (bool, error) {
	name, ok := actual.(string)
	if !ok {
		return false, fmt.Errorf("expected string, got %T", actual)
	}
	return name == m.expectedName, nil
}

func (m *haveNameMatcher) FailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected name to be %s, but got %s", m.expectedName, actual)
}

func (m *haveNameMatcher) NegatedFailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected name NOT to be %s", m.expectedName)
}

// HaveLabels returns a matcher that checks if an object has the expected labels.
func HaveLabels(expectedLabels map[string]string) types.GomegaMatcher {
	return &haveLabelsMatcher{expectedLabels: expectedLabels}
}

type haveLabelsMatcher struct {
	expectedLabels map[string]string
}

func (m *haveLabelsMatcher) Match(actual interface{}) (bool, error) {
	labels, ok := actual.(map[string]string)
	if !ok {
		return false, fmt.Errorf("expected map[string]string, got %T", actual)
	}

	for k, v := range m.expectedLabels {
		if labels[k] != v {
			return false, nil
		}
	}
	return true, nil
}

func (m *haveLabelsMatcher) FailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected labels to contain %v, but got %v", m.expectedLabels, actual)
}

func (m *haveLabelsMatcher) NegatedFailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected labels NOT to contain %v", m.expectedLabels)
}

// HaveReplicas returns a matcher that checks if a StatefulSet/Deployment has the expected replica count.
func HaveReplicas(expected int32) types.GomegaMatcher {
	return &haveReplicasMatcher{expected: expected}
}

type haveReplicasMatcher struct {
	expected int32
}

func (m *haveReplicasMatcher) Match(actual interface{}) (bool, error) {
	val := reflect.ValueOf(actual)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	// Try to get Spec.Replicas
	specField := val.FieldByName("Spec")
	if !specField.IsValid() {
		return false, fmt.Errorf("object does not have Spec field")
	}

	replicasField := specField.FieldByName("Replicas")
	if !replicasField.IsValid() {
		return false, fmt.Errorf("spec does not have Replicas field")
	}

	if replicasField.IsNil() {
		return m.expected == 0 || m.expected == 1, nil // Default replica count is usually 1
	}

	replicas := int32(0)
	switch r := replicasField.Interface().(type) {
	case *int32:
		if r != nil {
			replicas = *r
		}
	case int32:
		replicas = r
	default:
		return false, fmt.Errorf("unexpected Replicas type: %T", replicasField.Interface())
	}

	return replicas == m.expected, nil
}

func (m *haveReplicasMatcher) FailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected %d replicas", m.expected)
}

func (m *haveReplicasMatcher) NegatedFailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected NOT %d replicas", m.expected)
}
