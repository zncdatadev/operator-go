package inputgen

import (
	"fmt"
	"math"
	"reflect"
	"strings"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	"github.com/zncdatadev/operator-go/pkg/framework"
	"github.com/zncdatadev/operator-go/pkg/framework/input"
)

// CollectionLimit bounds nested configuration collections in generated CRDs.
// It is an admission-cost boundary, not a merge policy. Larger or more deeply
// nested product schemas require their own real API installation validation.
const CollectionLimit int64 = 32

const (
	imageInputField         = "image"
	clusterConfigInputField = "clusterConfig"
	schemaConfigField       = "config"
	schemaSpecField         = "spec"
	schemaObjectType        = "object"
	schemaStringType        = "string"
	schemaIntegerType       = "integer"
	schemaBooleanType       = "boolean"
	schemaArrayType         = "array"
	schemaInt32Format       = "int32"
	schemaInt64Format       = "int64"
	schemaReplaceField      = "replace"
	schemaRemoveField       = "remove"
	schemaSetField          = "set"
	schemaStatusField       = "status"
)

// configSchema generates the flat CR config from the common and product value
// types. All fields are optional and no schema default is emitted: admission
// must not erase the distinction between an absent field and an explicit zero.
// Nullable retains explicit null through pruning; enclosing CEL rules reject it.
func configSchema[C any]() (apiextensionsv1.JSONSchemaProps, error) {
	product := reflect.TypeFor[C]()
	if err := input.ValidateProductType(product); err != nil {
		return apiextensionsv1.JSONSchemaProps{}, fmt.Errorf("product config: %w", err)
	}
	if product.Kind() != reflect.Struct || product == quantityType || product == durationType {
		return apiextensionsv1.JSONSchemaProps{}, fmt.Errorf("product config must be a struct")
	}
	common, err := schemaForType(reflect.TypeFor[framework.CommonConfig](), make(map[reflect.Type]bool))
	if err != nil {
		return apiextensionsv1.JSONSchemaProps{}, err
	}
	specific, err := schemaForType(product, make(map[reflect.Type]bool))
	if err != nil {
		return apiextensionsv1.JSONSchemaProps{}, fmt.Errorf("product config: %w", err)
	}
	for name, property := range specific.Properties {
		if _, exists := common.Properties[name]; exists {
			return apiextensionsv1.JSONSchemaProps{}, fmt.Errorf("product config field %q collides with common config", name)
		}
		common.Properties[name] = property
	}
	return nonNullObjectSchema(common.Properties)
}

// clusterConfigSchema flattens framework operations and product cluster config.
// Optional fields preserve presence without workload fields or schema defaults.
func clusterConfigSchema[S any]() (apiextensionsv1.JSONSchemaProps, error) {
	typ := reflect.TypeFor[S]()
	if err := checkClusterConfigType(typ); err != nil {
		return apiextensionsv1.JSONSchemaProps{}, err
	}
	product, err := schemaForType(typ, make(map[reflect.Type]bool))
	if err != nil {
		return apiextensionsv1.JSONSchemaProps{}, err
	}
	operation, err := schemaForType(reflect.TypeFor[framework.ClusterOperation](), make(map[reflect.Type]bool))
	if err != nil {
		return apiextensionsv1.JSONSchemaProps{}, err
	}
	platform, err := schemaForType(reflect.TypeFor[framework.ClusterConfig](), make(map[reflect.Type]bool))
	if err != nil {
		return apiextensionsv1.JSONSchemaProps{}, err
	}
	for name, field := range platform.Properties {
		product.Properties[name] = field
	}
	for name, field := range operation.Properties {
		product.Properties[name] = field
	}
	return nonNullObjectSchema(product.Properties)
}

