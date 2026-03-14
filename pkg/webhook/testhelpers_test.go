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

package webhook_test

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// TestRuntimeCR is a minimal runtime.Object for adapter tests.
type TestRuntimeCR struct {
	Name    string
	Applied bool
}

func (t *TestRuntimeCR) GetObjectKind() schema.ObjectKind { return schema.EmptyObjectKind }
func (t *TestRuntimeCR) DeepCopyObject() runtime.Object {
	copy := *t
	return &copy
}

// OtherRuntimeCR is a different runtime.Object for type-mismatch tests.
type OtherRuntimeCR struct{}

func (o *OtherRuntimeCR) GetObjectKind() schema.ObjectKind { return schema.EmptyObjectKind }
func (o *OtherRuntimeCR) DeepCopyObject() runtime.Object   { return &OtherRuntimeCR{} }
