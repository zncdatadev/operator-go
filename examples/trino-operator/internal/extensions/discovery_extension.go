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

package extensions

import (
	"context"
	"fmt"

	trinov1alpha1 "github.com/zncdatadev/operator-go/examples/trino-operator/api/v1alpha1"
	"github.com/zncdatadev/operator-go/examples/trino-operator/internal/product"
	"github.com/zncdatadev/operator-go/pkg/common"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// TrinoDiscoveryKey is the key carrying the coordinator URI in the discovery ConfigMap.
const TrinoDiscoveryKey = "TRINO"

// DiscoveryExtension is a ClusterExtension that publishes the cluster's client connection
// info as a "discovery ConfigMap" named after the CR — the kubedoop pattern every product
// operator follows (clients and dependent operators read the connection info from it).
//
// It demonstrates two SDK pieces working together:
//   - the ClusterExtension PostReconcile hook (runs after all role groups are reconciled,
//     so the coordinator Service exists), and
//   - reconciler.EnsureDiscoveryConfigMap, which owns the ensure semantics (idempotent
//     CreateOrUpdate, controller owner reference, canonical labels) while the product only
//     computes the data map.
type DiscoveryExtension struct {
	common.BaseExtension
	scheme *runtime.Scheme
}

// NewDiscoveryExtension creates a new DiscoveryExtension.
func NewDiscoveryExtension(scheme *runtime.Scheme) *DiscoveryExtension {
	return &DiscoveryExtension{
		BaseExtension: common.NewBaseExtension("discovery-extension"),
		scheme:        scheme,
	}
}

var _ common.ClusterExtension[common.ClusterInterface] = &DiscoveryExtension{}

// PreReconcile is a no-op: the coordinator Service the URI points at does not exist yet.
func (e *DiscoveryExtension) PreReconcile(_ context.Context, _ client.Client, _ common.ClusterInterface) error {
	return nil
}

// PostReconcile publishes the discovery ConfigMap once the role groups (and therefore the
// coordinator Service) have been reconciled.
func (e *DiscoveryExtension) PostReconcile(ctx context.Context, c client.Client, cr common.ClusterInterface) error {
	trinoCR, ok := cr.(*trinov1alpha1.TrinoCluster)
	if !ok {
		// The extension registry is shared by every controller in the process; skip
		// clusters of other products.
		return nil
	}

	if err := reconciler.EnsureDiscoveryConfigMap(ctx, c, e.scheme, trinoCR, trinoCR.Name,
		map[string]string{TrinoDiscoveryKey: product.DiscoveryURI(trinoCR)},
		reconciler.WithDiscoveryProductName("trino"),
	); err != nil {
		return fmt.Errorf("failed to ensure discovery configmap %s/%s: %w", trinoCR.Namespace, trinoCR.Name, err)
	}

	log.FromContext(ctx).V(1).Info("ensured discovery configmap",
		"cluster", trinoCR.Name, "uri", product.DiscoveryURI(trinoCR))
	return nil
}

// OnReconcileError is a no-op.
func (e *DiscoveryExtension) OnReconcileError(_ context.Context, _ client.Client, _ common.ClusterInterface, _ error) error {
	return nil
}
