package inputgen

import apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

// The schema validates a single partial layer. Required selected-branch fields
// are checked after role/group inheritance, not filled by admission defaults.
func s3ConnectionSchema(out apiextensionsv1.JSONSchemaProps) apiextensionsv1.JSONSchemaProps {
	kind := out.Properties["type"]
	kind.Enum = []apiextensionsv1.JSON{{Raw: []byte(`"disabled"`)}, {Raw: []byte(`"inline"`)}, {Raw: []byte(`"reference"`)}}
	out.Properties["type"] = kind
	out.XValidations = append(out.XValidations, apiextensionsv1.ValidationRule{
		Rule: "!has(self.type) || (self.type == 'disabled' ? (!has(self.inline) && !has(self.reference)) : " +
			"(self.type == 'inline' ? !has(self.reference) : !has(self.inline)))",
		Message: "fields must belong to the selected S3 connection branch",
	})
	return out
}
