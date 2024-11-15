package config

import (
	"bytes"
	"text/template"

	ctrl "sigs.k8s.io/controller-runtime"
)

var logging = ctrl.Log.WithName("template-parser")

type TemplateParser struct {
	Value    interface{}
	Template string
}

func (t *TemplateParser) Parse() (string, error) {
	temp, err := template.New("").Parse(t.Template)
	if err != nil {
		logging.Error(err, "failed to parse template", "template", t.Template)
		return t.Template, err
	}
	var b bytes.Buffer
	if err := temp.Execute(&b, t.Value); err != nil {
		logging.Error(err, "failed to execute template", "template", t.Template, "data", t.Value)
		return t.Template, err
	}
	return b.String(), nil
}
