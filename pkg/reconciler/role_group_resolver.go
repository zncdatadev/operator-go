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
	"encoding/json"
	"fmt"
	"maps"

	corev1 "k8s.io/api/core/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/common"
	"github.com/zncdatadev/operator-go/pkg/listener"
)

// Contribution is what a product DERIVES from a role group's effective config: the config-file
// content and environment that follow from values the user may have changed.
//
// It is folded BENEATH the user's own configOverrides and envOverrides, per key, so a derived value
// is a default the user can still refine — never a decision made over their head.
//
// This is the seam that did not exist. A JVM heap sized from the effective memory limit was written
// by hand three times with the same 0.8 factor, and a fourth product gave up and froze the result
// into a literal `-Xmx419430k` — 0.8 of one role's default memory, applied to every JVM component
// including one whose own default is twice that, and immune to every user override. The framework
// made that inevitable: the effective config was not computed until after the role group's
// ConfigMap had already been built, so nothing derived from it could reach a config file at all.
type Contribution struct {
	// ConfigOverrides is per-file, per-key config content, folded beneath the user's per key.
	ConfigOverrides map[string]map[string]string

	// EnvVars are environment variables, folded beneath the user's envOverrides per key.
	EnvVars map[string]string

	// PodOverrides is a product-supplied pod template layer — a default toleration, a nodeSelector,
	// an extra volume — folded beneath the user's own podOverrides through the same strategic merge
	// patch, so the user still has the last word.
	//
	// It is also the only channel that can carry a container env var with a `valueFrom`: the
	// override maps are map[string]string and cannot express a downward-API reference or a
	// secretKeyRef at all.
	PodOverrides *corev1.PodTemplateSpec

	// ListenerClass overrides RoleDeclaration.ListenerClass for THIS role group, selecting the
	// client-facing Service's type.
	//
	// It exists because a product may expose the listener class as a user-settable field in its own
	// config block, and such a value arrives from the config fold PER ROLE GROUP — after the
	// declaration, which is fixed once per pass, has already been produced. This stage is the only
	// one that runs after the fold and before anything is built, so it is the only place that value
	// can be applied without the product patching the built Service afterwards, which lands after
	// podOverrides and beats the user.
	ListenerClass listener.ListenerClass
}

// There is deliberately no CLI-argument dimension here.
//
// cliOverrides merge by REPLACEMENT, not per key — a non-empty user slice replaces the layer
// beneath it wholesale — so a derived argument list would vanish the moment a user added one
// unrelated flag. That is not a default the user is refining; it is a computed prerequisite they
// have no way to reconstruct, silently deleted.
//
// Every derivation surveyed across twelve operators produces environment or config-file content
// except one, and the products' own convention already covers it: a derived JVM setting travels in
// an env var the start script reads (ZK_SERVER_HEAP, KAFKA_HEAP_OPTS, HADOOP_OPTS). A product that
// genuinely needs a fixed argument states it in RoleDeclaration.Command, which has no user layer.

// RoleGroupResolver is the seam a product implements to derive values from a role group's EFFECTIVE
// config — after the product's own defaults, the CR's role level and its role group level have been
// folded into one answer, and before anything is built.
//
// It is called once per role group per reconcile, inside the framework's best-effort role loop, so
// a failure is isolated to its own role group rather than failing the cluster.
//
// It must be DETERMINISTIC for a given (cr, effective config). Its output lands in the role group's
// ConfigMap, the framework owns that ConfigMap and applies it with CreateOrUpdate, and the
// reconciler watches what it owns — so a value that varies between passes rewrites the ConfigMap
// every pass, wakes the reconciler through its own watch, and never errors, which means the
// workqueue never backs the loop off. Put a timestamp or a random suffix here and the cluster
// rewrites itself forever.
type RoleGroupResolver[CR common.ClusterInterface] interface {
	ResolveRoleGroup(ctx context.Context, c client.Client, cr CR,
		rg *RoleGroupBuildContext) (*Contribution, error)
}

// RoleGroupResolverFunc adapts a plain function to RoleGroupResolver.
type RoleGroupResolverFunc[CR common.ClusterInterface] func(
	ctx context.Context, c client.Client, cr CR, rg *RoleGroupBuildContext) (*Contribution, error)

// ResolveRoleGroup implements RoleGroupResolver.
func (f RoleGroupResolverFunc[CR]) ResolveRoleGroup(ctx context.Context, c client.Client, cr CR,
	rg *RoleGroupBuildContext) (*Contribution, error) {
	return f(ctx, c, cr, rg)
}

// overrides renders a Contribution as the overrides layer the config merger folds. A nil
// Contribution contributes nothing, so a resolver with nothing to say for one role group returns
// nil rather than an empty struct.
//
// The pod template is marshalled because the CRD field is a schema-free RawExtension — the same
// shape a user writes — so the product's layer and the user's go through one strategic merge patch
// rather than two mechanisms that could disagree about precedence.
func (c *Contribution) overrides() (*v1alpha1.OverridesSpec, error) {
	if c == nil {
		return nil, nil
	}
	out := &v1alpha1.OverridesSpec{}
	if len(c.ConfigOverrides) > 0 {
		out.ConfigOverrides = make(map[string]map[string]string, len(c.ConfigOverrides))
		for file, keys := range c.ConfigOverrides {
			out.ConfigOverrides[file] = maps.Clone(keys)
		}
	}
	if len(c.EnvVars) > 0 {
		out.EnvOverrides = maps.Clone(c.EnvVars)
	}
	if c.PodOverrides != nil {
		raw, err := json.Marshal(c.PodOverrides)
		if err != nil {
			return nil, fmt.Errorf("encoding the derived podOverrides: %w", err)
		}
		out.PodOverrides = &k8sruntime.RawExtension{Raw: raw}
	}
	return out, nil
}

// HeapMB returns floor(limit * factor) in MiB from a role group's effective memory limit, and
// whether a limit was set at all.
//
// Three operators compute exactly this, all with factor 0.8, all slightly differently; a fourth
// froze the answer into a literal. It is here so the arithmetic — and the "no limit means no
// opinion" case, which the frozen literal got wrong — has one implementation.
func HeapMB(cfg *v1alpha1.RoleGroupConfigSpec, factor float64) (int64, bool) {
	if cfg == nil || cfg.Resources == nil || cfg.Resources.Memory == nil ||
		cfg.Resources.Memory.Limit == nil {
		return 0, false
	}
	limit := cfg.Resources.Memory.Limit.Value()
	if limit <= 0 {
		return 0, false
	}
	return int64(float64(limit/(1024*1024)) * factor), true
}
