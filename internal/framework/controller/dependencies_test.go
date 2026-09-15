package controller

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/zncdatadev/operator-go/pkg/framework"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/priorityqueue"
)

func TestFactsReaderCachesAPIObjectsAndErrorsPerPass(t *testing.T) {
	scheme := controllerScheme(t)
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "shared", Namespace: "facts",
		UID: "shared-uid", ResourceVersion: "7"}, Data: map[string]string{"value": "original"}}
	reads := map[string]int{}
	denied := apierrors.NewForbidden(schema.GroupResource{Resource: "configmaps"}, "denied", errors.New("private data"))
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, object client.Object,
			options ...client.GetOption) error {
			reads[key.Name]++
			if key.Name == "denied" {
				return denied
			}
			return c.Get(ctx, key, object, options...)
		},
	}).Build()
	cache := &factReadCache{client: c, scheme: scheme, reads: map[factReadKey]factRead{}}
	newReader := func() *trackedFactsReader {
		return &trackedFactsReader{cache: cache, observed: map[factReadKey]framework.FactObject{}}
	}
	first, second := newReader(), newReader()
	key := client.ObjectKeyFromObject(cm)
	var one, two corev1.ConfigMap
	if err := first.Get(t.Context(), key, &one); err != nil {
		t.Fatal(err)
	}
	one.Data["value"] = "caller mutation"
	cm.Data["value"] = "new API version"
	if err := c.Update(t.Context(), cm); err != nil {
		t.Fatal(err)
	}
	if err := second.Get(t.Context(), key, &two); err != nil {
		t.Fatal(err)
	}
	if two.Data["value"] != "original" || reads["shared"] != 1 {
		t.Fatalf("cache was shared mutably or reread: %+v %v", two.Data, reads)
	}
	// The same GVK may be requested through typed and unstructured objects.
	raw := &unstructured.Unstructured{}
	raw.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
	if err := second.Get(t.Context(), key, raw); err != nil {
		t.Fatal(err)
	}
	value, _, err := unstructured.NestedString(raw.Object, "data", "value")
	if err != nil || value != "original" || reads["shared"] != 1 {
		t.Fatalf("typed/unstructured cache differs: %s %v %v", value, err, reads)
	}
	for _, name := range []string{"missing", "denied"} {
		for _, reader := range []*trackedFactsReader{first, second} {
			err := reader.Get(t.Context(), client.ObjectKey{Namespace: "facts", Name: name}, &corev1.ConfigMap{})
			if (name == "missing" && !apierrors.IsNotFound(err)) || (name == "denied" && !apierrors.IsForbidden(err)) {
				t.Fatalf("%s: wrong cached error: %v", name, err)
			}
		}
		if reads[name] != 1 {
			t.Fatalf("%s error reread %d times", name, reads[name])
		}
	}
	observed := second.observations()
	if len(observed) != 3 || observed[0].Name != "denied" || observed[1].Name != "missing" ||
		observed[1].UID != "" || observed[2].UID != "shared-uid" || observed[2].ResourceVersion != "7" {
		t.Fatalf("missing or unstable source observations: %+v", observed)
	}
}

func TestNormalizeFactResultRejectsContradictionsAndRedactsErrorData(t *testing.T) {
	value := testFacts{}
	states := []framework.FactState{framework.FactsResolved, framework.FactsPending, framework.FactsInvalid,
		framework.FactsReadError, "unknown"}
	for _, state := range states {
		for _, present := range []bool{false, true} {
			result := framework.FactResult[testFacts]{Diagnostic: framework.FactDiagnostic{State: state}}
			if present {
				result.Value = &value
			}
			actual := normalizeFactResult(result, nil)
			valid := (state == framework.FactsResolved && present) ||
				((state == framework.FactsPending || state == framework.FactsInvalid ||
					state == framework.FactsReadError) && !present)
			if !valid && (actual.Diagnostic.State != framework.FactsInvalid || actual.Value != nil ||
				actual.Diagnostic.Reason != "InvalidFactResult") {
				t.Fatalf("state=%s value=%t was not rejected: %+v", state, present, actual)
			}
			if valid && !reflect.DeepEqual(actual, result) {
				t.Fatalf("valid result changed: %+v", actual)
			}
		}
	}
	err := apierrors.NewForbidden(schema.GroupResource{Resource: "secrets"}, "secret-name",
		errors.New("secret-value-must-not-appear"))
	actual := normalizeFactResult(framework.FactResult[testFacts]{Value: &value}, err)
	if actual.Diagnostic.State != framework.FactsReadError || actual.Value != nil ||
		!strings.Contains(actual.Diagnostic.Message, "StatusError") ||
		!strings.Contains(actual.Diagnostic.Message, "Forbidden") ||
		strings.Contains(actual.Diagnostic.Message, "secret-value") {
		t.Fatalf("read failure lost type or leaked data: %+v", actual)
	}
}

