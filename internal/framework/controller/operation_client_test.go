package controller

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	generatedtrino "github.com/zncdatadev/operator-go/internal/framework/pipeline/testinput"
)

// Every promoted API method panics if it reaches the nil underlying interface.
// Constructing a status/subresource client itself does not make an API request.
type operationPanicClient struct{ client.Client }
type operationPanicSubresource struct{ client.SubResourceClient }

func (operationPanicClient) Status() client.SubResourceWriter { return operationPanicSubresource{} }
func (operationPanicClient) SubResource(string) client.SubResourceClient {
	return operationPanicSubresource{}
}

func TestOperationClientRejectedRequestsNeverReachTheUnderlyingAPI(t *testing.T) {
	ctx, object := t.Context(), &corev1.ConfigMap{}
	patch := client.MergeFrom(object.DeepCopy())
	cases := map[string]func(*operationClient) error{
		"get":        func(c *operationClient) error { return c.Get(ctx, client.ObjectKey{}, object) },
		"list":       func(c *operationClient) error { return c.List(ctx, &corev1.ConfigMapList{}) },
		"create":     func(c *operationClient) error { return c.Create(ctx, object) },
		"update":     func(c *operationClient) error { return c.Update(ctx, object) },
		"dry-run":    func(c *operationClient) error { return c.Update(ctx, object, client.DryRunAll) },
		"patch":      func(c *operationClient) error { return c.Patch(ctx, object, patch) },
		"delete":     func(c *operationClient) error { return c.Delete(ctx, object) },
		"delete-all": func(c *operationClient) error { return c.DeleteAllOf(ctx, object) },
		"apply":      func(c *operationClient) error { return c.Apply(ctx, nil) },
		"status-create": func(c *operationClient) error {
			return c.Status().Create(ctx, object, object)
		},
		"status-update": func(c *operationClient) error { return c.Status().Update(ctx, object) },
		"status-patch":  func(c *operationClient) error { return c.Status().Patch(ctx, object, patch) },
		"status-apply":  func(c *operationClient) error { return c.Status().Apply(ctx, nil) },
		"subresource-get": func(c *operationClient) error {
			return c.SubResource("scale").Get(ctx, object, object)
		},
		"subresource-create": func(c *operationClient) error {
			return c.SubResource("scale").Create(ctx, object, object)
		},
		"subresource-update": func(c *operationClient) error { return c.SubResource("scale").Update(ctx, object) },
		"subresource-patch":  func(c *operationClient) error { return c.SubResource("scale").Patch(ctx, object, patch) },
		"subresource-apply":  func(c *operationClient) error { return c.SubResource("scale").Apply(ctx, nil) },
	}
	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			refused, checks := errors.New("operation refused"), 0
			guarded := &operationClient{Client: operationPanicClient{}, check: func(context.Context) error {
				checks++
				return refused
			}}
			if err := call(guarded); !errors.Is(err, refused) || checks != 1 {
				t.Fatalf("request did not preserve guard failure: checks=%d err=%v", checks, err)
			}
		})
	}
}

// Fake clients let this test keep generation unchanged deliberately, verifying
// the typed operation getter rather than relying on generation as the only gate.
func pauseOperationWithoutGenerationChange(ctx context.Context, c client.Client,
	observed *generatedtrino.TrinoCluster,
) error {
	current := observed.DeepCopy()
	if err := c.Get(ctx, client.ObjectKeyFromObject(observed), current); err != nil {
		return err
	}
	paused := true
	if current.Spec.ClusterConfig == nil {
		current.Spec.ClusterConfig = &generatedtrino.ClusterConfigInput{}
	}
	current.Spec.ClusterConfig.ReconciliationPaused = &paused
	current.Generation = observed.Generation
	return c.Update(ctx, current)
}

