package controller

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/zncdatadev/operator-go/pkg/framework"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	generatedtrino "github.com/zncdatadev/operator-go/internal/framework/pipeline/testinput"
	"github.com/zncdatadev/operator-go/pkg/framework/dataops"
)

func controllerScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := dataops.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := generatedtrino.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := storagev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := policyv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return scheme
}

func controllerInput() *generatedtrino.TrinoCluster {
	return &generatedtrino.TrinoCluster{ObjectMeta: metav1.ObjectMeta{
		Name: "observed", Namespace: "controller-unit", UID: "original-uid", Generation: 1,
	}}
}

func TestStatusRejectsSupersededInputAfterConflict(t *testing.T) {
	for _, change := range []string{"generation", "uid"} {
		t.Run(change, func(t *testing.T) {
			scheme := controllerScheme(t)
			observed := controllerInput()
			calls := 0
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(observed.DeepCopy()).
				WithStatusSubresource(observed).WithInterceptorFuncs(interceptor.Funcs{
				SubResourceUpdate: func(ctx context.Context, c client.Client, subresource string,
					object client.Object, _ ...client.SubResourceUpdateOption) error {
					calls++
					current := &generatedtrino.TrinoCluster{}
					if err := c.Get(ctx, client.ObjectKeyFromObject(object), current); err != nil {
						return err
					}
					if change == "generation" {
						current.Generation++
					} else {
						current.UID = "replacement-uid"
					}
					if err := c.Update(ctx, current); err != nil {
						return err
					}
					return apierrors.NewConflict(schema.GroupResource{Resource: subresource},
						object.GetName(), fmt.Errorf("concurrent input update"))
				},
			}).Build()
			r := newTestReconciler(c, scheme, testFacts{})
			err := r.writeStatus(t.Context(), observed, framework.ReconcileStatus{ObservedGeneration: 1})
			if !errors.Is(err, errSuperseded) || calls != 1 {
				t.Fatalf("stale status write retried: calls=%d err=%v", calls, err)
			}
			current := &generatedtrino.TrinoCluster{}
			if err := c.Get(t.Context(), client.ObjectKeyFromObject(observed), current); err != nil {
				t.Fatal(err)
			}
			if current.Status.ObservedGeneration != 0 {
				t.Fatalf("stale result was published: %+v", current.Status)
			}
		})
	}
}

func TestWorkloadObservationRequiresCurrentOwnedObject(t *testing.T) {
	for _, state := range []string{"ready", "foreign", "wrong-kind", "terminating", "unobserved", "old-revision"} {
		t.Run(state, func(t *testing.T) {
			scheme, cr := controllerScheme(t), controllerInput()
			controller, replicas := true, int32(1)
			sts := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "workload", Namespace: cr.Namespace,
				Generation: 2, OwnerReferences: []metav1.OwnerReference{{UID: cr.UID, Name: cr.Name,
					Kind: "TrinoCluster", APIVersion: generatedtrino.GroupVersion.String(), Controller: &controller}},
			}, Spec: appsv1.StatefulSetSpec{Replicas: &replicas}, Status: appsv1.StatefulSetStatus{
				ObservedGeneration: 2, Replicas: 1, ReadyReplicas: 1, UpdatedReplicas: 1,
				CurrentRevision: "rev2", UpdateRevision: "rev2",
			}}
			switch state {
			case "foreign":
				sts.OwnerReferences[0].UID = "other-uid"
			case "wrong-kind":
				sts.OwnerReferences[0].Kind = "WrongKind"
			case "terminating":
				now := metav1.Now()
				sts.DeletionTimestamp, sts.Finalizers = &now, []string{"test.design/hold"}
			case "unobserved":
				sts.Status.ObservedGeneration = 1
			case "old-revision":
				sts.Status.CurrentRevision = "rev1"
			}
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sts).Build()
			r := newTestReconciler(c, scheme, testFacts{})
			status := framework.GroupReconcileStatus{DesiredReplicas: 1}
			ready, err := r.observeWorkload(t.Context(), cr, sts, &status)
			foreign := state == "foreign" || state == "wrong-kind"
			if ready != (state == "ready") || (err != nil) != foreign {
				t.Fatalf("state=%s: ready=%t err=%v", state, ready, err)
			}
			if foreign && status.ReadyReplicas != 0 {
				t.Fatal("readiness was attributed from a foreign workload")
			}
		})
	}
}
