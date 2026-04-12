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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// AddressKey is the key in the ConfigMap that contains the aggregator address.
	AddressKey = "ADDRESS"
)

// DiscoverAggregatorAddress reads the aggregator address from a ConfigMap.
// The ConfigMap is expected to have an "ADDRESS" key in its data.
func DiscoverAggregatorAddress(ctx context.Context, c client.Client, namespace, configMapName string) (string, error) {
	cm := &corev1.ConfigMap{}
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: configMapName}, cm); err != nil {
		return "", fmt.Errorf("failed to get configmap %s/%s: %w", namespace, configMapName, err)
	}

	if cm.Data == nil {
		return "", fmt.Errorf("configmap %s/%s has no data", namespace, configMapName)
	}

	address, exists := cm.Data[AddressKey]
	if !exists {
		return "", fmt.Errorf("configmap %s/%s missing %q key", namespace, configMapName, AddressKey)
	}

	if address == "" {
		return "", fmt.Errorf("configmap %s/%s has empty %q value", namespace, configMapName, AddressKey)
	}

	return address, nil
}
