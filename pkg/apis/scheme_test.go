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

package apis_test

import (
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	authenticationv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/authentication/v1alpha1"
	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	databasev1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/database/v1alpha1"
	listenersv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/listeners/v1alpha1"
	s3v1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/s3/v1alpha1"
)

func TestSchemeRegistration(t *testing.T) {
	groups := []struct {
		name     string
		version  schema.GroupVersion
		register func(*runtime.Scheme) error
		objects  []runtime.Object
	}{
		{"authentication", authenticationv1alpha1.GroupVersion, authenticationv1alpha1.AddToScheme,
			[]runtime.Object{&authenticationv1alpha1.AuthenticationClass{}, &authenticationv1alpha1.AuthenticationClassList{}}},
		{"commons", commonsv1alpha1.GroupVersion, commonsv1alpha1.AddToScheme, nil},
		{"database", databasev1alpha1.GroupVersion, databasev1alpha1.AddToScheme,
			[]runtime.Object{&databasev1alpha1.DatabaseConnection{}, &databasev1alpha1.DatabaseConnectionList{}}},
		{"listeners", listenersv1alpha1.GroupVersion, listenersv1alpha1.AddToScheme,
			[]runtime.Object{&listenersv1alpha1.Listener{}, &listenersv1alpha1.ListenerList{},
				&listenersv1alpha1.ListenerClass{}, &listenersv1alpha1.ListenerClassList{},
				&listenersv1alpha1.PodListeners{}, &listenersv1alpha1.PodListenersList{}}},
		{"s3", s3v1alpha1.GroupVersion, s3v1alpha1.AddToScheme,
			[]runtime.Object{&s3v1alpha1.S3Bucket{}, &s3v1alpha1.S3BucketList{},
				&s3v1alpha1.S3Connection{}, &s3v1alpha1.S3ConnectionList{}}},
	}
	for _, group := range groups {
		t.Run(group.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			for range 2 {
				if err := group.register(scheme); err != nil {
					t.Fatal(err)
				}
			}
			for _, kind := range []string{"ListOptions", "WatchEvent"} {
				if !scheme.Recognizes(group.version.WithKind(kind)) {
					t.Fatalf("metadata kind %s is not registered", kind)
				}
			}
			codecs := serializer.NewCodecFactory(scheme)
			for _, object := range group.objects {
				kind := reflect.TypeOf(object).Elem().Name()
				t.Run(kind, func(t *testing.T) {
					wantGVK := group.version.WithKind(kind)
					gvks, unversioned, err := scheme.ObjectKinds(object)
					if err != nil || unversioned || !reflect.DeepEqual(gvks, []schema.GroupVersionKind{wantGVK}) {
						t.Fatalf("ObjectKinds: got %v, unversioned=%v, err=%v; want %v", gvks, unversioned, err, wantGVK)
					}
					data, err := runtime.Encode(codecs.LegacyCodec(group.version), object)
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
		})
	}
}
