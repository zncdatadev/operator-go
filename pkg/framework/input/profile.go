package input

import (
	"encoding"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	quantityType = reflect.TypeFor[resource.Quantity]()
	durationType = reflect.TypeFor[metav1.Duration]()
	affinityType = reflect.TypeFor[corev1.Affinity]()
)

// ValidateProductType checks the fixed data profile shared by runtime input
// validation and generation. Top-level C/S naming and common-field collisions
// are checked by inputgen. No type can register a custom merge or wire codec.
func ValidateProductType(typ reflect.Type) error {
	return checkProfile(typ, make(map[reflect.Type]bool), make(map[reflect.Type]bool))
}

func checkProfile(typ reflect.Type, active, checked map[reflect.Type]bool) error {
	if typ == nil {
		return fmt.Errorf("a concrete product data type is required")
	}
	if typ == quantityType || typ == durationType || checked[typ] {
		return nil
	}
	if active[typ] {
		return fmt.Errorf("%s: recursive product types are unsupported", typ)
	}
	active[typ] = true
	defer delete(active, typ)
	for _, codec := range []reflect.Type{
		reflect.TypeFor[json.Marshaler](), reflect.TypeFor[json.Unmarshaler](),
		reflect.TypeFor[encoding.TextMarshaler](), reflect.TypeFor[encoding.TextUnmarshaler](),
	} {
		if typ.Implements(codec) || reflect.PointerTo(typ).Implements(codec) {
			return fmt.Errorf("%s: custom codecs are unsupported", typ)
		}
	}
	if err := checkProfileFields(typ, active, checked); err != nil {
		return err
	}
	checked[typ] = true
	return nil
}

func checkProfileFields(typ reflect.Type, active, checked map[reflect.Type]bool) error {
	switch typ.Kind() {
	case reflect.Struct:
		fields, err := jsonFields(typ)
		if err != nil {
			return err
		}
		for _, name := range sortedKeys(fields) {
			if err := checkProfile(fields[name], active, checked); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
	case reflect.Map:
		if typ.Key().Kind() != reflect.String {
			return fmt.Errorf("%s: map keys must be strings", typ)
		}
		if err := checkProfile(typ.Key(), active, checked); err != nil {
			return err
		}
		return checkProfile(typ.Elem(), active, checked)
	case reflect.Slice:
		if typ.Elem().Kind() == reflect.Uint8 {
			return fmt.Errorf("%s: byte slices are unsupported", typ)
		}
		return checkProfile(typ.Elem(), active, checked)
	case reflect.Bool, reflect.String, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
	default:
		return fmt.Errorf("%s: unsupported product data type", typ)
	}
	return nil
}

func jsonFields(typ reflect.Type) (map[string]reflect.Type, error) {
	fields := make(map[string]reflect.Type, typ.NumField())
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

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