// crdSchema describes the supported image, cluster, role/group input envelope
// and the fixed observation status. Unsupported platform fields are not emitted.
func crdSchema[C, S any](roles []string) (apiextensionsv1.JSONSchemaProps, error) {
	if _, err := inputRoleNames(roles); err != nil {
		return apiextensionsv1.JSONSchemaProps{}, err
	}
	config, err := configSchema[C]()
	if err != nil {
		return apiextensionsv1.JSONSchemaProps{}, err
	}
	zero := float64(0)
	replicas := apiextensionsv1.JSONSchemaProps{
		Type: schemaIntegerType, Format: schemaInt32Format, Nullable: true, Minimum: &zero,
	}
	groupFields, err := overrideInputSchemas()
	if err != nil {
		return apiextensionsv1.JSONSchemaProps{}, err
	}
	groupFields[schemaConfigField], groupFields["replicas"] = config, replicas
	group, err := nonNullObjectSchema(groupFields)
	if err != nil {
		return apiextensionsv1.JSONSchemaProps{}, err
	}
	roleFields := group.DeepCopy().Properties
	roleFields["roleGroups"] = nonNullMapSchema(group)
	management, err := schemaForType(reflect.TypeFor[framework.RoleConfig](), make(map[reflect.Type]bool))
	if err != nil {
		return apiextensionsv1.JSONSchemaProps{}, err
	}
	pdb := management.Properties["podDisruptionBudget"]
	maxUnavailable := pdb.Properties["maxUnavailable"]
	maxUnavailable.Minimum = &zero
	pdb.Properties["maxUnavailable"] = maxUnavailable
	management.Properties["podDisruptionBudget"] = pdb
	roleFields["roleConfig"] = management
	role, err := nonNullObjectSchema(roleFields)
	if err != nil {
		return apiextensionsv1.JSONSchemaProps{}, err
	}
	image, err := imageInputSchema()
	if err != nil {
		return apiextensionsv1.JSONSchemaProps{}, err
	}
	cluster, err := clusterConfigSchema[S]()
	if err != nil {
		return apiextensionsv1.JSONSchemaProps{}, err
	}
	properties := map[string]apiextensionsv1.JSONSchemaProps{imageInputField: image, clusterConfigInputField: cluster}
	for _, name := range roles {
		properties[name] = *role.DeepCopy()
	}
	spec, err := nonNullObjectSchema(properties)
	if err != nil {
		return apiextensionsv1.JSONSchemaProps{}, err
	}
	// Kubernetes exposes these metadata fields implicitly in CEL. The root
	// count must include them even though this schema does not redefine them.
	return apiextensionsv1.JSONSchemaProps{
		Type: schemaObjectType, Required: []string{schemaSpecField},
		Properties: map[string]apiextensionsv1.JSONSchemaProps{
			schemaSpecField: spec, schemaStatusField: reconcileStatusSchema(),
		},
		XValidations: apiextensionsv1.ValidationRules{{
			Rule: "size(dyn(self)) == " +
				"[has(self.spec),has(self.status),has(self.apiVersion),has(self.kind),has(self.metadata)].filter(v,v).size()",
			Message: "explicit null spec is not allowed",
		}},
	}, nil
}

// Image source values are validated after presence-based selection. The fixed
// pull policy is independent of that selection and can be bounded at admission.
func imageInputSchema() (apiextensionsv1.JSONSchemaProps, error) {
	image, err := schemaForType(reflect.TypeFor[framework.ImageConfig](), make(map[reflect.Type]bool))
	if err != nil {
		return apiextensionsv1.JSONSchemaProps{}, err
	}
	policy := image.Properties["pullPolicy"]
	policy.Enum = []apiextensionsv1.JSON{
		{Raw: []byte(`"Always"`)}, {Raw: []byte(`"IfNotPresent"`)}, {Raw: []byte(`"Never"`)},
	}
	image.Properties["pullPolicy"] = policy
	return image, nil
}

