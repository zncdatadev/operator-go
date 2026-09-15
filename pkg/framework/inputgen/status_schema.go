package inputgen

import apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

const (
	statusTypeField      = "type"
	statusAppliedField   = "applied"
	statusRolesField     = "roles"
	statusNameField      = "name"
	statusReasonField    = "reason"
	statusRoleField      = "role"
	statusMessageField   = "message"
	statusStateField     = "state"
	schemaNamespaceField = "namespace"
)

// Status is a fixed observation domain. It has no inherited configuration, CEL
// presence rules, defaults or CollectionLimit restriction.
func reconcileStatusSchema() apiextensionsv1.JSONSchemaProps {
	zero := float64(0)
	mapType := "map"
	text := apiextensionsv1.JSONSchemaProps{Type: schemaStringType}
	identity := *text.DeepCopy()
	minimumLength := int64(1)
	identity.MinLength = &minimumLength
	count := apiextensionsv1.JSONSchemaProps{Type: schemaIntegerType, Format: schemaInt32Format, Minimum: &zero}
	check := apiextensionsv1.JSONSchemaProps{
		Type: schemaObjectType, Required: []string{"subject", statusStateField},
		Properties: map[string]apiextensionsv1.JSONSchemaProps{
			"subject": text, statusReasonField: text,
			statusStateField: {Type: schemaStringType, Enum: []apiextensionsv1.JSON{
				{Raw: []byte(`"consistent"`)}, {Raw: []byte(`"conflict"`)}, {Raw: []byte(`"unknown"`)},
			}},
		},
	}
	group := apiextensionsv1.JSONSchemaProps{
		Type:     schemaObjectType,
		Required: []string{statusRoleField, statusNameField, "desiredReplicas", "readyReplicas", statusAppliedField},
		Properties: map[string]apiextensionsv1.JSONSchemaProps{
			statusRoleField: identity, statusNameField: identity,
			"desiredReplicas": count, "readyReplicas": count, "executionReplicas": count,
			statusAppliedField: {Type: schemaBooleanType}, statusMessageField: text,
			"checks":   {Type: schemaArrayType, Items: &apiextensionsv1.JSONSchemaPropsOrArray{Schema: &check}},
			"facts":    factDiagnosticSchema(),
			"platform": platformObservationSchema(),
		},
	}
	role := apiextensionsv1.JSONSchemaProps{
		Type: schemaObjectType, Required: []string{statusNameField, statusAppliedField},
		Properties: map[string]apiextensionsv1.JSONSchemaProps{
			statusNameField: identity, statusAppliedField: {Type: schemaBooleanType}, statusMessageField: text,
		}}
	condition := conditionStatusSchema()
	return apiextensionsv1.JSONSchemaProps{Type: schemaObjectType,
		Properties: map[string]apiextensionsv1.JSONSchemaProps{
			"observedGeneration": {Type: schemaIntegerType, Format: schemaInt64Format, Minimum: &zero},
			"conditions": {Type: schemaArrayType, Items: &apiextensionsv1.JSONSchemaPropsOrArray{Schema: &condition},
				XListType: &mapType, XListMapKeys: []string{statusTypeField}},
			statusRolesField: {Type: schemaArrayType, Items: &apiextensionsv1.JSONSchemaPropsOrArray{Schema: &role},
				XListType: &mapType, XListMapKeys: []string{statusNameField}},
			"groups": {Type: schemaArrayType, Items: &apiextensionsv1.JSONSchemaPropsOrArray{Schema: &group},
				XListType: &mapType, XListMapKeys: []string{statusRoleField, statusNameField}},
		},
	}
}

// These constraints follow metav1.Condition's wire contract; condition names
// are not limited to the controller's current set of observations.
func conditionStatusSchema() apiextensionsv1.JSONSchemaProps {
	zero, one := float64(0), int64(1)
	maxType, maxReason, maxMessage := int64(316), int64(1024), int64(32768)
	conditionTypePattern := `^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*/)?` +
		`(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])$`
	return apiextensionsv1.JSONSchemaProps{
		Type:     schemaObjectType,
		Required: []string{statusTypeField, schemaStatusField, statusReasonField, statusMessageField, "lastTransitionTime"},
		Properties: map[string]apiextensionsv1.JSONSchemaProps{
			statusTypeField: {Type: schemaStringType, MaxLength: &maxType, Pattern: conditionTypePattern},
			schemaStatusField: {Type: schemaStringType, Enum: []apiextensionsv1.JSON{
				{Raw: []byte(`"True"`)}, {Raw: []byte(`"False"`)}, {Raw: []byte(`"Unknown"`)},
			}},
			statusReasonField: {Type: schemaStringType, MinLength: &one, MaxLength: &maxReason,
				Pattern: `^[A-Za-z]([A-Za-z0-9_,:]*[A-Za-z0-9_])?$`},
			statusMessageField:   {Type: schemaStringType, MaxLength: &maxMessage},
			"lastTransitionTime": {Type: schemaStringType, Format: "date-time"},
			"observedGeneration": {Type: schemaIntegerType, Format: schemaInt64Format, Minimum: &zero},
		},
	}
}

func factDiagnosticSchema() apiextensionsv1.JSONSchemaProps {
	text := apiextensionsv1.JSONSchemaProps{Type: schemaStringType}
	object := apiextensionsv1.JSONSchemaProps{Type: schemaObjectType,
		Required: []string{"apiVersion", "kind", schemaNamespaceField, statusNameField},
		Properties: map[string]apiextensionsv1.JSONSchemaProps{
			"apiVersion": text, "kind": text, schemaNamespaceField: text, statusNameField: text,
			"uid": text, "resourceVersion": text,
		},
	}
	return apiextensionsv1.JSONSchemaProps{Type: schemaObjectType, Required: []string{statusStateField},
		Properties: map[string]apiextensionsv1.JSONSchemaProps{
			statusStateField: {Type: schemaStringType, Enum: []apiextensionsv1.JSON{
				{Raw: []byte(`"resolved"`)}, {Raw: []byte(`"pending"`)},
				{Raw: []byte(`"invalid"`)}, {Raw: []byte(`"readError"`)},
			}},
			"reason": text, "message": text,
			"observed": {Type: schemaArrayType, Items: &apiextensionsv1.JSONSchemaPropsOrArray{Schema: &object}},
		},
	}
}

func platformObservationSchema() apiextensionsv1.JSONSchemaProps {
	text := apiextensionsv1.JSONSchemaProps{Type: schemaStringType}
	port := apiextensionsv1.JSONSchemaProps{Type: schemaIntegerType, Format: schemaInt32Format}
	address := apiextensionsv1.JSONSchemaProps{Type: schemaObjectType, Required: []string{"pod", "directory", "address", "ports"}, Properties: map[string]apiextensionsv1.JSONSchemaProps{
		"pod": text, "directory": text, "address": text, "ports": {Type: schemaObjectType, AdditionalProperties: &apiextensionsv1.JSONSchemaPropsOrBool{Allows: true, Schema: &port}},
	}}
	return apiextensionsv1.JSONSchemaProps{Type: schemaObjectType, Required: []string{"phase", "diagnostic"}, Properties: map[string]apiextensionsv1.JSONSchemaProps{
		"phase": text, "diagnostic": factDiagnosticSchema(), "listeners": {Type: schemaArrayType, Items: &apiextensionsv1.JSONSchemaPropsOrArray{Schema: &address}},
	}}
}
