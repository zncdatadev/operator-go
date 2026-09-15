package pipeline

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/zncdatadev/operator-go/pkg/framework"
	"github.com/zncdatadev/operator-go/pkg/framework/input"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var quantityType = reflect.TypeFor[resource.Quantity]()
var durationType = reflect.TypeFor[metav1.Duration]()
var affinityType = reflect.TypeFor[corev1.Affinity]()

const jsonNull = "null"

// ResolveConfig is a bounded fixed-rule interpreter, not a schema/type generator.
// C must be a non-embedded public struct using ordinary JSON scalars, structs,
// string-keyed maps, slices, and Quantity/Duration leaves. Pointers, interfaces, byte
// slices and custom codecs are unsupported. No field can select a merge policy.
// Objects inherit by field/key; sequences replace. Each user layer is checked
// before merging, so a later layer cannot conceal an invalid earlier layer.
func ResolveConfig[C any](defaults framework.Config[C], role, group json.RawMessage) (framework.Config[C], error) {
	var empty framework.Config[C]
	commonType, productType := reflect.TypeFor[CommonConfig](), reflect.TypeFor[C]()
	if err := checkConfigType(productType); err != nil {
		return empty, err
	}
	commonFields, _ := jsonFields(commonType)
	fields, err := jsonFields(productType)
	if err != nil {
		return empty, err
	}
	for name, typ := range commonFields {
		if _, exists := fields[name]; exists {
			return empty, fmt.Errorf("product config field %q collides with common config", name)
		}
		fields[name] = typ
	}
	base := make(map[string]json.RawMessage)
	for _, source := range []any{defaults.Common, defaults.Product} {
		data, err := json.Marshal(source)
		if err != nil {
			return empty, fmt.Errorf("defaults: %w", err)
		}
		var values map[string]json.RawMessage
		if err := json.Unmarshal(data, &values); err != nil {
			return empty, err
		}
		for key, value := range values {
			base[key] = value
		}
	}
	for index, layer := range []json.RawMessage{role, group} {
		if len(layer) == 0 {
			continue
		}
		scope := []string{"role.config", "roleGroup.config"}[index]
		values, err := decodeConfigLayer(layer, scope)
		if err != nil {
			return empty, err
		}
		for _, key := range sortedKeys(values) {
			typ, exists := fields[key]
			if !exists {
				return empty, fmt.Errorf("%s.%s: unknown field", scope, key)
			}
			if err := validateJSON(values[key], typ, scope+"."+key); err != nil {
				return empty, err
			}
		}
		// Scheduling branches are a fixed common domain; their internal rules
		// never enter the generic object merger.
		if affinity, present := values[affinityConfigField]; present {
			merged, err := mergeAffinityJSON(base[affinityConfigField], affinity)
			if err != nil {
				return empty, fmt.Errorf("%s.affinity: %w", scope, err)
			}
			base[affinityConfigField] = merged
			delete(values, affinityConfigField)
		}
		if err := foldStorageLayer(base, values); err != nil {
			return empty, fmt.Errorf("%s.resources.storage: %w", scope, err)
		}
		if err := foldS3Domains(base, values, fields); err != nil {
			return empty, fmt.Errorf("%s: %w", scope, err)
		}
		base = mergeObjects(base, values)
	}
	common, product := make(map[string]json.RawMessage), make(map[string]json.RawMessage)
	for key, value := range base {
		if _, exists := commonFields[key]; exists {
			common[key] = value
		} else {
			product[key] = value
		}
	}
	var result framework.Config[C]
	for _, part := range []struct{ source, destination any }{{common, &result.Common}, {product, &result.Product}} {
		data, err := json.Marshal(part.source)
		if err != nil {
			return empty, err
		}
		if err := json.Unmarshal(data, part.destination); err != nil {
			return empty, err
		}
	}
	if result.Common.Resources.Storage.Type == "" {
		result.Common.Resources.Storage.Type = framework.StorageEphemeral
	}
	if err := ValidateCommon(result.Common); err != nil {
		return empty, err
	}
	if err := validateS3Domains(reflect.ValueOf(result.Product)); err != nil {
		return empty, fmt.Errorf("config: %w", err)
	}
	return result, nil
}

