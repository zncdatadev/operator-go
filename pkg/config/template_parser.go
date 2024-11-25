/*
Copyright 2024 ZNCDataDev.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
