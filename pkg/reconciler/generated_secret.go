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
	"maps"
	"sort"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/zncdatadev/operator-go/pkg/constant"
)

// GeneratedSecretOptions carries the optional metadata for EnsureGeneratedSecret.
// Use the WithGeneratedSecret* option functions to populate it.
type GeneratedSecretOptions struct {
	// ProductName, when set, is published as the "app.kubernetes.io/name" label.
	ProductName string

	// ExtraLabels are merged into the Secret's labels. The canonical labels the framework owns
	// (app.kubernetes.io/instance, app.kubernetes.io/managed-by, and — when ProductName is set —
	// app.kubernetes.io/name) always win over extra labels with the same key.
	ExtraLabels map[string]string

	// Annotations are merged into the Secret's annotations. Keys not listed here are preserved.
	Annotations map[string]string

	// Type overrides the Secret type. Defaults to corev1.SecretTypeOpaque. It is only applied
	// when the Secret is created: `type` is immutable, so setting it on an existing Secret would
	// make every reconcile fail.
	Type corev1.SecretType
}

// GeneratedSecretOption customizes EnsureGeneratedSecret.
type GeneratedSecretOption func(*GeneratedSecretOptions)

// WithGeneratedSecretProductName sets the product name published as the
// "app.kubernetes.io/name" label.
func WithGeneratedSecretProductName(product string) GeneratedSecretOption {
	return func(o *GeneratedSecretOptions) { o.ProductName = product }
}

// WithGeneratedSecretExtraLabels merges extra labels into the Secret.
func WithGeneratedSecretExtraLabels(labels map[string]string) GeneratedSecretOption {
	return func(o *GeneratedSecretOptions) { o.ExtraLabels = labels }
}

// WithGeneratedSecretAnnotations merges annotations into the Secret.
func WithGeneratedSecretAnnotations(annotations map[string]string) GeneratedSecretOption {
	return func(o *GeneratedSecretOptions) { o.Annotations = annotations }
}

// WithGeneratedSecretType sets the Secret type, applied only at creation.
func WithGeneratedSecretType(t corev1.SecretType) GeneratedSecretOption {
	return func(o *GeneratedSecretOptions) { o.Type = t }
}

