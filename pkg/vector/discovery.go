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
	"net"
	"strconv"
	"strings"

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
//
// The value is validated to be a single-line "host:port" (see validateAggregatorAddress): it is
// interpolated into the generated vector.yaml, so a malformed value would otherwise produce a
// config the sidecar rejects at startup — a failure that surfaces as a crash-looping pod rather
// than a reconcile error naming the ConfigMap.
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

	if err := validateAggregatorAddress(address); err != nil {
		return "", fmt.Errorf("configmap %s/%s has an invalid %q value: %w", namespace, configMapName, AddressKey, err)
	}

	return address, nil
}

// validateAggregatorAddress checks that s is a single-line "host:port" that can be embedded in
// the generated vector.yaml. Characters that would terminate the surrounding scalar (line
// breaks, quotes, backslashes) are rejected outright rather than escaped, because a value
// carrying them is a misconfigured ConfigMap, not an address.
func validateAggregatorAddress(s string) error {
	if strings.ContainsAny(s, "\n\r") {
		return fmt.Errorf("address %q must be a single line", s)
	}
	if strings.ContainsAny(s, "\"'\\") {
		return fmt.Errorf("address %q must not contain quotes or backslashes", s)
	}
	if strings.TrimSpace(s) != s {
		return fmt.Errorf("address %q must not have surrounding whitespace", s)
	}

	// Vector's own sink accepts a URL-form address ("https://aggregator:6000"), so a scheme is
	// stripped before the host:port parse rather than rejected — SplitHostPort would otherwise
	// read the scheme's colon and fail with "too many colons".
	hostPort := s
	if i := strings.Index(hostPort, "://"); i >= 0 {
		hostPort = hostPort[i+len("://"):]
	}

	host, port, err := net.SplitHostPort(hostPort)
	if err != nil {
		return fmt.Errorf("address %q must be in host:port form: %w", s, err)
	}
	if host == "" {
		return fmt.Errorf("address %q has an empty host", s)
	}
	portNum, err := strconv.Atoi(port)
	if err != nil || portNum < 1 || portNum > 65535 {
		return fmt.Errorf("address %q has an invalid port %q: expected 1-65535", s, port)
	}

	return nil
}
