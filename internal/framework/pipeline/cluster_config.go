package pipeline

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/zncdatadev/operator-go/pkg/framework"
)

func checkClusterConfigType(typ reflect.Type) error {
	if typ.Kind() != reflect.Struct || typ == quantityType || typ == durationType {
		return fmt.Errorf("product cluster config must be a struct")
	}
	if err := checkProfile(typ, make(map[reflect.Type]bool)); err != nil {
		return fmt.Errorf("product cluster config: %w", err)
	}
	return checkFlatFields([]reflect.Type{reflect.TypeFor[ClusterOperation](),
		reflect.TypeFor[framework.ClusterConfig](), typ})
}

// ResolveClusterConfig folds one cluster-wide user layer over product defaults.
// It uses the same fixed data profile and object/key inheritance as role config;
// sequences replace, and explicit null is invalid. CommonConfig and role layers
// do not participate. Product business validation remains in product callbacks.
func ResolveClusterConfig[S any](defaults S, raw json.RawMessage) (S, error) {
	var out S
	typ := reflect.TypeFor[S]()
	if err := checkClusterConfigType(typ); err != nil {
		return out, fmt.Errorf("clusterConfig: %w", err)
	}
	data, err := json.Marshal(defaults)
	if err != nil {
		return out, fmt.Errorf("clusterConfig defaults: %w", err)
	}
	var base map[string]json.RawMessage
	if err := json.Unmarshal(data, &base); err != nil {
		return out, fmt.Errorf("clusterConfig defaults: %w", err)
	}
	if len(raw) != 0 {
		var err error
		_, raw, err = splitPlatformConfig(raw)
		if err != nil {
			return out, err
		}
		if err := validateJSON(raw, typ, "clusterConfig"); err != nil {
			return out, err
		}
		values, err := decodeConfigLayer(raw, "clusterConfig")
		if err != nil {
			return out, err
		}
		fields, _ := jsonFields(typ)
		if err := foldS3Domains(base, values, fields); err != nil {
			return out, fmt.Errorf("clusterConfig: %w", err)
		}
		base = mergeObjects(base, values)
	}
	data, err = json.Marshal(base)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, fmt.Errorf("clusterConfig: %w", err)
	}
	if err := validateS3Domains(reflect.ValueOf(out)); err != nil {
		return out, fmt.Errorf("clusterConfig: %w", err)
	}
	return out, nil
}

// splitPlatformConfig validates framework fields independently of product S.
func splitPlatformConfig(raw json.RawMessage) (framework.ClusterConfig, json.RawMessage, error) {
	var out framework.ClusterConfig
	if len(raw) == 0 {
		return out, nil, nil
	}
	values, err := decodeConfigLayer(raw, "clusterConfig")
	if err != nil {
		return out, nil, err
	}
	fields, err := jsonFields(reflect.TypeFor[framework.ClusterConfig]())
	if err != nil {
		return out, nil, err
	}
	common := make(map[string]json.RawMessage)
	for _, key := range sortedKeys(fields) {
		typ := fields[key]
		if value, ok := values[key]; ok {
			if err := validateJSON(value, typ, "clusterConfig."+key); err != nil {
				return out, nil, err
			}
			common[key] = value
			delete(values, key)
		}
	}
	data, err := json.Marshal(common)
	if err != nil {
		return out, nil, err
	}
	if err = json.Unmarshal(data, &out); err != nil {
		return out, nil, err
	}
	product, err := json.Marshal(values)
	return out, product, err
}
