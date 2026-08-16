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
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/common"
	"github.com/zncdatadev/operator-go/pkg/listener"
	"github.com/zncdatadev/operator-go/pkg/productlogging"
)

// RoleDeclaration is everything a product knows about ONE role.
//
// Almost none of it has a user layer above it: a role's ports, its primary container's name and
// command, whether it has a data PVC, which of its containers produce logs — the CRD offers the
// user no way to state any of these, so nothing merges them. ConfigDefaults is the exception, and
// is folded BENEATH the user's two levels.
//
// It replaces seven role-keyed maps on the handler, their handler-global fallbacks, and four
// per-call fields on the build context that outranked them. Those existed only because the
// declaration was shredded across three objects, each with its own precedence rule — and the rule
// for one of them dropped sibling fields, which is the defect that started this. One struct,
// produced per reconcile with the cr in hand, removes the precedence question rather than fixing
// it.
//
// Being produced per reconcile is also what retires the per-call fields: a port that moves because
// the CR enabled TLS is computed HERE, from THIS cr, instead of being assigned into process-wide
// handler state that the next cluster inherits.
type RoleDeclaration struct {
	// Image is this role's image spec, folded per field UNDER the CR's own spec.image and OVER
	// GenericReconcilerConfig.ImageDefaults. Nil is the common case: the role runs the cluster's
	// image.
	//
	// The CR still outranks it. A product pinning a role to a different image must not silently
	// beat a user who pinned spec.image.custom to an air-gapped mirror.
	Image *v1alpha1.ImageSpec

	// ContainerPorts are the primary container's ports.
	//
	// Ports[0] backs the generated TCP readiness probe, so the first entry is part of the
	// contract rather than decoration: put the port that means "this pod can serve" first. The
	// framework generates no liveness probe — it knows neither which port means healthy nor how
	// long the product takes to open it, and a liveness probe on a wrong guess kills the container
	// on a timer forever.
	ContainerPorts []corev1.ContainerPort

	// ServicePorts are the client-facing Service's ports. Empty means the role gets no
	// client-facing Service — the headless one is always built.
	ServicePorts []corev1.ServicePort

	// MainContainerName renames the primary container. Empty uses the role group's resource name.
	//
	// It is significant when it must match a per-container logging key
	// (logging.containers.<name>), which is why products whose container name differs per role
	// (namenode / datanode / journalnode) set it here rather than globally.
	MainContainerName string

	// Command replaces the primary container's entrypoint.
	//
	// This is a DECLARATION, not an override: the CRD gives the user no `command`, so nothing
	// merges and nothing is beaten. Arguments are deliberately absent — they reach the container
	// through cliOverrides, which the user CAN state, so a product that appended args here would
	// silently delete what the user wrote.
	Command []string

	// Lifecycle is the primary container's lifecycle hooks. Same reasoning as Command: no user
	// layer exists, so declaring it beats nobody.
	Lifecycle *corev1.Lifecycle

	// ReadinessProbe, LivenessProbe and StartupProbe replace the framework's defaults on the
	// primary container. A nil ReadinessProbe keeps the generated TCP probe on ContainerPorts[0];
	// nil Liveness and Startup mean none, which is the framework's deliberate position rather than
	// an omission (see ContainerPorts).
	ReadinessProbe *corev1.Probe
	LivenessProbe  *corev1.Probe
	StartupProbe   *corev1.Probe

	// DataVolume opts the role into a data PVC built from the effective
	// `config.resources.storage` and mounted at MountPath. Nil means the role has no data volume,
	// which is a structural property of the role rather than a consequence of what the user wrote
	// — a product with one stateful and one stateless role could previously only choose between
	// "every role" and "none".
	DataVolume *DataVolume

	// ListenerClass selects the client-facing Service's type. Empty leaves the builder's default
	// (ClusterIP).
	//
	// A product whose listener class is USER-settable — one that put a listenerClass field in its
	// own config block — must not set this: that value arrives through the config fold and is read
	// off the resolved config, because a declaration is fixed before the fold runs and would beat
	// the user.
	ListenerClass listener.ListenerClass

	// PublishNotReadyAddresses sets the flag on the headless Service so peers resolve each other's
	// DNS before readiness. Quorum systems require it: a ZooKeeper member is not Ready until it
	// sees a quorum that cannot form until its peers resolve.
	PublishNotReadyAddresses bool

	// LogProducers declares which containers produce logs, and how each one's logging config file
	// is rendered from the merged CRD logging spec into the role group ConfigMap.
	//
	// The same list names the producers of the Vector log pipeline: the Vector provider creates the
	// shared log emptyDir, RW-mounts it on each producer, and pre-creates each producer's log
	// directory. A declaration naming no container in the assembled pod is a build failure — it
	// used to be skipped in silence, producing a pod whose config points into a directory nothing
	// mounts, where the appender writes into the container filesystem, Vector collects nothing, and
	// every signal reports healthy.
	LogProducers []productlogging.ContainerLogging

	// OwnsVectorConfig declares that the product writes vector.yaml itself, rather than the
	// framework rendering it from the CR's aggregator ConfigMap.
	//
	// It replaces only ONE branch of the "does vector.yaml have a source" gate. A CR that
	// implements VectorAggregatorProvider still satisfies the other, so leaving this false does not
	// switch the sidecar off for an aggregator-based product.
	OwnsVectorConfig bool

	// LogVolumeSize overrides the SizeLimit of the shared log emptyDir the Vector provider owns.
	// Empty uses vector.DefaultLogVolumeSize.
	//
	// It is per-role because log volume is: a DataNode writes far more than a JournalNode, and the
	// producers it sizes are already declared per role. An unparsable value fails the role rather
	// than being logged and ignored — silently reverting to the 33Mi default is how a chatty pod
	// gets evicted by the kubelet with nothing naming the operator.
	LogVolumeSize string

	// Env are environment variables on the primary container that the product declares rather than
	// computes — a downward-API POD_NAME, a secretKeyRef.
	//
	// It exists because the merged-override channel is map[string]string and cannot carry a
	// `valueFrom` at all; a value the product COMPUTES belongs in Contribution.EnvVars, which folds
	// per key.
	//
	// These are emitted BENEATH the merged overrides: Kubernetes resolves a duplicate env name to
	// the last entry, so a user's envOverrides of the same name wins. That ordering is the
	// precedence, and it is the reason this is a declaration rather than a post-build edit — the
	// deleted container callback ran after the merged env was already written into the container,
	// so a product setting env there silently deleted what the user wrote.
	Env []corev1.EnvVar

	// ConfigDefaults is the product's default for the FRAMEWORK-owned half of this role's config
	// block — resources, affinity, gracefulShutdownTimeout, logging — folded BENEATH the CR's role
	// and role group levels by FoldCommonConfig. Anything the user states anywhere wins.
	//
	// It is the one field here with a user layer above it, and the only place in this struct where
	// precedence is a question at all. The product's defaults for its OWN config fields are not
	// here: those are the product's half, folded by the product with FoldProductConfig.
	//
	// An anti-affinity default belongs here and could not live on the old handler, because its
	// selector names the CLUSTER and the handler is a process-wide singleton shared by every
	// cluster the operator serves.
	ConfigDefaults *v1alpha1.RoleGroupConfigSpec

	// Optional marks a role a CR need not declare. The zero value means a CR that omits this role
	// is a configuration error.
	Optional bool
}