func TestFactResolutionDoesNotBlockHealthyGroupsOrRetirement(t *testing.T) {
	for _, state := range []framework.FactState{framework.FactsPending, framework.FactsInvalid, framework.FactsReadError} {
		t.Run(string(state), func(t *testing.T) {
			cr := controllerInput()
			kept, removed := retirementObjects(t, cr, "blocked", 1), retirementObjects(t, cr, "removed", 0)
			r, input := factsTestReconciler(t, append(kept, removed...))
			r.ResolveFacts = func(_ context.Context, _ framework.FactsReader,
				in framework.FactInput[testConfig, testClusterConfig, testFacts],
			) (
				framework.FactResult[testFacts], error) {
				if in.Group.Name == "blocked" {
					if state == framework.FactsReadError {
						return framework.FactResult[testFacts]{}, errors.New("API connection failed")
					}
					return framework.FactResult[testFacts]{Diagnostic: framework.FactDiagnostic{
						State: state, Reason: "Fixture", Message: "not resolved"}}, nil
				}
				return framework.FactResult[testFacts]{Value: &in.Shared,
					Diagnostic: framework.FactDiagnostic{State: framework.FactsResolved}}, nil
			}
			result, err := r.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(input)})
			if err != nil || result.RequeueAfter != retirementPoll {
				t.Fatalf("facts hid retirement cadence: %+v %v", result, err)
			}
			current := input.DeepCopy()
			applyTestGet(t, r.Client, current)
			if len(current.Status.Groups) != 2 || meta.IsStatusConditionTrue(current.Status.Conditions, "Built") ||
				meta.IsStatusConditionTrue(current.Status.Conditions, "Applied") {
				t.Fatalf("incomplete facts reported success: %+v", current.Status)
			}
			for _, group := range current.Status.Groups {
				if group.Name == "healthy" && !group.Applied {
					t.Fatalf("independent group blocked: %+v", group)
				}
				if group.Name == "blocked" && (group.Applied || group.Facts == nil || group.Facts.State != state) {
					t.Fatalf("classified result lost: %+v", group)
				}
			}
			sts := kept[0].(*appsv1.StatefulSet).DeepCopy()
			applyTestGet(t, r.Client, sts)
			if sts.UID != kept[0].GetUID() || *sts.Spec.Replicas != 1 {
				t.Fatal("blocked desired group was retired or replaced")
			}
			err = r.Client.Get(t.Context(), client.ObjectKeyFromObject(removed[0]), &appsv1.StatefulSet{})
			if !apierrors.IsNotFound(err) {
				t.Fatalf("removed group did not progress: %v", err)
			}
		})
	}
}

func TestResolvedFactsRefreshWithoutCRChangeAndSettle(t *testing.T) {
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "external", Namespace: controllerInput().Namespace,
		UID: "external-uid"}, Data: map[string]string{"value": "one"}}
	r, input := factsTestReconciler(t, []client.Object{cm})
	r.FactRefreshInterval = 7 * time.Second
	reads := 0
	r.ResolveFacts = func(ctx context.Context, reader framework.FactsReader,
		in framework.FactInput[testConfig, testClusterConfig, testFacts],
	) (
		framework.FactResult[testFacts], error) {
		reads++
		if in.Shared.Catalogs["test"]["key"] != "base" || in.Topology[0].Config == nil {
			t.Fatal("resolver input was shared or a fact gate erased valid topology")
		}
		var object corev1.ConfigMap
		if err := reader.Get(ctx, client.ObjectKeyFromObject(cm), &object); err != nil {
			return framework.FactResult[testFacts]{}, err
		}
		in.Shared.Catalogs["test"]["key"] = object.Data["value"]
		in.Topology[0].Config = nil // must not affect the next consumer or pure build
		return framework.FactResult[testFacts]{Value: &in.Shared,
			Diagnostic: framework.FactDiagnostic{State: framework.FactsResolved,
				Observed: []framework.FactObject{{Name: "fabricated"}}}}, nil
	}
	request := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(input)}
	versions := map[string]string{}
	for pass := range 3 {
		if pass == 2 {
			applyTestGet(t, r.Client, cm)
			cm.Data["value"] = "two"
			if err := r.Client.Update(t.Context(), cm); err != nil {
				t.Fatal(err)
			}
		}
		before := input.DeepCopy()
		applyTestGet(t, r.Client, before)
		result, err := r.Reconcile(t.Context(), request)
		if err != nil || result.RequeueAfter != 7*time.Second {
			t.Fatalf("resolved facts lost refresh: %+v %v", result, err)
		}
		current := input.DeepCopy()
		applyTestGet(t, r.Client, current)
		if pass == 1 && current.ResourceVersion != before.ResourceVersion {
			t.Fatal("unchanged facts rewrote status")
		}
		if current.Generation != input.Generation {
			t.Fatal("test changed CR generation to trigger external refresh")
		}
		want := "one"
		if pass == 2 {
			want = "two"
		}
		for _, group := range current.Status.Groups {
			if group.Facts == nil || len(group.Facts.Observed) != 1 || group.Facts.Observed[0].Name != "external" ||
				group.Facts.Observed[0].UID != "external-uid" {
				t.Fatalf("reader did not replace resolver observations: %+v", group.Facts)
			}
			var sts appsv1.StatefulSet
			key := client.ObjectKey{Namespace: input.Namespace, Name: input.Name + "-workers-" + group.Name}
			if err := r.Client.Get(t.Context(), key, &sts); err != nil {
				t.Fatal(err)
			}
			if sts.Spec.Template.Spec.Containers[0].Env[0].Value != want {
				t.Fatalf("external value was not refreshed: %+v", sts.Spec.Template.Spec.Containers[0].Env)
			}
			if pass == 1 && versions[group.Name] != sts.ResourceVersion {
				t.Fatal("unchanged resolved facts rewrote the StatefulSet")
			}
			versions[group.Name] = sts.ResourceVersion
		}
	}
	if reads != 6 {
		t.Fatalf("resolver was not called for both consumers each pass: %d", reads)
	}
}

