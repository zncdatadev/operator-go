/*
Copyright 2026 ZNCDataDev.

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

package v1alpha1_test

import (
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	trinov1alpha1 "github.com/zncdatadev/operator-go/examples/trino-operator/api/v1alpha1"
)

func TestSchemeRegistration(t *testing.T) {
	scheme := runtime.NewScheme()
	for range 2 {
		if err := trinov1alpha1.AddToScheme(scheme); err != nil {
			t.Fatal(err)
		}
	}
	for _, kind := range []string{"ListOptions", "WatchEvent"} {
		if !scheme.Recognizes(trinov1alpha1.GroupVersion.WithKind(kind)) {
			t.Fatalf("metadata kind %s is not registered", kind)
		}
	}
	codecs := serializer.NewCodecFactory(scheme)
	for _, object := range []runtime.Object{&trinov1alpha1.TrinoCluster{}, &trinov1alpha1.TrinoClusterList{}} {
		kind := reflect.TypeOf(object).Elem().Name()
		t.Run(kind, func(t *testing.T) {
			wantGVK := trinov1alpha1.GroupVersion.WithKind(kind)
			gvks, unversioned, err := scheme.ObjectKinds(object)
			if err != nil || unversioned || !reflect.DeepEqual(gvks, []schema.GroupVersionKind{wantGVK}) {
				t.Fatalf("ObjectKinds: got %v, unversioned=%v, err=%v; want %v", gvks, unversioned, err, wantGVK)
			}
			data, err := runtime.Encode(codecs.LegacyCodec(trinov1alpha1.GroupVersion), object)
			if err != nil {
				t.Fatal(err)
			}
			decoded, gvk, err := codecs.UniversalDeserializer().Decode(data, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if *gvk != wantGVK || reflect.TypeOf(decoded) != reflect.TypeOf(object) {
				t.Fatalf("round trip: got %T %v; want %T %v", decoded, gvk, object, wantGVK)
			}
		})
	}
}
