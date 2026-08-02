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

package v1alpha1

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// ImageSpec defines the container image configuration for a product workload.
// If Custom is set, it takes precedence and the image is used as-is.
// Otherwise, the image reference is constructed as:
//
//	{Repo}/{productName}:{ProductVersion}-kubedoop{KubedoopVersion}
//
// Product operators are expected to set default values for all fields via their webhook.
type ImageSpec struct {
	// Custom is a fully qualified image reference (e.g. "my-registry.com/ns/product:3.4.1").
	// When set, Repo, ProductVersion and KubedoopVersion are ignored.
	// +kubebuilder:validation:Optional
	Custom string `json:"custom,omitempty"`

	// Repo is the image repository (e.g. "quay.io/kubedoop").
	// Used only when Custom is not set.
	// +kubebuilder:validation:Optional
	Repo string `json:"repo,omitempty"`

	// ProductVersion is the version of the product to deploy (e.g. "3.4.1").
	// Used only when Custom is not set.
	// +kubebuilder:validation:Optional
	ProductVersion string `json:"productVersion,omitempty"`

	// KubedoopVersion is the version of the kubedoop operator stack (e.g. "0.2.0").
	// Used only when Custom is not set.
	// +kubebuilder:validation:Optional
	KubedoopVersion string `json:"kubedoopVersion,omitempty"`

	// PullPolicy defines the image pull policy for the container.
	// Defaults to IfNotPresent.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=IfNotPresent
	// +kubebuilder:validation:Enum=Always;Never;IfNotPresent
	PullPolicy corev1.PullPolicy `json:"pullPolicy,omitempty"`
}

// ResolveImage builds the container image reference from this spec, filling every field the user
// left empty from defaults, and reports what it could not resolve.
//
// # Why defaults are a parameter rather than a webhook's job
//
// The kubedoop tag convention is `{repo}/{product}:{productVersion}-kubedoop{kubedoopVersion}`, and
// the natural value of that last part is the OPERATOR's own build version — a reconcile-time fact
// that moves when the operator binary is upgraded. A mutating webhook cannot supply it: webhook
// defaults are persisted into the spec at admission and never recomputed (docs/architecture.md
// §2.6), so a cluster admitted by operator 0.1.0 would keep asking for `-kubedoop0.1.0` images
// forever. Passing defaults here evaluates them on every reconcile, so an operator upgrade moves
// existing clusters onto the co-released product image.
//
// # Resolution order
//
//   - The spec's Custom wins outright — a fully qualified reference is the whole answer.
//   - Otherwise, if the spec states ANY structured field (repo / productVersion / kubedoopVersion),
//     the reference is built from those fields with defaults filling the gaps. `defaults.Custom` is
//     deliberately ignored on this path: a product pinning a custom default must not silently
//     discard a version the user asked for.
//   - Otherwise the user has no opinion, and defaults decide alone — including `defaults.Custom`.
//
// PullPolicy is excluded from "states anything" on purpose: it carries a CRD default, so it is
// filled the moment `image: {}` exists and would make every spec look non-empty.
//
// # Errors
//
// A nil/empty result with a nil error means "nobody expressed an opinion"; the caller falls back to
// whatever image it would have used anyway. A non-nil error means the user (or the product's
// defaults) asked for something that cannot be turned into a reference, and names the missing
// fields — running some other image instead would silently deploy a version nobody requested.
// An empty productName is NOT an error: it means the caller resolves images itself, and only
// Custom can be honored.
func (i *ImageSpec) ResolveImage(productName string, defaults ImageSpec) (string, error) {
	spec := ImageSpec{}
	if i != nil {
		spec = *i
	}

	if spec.Custom != "" {
		return spec.Custom, nil
	}

	stated := spec.Repo != "" || spec.ProductVersion != "" || spec.KubedoopVersion != ""
	if !stated {
		if defaults.Custom != "" {
			return defaults.Custom, nil
		}
	}

	repo := firstNonEmpty(spec.Repo, defaults.Repo)
	productVersion := firstNonEmpty(spec.ProductVersion, defaults.ProductVersion)
	kubedoopVersion := firstNonEmpty(spec.KubedoopVersion, defaults.KubedoopVersion)

	if productName == "" {
		// The caller did not declare a product, so there is no repository path segment to build
		// with. That is the caller's own arrangement (it resolves images itself), not a user error.
		return "", nil
	}
	if repo == "" || productVersion == "" {
		if !stated && defaults.Repo == "" && defaults.ProductVersion == "" {
			// Nothing anywhere. No opinion to honor and nothing to complain about.
			return "", nil
		}
		return "", fmt.Errorf(
			"cannot resolve spec.image for product %q: %s. Set them in spec.image or in the "+
				"handler's ImageDefaults", productName, missingImageFields(repo, productVersion))
	}

	image := repo + "/" + productName + ":" + productVersion
	if kubedoopVersion != "" {
		image += "-kubedoop" + kubedoopVersion
	}
	return image, nil
}

