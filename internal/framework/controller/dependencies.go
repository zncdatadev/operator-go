package controller

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/zncdatadev/operator-go/internal/framework/pipeline"
	"github.com/zncdatadev/operator-go/pkg/framework"
	"github.com/zncdatadev/operator-go/pkg/framework/input"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

const defaultFactRefresh = 30 * time.Second

type factReadKey struct {
	GVK schema.GroupVersionKind
	Key client.ObjectKey
}

type factRead struct {
	object map[string]any
	stamp  framework.FactObject
	err    error
}

// A cache belongs to one reconcile, never to a controller or CR across passes.
type factReadCache struct {
	client client.Reader
	scheme *runtime.Scheme
	reads  map[factReadKey]factRead
}

type trackedFactsReader struct {
	cache    *factReadCache
	observed map[factReadKey]framework.FactObject
	failure  error
}

func (r *trackedFactsReader) Get(ctx context.Context, key client.ObjectKey, object framework.FactResource) (err error) {
	defer func() {
		if err != nil && !apierrors.IsNotFound(err) && r.failure == nil {
			r.failure = err
		}
	}()
	if object == nil || reflect.ValueOf(object).Kind() != reflect.Pointer || reflect.ValueOf(object).IsNil() {
		return fmt.Errorf("facts Get requires a non-nil object pointer")
	}
	gvk, err := apiutil.GVKForObject(object, r.cache.scheme)
	if err != nil {
		return fmt.Errorf("facts Get object kind: %w", err)
	}
	if key.Name == "" || gvk.Empty() {
		return fmt.Errorf("facts Get requires an object kind and exact name")
	}
	cacheKey := factReadKey{GVK: gvk, Key: key}
	read, cached := r.cache.reads[cacheKey]
	if !cached {
		read.stamp = framework.FactObject{APIVersion: gvk.GroupVersion().String(), Kind: gvk.Kind,
			Namespace: key.Namespace, Name: key.Name}
		fresh := reflect.New(reflect.TypeOf(object).Elem()).Interface().(client.Object)
		fresh.GetObjectKind().SetGroupVersionKind(gvk)
		read.err = r.cache.client.Get(ctx, key, fresh)
		if read.err == nil {
			// Typed clients may omit TypeMeta in the decoded value. Retain the
			// known request GVK so another typed/unstructured consumer can copy it.
			fresh.GetObjectKind().SetGroupVersionKind(gvk)
			read.stamp.UID = string(fresh.GetUID())
			read.stamp.ResourceVersion = fresh.GetResourceVersion()
			read.object, read.err = runtime.DefaultUnstructuredConverter.ToUnstructured(fresh)
		}
		r.cache.reads[cacheKey] = read
	}
	r.observed[cacheKey] = read.stamp
	if read.err != nil {
		return read.err
	}
	// Deep-copy the canonical API snapshot for each caller, including repeated
	// reads of the same reference within a resolver. No mutations reach the cache.
	reflect.ValueOf(object).Elem().Set(reflect.Zero(reflect.TypeOf(object).Elem()))
	return runtime.DefaultUnstructuredConverter.FromUnstructured(runtime.DeepCopyJSON(read.object), object)
}

func (r *trackedFactsReader) observations() []framework.FactObject {
	objects := make([]framework.FactObject, 0, len(r.observed))
	for _, observed := range r.observed {
		objects = append(objects, observed)
	}
	slices.SortFunc(objects, func(a, b framework.FactObject) int {
		return strings.Compare(factObjectKey(a), factObjectKey(b))
	})
	return objects
}

func factObjectKey(object framework.FactObject) string {
	return object.APIVersion + "/" + object.Kind + "/" + object.Namespace + "/" + object.Name
}

