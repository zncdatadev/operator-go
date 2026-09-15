package inputgen

import (
	"fmt"
	"go/ast"
	"go/token"
	"reflect"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/zncdatadev/operator-go/pkg/framework"
	"github.com/zncdatadev/operator-go/pkg/framework/input"
)

var (
	quantityType = reflect.TypeFor[resource.Quantity]()
	durationType = reflect.TypeFor[metav1.Duration]()
	affinityType = reflect.TypeFor[corev1.Affinity]()
)

// The generated API does not import C/S yet, but its future registration bridge
// must be able to reference the same product types from another package.
func validateRootReference(typ reflect.Type) error {
	if typ.Kind() != reflect.Struct || typ == quantityType || typ == durationType {
		return fmt.Errorf("product configuration must be a struct: %s", typ)
	}
	if typ.Name() == "" && typ.NumField() == 0 {
		return nil
	}
	if !token.IsIdentifier(typ.Name()) || !ast.IsExported(typ.Name()) || typ.PkgPath() == "" {
		return fmt.Errorf("product configuration must be an exported named struct or struct{}: %s", typ)
	}
	return nil
}

func checkClusterConfigType(typ reflect.Type) error {
	if typ.Kind() != reflect.Struct || typ == quantityType || typ == durationType {
		return fmt.Errorf("product cluster config must be a struct")
	}
	if err := input.ValidateProductType(typ); err != nil {
		return fmt.Errorf("product cluster config: %w", err)
	}
	return checkFlatFields([]reflect.Type{reflect.TypeFor[framework.ClusterOperation](), reflect.TypeFor[framework.ClusterConfig](), typ})
}

// Keep JSON field traversal local to the generator; product type admissibility
// is owned by the input contract, not by a second merge or validation engine.
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

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func durationSchema() apiextensionsv1.JSONSchemaProps {
	limit := int64(128)
	return apiextensionsv1.JSONSchemaProps{Type: schemaStringType, Nullable: true, MaxLength: &limit,
		XValidations: apiextensionsv1.ValidationRules{{
			Rule: "duration(self) == duration(self)", Message: "must be a valid duration string",
		}},
	}
}