func TestOperationClientPauseInterruptsRetainedReceiptRetry(t *testing.T) {
	fixture := retainedFixture(t)
	paused, receiptWrites, readsAfterPause := false, 0, 0
	c := fake.NewClientBuilder().WithScheme(controllerScheme(t)).
		WithObjects(fixture.cr, fixture.class, fixture.sts, fixture.claim, fixture.pv).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, object client.Object,
				options ...client.GetOption) error {
				if _, crRead := object.(*generatedtrino.TrinoCluster); paused && !crRead {
					readsAfterPause++
				}
				return c.Get(ctx, key, object, options...)
			},
			Update: func(ctx context.Context, c client.WithWatch, object client.Object,
				options ...client.UpdateOption) error {
				if _, claim := object.(*corev1.PersistentVolumeClaim); !claim {
					return c.Update(ctx, object, options...)
				}
				receiptWrites++
				if err := pauseOperationWithoutGenerationChange(ctx, c, fixture.cr); err != nil {
					return err
				}
				paused = true
				return apierrors.NewConflict(schema.GroupResource{Resource: "persistentvolumeclaims"}, object.GetName(),
					errors.New("pause requested during receipt write"))
			},
		}).Build()
	r := newTestReconciler(c, c.Scheme(), testFacts{})
	guarded := &operationClient{Client: c, check: func(ctx context.Context) error {
		return r.currentInput(ctx, fixture.cr)
	}}
	err := checkRetainedStorage(t.Context(), guarded, fixture.cr, fixture.group,
		fixture.desired(), fixture.data, fixture.sts)
	if !errors.Is(err, errSuperseded) || receiptWrites != 1 || readsAfterPause != 0 {
		t.Fatalf("pause failed to stop receipt retry: err=%v writes=%d later reads=%d", err, receiptWrites, readsAfterPause)
	}
	claim := fixture.claim.DeepCopy()
	applyTestGet(t, c, claim)
	if _, recorded := claim.Annotations[retainedBindingAnnotation]; recorded {
		t.Fatal("a rejected receipt write was nevertheless persisted")
	}
	current := fixture.cr.DeepCopy()
	applyTestGet(t, c, current)
	if current.Generation != fixture.cr.Generation || !generatedtrino.Operation(current).ReconciliationPaused {
		t.Fatal("test did not pause through the typed getter while keeping generation unchanged")
	}
}

func TestOperationClientPauseInterruptsApplyConflictBeforeFurtherIO(t *testing.T) {
	cr, scheme := controllerInput(), retirementScheme(t)
	live := retirementObjects(t, cr, "one", 1)[3].(*corev1.ConfigMap)
	live.Data = map[string]string{"value": "old"}
	desired := live.DeepCopy()
	desired.Annotations, desired.OwnerReferences = nil, nil
	desired.Data = map[string]string{"value": "new"}
	paused, writes, dryRuns, readsAfterPause := false, 0, 0, 0
	c := retirementClient(scheme, cr, []client.Object{live}, interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, object client.Object,
			options ...client.GetOption) error {
			if _, crRead := object.(*generatedtrino.TrinoCluster); paused && !crRead {
				readsAfterPause++
			}
			return c.Get(ctx, key, object, options...)
		},
		Update: func(ctx context.Context, c client.WithWatch, object client.Object,
			options ...client.UpdateOption) error {
			if applyDryRun(options) {
				dryRuns++
				return c.Update(ctx, object, options...)
			}
			writes++
			if err := pauseOperationWithoutGenerationChange(ctx, c, cr); err != nil {
				return err
			}
			paused = true
			return apierrors.NewConflict(schema.GroupResource{Resource: "configmaps"}, object.GetName(),
				errors.New("pause requested during apply"))
		},
	})
	r := newTestReconciler(c, scheme, testFacts{})
	guarded := &operationClient{Client: c, check: func(ctx context.Context) error { return r.currentInput(ctx, cr) }}
	slot := groupSlot{Role: "workers", Group: "one", Slot: slotConfigmap}
	changed, err := applyObject(t.Context(), guarded, cr, desired, scheme, &slot, nil)
	if changed || !errors.Is(err, errSuperseded) || writes != 1 || dryRuns != 1 || readsAfterPause != 0 {
		t.Fatalf("apply continued after pause: changed=%t err=%v writes=%d dry-runs=%d later reads=%d",
			changed, err, writes, dryRuns, readsAfterPause)
	}
	applyTestGet(t, c, live)
	if live.Data["value"] != "old" {
		t.Fatal("failed apply was persisted after pause")
	}
	current := cr.DeepCopy()
	applyTestGet(t, c, current)
	if current.Generation != cr.Generation || !generatedtrino.Operation(current).ReconciliationPaused {
		t.Fatal("test did not pause without a generation change")
	}
}