func TestFactsReaderFailureCannotBecomeResolved(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{true: "optional-absence", false: "ignored-read-failure"}[missing], func(t *testing.T) {
			r, input := factsTestReconciler(t, nil)
			reads := 0
			r.Client = interceptor.NewClient(r.Client.(client.WithWatch), interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, object client.Object,
					options ...client.GetOption) error {
					if key.Name == "external" {
						reads++
						resource := schema.GroupResource{Resource: "configmaps"}
						if missing {
							return apierrors.NewNotFound(resource, key.Name)
						}
						return apierrors.NewForbidden(resource, key.Name, errors.New("private content"))
					}
					return c.Get(ctx, key, object, options...)
				},
			})
			r.ResolveFacts = func(ctx context.Context, reader framework.FactsReader,
				in framework.FactInput[testConfig, testClusterConfig, testFacts],
			) (
				framework.FactResult[testFacts], error) {
				_ = reader.Get(ctx, client.ObjectKey{Namespace: input.Namespace, Name: "external"}, &corev1.ConfigMap{})
				return framework.FactResult[testFacts]{Value: &in.Shared,
					Diagnostic: framework.FactDiagnostic{State: framework.FactsResolved}}, nil
			}
			result, err := r.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(input)})
			if err != nil || result.RequeueAfter != defaultFactRefresh || reads != 1 {
				t.Fatalf("reader failure broke classification/cache/refresh: %+v %v reads=%d", result, err, reads)
			}
			current := input.DeepCopy()
			applyTestGet(t, r.Client, current)
			want := framework.FactsReadError
			if missing {
				want = framework.FactsResolved
			}
			for _, group := range current.Status.Groups {
				if group.Facts == nil || group.Facts.State != want || group.Applied != missing ||
					len(group.Facts.Observed) != 1 || group.Facts.Observed[0].Name != "external" {
					t.Fatalf("reader outcome was overwritten: %+v", group)
				}
			}
		})
	}
}

func TestFactsRefreshCadence(t *testing.T) {
	r, _ := factsTestReconciler(t, nil)
	if r.nextRefresh(false) != defaultFactRefresh || r.nextRefresh(true) != retirementPoll {
		t.Fatal("built-in platform references must refresh without a product resolver")
	}
	r.ResolveFacts = func(context.Context, framework.FactsReader,
		framework.FactInput[testConfig, testClusterConfig, testFacts],
	) (
		framework.FactResult[testFacts], error) {
		return framework.FactResult[testFacts]{}, nil
	}
	for _, interval := range []time.Duration{0, -1, time.Second, time.Minute} {
		r.FactRefreshInterval = interval
		want := interval
		if want <= 0 {
			want = defaultFactRefresh
		}
		if r.nextRefresh(false) != want || r.nextRefresh(true) != min(want, retirementPoll) {
			t.Fatalf("interval %v does not preserve earliest refresh", interval)
		}
	}
}

