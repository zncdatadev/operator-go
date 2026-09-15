package pipeline

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/zncdatadev/operator-go/pkg/framework"
)

var s3ConnectionType = reflect.TypeFor[framework.S3Connection]()

// S3 is a named business domain with its own inheritance contract. Traversal
// finds that exact type inside product structs/maps; products do not register
// union handlers or attach merge-policy tags to fields.
func foldS3Domains(base, layer map[string]json.RawMessage, fields map[string]reflect.Type) error {
	for _, key := range sortedKeys(layer) {
		typ := fields[key]
		if typ == s3ConnectionType {
			merged, err := foldS3Connection(base[key], layer[key])
			if err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
			base[key] = merged
			delete(layer, key)
			continue
		}
		if typ == nil || (typ.Kind() != reflect.Struct && typ.Kind() != reflect.Map) ||
			typ == quantityType || typ == durationType || typ == affinityType {
			continue
		}
		var inherited, patch map[string]json.RawMessage
		if err := json.Unmarshal(layer[key], &patch); err != nil {
			return err
		}
		if err := json.Unmarshal(base[key], &inherited); err != nil && len(base[key]) != 0 {
			return err
		}
		if inherited == nil {
			inherited = map[string]json.RawMessage{}
		}
		children := map[string]reflect.Type{}
		if typ.Kind() == reflect.Struct {
			children, _ = jsonFields(typ)
		} else {
			for name := range patch {
				children[name] = typ.Elem()
			}
		}
		if err := foldS3Domains(inherited, patch, children); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		var err error
		base[key], err = json.Marshal(inherited)
		if err != nil {
			return err
		}
		layer[key], err = json.Marshal(patch)
		if err != nil {
			return err
		}
	}
	return nil
}

func foldS3Connection(base, layer json.RawMessage) (json.RawMessage, error) {
	var prior, next map[string]json.RawMessage
	if len(base) != 0 {
		if err := json.Unmarshal(base, &prior); err != nil {
			return nil, err
		}
	}
	if err := json.Unmarshal(layer, &next); err != nil {
		return nil, err
	}
	if raw, present := next["type"]; present {
		var before, after framework.S3ConnectionType
		_ = json.Unmarshal(prior["type"], &before)
		if err := json.Unmarshal(raw, &after); err != nil {
			return nil, err
		}
		if before == "" {
			before = framework.S3Disabled
		}
		if before != after {
			prior = nil
		}
	}
	if prior == nil {
		prior = map[string]json.RawMessage{}
	}
	return json.Marshal(mergeObjects(prior, next))
}

func validateS3Layer(data json.RawMessage, scope string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return fmt.Errorf("%s: S3 connection must be an object", scope)
	}
	if raw, present := fields["type"]; present {
		var kind framework.S3ConnectionType
		if err := json.Unmarshal(raw, &kind); err != nil ||
			(kind != framework.S3Disabled && kind != framework.S3Inline && kind != framework.S3Reference) {
			return fmt.Errorf("%s.type: must be disabled, inline or reference", scope)
		}
		_, hasInline := fields["inline"]
		_, hasReference := fields["reference"]
		if (kind == framework.S3Disabled && (hasInline || hasReference)) ||
			(kind == framework.S3Inline && hasReference) || (kind == framework.S3Reference && hasInline) {
			return fmt.Errorf("%s: fields belong to another S3 connection branch", scope)
		}
	}
	return nil
}

func validateS3Domains(value reflect.Value) error {
	if value.Type() == s3ConnectionType {
		return value.Interface().(framework.S3Connection).Validate()
	}
	switch value.Kind() {
	case reflect.Struct:
		if value.Type() == quantityType || value.Type() == durationType || value.Type() == affinityType {
			return nil
		}
		for index := 0; index < value.NumField(); index++ {
			if err := validateS3Domains(value.Field(index)); err != nil {
				return fmt.Errorf("%s: %w", value.Type().Field(index).Name, err)
			}
		}
	case reflect.Map:
		keys := make(map[string]reflect.Value, value.Len())
		for _, key := range value.MapKeys() {
			keys[key.String()] = key
		}
		for _, name := range sortedKeys(keys) {
			if err := validateS3Domains(value.MapIndex(keys[name])); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
	case reflect.Slice:
		for index := 0; index < value.Len(); index++ {
			if err := validateS3Domains(value.Index(index)); err != nil {
				return fmt.Errorf("[%d]: %w", index, err)
			}
		}
	}
	return nil
}
