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

// TestDiscoverAggregatorAddress_InvalidAddressValue asserts the discovered value is rejected
// here rather than interpolated into vector.yaml, where a malformed address surfaces only as a
// crash-looping sidecar.
func TestDiscoverAggregatorAddress_InvalidAddressValue(t *testing.T) {
	tests := []struct {
		name    string
		address string
	}{
		{name: "no port", address: "vector-aggregator"},
		{name: "empty host", address: ":9000"},
		{name: "non numeric port", address: "vector-aggregator:http"},
		{name: "port out of range", address: "vector-aggregator:70000"},
		{name: "zero port", address: "vector-aggregator:0"},
		{name: "trailing newline", address: "vector-aggregator:9000\n"},
		{name: "injected line", address: "vector-aggregator:9000\nplayground: true"},
		{name: "double quote", address: `vector-"aggregator":9000`},
		{name: "backslash", address: `vector-aggregator\:9000`},
		{name: "surrounding whitespace", address: " vector-aggregator:9000 "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "aggregator-config",
				},
				Data: map[string]string{"ADDRESS": tt.address},
			}

			c := newFakeClient(cm)
			if _, err := DiscoverAggregatorAddress(context.Background(), c, "test-ns", "aggregator-config"); err == nil {
				t.Fatalf("DiscoverAggregatorAddress() expected error for address %q, got nil", tt.address)
			}
		})
	}
}

func TestDiscoverAggregatorAddress_ValidAddressShapes(t *testing.T) {
	addresses := []string{
		"vector-aggregator:9000",
		"vector-aggregator.test-ns.svc.cluster.local:6000",
		"10.0.0.1:1",
		"[fd00::1]:9000",
	}

	for _, address := range addresses {
		t.Run(address, func(t *testing.T) {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "aggregator-config",
				},
				Data: map[string]string{"ADDRESS": address},
			}

			c := newFakeClient(cm)
			got, err := DiscoverAggregatorAddress(context.Background(), c, "test-ns", "aggregator-config")
			if err != nil {
				t.Fatalf("DiscoverAggregatorAddress() error = %v", err)
			}
			if got != address {
				t.Errorf("DiscoverAggregatorAddress() = %q, want %q", got, address)
			}
		})
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

// Vector's own sink accepts a URL-form address, so discovery must not reject one.
func TestValidateAggregatorAddress_SchemeQualified(t *testing.T) {
	for _, addr := range []string{"https://vector-aggregator:6000", "http://10.22.212.22:9000", "vector-aggregator:6000"} {
		if err := validateAggregatorAddress(addr); err != nil {
			t.Errorf("validateAggregatorAddress(%q) = %v, want nil", addr, err)
		}
	}
	for _, addr := range []string{"vector-aggregator", "https://vector-aggregator", "host:0", "host:70000"} {
		if err := validateAggregatorAddress(addr); err == nil {
			t.Errorf("validateAggregatorAddress(%q) = nil, want an error", addr)
		}
	}
}
