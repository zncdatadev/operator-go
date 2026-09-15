package input

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	kubernetesjson "sigs.k8s.io/json"
)

const (
	clusterConfigField = "clusterConfig"
	jsonNull           = "null"
)

// DecodeJSON strictly decodes a generated root object. It rejects ambiguity
// before native codecs can normalize it, and leaves into unchanged on failure.
// A typed API-server GET is the other input path; API unknown-field pruning is
// not the same contract as this local rejection.
func DecodeJSON(data []byte, into any) error {
	destination := reflect.ValueOf(into)
	if !destination.IsValid() || destination.Kind() != reflect.Pointer || destination.IsNil() ||
		destination.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("generated input must be a non-nil pointer to a struct")
	}
	typ := destination.Elem().Type()
	field, ok := typ.FieldByName("Spec")
	if !ok || !field.IsExported() {
		return fmt.Errorf("generated input must declare exported Spec")
	}
	document, err := decodeObject(data, "input")
	if err != nil {
		return err
	}
	spec, ok := document["spec"]
	if !ok {
		return fmt.Errorf("spec is required")
	}
	if err := validateJSON(spec, field.Type, "spec"); err != nil {
		return err
	}
	value := reflect.New(typ)
	strict, err := kubernetesjson.UnmarshalStrict(data, value.Interface())
	if failure := errors.Join(append(strict, err)...); failure != nil {
		return failure
	}
	destination.Elem().Set(value.Elem())
	return nil
}

// ConfigJSON preserves absence versus an explicitly empty object. Generated
// callers pass only presence input types after DecodeJSON or a typed API read.
func ConfigJSON[T any](config *T) (json.RawMessage, error) {
	if config == nil {
		return nil, nil
	}
	data, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	if err := rejectNulls(data, "config"); err != nil {
		return nil, err
	}
	return data, nil
}

// ClusterConfigJSON strips fixed controls from the product cluster layer.
// Operation is generated independently and never calls this function.
func ClusterConfigJSON(value any) (json.RawMessage, error) {
	if value == nil || (reflect.ValueOf(value).Kind() == reflect.Pointer && reflect.ValueOf(value).IsNil()) {
		return nil, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("clusterConfig: %w", err)
	}
	values, err := decodeObject(data, clusterConfigField)
	if err != nil {
		return nil, err
	}
	if err := rejectNulls(data, clusterConfigField); err != nil {
		return nil, err
	}
	for _, name := range []string{"stopped", "reconciliationPaused"} {
		if raw, present := values[name]; present {
			var flag bool
			if err := json.Unmarshal(raw, &flag); err != nil {
				return nil, fmt.Errorf("clusterConfig.%s: %w", name, err)
			}
			delete(values, name)
		}
	}
	return json.Marshal(values)
}

func decodeObject(data json.RawMessage, path string) (map[string]json.RawMessage, error) {
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

func validateJSON(data json.RawMessage, typ reflect.Type, path string) error {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if strings.TrimSpace(string(data)) == jsonNull {
		return fmt.Errorf("%s: null is not an inheritance or deletion operation", path)
	}
	if handled, err := validateNative(data, typ, path); handled {
		return err
	}
	switch typ.Kind() {
	case reflect.Struct, reflect.Map:
		return validateObjectFields(data, typ, path)
	case reflect.Slice:
		var values []json.RawMessage
		if err := json.Unmarshal(data, &values); err != nil {
			return fmt.Errorf("%s: expected an array", path)
		}
		for index, value := range values {
			if err := validateJSON(value, typ.Elem(), fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	default:
		if err := json.Unmarshal(data, reflect.New(typ).Interface()); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	return nil
}

func validateObjectFields(data json.RawMessage, typ reflect.Type, path string) error {
	values, err := decodeObject(data, path)
	if err != nil {
		return err
	}
	var fields map[string]reflect.Type
	if typ.Kind() == reflect.Struct {
		fields, err = jsonFields(typ)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	for _, key := range sortedKeys(values) {
		var child reflect.Type
		if typ.Kind() == reflect.Map {
			child = typ.Elem()
		} else if child = fields[key]; child == nil {
			return fmt.Errorf("%s[%q]: unknown field", path, key)
		}
		if err := validateJSON(values[key], child, fmt.Sprintf("%s[%q]", path, key)); err != nil {
			return err
		}
	}
	return nil
}

func validateNative(data json.RawMessage, typ reflect.Type, path string) (bool, error) {
	switch typ {
	case reflect.TypeFor[json.RawMessage]():
		// RawMessage is reserved for PodTemplate patches. Their nested null and
		// $patch directives are valid; duplicate keys are still ambiguous.
		var object map[string]any
		strict, err := kubernetesjson.UnmarshalStrict(data, &object)
		if failure := errors.Join(append(strict, err)...); failure != nil {
			return true, fmt.Errorf("%s: %w", path, failure)
		}
		if object == nil {
			return true, fmt.Errorf("%s: pod override must be an object", path)
		}
	case affinityType:
		if err := rejectNulls(data, path); err != nil {
			return true, err
		}
		var affinity corev1.Affinity
		strict, err := kubernetesjson.UnmarshalStrict(data, &affinity)
		if failure := errors.Join(append(strict, err)...); failure != nil {
			return true, fmt.Errorf("%s: %w", path, failure)
		}
	case quantityType:
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return true, fmt.Errorf("%s: quantity must be a string", path)
		}
		if _, err := resource.ParseQuantity(value); err != nil {
			return true, fmt.Errorf("%s: invalid quantity: %w", path, err)
		}
	case durationType:
		var value string
		if err := json.Unmarshal(data, &value); err != nil || utf8.RuneCountInString(value) > 128 {
			return true, fmt.Errorf("%s: duration must be a string of at most 128 characters", path)
		}
		if _, err := time.ParseDuration(value); err != nil {
			return true, fmt.Errorf("%s: invalid duration: %w", path, err)
		}
	default:
		return false, nil
	}
	return true, nil
}

func rejectNulls(data json.RawMessage, path string) error {
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