func (r *Reconciler[CR, C, S, F]) resolvePreparedFacts(ctx context.Context,
	prepared pipeline.PreparedInputs[C, S, F],
) map[pipeline.GroupKey]framework.FactResult[F] {
	if r.ResolveFacts == nil {
		return nil
	}
	cache := &factReadCache{client: r.Client, scheme: r.Scheme, reads: map[factReadKey]factRead{}}
	results := make(map[pipeline.GroupKey]framework.FactResult[F], len(prepared.Topology))
	for _, group := range prepared.Topology {
		if group.Config == nil || group.Error != "" {
			continue
		}
		reader := &trackedFactsReader{cache: cache, observed: map[factReadKey]framework.FactObject{}}
		resolvedInput := input.Clone(framework.FactInput[C, S, F]{Platform: prepared.Platform, Group: group.Group,
			Config:        *group.Config,
			ClusterConfig: prepared.ClusterConfig, Image: prepared.Image,
			Shared: prepared.Source.Shared, Topology: prepared.Topology})
		result, err := r.ResolveFacts(ctx, reader, resolvedInput)
		if reader.failure != nil {
			// Optional absence is a product decision; a failed API observation
			// cannot become resolved merely because a resolver ignored its error.
			err = reader.failure
		}
		result = normalizeFactResult(result, err)
		result.Diagnostic.Observed = reader.observations()
		results[pipeline.GroupKey{Role: group.Group.Role, Name: group.Group.Name}] = input.Clone(result)
	}
	return results
}

func normalizeFactResult[F any](result framework.FactResult[F], err error) framework.FactResult[F] {
	if err != nil {
		return framework.FactResult[F]{Diagnostic: framework.FactDiagnostic{
			State: framework.FactsReadError, Reason: "ReadError", Message: safeFactError(err),
		}}
	}
	valid := false
	switch result.Diagnostic.State {
	case framework.FactsResolved:
		valid = result.Value != nil
	case framework.FactsPending, framework.FactsInvalid, framework.FactsReadError:
		valid = result.Value == nil
	}
	if !valid {
		return framework.FactResult[F]{Diagnostic: framework.FactDiagnostic{State: framework.FactsInvalid,
			Reason: "InvalidFactResult", Message: "Resolver returned an invalid state/value combination"}}
	}
	return result
}

// Raw API/error messages can contain returned object data. Preserve the error
// type and structured API reason, while object references are recorded separately.
func safeFactError(err error) string {
	message := fmt.Sprintf("External fact resolution failed (%T)", err)
	var status apierrors.APIStatus
	if errors.As(err, &status) {
		message += "; API reason=" + string(status.Status().Reason)
	}
	return message
}

func cloneFactDiagnostic(in *framework.FactDiagnostic) *framework.FactDiagnostic {
	if in == nil {
		return nil
	}
	out := *in
	out.Observed = slices.Clone(in.Observed)
	return &out
}

func factsBlockGroup(observation *framework.GroupReconcileStatus) (blocked, pending bool) {
	facts := observation.Facts
	if facts == nil || facts.State == framework.FactsResolved {
		return false, false
	}
	observation.Message = facts.Message
	if observation.Message == "" {
		observation.Message = "External facts are " + string(facts.State)
	}
	return true, facts.State == framework.FactsPending
}

func (r *Reconciler[CR, C, S, F]) nextRefresh(pending bool) time.Duration {
	delay := r.FactRefreshInterval
	if delay <= 0 {
		delay = defaultFactRefresh
	}
	if pending {
		delay = min(delay, retirementPoll)
	}
	return delay
}

func (r *Reconciler[CR, C, S, F]) controllerOptions() controller.Options {
	// Reconcile errors discard RequeueAfter. Cap the default priority queue's
	// per-key exponential backoff so a failing sibling cannot postpone external
	// reads or retirement indefinitely. REST client throttling is unchanged.
	// This bounds queue delay, not wall-clock progress during API or worker stalls.
	maximum := r.nextRefresh(true)
	return controller.Options{RateLimiter: workqueue.NewTypedItemExponentialFailureRateLimiter[ctrl.Request](
		min(5*time.Millisecond, maximum), maximum)}
}