// overrideInputSchemas describes fixed framework operation domains. These
// schemas are not inferred from product fields and expose no merge strategies.
// Only podOverrides preserves arbitrary nested keys and null: that field carries
// a native Kubernetes strategic merge patch, including its deletion directives.
func overrideInputSchemas() (map[string]apiextensionsv1.JSONSchemaProps, error) {
	stringValue := apiextensionsv1.JSONSchemaProps{Type: schemaStringType, Nullable: true}
	stringMap := nonNullMapSchema(stringValue)
	stringsList := nonNullListSchema(stringValue)
	properties, err := nonNullObjectSchema(map[string]apiextensionsv1.JSONSchemaProps{
		schemaSetField: stringMap, schemaRemoveField: stringsList, schemaReplaceField: stringMap,
	})
	if err != nil {
		return nil, err
	}
	properties.XValidations = append(properties.XValidations,
		apiextensionsv1.ValidationRule{
			Rule:    "!has(self.replace) || (!has(self.set) && !has(self.remove))",
			Message: "properties.replace cannot be combined with set or remove",
		},
		apiextensionsv1.ValidationRule{
			Rule:    "!has(self.set) || !has(self.remove) || self.remove.all(k, !(k in self.set))",
			Message: "a property cannot be both set and removed in one layer",
		},
	)
	lineValue := *stringValue.DeepCopy()
	lineValue.Pattern = `^[^\r\n]*$`
	file, err := nonNullObjectSchema(map[string]apiextensionsv1.JSONSchemaProps{
		"properties": properties, "lines": nonNullListSchema(lineValue), "text": stringValue,
		schemaRemoveField: {Type: schemaBooleanType, Nullable: true, Enum: []apiextensionsv1.JSON{{Raw: []byte("true")}}},
	})
	if err != nil {
		return nil, err
	}
	file.XValidations = append(file.XValidations, apiextensionsv1.ValidationRule{
		Rule:    "[has(self.properties),has(self.lines),has(self.text),has(self.remove)].filter(v,v).size() == 1",
		Message: "select exactly one of properties, lines, text or remove",
	})
	preserve := true
	return map[string]apiextensionsv1.JSONSchemaProps{
		"configOverrides": nonNullMapSchema(file),
		"envOverrides":    stringMap,
		"cliOverrides":    stringsList,
		"podOverrides": {
			Type: schemaObjectType, Nullable: true, XPreserveUnknownFields: &preserve,
		},
	}, nil
}

func schemaForType(typ reflect.Type, active map[reflect.Type]bool) (apiextensionsv1.JSONSchemaProps, error) {
	if typ == affinityType {
		return affinitySchema()
	}
	if typ == durationType {
		return durationSchema(), nil
	}
	if active[typ] {
		return apiextensionsv1.JSONSchemaProps{},
			fmt.Errorf("%s: recursive input types cannot generate a finite structural schema", typ)
	}
	active[typ] = true
	defer delete(active, typ)
	if typ == quantityType {
		// Quantity is a fixed domain scalar. Syntax belongs to the individual
		// input layer; positivity and min <= max belong to the effective input.
		length := int64(128)
		return apiextensionsv1.JSONSchemaProps{Type: schemaStringType, Nullable: true, MaxLength: &length,
			XValidations: apiextensionsv1.ValidationRules{{
				Rule: "isQuantity(self)", Message: "must be a Kubernetes quantity string",
			}},
		}, nil
	}
	out := apiextensionsv1.JSONSchemaProps{Nullable: true}
	switch typ.Kind() {
	case reflect.Struct:
		fields, err := jsonFields(typ)
		if err != nil {
			return out, err
		}
		properties := make(map[string]apiextensionsv1.JSONSchemaProps, len(fields))
		for _, name := range sortedKeys(fields) {
			child, err := schemaForType(fields[name], active)
			if err != nil {
				return out, fmt.Errorf("%s: %w", name, err)
			}
			properties[name] = child
		}
		out, err := nonNullObjectSchema(properties)
		if typ == reflect.TypeFor[framework.S3Connection]() && err == nil {
			out = s3ConnectionSchema(out)
		}
		if typ == reflect.TypeFor[framework.Storage]() && err == nil {
			kind := out.Properties["type"]
			kind.Enum = []apiextensionsv1.JSON{{Raw: []byte(`"ephemeral"`)}, {Raw: []byte(`"persistent"`)}}
			out.Properties["type"] = kind
			out.XValidations = append(out.XValidations, apiextensionsv1.ValidationRule{
				Rule:    "!has(self.type) || self.type != 'ephemeral' || (!has(self.storageClassName) && !has(self.capacity))",
				Message: "ephemeral storage cannot specify storageClassName or capacity",
			})
		}
		return out, err
	case reflect.Map:
		child, err := schemaForType(typ.Elem(), active)
		if err != nil {
			return out, err
		}
		return nonNullMapSchema(child), nil
	case reflect.Slice:
		child, err := schemaForType(typ.Elem(), active)
		if err != nil {
			return out, err
		}
		return nonNullListSchema(child), nil
	case reflect.Bool:
		out.Type = schemaBooleanType
	case reflect.String:
		out.Type = schemaStringType
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		out.Type, out.Format = schemaIntegerType, schemaInt64Format
		if typ.Bits() <= 32 {
			out.Format = schemaInt32Format
			maximum := float64(int64(1)<<(typ.Bits()-1) - 1)
			minimum := -maximum - 1
			out.Maximum, out.Minimum = &maximum, &minimum
		}
	case reflect.Uint8, reflect.Uint16, reflect.Uint32:
		minimum, maximum := float64(0), float64(uint64(1)<<typ.Bits()-1)
		out.Type, out.Format, out.Minimum, out.Maximum = schemaIntegerType, schemaInt64Format, &minimum, &maximum
	case reflect.Uint, reflect.Uint64:
		return out, fmt.Errorf("%s: unsigned 64-bit input exceeds Kubernetes integer representation", typ)
	case reflect.Float32, reflect.Float64:
		out.Type, out.Format = "number", "double"
		if typ.Kind() == reflect.Float32 {
			minimum, maximum := -float64(math.MaxFloat32), float64(math.MaxFloat32)
			out.Format, out.Minimum, out.Maximum = "float", &minimum, &maximum
		}
	default:
		return out, fmt.Errorf("%s: unsupported schema type", typ)
	}
	return out, nil
}

