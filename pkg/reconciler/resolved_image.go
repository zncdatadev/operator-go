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
	corev1 "k8s.io/api/core/v1"

	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
)

// ResolvedImage is everything that follows from a role's image, resolved ONCE per role group and
// published on the build context.
//
// It is one struct rather than four lookups because the four have to agree and used to be computed
// independently, from handler state, at four different call sites: the primary container's image,
// the sidecars' image (a Vector agent ships inside the product image, so it must be the SAME
// reference), the pod's imagePullSecrets, and the app.kubernetes.io/version label.
//
// The pull secret is why this is a struct and not a string. It is a property of WHERE the image
// lives, not of the image reference, so it applies on all three resolution paths — the assembled
// reference, spec.image.custom, and a product that resolves its own images. Ten product CRDs
// declared pullSecretName and, when they migrated onto the commons ImageSpec, every one of them
// silently stopped applying it: the CRD still accepted the value and no pod ever carried the entry,
// so a private-registry install failed with ImagePullBackOff and nothing named the cause. Carrying
// it beside the reference is what stops that from being re-derivable-but-forgotten.
type ResolvedImage struct {
	// Reference is the container image. Empty means neither the CR nor the product expressed an
	// opinion — with ImageResolution.ProductName unset, nothing beyond spec.image.custom is read,
	// so a CR stating only spec.image.repo resolves to "".
	//
	// There is no fallback left to take: the handler's static image is gone, so an empty reference
	// reaches BuildResources, which fails the role group with a *ValidationError naming the three
	// knobs that could set it. Saying "the caller falls back" described the pre-refactor shape and
	// pointed a reader at a mechanism that no longer exists. The one legitimate way to leave this
	// empty is to supply the image through podOverrides, which is merged onto the container later.
	Reference string

	// PullPolicy is the effective imagePullPolicy.
	PullPolicy corev1.PullPolicy

	// PullSecretName names a docker-registry Secret added to every pod's imagePullSecrets. Empty
	// means the deployment needs no registry credential, which is the common case.
	PullSecretName string

	// ProductVersion is the resolved product version, which becomes app.kubernetes.io/version. It
	// follows the RESOLVED image rather than the CR, so it is present whenever the version came
	// from the operator's own defaults too.
	ProductVersion string
}

// ImageResolution is the reconcile-invariant half of image resolution: the product's name and the
// defaults its operator ships. It moves off the handler and onto the reconciler config because the
// reconciler is what resolves the image now — once per role group, before anything is built, so the
// container and the sidecars cannot be told different things.
type ImageResolution struct {
	// ProductName is the repo path segment and the app.kubernetes.io/name label. Empty means the
	// product resolves its own images, and only spec.image.custom is honoured.
	ProductName string

	// Defaults fill in whatever spec.image leaves empty, per field, user first.
	//
	// They are read EVERY reconcile, which is the whole point and is what a webhook cannot do:
	// webhook defaults are persisted at admission and never recomputed, so a cluster admitted by
	// one operator version would keep asking for that version's images forever. Evaluated here, an
	// operator upgrade moves existing clusters onto the co-released product image.
	Defaults v1alpha1.ImageSpec
}

// resolveImage folds, in decreasing precedence: the CR's spec.image, the role's declared Image, and
// the operator's ImageDefaults — then answers all four questions at once.
//
// The CR outranks the declaration deliberately. A product pinning one role to a different image
// must not silently beat a user who pinned spec.image.custom to an air-gapped mirror; that is the
// same "a product layer cannot beat a user-stated value" rule the config fold enforces.
//
// An unresolvable image is an ERROR rather than a fallback. Falling back means silently deploying a
// version nobody asked for, and the API server does not validate container.image at all — an
// unparsable reference is accepted, stored, and surfaces only as InvalidImageName on a pod while
// the reconcile reports success.
func resolveImage(
	clusterSpec *v1alpha1.GenericClusterSpec, decl RoleDeclaration, res ImageResolution,
) (ResolvedImage, error) {
	var crImage *v1alpha1.ImageSpec
	if clusterSpec != nil {
		crImage = clusterSpec.Image
	}

	// The role's declaration sits between the CR and the operator's defaults: it is a default the
	// CR overrides, and it overrides the operator-wide one.
	defaults := res.Defaults
	if decl.Image != nil {
		defaults = foldImageSpec(*decl.Image, defaults)
	}

	reference, err := crImage.ResolveImage(res.ProductName, defaults)
	if err != nil {
		return ResolvedImage{}, err
	}

	out := ResolvedImage{
		Reference:      reference,
		PullPolicy:     crImage.ResolvedPullPolicy(defaults),
		PullSecretName: crImage.ResolvedPullSecretName(defaults),
	}

	// ProductVersion is published only when the framework actually assembled the reference. With an
	// empty ProductName the product resolves its own images, so a version read from spec.image
	// describes an image the framework did not build — a guess, stamped on every resource as
	// app.kubernetes.io/version. A spec.image.custom WITH a productVersion still publishes it:
	// `custom` replaces the reference, and that field remains the user's declaration of which
	// product version it is.
	if res.ProductName != "" {
		out.ProductVersion = crImage.ResolvedProductVersion(defaults)
	}
	return out, nil
}

// foldImageSpec folds `over` beneath `spec`, per field: a field the upper layer states wins, and
// the rest are inherited. Same rule the config fold uses, for the same reason — replacing an image
// spec wholesale would drop the sibling fields the upper layer did not restate.
func foldImageSpec(spec, over v1alpha1.ImageSpec) v1alpha1.ImageSpec {
	out := over
	if spec.Custom != "" {
		out.Custom = spec.Custom
	}
	if spec.Repo != "" {
		out.Repo = spec.Repo
	}
	if spec.ProductVersion != "" {
		out.ProductVersion = spec.ProductVersion
	}
	if spec.KubedoopVersion != "" {
		out.KubedoopVersion = spec.KubedoopVersion
	}
	if spec.PullPolicy != "" {
		out.PullPolicy = spec.PullPolicy
	}
	if spec.PullSecretName != "" {
		out.PullSecretName = spec.PullSecretName
	}
	return out
}
