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

package util

import (
	"encoding/json"
	"fmt"
	"reflect"

	jsonpatch "github.com/evanphx/json-patch"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
)

// MergeObject merges the original config with the override config
// The override config will override the original config
// The merged config will be returned
// Example:
//
//	// Merge OverridesSpec with the original config
//	func mergeOverrides() error {
//		original := &OverridesSpec{EnvOverrides: map[string]string{"key1": "value", "key2": "value2"}}
//		override := &OverridesSpec{EnvOverrides: map[string]string{"key1": "new-value"}}
//
//		merged, err := MergeObject(original, override)
//		if err != nil {
//			return err
//		}
//
//		fmt.Println(merged.EnvOverrides) // Output: map[string]string{"key1": "new-value", "key2": "value2"}
//		return nil
//	}
//	// Merge RoleGroupConfigSpec with the original config
//	type RoleGroupConfigSpec struct {
//		// +kubebuilder:validation:Optional
//		Resources *commonsv1alpha1.ResourcesSpec `json:"resources,omitempty"`
//		// +kubebuilder:validation:Optional
//		Logging *commonsv1alpha1.LoggingSpec `json:"logging,omitempty"`
//		// +kubebuilder:validation:Optional
//		QueryMaxMemory string `json:"queryMaxMemory,omitempty"`
//	}
//	func mergeRoleGroupConfig() error {
//		original := &RoleGroupConfigSpec{
//			QueryMaxMemory: "1Gi"
//			Logging: &commonsv1alpha1.LoggingSpec{
//				Containers: map[string]commonsv1alpha1.LoggingConfigSpec{
//					"container1": commonsv1alpha1.LoggingConfigSpec{
//						Loggers: map[string]*commonsv1alpha1.LogLevelSpec{
//							"logger1": &commonsv1alpha1.LogLevelSpec{Level: "INFO"},
//						},
//					},
//				},
//			},
//		}
//		override := &RoleGroupConfigSpec{
//			QueryMaxMemory: "2Gi"
//			Logging: &commonsv1alpha1.LoggingSpec{
//				Containers: map[string]commonsv1alpha1.LoggingConfigSpec{
//					"container1": commonsv1alpha1.LoggingConfigSpec{
//						File: &commonsv1alpha1.LogLevelSpec{Level: "DEBUG"},
//					},
//				},
//			},
//		}
//
//		merged, err := MergeObject(original, override)
//		if err != nil {
//			return err
//		}
//
//		fmt.Println(merged)
//		return nil
//	}

// MergeObject merges the original config with the override config
func MergeObject[T any](original, override T) (T, error) {

	if empty, err := validateStruct(original, override); err != nil {
		return original, err
	} else if empty {
		return original, nil
	}

	originalValue := reflect.ValueOf(original)
	overrideValue := reflect.ValueOf(override)

	if originalValue.Kind() == reflect.Ptr && originalValue.IsNil() {
		return override, nil
	}

	if overrideValue.Kind() == reflect.Ptr && overrideValue.IsNil() {
		return original, nil
	}

	originalJson, err := json.Marshal(original)
	if err != nil {
		return original, err
	}

	overrideJson, err := json.Marshal(override)
	if err != nil {
		return original, err
	}

	var originalMap, overrideMap map[string]interface{}
	if err := json.Unmarshal(originalJson, &originalMap); err != nil {
		return original, err
	}
	if err := json.Unmarshal(overrideJson, &overrideMap); err != nil {
		return original, err
	}

	mergedMap := mergeMaps(originalMap, overrideMap)

	mergedJson, err := json.Marshal(mergedMap)
	if err != nil {
		return original, err
	}

	var merged T
	if err := json.Unmarshal(mergedJson, &merged); err != nil {
		return original, err
	}

	return merged, nil
}

