package pipeline

import (
	"encoding/json"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	kubernetesjson "sigs.k8s.io/json"
)

const affinityConfigField = "affinity"

// Affinity is one fixed common domain. Each supplied scheduling branch replaces
// that complete branch; absent branches inherit. An empty affinity object has no
// new branches, while nodeAffinity: {} explicitly clears node scheduling rules.
func mergeAffinityJSON(base, patch json.RawMessage) (json.RawMessage, error) {
	var lower, upper corev1.Affinity
	if err := json.Unmarshal(base, &lower); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(patch, &upper); err != nil {
		return nil, err
	}
	result := lower.DeepCopy()
	if upper.NodeAffinity != nil {
		result.NodeAffinity = upper.NodeAffinity.DeepCopy()
	}
	if upper.PodAffinity != nil {
		result.PodAffinity = upper.PodAffinity.DeepCopy()
	}
	if upper.PodAntiAffinity != nil {
		result.PodAntiAffinity = upper.PodAntiAffinity.DeepCopy()
	}
	return json.Marshal(result)
}

func validateAffinityJSON(data json.RawMessage, path string) error {
	if err := rejectInputNulls(data, path); err != nil {
		return err
	}
	var affinity corev1.Affinity
	strict, err := kubernetesjson.UnmarshalStrict(data, &affinity)
	if failure := errors.Join(append(strict, err)...); failure != nil {
		return fmt.Errorf("%s: %w", path, failure)
	}
	return nil
}

// RawMessage keeps each domain unnormalized until its own validation. The outer
// strict decode rejects duplicate config field names before selecting a domain.
func decodeConfigLayer(data json.RawMessage, path string) (map[string]json.RawMessage, error) {
	var values map[string]json.RawMessage
	strict, err := kubernetesjson.UnmarshalStrict(data, &values)
	if failure := errors.Join(append(strict, err)...); failure != nil {
		return nil, fmt.Errorf("%s: %w", path, failure)
	}
	if values == nil {
		return nil, fmt.Errorf("%s must be an object", path)
	}
	return values, nil
}

func rejectInputNulls(data json.RawMessage, path string) error {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	return rejectNullValue(value, path)
}

func rejectNullValue(value any, path string) error {
	switch node := value.(type) {
	case nil:
		return fmt.Errorf("%s: explicit null is not allowed", path)
	case map[string]any:
		for _, key := range sortedKeys(node) {
			if err := rejectNullValue(node[key], fmt.Sprintf("%s[%q]", path, key)); err != nil {
				return err
			}
		}
	case []any:
		for index, child := range node {
			if err := rejectNullValue(child, fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	}
	return nil
}
