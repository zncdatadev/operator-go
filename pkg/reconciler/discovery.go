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

package reconciler

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/zncdatadev/operator-go/pkg/common"
)

// DiscoveryConfigMapOptions carries the optional metadata for EnsureDiscoveryConfigMap.
// Use the WithDiscovery* option functions to populate it.
type DiscoveryConfigMapOptions struct {
	// ProductName, when set, is published as the "app.kubernetes.io/name" label
	// (e.g. "kafka", "zookeeper", "hdfs").
	ProductName string

	// ExtraLabels are merged into the ConfigMap labels, e.g. the product-owned identity
	// labels (see ClusterLabelKey). The canonical labels the framework owns
	// (app.kubernetes.io/instance, app.kubernetes.io/managed-by, and — when ProductName is
	// set — app.kubernetes.io/name) always win over extra labels with the same key.
	ExtraLabels map[string]string

	// Annotations are merged into the ConfigMap annotations. Unlike labels, annotation keys
	// not listed here are preserved (e.g. kubectl.kubernetes.io/last-applied-configuration),
	// so stale keys from earlier reconciles are not pruned.
	Annotations map[string]string
}

// DiscoveryConfigMapOption customizes EnsureDiscoveryConfigMap.
type DiscoveryConfigMapOption func(*DiscoveryConfigMapOptions)

// WithDiscoveryProductName sets the product name published as the
// "app.kubernetes.io/name" label.
func WithDiscoveryProductName(product string) DiscoveryConfigMapOption {
	return func(o *DiscoveryConfigMapOptions) { o.ProductName = product }
}

// WithDiscoveryExtraLabels merges extra labels into the discovery ConfigMap, e.g. the
// product identity labels (ClusterLabelKey(domain): cluster name).
func WithDiscoveryExtraLabels(labels map[string]string) DiscoveryConfigMapOption {
	return func(o *DiscoveryConfigMapOptions) {
		if o.ExtraLabels == nil {
			o.ExtraLabels = make(map[string]string, len(labels))
		}
		for k, v := range labels {
			o.ExtraLabels[k] = v
		}
	}
}

// WithDiscoveryAnnotations merges annotations into the discovery ConfigMap.
func WithDiscoveryAnnotations(annotations map[string]string) DiscoveryConfigMapOption {
	return func(o *DiscoveryConfigMapOptions) {
		if o.Annotations == nil {
			o.Annotations = make(map[string]string, len(annotations))
		}
		for k, v := range annotations {
			o.Annotations[k] = v
		}
	}
}

// EnsureDiscoveryConfigMap creates or updates a product "discovery ConfigMap": a
// cluster-level ConfigMap (namespaced, written into the CR's namespace), usually named
// after the CR (optionally suffixed, e.g. "<cluster>-nodeport"), that publishes client
// connection info for the product.
//
// The split of responsibilities is deliberately modest. The framework owns the ensure
// semantics only:
//   - idempotent CreateOrUpdate (repeated calls with the same data are no-ops, changed
//     data is updated in place),
//   - a controller owner reference on the CR, so the ConfigMap is garbage-collected with it,
//   - the canonical labels: app.kubernetes.io/instance (the CR name) and
//     app.kubernetes.io/managed-by ("operator-go", matching the role-group resources built
//     by BaseRoleGroupHandler), plus app.kubernetes.io/name when WithDiscoveryProductName
//     is given and any extra labels/annotations from the options.
//
// The PRODUCT owns computing the data map — address aggregation differs per product
// (Kafka aggregates bootstrap Listener ingress addresses into a KAFKA key, ZooKeeper
// renders ZOOKEEPER/ZOOKEEPER_HOSTS/... from pod FQDNs or NodePorts, HDFS renders client
// core-site.xml/hdfs-site.xml) and is passed in as-is. Data is replaced wholesale, so keys
// removed by the product disappear from the ConfigMap.
//
// The ConfigMap lives in the owner's namespace. It lives in pkg/reconciler because this
// package already owns the framework's apply/ensure path and the canonical label helpers
// (ClusterLabelKey and friends), and — unlike pkg/common — may depend on them.
//
// Intended consumers (each currently hand-rolls this in a ClusterExtension PostReconcile):
//   - kafka-operator internal/controller/discovery_extension.go
//   - zookeeper-operator internal/controller/cluster_extension.go (cluster-level discovery)
//     and its ZookeeperZnode per-znode discovery
//   - hdfs-operator discovery extension
func EnsureDiscoveryConfigMap(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	owner common.ClusterInterface,
	name string,
	data map[string]string,
	opts ...DiscoveryConfigMapOption,
) error {
	if name == "" {
		return fmt.Errorf("discovery configmap name must not be empty")
	}
	// SetControllerReference needs a client.Object; every real CR is one, so the runtime
	// object of a ClusterInterface is expected to be too (same contract the reconciler's
	// own apply path relies on).
	runtimeObj := owner.GetRuntimeObject()
	ownerObj, ok := runtimeObj.(client.Object)
	if !ok {
		return fmt.Errorf("discovery configmap owner %q: runtime object %T is not a client.Object", owner.GetName(), runtimeObj)
	}

	options := &DiscoveryConfigMapOptions{}
	for _, opt := range opts {
		opt(options)
	}

	labels := make(map[string]string, len(options.ExtraLabels)+3)
	for k, v := range options.ExtraLabels {
		labels[k] = v
	}
	// Canonical labels are framework-owned and set last, so extras cannot override them:
	// consumers select discovery ConfigMaps by these keys.
	labels["app.kubernetes.io/instance"] = owner.GetName()
	labels["app.kubernetes.io/managed-by"] = "operator-go"
	if options.ProductName != "" {
		labels["app.kubernetes.io/name"] = options.ProductName
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: owner.GetNamespace(),
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, c, cm, func() error {
		cm.Labels = labels
		for k, v := range options.Annotations {
			if cm.Annotations == nil {
				cm.Annotations = make(map[string]string, len(options.Annotations))
			}
			cm.Annotations[k] = v
		}
		cm.Data = data
		return controllerutil.SetControllerReference(ownerObj, cm, scheme)
	})
	if err != nil {
		return fmt.Errorf("failed to ensure discovery configmap %s/%s: %w", owner.GetNamespace(), name, err)
	}
	return nil
}