// ValidateCommon checks final effective resource values. This bounded profile
// requires all three quantities; it does not model unset resources. An invalid
// business value in a lower layer may be corrected by a higher layer.
func ValidateCommon(config CommonConfig) error {
	cpu, memory := config.Resources.CPU, config.Resources.Memory
	if cpu.Min.Sign() <= 0 || cpu.Max.Sign() <= 0 {
		return fmt.Errorf("config.resources.cpu.min and max must be positive")
	}
	if cpu.Min.Cmp(cpu.Max) > 0 {
		return fmt.Errorf("config.resources.cpu.min must not exceed max")
	}
	if memory.Limit.Sign() <= 0 {
		return fmt.Errorf("config.resources.memory.limit must be positive")
	}
	if err := validateStorage(config.Resources.Storage); err != nil {
		return err
	}
	return validateShutdownDuration(config.GracefulShutdownTimeout.Duration)
}

func jsonFields(typ reflect.Type) (map[string]reflect.Type, error) {
	fields := make(map[string]reflect.Type)
	for index := 0; index < typ.NumField(); index++ {
		field := typ.Field(index)
		if !field.IsExported() || field.Anonymous {
			return nil, fmt.Errorf("%s: fields must be public and non-embedded", typ)
		}
		tag := strings.Split(field.Tag.Get("json"), ",")
		name := tag[0]
		if name == "-" {
			return nil, fmt.Errorf("%s.%s: ignored fields are unsupported", typ, field.Name)
		}
		if name == "" {
			name = field.Name
		}
		for _, option := range tag[1:] {
			if option != "omitempty" {
				return nil, fmt.Errorf("%s.%s: unsupported JSON option %q", typ, field.Name, option)
			}
		}
		if _, exists := fields[name]; exists {
			return nil, fmt.Errorf("%s: duplicate JSON field %q", typ, name)
		}
		fields[name] = field.Type
	}
	return fields, nil
}

func checkProfile(typ reflect.Type, _ map[reflect.Type]bool) error {
	return input.ValidateProductType(typ)
}

func validateJSON(data json.RawMessage, typ reflect.Type, path string) error {
	// Generated presence fields wrap the same domain types in pointers. Unwrap
	// only for shape validation; this never decodes away an explicit null.
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if strings.TrimSpace(string(data)) == jsonNull {
		return fmt.Errorf("%s: null is not an inheritance or deletion operation", path)
	}
	if typ == reflect.TypeFor[json.RawMessage]() {
		// The generated input uses RawMessage only for the PodTemplate patch.
		// Its internal null and $patch directives have Kubernetes patch semantics.
		var object map[string]json.RawMessage
		if err := json.Unmarshal(data, &object); err != nil || object == nil {
			return fmt.Errorf("%s: pod override must be an object", path)
		}
		return nil
	}
	if typ == affinityType {
		return validateAffinityJSON(data, path)
	}
	if typ == durationType {
		return validateDurationJSON(data, path)
	}
	if typ == quantityType {
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return fmt.Errorf("%s: quantity must be a string", path)
		}
		if _, err := resource.ParseQuantity(value); err != nil {
			return fmt.Errorf("%s: invalid quantity: %w", path, err)
		}
		return nil
	}
	if typ == s3ConnectionType {
		if err := validateS3Layer(data, path); err != nil {
			return err
		}
	}
	if typ == reflect.TypeFor[framework.Storage]() {
		if err := validateStorageLayer(data, path); err != nil {
			return err
		}
	}
	switch typ.Kind() {
	case reflect.Struct, reflect.Map:
		values, err := decodeConfigLayer(data, path)
		if err != nil {
			return err
		}
		var fields map[string]reflect.Type
		if typ.Kind() == reflect.Struct {
			fields, _ = jsonFields(typ)
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

func mergeObjects(base, patch map[string]json.RawMessage) map[string]json.RawMessage {
	for key, value := range patch {
		var left, right map[string]json.RawMessage
		if json.Unmarshal(base[key], &left) == nil && left != nil &&
			json.Unmarshal(value, &right) == nil && right != nil {
			value, _ = json.Marshal(mergeObjects(left, right))
		}
		base[key] = value
	}
	return base
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
