package pipeline

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/zncdatadev/operator-go/pkg/framework"
)

// ValidateDefinition checks the static registration contract without executing
// product callbacks or resolving default values. A CR can still override an
// otherwise incomplete or invalid default before effective-input validation.
// This is framework plumbing, not another product-author lifecycle callback.
func ValidateDefinition[C, S, F any](definition framework.ProductDefinition[C, S, F]) error {
	if definition.Name == "" || len(definition.Roles) == 0 || definition.GenerateGroup == nil {
		return fmt.Errorf("product name, role defaults and GenerateGroup are required")
	}
	if err := checkConfigType(reflect.TypeFor[C]()); err != nil {
		return err
	}
	if err := checkClusterConfigType(reflect.TypeFor[S]()); err != nil {
		return err
	}
	if err := checkProfile(reflect.TypeFor[F](), make(map[reflect.Type]bool)); err != nil {
		return fmt.Errorf("shared facts: %w", err)
	}
	return nil
}

// Only static field/profile compatibility is checked here. Invalid default
// business values may be repaired by user layers before effective validation.
func checkConfigType(product reflect.Type) error {
	if product.Kind() != reflect.Struct || product == quantityType || product == durationType {
		return fmt.Errorf("product config must be a struct")
	}
	if err := checkProfile(product, make(map[reflect.Type]bool)); err != nil {
		return fmt.Errorf("product config: %w", err)
	}
	return checkFlatFields([]reflect.Type{reflect.TypeFor[CommonConfig](), product})
}

func checkFlatFields(sources []reflect.Type) error {
	goNames, jsonNames := make(map[string]bool), make(map[string]bool)
	for _, source := range sources {
		if _, err := jsonFields(source); err != nil {
			return err
		}
		for index := 0; index < source.NumField(); index++ {
			field := source.Field(index)
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name == "" {
				name = field.Name
			}
			if goNames[field.Name] || jsonNames[name] {
				return fmt.Errorf("common/product field %s (%q) collides", field.Name, name)
			}
			goNames[field.Name], jsonNames[name] = true, true
		}
	}
	return nil
}
