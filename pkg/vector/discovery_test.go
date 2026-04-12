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
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newFakeClient(objects ...client.Object) client.Client {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
}

func TestDiscoverAggregatorAddress_Success(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name:      "aggregator-config",
		},
		Data: map[string]string{
			"ADDRESS": "vector-aggregator.test-ns.svc:9000",
		},
	}

	c := newFakeClient(cm)
	address, err := DiscoverAggregatorAddress(context.Background(), c, "test-ns", "aggregator-config")
	if err != nil {
		t.Fatalf("DiscoverAggregatorAddress() error = %v", err)
	}
	if address != "vector-aggregator.test-ns.svc:9000" {
		t.Errorf("DiscoverAggregatorAddress() = %q, want %q", address, "vector-aggregator.test-ns.svc:9000")
	}
}

func TestDiscoverAggregatorAddress_MissingConfigMap(t *testing.T) {
	c := newFakeClient()
	_, err := DiscoverAggregatorAddress(context.Background(), c, "test-ns", "nonexistent-config")
	if err == nil {
		t.Fatal("DiscoverAggregatorAddress() expected error for missing ConfigMap, got nil")
	}
}

func TestDiscoverAggregatorAddress_MissingAddressKey(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name:      "aggregator-config",
		},
		Data: map[string]string{
			"OTHER_KEY": "some-value",
		},
	}

	c := newFakeClient(cm)
	_, err := DiscoverAggregatorAddress(context.Background(), c, "test-ns", "aggregator-config")
	if err == nil {
		t.Fatal("DiscoverAggregatorAddress() expected error for missing ADDRESS key, got nil")
	}
}

func TestDiscoverAggregatorAddress_EmptyAddressValue(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name:      "aggregator-config",
		},
		Data: map[string]string{
			"ADDRESS": "",
		},
	}

	c := newFakeClient(cm)
	_, err := DiscoverAggregatorAddress(context.Background(), c, "test-ns", "aggregator-config")
	if err == nil {
		t.Fatal("DiscoverAggregatorAddress() expected error for empty ADDRESS value, got nil")
	}
}

func TestDiscoverAggregatorAddress_NilData(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name:      "aggregator-config",
		},
	}

	c := newFakeClient(cm)
	_, err := DiscoverAggregatorAddress(context.Background(), c, "test-ns", "aggregator-config")
	if err == nil {
		t.Fatal("DiscoverAggregatorAddress() expected error for nil data, got nil")
	}
}
