package controller

import (
	"context"
	"slices"
	"strings"

	"github.com/zncdatadev/operator-go/internal/framework/pipeline"
	"github.com/zncdatadev/operator-go/pkg/framework"
	"github.com/zncdatadev/operator-go/pkg/framework/input"
)

// Runtime declarations decide this dependency: neither disabled groups nor
// groups that produce no log files require the central destination. Generation
// runs exactly once before these reads; assembly remains a pure later stage.
func (r *Reconciler[CR, C, S, F]) resolvePlatformInputs(ctx context.Context, prepared pipeline.PreparedInputs[C, S, F],
	generated []pipeline.GeneratedGroup[C, S, F], options framework.AssemblyOptions,
) framework.AssemblyOptions {
	if prepared.Platform.VectorAgentConfigMap == "" {
		return options
	}
	active := false
	for _, group := range generated {
		active = active || collectsLogs(group)
	}
	if !active {
		return options
	}
	reader := &trackedFactsReader{cache: &factReadCache{client: r.Client, scheme: r.Scheme,
		reads: map[factReadKey]factRead{}}, observed: map[factReadKey]framework.FactObject{}}
	result, err := framework.ResolveVectorDestination(ctx, reader, prepared.Source.Cluster.Namespace,
		prepared.Platform.VectorAgentConfigMap)
	if reader.failure != nil {
		err = reader.failure
	}
	result = normalizeFactResult(result, err)
	result.Diagnostic.Observed = reader.observations()
	for index := range generated {
		group := &generated[index]
		if !collectsLogs(*group) {
			continue
		}
		prior := group.Outcome.Facts
		diagnostic := input.Clone(result.Diagnostic)
		if prior != nil {
			diagnostic.Observed = combineFactObservations(prior.Observed, diagnostic.Observed)
		}
		group.Outcome.Facts = &diagnostic
		if result.Diagnostic.State != framework.FactsResolved {
			group.Runtime = nil
			group.Outcome.GeneratedEndpoints = nil
		}
	}
	options.VectorDestination = input.Clone(result.Value)
	return options
}

func collectsLogs[C, S, F any](group pipeline.GeneratedGroup[C, S, F]) bool {
	return group.Outcome.Error == "" && group.Input != nil && group.Runtime != nil &&
		group.Input.Config.Common.Logging.EnableVectorAgent && len(group.Runtime.LogOutputs) > 0
}

func combineFactObservations(prior, current []framework.FactObject) []framework.FactObject {
	unique := make(map[string]framework.FactObject, len(prior)+len(current))
	for _, observations := range [][]framework.FactObject{prior, current} {
		for _, object := range observations {
			unique[factObjectKey(object)] = object
		}
	}
	out := make([]framework.FactObject, 0, len(unique))
	for _, object := range unique {
		out = append(out, object)
	}
	slices.SortFunc(out, func(a, b framework.FactObject) int {
		return strings.Compare(factObjectKey(a), factObjectKey(b))
	})
	return out
}