// DataVolume describes a role's data PVC.
type DataVolume struct {
	// Name is the volume and claim-template name. Empty uses DefaultDataVolumeName.
	Name string
	// MountPath is where it is mounted in the primary container. Required.
	MountPath string
}

// DefaultDataVolumeName is the claim-template name a DataVolume gets when it names none. It is
// defined by the builder, which owns the reserved-name rule.
const DefaultDataVolumeName = builder.DefaultDataVolumeName

// RoleCatalog is a product's complete statement about the roles it supports, for ONE cluster. The
// key set IS the set of role names that cluster may declare.
type RoleCatalog map[string]RoleDeclaration

// RoleProvider is the seam a product implements to declare its roles. It is called ONCE per
// reconcile pass, after the dependency check and before any role is reconciled.
//
// Once per reconcile rather than once per role group: a product resolving a cluster-wide fact —
// an authentication class that decides whether every role speaks TLS, an S3 connection — does that
// lookup here and pays for it once, instead of N times for N role groups.
//
// It takes the cr, and that is what lets the declaration be static data: every input that used to
// need a per-call escape hatch (a TLS-dependent port, an affinity selector naming the cluster) is
// simply in scope. Returning a *common.RequeueAfterError reports "not ready yet" without the
// cluster going Degraded; any other error fails the pass.
type RoleProvider[CR common.ClusterInterface] interface {
	DeclareRoles(ctx context.Context, c client.Client, cr CR) (RoleCatalog, error)
}

