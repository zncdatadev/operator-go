package inputgen

import (
	"fmt"
	"reflect"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// AffinityCollectionLimit bounds the native scheduling collections in this
// generated CRD so nested null checks fit the API admission cost budget. It is
// a schema capacity boundary, not a product merge policy.
const AffinityCollectionLimit int64 = 16

// This traversal is reachable only from the fixed corev1.Affinity domain. It
// follows native optional pointers without admitting arbitrary product pointers
// or generating another set of Kubernetes Go types. Kubernetes still validates
// scheduling constraints when the final workload is admitted.
func affinitySchema() (apiextensionsv1.JSONSchemaProps, error) {
	return affinityNodeSchema(affinityType)
}

func affinityNodeSchema(typ reflect.Type) (apiextensionsv1.JSONSchemaProps, error) {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	switch typ.Kind() {
	case reflect.Struct:
		fields, err := jsonFields(typ)
		if err != nil {
			return apiextensionsv1.JSONSchemaProps{}, err
		}
		properties := make(map[string]apiextensionsv1.JSONSchemaProps, len(fields))
		for _, name := range sortedKeys(fields) {
			child, err := affinityNodeSchema(fields[name])
			if err != nil {
				return apiextensionsv1.JSONSchemaProps{}, fmt.Errorf("%s: %w", name, err)
			}
			properties[name] = child
		}
		return nonNullObjectSchema(properties)
	case reflect.Map:
		child, err := affinityNodeSchema(typ.Elem())
		out := nonNullMapSchema(child)
		limit := AffinityCollectionLimit
		out.MaxProperties = &limit
		return out, err
	case reflect.Slice:
		child, err := affinityNodeSchema(typ.Elem())
		out := nonNullListSchema(child)
		limit := AffinityCollectionLimit
		out.MaxItems = &limit
		return out, err
	default:
		return schemaForType(typ, make(map[reflect.Type]bool))
	}
}
