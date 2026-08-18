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
	"testing"

	"k8s.io/utils/ptr"

	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/config"
	"github.com/zncdatadev/operator-go/pkg/productlogging"
)

// stageOneBuildContext is a build context in the state buildRoleGroupContext leaves it in during
// stages 1b and 2: the typed config is folded, and MergedConfig has NOT been assigned yet — that
// happens in stage 3.
//
// Three things run in that window and all of them ask whether Vector is enabled:
// resolveVectorAggregatorAddress, buildSidecarManager, and LogFileTarget from a product's
// RoleGroupResolver. A gate reading MergedConfig answers "no" to all three.
func stageOneBuildContext(agentEnabled bool) *RoleGroupBuildContext {
	return &RoleGroupBuildContext{
		RoleName:      "coordinator",
		RoleGroupName: "default",
		RoleGroupSpec: v1alpha1.RoleGroupSpec{
			Config: &v1alpha1.RoleGroupConfigSpec{
				Logging: &v1alpha1.LoggingSpec{EnableVectorAgent: ptr.To(agentEnabled)},
			},
		},
		Declaration: RoleDeclaration{
			LogProducers: []productlogging.ContainerLogging{
				{Container: "trino", Framework: productlogging.LoggingFrameworkLogback},
			},
		},
		MergedConfig: nil, // stage 3 has not run
	}
}

// The enablement gate must answer from the FOLD, which exists from stage 1, not from
// MergedConfig.Logging, which is a copy assigned to it in stage 3.
//
// Reading the copy meant resolveVectorAggregatorAddress returned at its first line on every call,
// so VectorAggregatorAddress was never set and the framework never wrote vector.yaml — while
// buildSidecarManager, which reads the fold, still registered the Vector sidecar. Its Validate then
// failed on the missing key and every role group with the agent enabled stayed Degraded, with no
// StatefulSet ever applied.
func TestVectorEnabledReadsTheFoldBeforeTheMerge(t *testing.T) {
	if !vectorEnabledFor(stageOneBuildContext(true)) {
		t.Fatal("agent enabled in the folded config but the gate said no before stage 3: " +
			"the aggregator address is never resolved and vector.yaml is never written")
	}
	if vectorEnabledFor(stageOneBuildContext(false)) {
		t.Error("agent disabled in the folded config but the gate said yes")
	}
}

// LogFileTarget is the API a product calls from its RoleGroupResolver — stage 2, still before the
// merge — to learn where its own logging config should write. Answering "" there is not a harmless
// default: it means console-only, so the product emits a config with no file appender while the
// Vector sidecar is mounted and collecting nothing, and every signal stays green.
func TestLogFileTargetIsAnsweredBeforeTheMerge(t *testing.T) {
	decl := productlogging.ContainerLogging{
		Container: "trino",
		Framework: productlogging.LoggingFrameworkLogback,
	}

	got := stageOneBuildContext(true).LogFileTarget(decl)
	if got == "" {
		t.Fatal(`LogFileTarget returned "" (console only) at stage 2 while the agent is enabled`)
	}
	if want := productlogging.LogDirFor(decl); len(got) <= len(want) || got[:len(want)] != want {
		t.Errorf("LogFileTarget = %q, want a path under %q", got, want)
	}

	if off := stageOneBuildContext(false).LogFileTarget(decl); off != "" {
		t.Errorf(`agent disabled: LogFileTarget = %q, want "" (console only)`, off)
	}
}

// ContainerLogging is the other accessor a product calls from its RoleGroupResolver, and it had the
// same stage-2 nil: it read MergedConfig, so a product rendering its own logging config saw no
// per-container settings at all and silently emitted defaults with the user's levels dropped.
func TestContainerLoggingIsAnsweredBeforeTheMerge(t *testing.T) {
	buildCtx := stageOneBuildContext(true)
	buildCtx.RoleGroupSpec.Config.Logging.Containers = map[string]v1alpha1.LoggingConfigSpec{
		"trino": {Console: &v1alpha1.LogLevelSpec{Level: "DEBUG"}},
	}

	got := buildCtx.ContainerLogging("trino")
	if got == nil {
		t.Fatal("ContainerLogging returned nil at stage 2: the user's levels are silently dropped")
	}
	if got.Console == nil || got.Console.Level != "DEBUG" {
		t.Errorf("ContainerLogging = %+v, want the folded console level DEBUG", got)
	}
	if buildCtx.ContainerLogging("not-a-container") != nil {
		t.Error("an undeclared container must resolve to nil")
	}
}

// A context a product assembled by hand carries no folded spec. Such a caller sets
// MergedConfig.Logging, and must keep the behaviour it already had.
func TestVectorEnabledFallsBackForAHandBuiltContext(t *testing.T) {
	buildCtx := &RoleGroupBuildContext{
		MergedConfig: &config.MergedConfig{
			Logging: &v1alpha1.LoggingSpec{EnableVectorAgent: ptr.To(true)},
		},
	}
	if !vectorEnabledFor(buildCtx) {
		t.Error("a hand-built context supplying only MergedConfig.Logging lost its enablement")
	}
	if vectorEnabledFor(&RoleGroupBuildContext{}) {
		t.Error("an empty context must not report the agent enabled")
	}
	if vectorEnabledFor(nil) {
		t.Error("a nil context must not report the agent enabled")
	}
}