// RoleProviderFunc adapts a plain function to RoleProvider.
type RoleProviderFunc[CR common.ClusterInterface] func(
	ctx context.Context, c client.Client, cr CR) (RoleCatalog, error)

// DeclareRoles implements RoleProvider.
func (f RoleProviderFunc[CR]) DeclareRoles(
	ctx context.Context, c client.Client, cr CR) (RoleCatalog, error) {
	return f(ctx, c, cr)
}

// ValidateCatalog checks a catalog against the roles a CR declares, and returns the names a
// product configured but the CR does not use.
//
// The two directions are not symmetric, deliberately:
//
//   - a role in spec.roles with NO declaration is a hard error. The framework has no ports, no
//     container name and no logging for it, so it would build a portless, Service-less workload
//     and report success — the failure a typo in a role name used to produce.
//   - a declaration the CR does not use is a WARNING, returned for the caller to emit as an event.
//     A product may legitimately declare optional roles, and Optional silences it entirely.
//
// This replaces ConfiguredRoleNames(), a seven-way union of map keys that existed only because the
// declaration was shredded across seven maps. A catalog's key set is the same information without
// the bookkeeping.
func ValidateCatalog(catalog RoleCatalog, specRoles map[string]v1alpha1.RoleSpec) (unused []string, err error) {
	var missing []string
	for name := range specRoles {
		if _, ok := catalog[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		known := make([]string, 0, len(catalog))
		for name := range catalog {
			known = append(known, name)
		}
		slices.Sort(known)
		return nil, fmt.Errorf(
			"spec.roles declares %s, which this product does not support; known roles are %s",
			strings.Join(missing, ", "), strings.Join(known, ", "))
	}

	for name, decl := range catalog {
		if decl.Optional {
			continue
		}
		if _, ok := specRoles[name]; !ok {
			unused = append(unused, name)
		}
	}
	slices.Sort(unused)
	return unused, nil
}

// Validate reports whether a declaration is internally coherent. It runs before any resource is
// applied, so a product's mistake fails where someone can read it rather than as a rejected
// StatefulSet quoting a field the user never wrote.
func (d RoleDeclaration) Validate(roleName string) error {
	var problems []string

	if d.DataVolume != nil && d.DataVolume.MountPath == "" {
		problems = append(problems, "dataVolume declares no mountPath")
	}
	if len(d.ServicePorts) > 0 && len(d.ContainerPorts) == 0 {
		problems = append(problems,
			"servicePorts are declared but containerPorts are empty, so the Service would target nothing")
	}
	if d.ConfigDefaults != nil {
		if _, err := DecodeAffinity(d.ConfigDefaults.Affinity); err != nil {
			problems = append(problems, fmt.Sprintf("configDefaults.affinity does not decode: %v", err))
		}
	}
	for i, p := range d.ContainerPorts {
		if p.Name == "" {
			problems = append(problems, fmt.Sprintf("containerPorts[%d] has no name", i))
		}
	}
	// Checked HERE, once per pass, rather than where it is consumed: the consumer could only log
	// and fall back to the 33Mi default, and that is the failure this field exists to prevent — the
	// emptyDir fills, the kubelet evicts the pod, and nothing names the operator.
	if d.LogVolumeSize != "" {
		if _, err := resource.ParseQuantity(d.LogVolumeSize); err != nil {
			problems = append(problems, fmt.Sprintf("logVolumeSize %q is not a quantity: %v",
				d.LogVolumeSize, err))
		}
	}
	if err := productlogging.ValidateProducers(d.LogProducers); err != nil {
		problems = append(problems, err.Error())
	}

	if len(problems) > 0 {
		return fmt.Errorf("role %q declaration: %s", roleName, strings.Join(problems, "; "))
	}
	return nil
}