func TestFactsErrorBackoffIsBoundedPerKey(t *testing.T) {
	r, input := factsTestReconciler(t, nil)
	if r.controllerOptions().RateLimiter == nil {
		t.Fatal("built-in platform references need bounded error backoff without a product resolver")
	}
	r.ResolveFacts = func(context.Context, framework.FactsReader,
		framework.FactInput[testConfig, testClusterConfig, testFacts],
	) (
		framework.FactResult[testFacts], error) {
		return framework.FactResult[testFacts]{}, nil
	}
	key := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(input)}
	for _, interval := range []time.Duration{0, -1, time.Millisecond, 30 * time.Second} {
		r.FactRefreshInterval = interval
		limiter := r.controllerOptions().RateLimiter
		maximum := r.nextRefresh(true)
		for range 64 {
			if delay := limiter.When(key); delay <= 0 || delay > maximum {
				t.Fatalf("configured interval=%v: per-key backoff %v exceeds %v", interval, delay, maximum)
			}
		}
		if limiter.NumRequeues(key) != 64 {
			t.Fatal("the cap discarded error retry accounting")
		}
		other := ctrl.Request{NamespacedName: client.ObjectKey{Namespace: input.Namespace, Name: "independent"}}
		base := min(5*time.Millisecond, maximum)
		if limiter.When(other) != base {
			t.Fatal("one key's failure count delayed an independent key")
		}
		limiter.Forget(key)
		if limiter.NumRequeues(key) != 0 || limiter.When(key) != base {
			t.Fatal("successful convergence did not reset the exponential backoff")
		}
	}
}

func TestFactsMixedFailureRefreshesThroughErrorQueue(t *testing.T) {
	r, input := factsTestReconciler(t, nil)
	r.FactRefreshInterval = 20 * time.Millisecond
	generate := r.Definition.GenerateGroup
	r.Definition.GenerateGroup = func(in framework.EffectiveInput[testConfig, testClusterConfig, testFacts]) (
		framework.RuntimeDescription, error) {
		if in.Group.Name == "healthy" {
			return framework.RuntimeDescription{}, errors.New("continuous sibling build failure")
		}
		return generate(in)
	}
	external := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "external", Namespace: input.Namespace},
		Data: map[string]string{"value": "arrived-without-a-CR-update"}}
	r.ResolveFacts = func(ctx context.Context, reader framework.FactsReader,
		in framework.FactInput[testConfig, testClusterConfig, testFacts],
	) (
		framework.FactResult[testFacts], error) {
		if in.Group.Name == "blocked" {
			var observed corev1.ConfigMap
			if err := reader.Get(ctx, client.ObjectKeyFromObject(external), &observed); err != nil {
				if apierrors.IsNotFound(err) {
					return framework.FactResult[testFacts]{Diagnostic: framework.FactDiagnostic{
						State: framework.FactsPending, Reason: "Missing", Message: "Waiting for the external ConfigMap"}}, nil
				}
				return framework.FactResult[testFacts]{}, err
			}
			in.Shared.Catalogs["test"]["key"] = observed.Data["value"]
		}
		return framework.FactResult[testFacts]{Value: &in.Shared,
			Diagnostic: framework.FactDiagnostic{State: framework.FactsResolved}}, nil
	}
	key := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(input)}
	limiter := r.controllerOptions().RateLimiter
	// Put the real queue in the long-lived failure case immediately. An uncapped
	// default limiter would now defer the next queue delivery by 1000 seconds.
	for range 64 {
		limiter.When(key)
	}
	queue := priorityqueue.New[ctrl.Request]("", func(options *priorityqueue.Opts[ctrl.Request]) {
		options.RateLimiter = limiter
	})
	defer queue.ShutDown()
	queue.Add(key)
	for pass := range 2 {
		queued := make(chan ctrl.Request, 1)
		go func() {
			item, shutdown := queue.Get()
			if !shutdown {
				queued <- item
			}
		}()
		select {
		case got := <-queued:
			if got != key {
				t.Fatalf("queued the wrong CR: %+v", got)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("error queue backoff postponed external refresh past the test deadline")
		}
		result, err := r.Reconcile(t.Context(), key)
		if err == nil || !strings.Contains(err.Error(), "continuous sibling build failure") || !result.IsZero() {
			t.Fatalf("mixed failure was swallowed or recast as successful polling: %+v %v", result, err)
		}
		current := input.DeepCopy()
		applyTestGet(t, r.Client, current)
		want := framework.FactsPending
		if pass == 1 {
			want = framework.FactsResolved
		}
		for _, group := range current.Status.Groups {
			if group.Name == "blocked" && (group.Facts == nil || group.Facts.State != want || group.Applied != (pass == 1)) {
				t.Fatalf("independent fact did not progress through error retries: %+v", group)
			}
		}
		if current.Generation != input.Generation {
			t.Fatal("a CR input event masked the missing external refresh")
		}
		if pass == 0 {
			if err := r.Client.Create(t.Context(), external); err != nil {
				t.Fatal(err)
			}
			// Match controller-runtime's error branch: enqueue with RateLimited
			// while processing, then Done releases the key. No CR/watch event.
			queue.AddWithOpts(priorityqueue.AddOpts{RateLimited: true}, key)
		}
		queue.Done(key)
	}
}