func nonNullObjectSchema(
	properties map[string]apiextensionsv1.JSONSchemaProps,
) (apiextensionsv1.JSONSchemaProps, error) {
	present := make([]string, 0, len(properties))
	for _, name := range sortedKeys(properties) {
		escaped, ok := inputCELFieldName(name)
		if !ok {
			return apiextensionsv1.JSONSchemaProps{}, fmt.Errorf("JSON field %q cannot be addressed by Kubernetes CEL", name)
		}
		present = append(present, "has(self."+escaped+")")
	}
	return apiextensionsv1.JSONSchemaProps{Type: schemaObjectType, Nullable: true, Properties: properties,
		XValidations: apiextensionsv1.ValidationRules{{
			Rule:    "size(dyn(self)) == [" + strings.Join(present, ",") + "].filter(v,v).size()",
			Message: "explicit null fields are not allowed",
		}},
	}, nil
}

// inputCELFieldName implements Kubernetes' documented field-name escaping.
// Keeping this tiny conversion here avoids importing the apiserver runtime and
// CEL evaluator into the product-input generator. CRD installation tests compile
// the resulting expressions against the real API server.
func inputCELFieldName(name string) (string, bool) {
	if name == "" || name[0] >= '0' && name[0] <= '9' {
		return "", false
	}
	for _, char := range name {
		allowed := char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' ||
			char == '_' || char == '.' || char == '-' || char == '/'
		if !allowed {
			return "", false
		}
	}
	switch name {
	case "true", "false", "null", "in", "as", "break", "const", "continue", "else", "for", "function", "if",
		"import", "let", "loop", "package", schemaNamespaceField, "return", "var", "void", "while":
		return "__" + name + "__", true
	default:
		replacer := strings.NewReplacer("__", "__underscores__", ".", "__dot__", "-", "__dash__", "/", "__slash__")
		return replacer.Replace(name), true
	}
}

func nonNullMapSchema(value apiextensionsv1.JSONSchemaProps) apiextensionsv1.JSONSchemaProps {
	limit := CollectionLimit
	return apiextensionsv1.JSONSchemaProps{Type: schemaObjectType, Nullable: true, MaxProperties: &limit,
		AdditionalProperties: &apiextensionsv1.JSONSchemaPropsOrBool{Allows: true, Schema: &value},
		XValidations: apiextensionsv1.ValidationRules{{
			Rule: "self.all(k, dyn(self[k]) != null)", Message: "null map values are not allowed",
		}},
	}
}

func nonNullListSchema(value apiextensionsv1.JSONSchemaProps) apiextensionsv1.JSONSchemaProps {
	limit := CollectionLimit
	return apiextensionsv1.JSONSchemaProps{Type: schemaArrayType, Nullable: true, MaxItems: &limit,
		Items: &apiextensionsv1.JSONSchemaPropsOrArray{Schema: &value},
		XValidations: apiextensionsv1.ValidationRules{{
			Rule: "self.all(v, dyn(v) != null)", Message: "null array elements are not allowed",
		}},
	}
}
