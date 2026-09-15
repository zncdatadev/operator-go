package inputgen

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/zncdatadev/operator-go/pkg/framework"
	"github.com/zncdatadev/operator-go/pkg/framework/input"
)

const inputConfigPath = "config"

// emitConfigTypes emits declarations only; its caller supplies the package and
// resource import. Generated pointers belong to input field presence, never to
// collection entries. The effective product type is not reused as a partial CR.
func emitConfigTypes[C any]() (string, error) {
	productType := reflect.TypeFor[C]()
	if productType.Kind() != reflect.Struct || productType == quantityType || productType == durationType {
		return "", fmt.Errorf("product config must be a struct")
	}
	if err := input.ValidateProductType(productType); err != nil {
		return "", fmt.Errorf("product config: %w", err)
	}
	sources := []reflect.Type{reflect.TypeFor[framework.CommonConfig](), productType}
	if err := checkFlatFields(sources); err != nil {
		return "", err
	}
	return emitInputTypes("ConfigInput", inputConfigPath, sources)
}

// The cluster wire object flattens framework operations and the independent
// product object. Common workload fields and role/group inheritance stay absent.
func emitClusterConfigTypes[S any]() (string, error) {
	clusterType := reflect.TypeFor[S]()
	if err := checkClusterConfigType(clusterType); err != nil {
		return "", err
	}
	return emitInputTypes("ClusterConfigInput", clusterConfigInputField,
		[]reflect.Type{reflect.TypeFor[framework.ClusterOperation](), reflect.TypeFor[framework.ClusterConfig](), clusterType})
}

func emitInputTypes(typeName, path string, sources []reflect.Type) (string, error) {
	emitter := configTypeEmitter{
		active: make(map[reflect.Type]bool),
		names:  map[string]string{typeName: path},
	}
	var fields strings.Builder
	for _, source := range sources {
		emitter.active[source] = true
		part, err := emitter.fields(source, typeName, path)
		delete(emitter.active, source)
		if err != nil {
			return "", err
		}
		fields.WriteString(part)
	}
	emitter.declarations = append(emitter.declarations, "type "+typeName+" struct {\n"+fields.String()+"}\n")
	return strings.Join(emitter.declarations, "\n"), nil
}

// Common and product fields share one struct and one JSON object, so both Go
// member names and JSON names must be unique at that boundary.
func checkFlatFields(sources []reflect.Type) error {
	goNames, jsonNames := make(map[string]bool), make(map[string]bool)
	for _, source := range sources {
		for index := 0; index < source.NumField(); index++ {
			field := source.Field(index)
			name := inputJSONName(field)
			if goNames[field.Name] || jsonNames[name] {
				return fmt.Errorf("common/product config field %s (%q) collides", field.Name, name)
			}
			goNames[field.Name], jsonNames[name] = true, true
		}
	}
	return nil
}

type configTypeEmitter struct {
	active       map[reflect.Type]bool
	names        map[string]string
	declarations []string
}

func (e *configTypeEmitter) fields(typ reflect.Type, typeName, path string) (string, error) {
	var result strings.Builder
	for index := 0; index < typ.NumField(); index++ {
		field := typ.Field(index)
		name := inputJSONName(field)
		valueType, err := e.valueType(field.Type, typeName+field.Name, path+"."+name)
		if err != nil {
			return "", err
		}
		tag := "json:" + strconv.Quote(name+",omitempty")
		quotedTag := strconv.Quote(tag)
		if strconv.CanBackquote(tag) {
			quotedTag = "`" + tag + "`"
		}
		fmt.Fprintf(&result, "\t%s *%s %s\n", field.Name, valueType, quotedTag)
	}
	return result.String(), nil
}

func (e *configTypeEmitter) valueType(typ reflect.Type, typeName, path string) (string, error) {
	if typ == quantityType {
		return "resource.Quantity", nil
	}
	if typ == durationType {
		return "metav1.Duration", nil
	}
	if typ == affinityType {
		return "corev1.Affinity", nil
	}
	if e.active[typ] {
		return "", fmt.Errorf("%s: recursive config type %s is unsupported", path, typ)
	}
	e.active[typ] = true
	defer delete(e.active, typ)
	switch typ.Kind() {
	case reflect.Struct:
		return e.structType(typ, typeName, path)
	case reflect.Map:
		value, err := e.valueType(typ.Elem(), typeName+"Value", path+"[value]")
		return "map[string]" + value, err
	case reflect.Slice:
		value, err := e.valueType(typ.Elem(), typeName+"Item", path+"[item]")
		return "[]" + value, err
	default:
		// Product profile validation excludes codecs and unsupported kinds. The kind
		// spelling erases named scalar aliases without importing product code.
		return typ.Kind().String(), nil
	}
}

func (e *configTypeEmitter) structType(typ reflect.Type, typeName, path string) (string, error) {
	if previous, exists := e.names[typeName]; exists {
		return "", fmt.Errorf("generated type name %s collides between %s and %s", typeName, previous, path)
	}
	e.names[typeName] = path
	fields, err := e.fields(typ, typeName, path)
	if err != nil {
		return "", err
	}
	e.declarations = append(e.declarations, "type "+typeName+" struct {\n"+fields+"}\n")
	return typeName, nil
}

func inputJSONName(field reflect.StructField) string {
	name := strings.Split(field.Tag.Get("json"), ",")[0]
	if name == "" {
		return field.Name
	}
	return name
}
