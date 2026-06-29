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

package vector

import (
	"testing"

	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"k8s.io/utils/ptr"
)

func TestIsAgentEnabled(t *testing.T) {
	cases := []struct {
		name    string
		logging *v1alpha1.LoggingSpec
		want    bool
	}{
		{name: "nil spec", logging: nil, want: false},
		{name: "nil EnableVectorAgent", logging: &v1alpha1.LoggingSpec{}, want: false},
		{name: "explicit false", logging: &v1alpha1.LoggingSpec{EnableVectorAgent: ptr.To(false)}, want: false},
		{name: "explicit true", logging: &v1alpha1.LoggingSpec{EnableVectorAgent: ptr.To(true)}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsAgentEnabled(tc.logging); got != tc.want {
				t.Fatalf("IsAgentEnabled(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}