// ResolvedProductVersion returns the product version to publish as app.kubernetes.io/version for
// the image ResolveImage would build from the same inputs.
//
// On the structured path it is the version that actually reaches the container: the user's when
// they stated one, the defaults' otherwise. That second half is the fix — a cluster running the
// operator's default version used to carry no version label at all, because the field the label was
// read from was empty.
//
// With a Custom reference in the SPEC, the user's own productVersion is still published: `custom`
// replaces the image *reference*, and productVersion remains their declaration of which product
// version that reference is. It is empty when they declared none, and empty when the custom
// reference came from `defaults` rather than the spec — a product's default version says nothing
// about an image it did not build.
func (i *ImageSpec) ResolvedProductVersion(defaults ImageSpec) string {
	spec := ImageSpec{}
	if i != nil {
		spec = *i
	}
	if spec.Custom != "" {
		return spec.ProductVersion
	}
	stated := spec.Repo != "" || spec.ProductVersion != "" || spec.KubedoopVersion != ""
	if !stated && defaults.Custom != "" {
		return ""
	}
	return firstNonEmpty(spec.ProductVersion, defaults.ProductVersion)
}

// missingImageFields renders the unresolved half of a reference for an error message.
func missingImageFields(repo, productVersion string) string {
	var missing []string
	if repo == "" {
		missing = append(missing, "repo is unset")
	}
	if productVersion == "" {
		missing = append(missing, "productVersion is unset")
	}
	return strings.Join(missing, " and ")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// GetImage returns the resolved container image reference for the given product name, with no
// defaults layer.
//
// Retained for callers that resolve against a spec a webhook has already filled (the trino example
// validates with it). It is ResolveImage with empty defaults and the error dropped, so the two can
// never disagree; new code should prefer ResolveImage, which reports why it could not resolve
// instead of returning "" and letting the caller run some other image.
func (i *ImageSpec) GetImage(productName string) string {
	image, _ := i.ResolveImage(productName, ImageSpec{})
	return image
}

// GetPullPolicy returns the configured pull policy, defaulting to IfNotPresent.
//
// Nil-safe: `spec.image` is an optional pointer, so a CR that declares no image at all reaches this
// as a nil receiver.
func (i *ImageSpec) GetPullPolicy() corev1.PullPolicy {
	if i == nil || i.PullPolicy == "" {
		return corev1.PullIfNotPresent
	}
	return i.PullPolicy
}

// ResolvedPullPolicy is GetPullPolicy with a defaults layer, matching ResolveImage: the user's
// choice when they made one, the product's otherwise, IfNotPresent if neither.
func (i *ImageSpec) ResolvedPullPolicy(defaults ImageSpec) corev1.PullPolicy {
	if i != nil && i.PullPolicy != "" {
		return i.PullPolicy
	}
	if defaults.PullPolicy != "" {
		return defaults.PullPolicy
	}
	return corev1.PullIfNotPresent
}