func mergeMaps(original, override map[string]interface{}) map[string]interface{} {
	for key, overrideValue := range override {
		if originalValue, exists := original[key]; exists {
			switch originalValueTyped := originalValue.(type) {
			case map[string]interface{}:
				if overrideValueTyped, ok := overrideValue.(map[string]interface{}); ok {
					original[key] = mergeMaps(originalValueTyped, overrideValueTyped)
				} else {
					original[key] = overrideValue
				}
			case []interface{}:
				if overrideValueTyped, ok := overrideValue.([]interface{}); ok {
					original[key] = mergeSlices(originalValueTyped, overrideValueTyped)
				} else {
					original[key] = overrideValue
				}
			default:
				original[key] = overrideValue
			}
		} else {
			original[key] = overrideValue
		}
	}
	return original
}

func mergeSlices(original, override []interface{}) []interface{} {
	return append(original, override...)
}

// validateStruct validates the original and override structs
// It returns an error if the original or override is not a struct or pointer to struct
// It returns true if the original and override are both nil
func validateStruct[T any](origin, override T) (empty bool, err error) {
	originValue := reflect.ValueOf(origin)
	overrideValue := reflect.ValueOf(override)

	if originValue.Kind() == reflect.Ptr {
		if originValue.IsNil() {
			if overrideValue.Kind() == reflect.Ptr && overrideValue.IsNil() {
				return true, nil
			}
			return false, nil
		}
		originValue = originValue.Elem()
	}
	if originValue.Kind() != reflect.Struct {
		return false, fmt.Errorf("original must be a struct or pointer to struct")
	}

	if overrideValue.Kind() == reflect.Ptr {
		if overrideValue.IsNil() {
			return false, nil
		}
		overrideValue = overrideValue.Elem()
	}
	if overrideValue.Kind() != reflect.Struct {
		return false, fmt.Errorf("override must be a struct or pointer to struct")
	}
	return false, nil
}

// MergeObjectWithJson merges the original config with the override config
// It uses json merge patch to merge the two configs.
func MergeObjectWithJson[T any](original, override T) (T, error) {

	if empty, err := validateStruct(original, override); err != nil {
		return original, err
	} else if empty {
		return original, nil
	}

	originalValue := reflect.ValueOf(original)
	overrideValue := reflect.ValueOf(override)

	if originalValue.Kind() == reflect.Ptr && originalValue.IsNil() {
		return override, nil
	}

	if overrideValue.Kind() == reflect.Ptr && overrideValue.IsNil() {
		return original, nil
	}

	originalJson, err := json.Marshal(original)
	if err != nil {
		return original, err
	}

	overrideJson, err := json.Marshal(override)
	if err != nil {
		return original, err
	}

	mergedJson, err := jsonpatch.MergePatch(originalJson, overrideJson)
	if err != nil {
		return original, err
	}

	var merged T
	if err := json.Unmarshal(mergedJson, &merged); err != nil {
		return original, err
	}

	return merged, nil
}

// MergeObjectWithStrategic merges the original config with the override config
// It uses strategic merge patch to merge the two configs.
// Some k8s resources define a merge strategy for their fields, strategic merge patch is used to merge these fields.
func MergeObjectWithStrategic[T any](original, override T) (T, error) {

	if empty, err := validateStruct(original, override); err != nil {
		return original, err
	} else if empty {
		return original, nil
	}

	originalValue := reflect.ValueOf(original)
	overrideValue := reflect.ValueOf(override)

	if originalValue.Kind() == reflect.Ptr && originalValue.IsNil() {
		return override, nil
	}

	if overrideValue.Kind() == reflect.Ptr && overrideValue.IsNil() {
		return original, nil
	}

	originalJson, err := json.Marshal(original)
	if err != nil {
		return original, err
	}

	overrideJson, err := json.Marshal(override)
	if err != nil {
		return original, err
	}

	patch, err := strategicpatch.StrategicMergePatch(originalJson, overrideJson, original)
	if err != nil {
		return original, err
	}

	mergedJson, err := jsonpatch.MergePatch(originalJson, patch)
	if err != nil {
		return original, err
	}

	var merged T
	if err := json.Unmarshal(mergedJson, &merged); err != nil {
		return original, err
	}

	return merged, nil
}