// EnsureGeneratedSecret creates a Secret whose values are generated once and then never change.
//
// This is the counterpart to EnsureDiscoveryConfigMap for the case where the framework needs an
// object whose CONTENT must not converge. Everything else the SDK applies is idempotent against a
// desired state: the handler rebuilds it every reconcile and the apply path overwrites the live
// object. A generated secret is the exact opposite — rewriting it is the failure. The oauth2-proxy
// session cookie key is the shipped example: a fresh value on every pass rolls the pods and logs
// every user out, which is why sidecar.GenerateCookieSecret's doc says to call it once and store
// the result, and why RoleGroupResources.ExtraResources cannot serve here.
//
// Semantics, in the order they matter:
//
//   - **An existing value is never rewritten.** That is the whole point.
//   - A MISSING key is filled, whether the Secret was just created or has lost a key since. The
//     alternative is worse: sidecar providers fail the reconcile on a missing key
//     (OAuth2ProxySidecarProvider.Validate does), so a Secret that lost one key — a partial
//     restore from backup, a hand-edit — would wedge the cluster with no recovery short of
//     deleting the whole Secret, which rotates every OTHER key too and logs out every user to fix
//     one. Filling only what is absent keeps the blast radius at the key that was actually lost.
//   - A controller owner reference is set, so the Secret is garbage-collected with the CR.
//   - The write is skipped entirely when nothing is missing and the metadata already matches, so
//     a steady-state cluster costs one read per reconcile.
//   - `IsAlreadyExists` from a concurrent reconcile is not an error: the object is re-read and the
//     pass continues against whichever value won, which is exactly the intended outcome — one
//     generated value, whoever generated it.
//
// Generators run only for keys that are absent, so a generator with a side effect (a call to an
// external KMS) is not invoked on the steady-state path.
//
// Call it where a ctx and a client exist and before the workload is built — a
// common.ClusterExtension PreReconcile hook is the natural home, mirroring how discovery
// ConfigMaps are published from PostReconcile:
//
//	reconciler.EnsureGeneratedSecret(ctx, c, scheme, cr, cr.GetName()+"-oauth2-cookie",
//	    map[string]func() (string, error){"cookie-secret": sidecar.GenerateCookieSecret},
//	    reconciler.WithGeneratedSecretProductName("trino"),
//	)
//
// The Secret is deliberately NOT created from the sidecar provider's Validate: a validation hook
// that creates objects is a side effect in the one step whose job is to have none.
func EnsureGeneratedSecret(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	owner client.Object,
	name string,
	generators map[string]func() (string, error),
	opts ...GeneratedSecretOption,
) (*corev1.Secret, error) {
	if owner == nil {
		return nil, fmt.Errorf("generated secret %q: owner must not be nil", name)
	}
	if name == "" {
		return nil, fmt.Errorf("generated secret: name must not be empty")
	}
	if len(generators) == 0 {
		return nil, fmt.Errorf("generated secret %q: at least one key generator is required", name)
	}

	options := &GeneratedSecretOptions{}
	for _, opt := range opts {
		opt(options)
	}

	labels := map[string]string{}
	maps.Copy(labels, options.ExtraLabels)
	labels[constant.LabelKubernetesInstance] = owner.GetName()
	labels[constant.LabelKubernetesManagedBy] = managedByValue
	if options.ProductName != "" {
		labels[constant.LabelKubernetesName] = options.ProductName
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: owner.GetNamespace(),
		},
	}

	var generateErr error
	_, err := controllerutil.CreateOrUpdate(ctx, c, secret, func() error {
		// Runs against the LIVE object on update, so secret.Data already holds whatever exists.
		// Only absent keys are generated; present ones are left untouched.
		for _, key := range sortedKeys(generators) {
			if _, exists := secret.Data[key]; exists {
				continue
			}
			value, err := generators[key]()
			if err != nil {
				// Reported through the closure rather than returned from it: CreateOrUpdate would
				// otherwise wrap it, and the caller wants to know which key failed.
				generateErr = fmt.Errorf("generated secret %q: generating key %q: %w", name, key, err)
				return generateErr
			}
			if secret.Data == nil {
				secret.Data = map[string][]byte{}
			}
			secret.Data[key] = []byte(value)
		}

		secret.Labels = labels
		for k, v := range options.Annotations {
			if secret.Annotations == nil {
				secret.Annotations = make(map[string]string, len(options.Annotations))
			}
			secret.Annotations[k] = v
		}
		// Type is immutable; setting it on an existing Secret makes every later reconcile fail.
		if secret.CreationTimestamp.IsZero() {
			secret.Type = options.Type
			if secret.Type == "" {
				secret.Type = corev1.SecretTypeOpaque
			}
		}
		return controllerutil.SetControllerReference(owner, secret, scheme)
	})

	if generateErr != nil {
		return nil, generateErr
	}
	if err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return nil, fmt.Errorf("ensuring generated secret %q: %w", name, err)
		}
		// A concurrent reconcile created it between our Get and Create. Its value is as good as
		// ours would have been, and re-reading is what makes "generated once" hold across
		// reconcilers rather than only within one.
		if err := c.Get(ctx, client.ObjectKeyFromObject(secret), secret); err != nil {
			return nil, fmt.Errorf("re-reading generated secret %q after a concurrent create: %w", name, err)
		}
	}
	return secret, nil
}

// sortedKeys returns the generator keys in a stable order, so a failure names the same key on
// every run instead of whichever one map iteration reached first.
func sortedKeys(generators map[string]func() (string, error)) []string {
	keys := make([]string, 0, len(generators))
	for k := range generators {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
