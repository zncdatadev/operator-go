package controller

import (
	"testing"

	"github.com/zncdatadev/operator-go/internal/framework/pipeline"
	"github.com/zncdatadev/operator-go/pkg/framework"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func platformStorageRuntime() *framework.RuntimeDescription {
	return &framework.RuntimeDescription{Directories: []framework.Directory{
		{Name: "listener", Listener: &framework.ListenerVolume{Class: "external"}},
		{Name: "tls", Secret: &framework.SecretVolume{SecretClass: "tls", Format: "tls-pem"}},
	}}
}

func TestPlatformClaimsApplyStopAndRetire(t *testing.T) {
	cr, scheme := controllerInput(), controllerScheme(t)
	slot := groupSlot{Role: "workers", Group: "default", Slot: slotStatefulset}
	runtime := platformStorageRuntime()
	desired := retirementObjects(t, cr, "default", 0)[0].(*appsv1.StatefulSet)
	desired.Annotations, desired.OwnerReferences = nil, nil
	desired.Spec.Template.Spec.Volumes = pipeline.DeclaredPlatformClaims(runtime)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cr).Build()
	r := newTestReconciler(c, scheme, testFacts{})
	if err := r.preflightStorage(t.Context(), cr, slot, desired, nil, runtime); err != nil {
		t.Fatal(err)
	}
	if _, err := applyScopedObject(t.Context(), c, cr, desired, scheme, &slot, nil, nil, nil, runtime); err != nil {
		t.Fatalf("declared platform claims cannot apply: %v", err)
	}
	live := &appsv1.StatefulSet{}
	if err := c.Get(t.Context(), client.ObjectKeyFromObject(desired), live); err != nil {
		t.Fatal(err)
	}
	if live.Annotations[platformClaimsAnnotation] == "" {
		t.Fatal("missing durable platform declaration")
	}
	// A fresh controller can stop and retire without access to the old runtime.
	r = newTestReconciler(c, scheme, testFacts{})
	if pending, err := r.stopWorkload(t.Context(), cr, slot); pending || err != nil {
		t.Fatalf("stop: %v %v", pending, err)
	}
	if _, err := r.retireGroup(t.Context(), cr, slot); err != nil {
		t.Fatalf("retire: %v", err)
	}
}

func TestPlatformClaimsCoexistWithRetainedDataAndRejectUndeclaredClaims(t *testing.T) {
	f := retainedFixture(t)
	runtime := platformStorageRuntime()
	f.sts.Spec.Template.Spec.Volumes = pipeline.DeclaredPlatformClaims(runtime)
	stamped, err := stampPlatformClaims(f.cr, f.sts, &f.group, runtime)
	if err != nil {
		t.Fatal(err)
	}
	sts := stamped.(*appsv1.StatefulSet)
	if _, err := declaredRetainedSource(sts, f.cr, f.group); err != nil {
		t.Fatalf("platform claim blocks retained data: %v", err)
	}
	// Even an exact clone of a valid platform claim is not a declaration when it
	// is introduced solely by Pod overrides under a new volume name.
	injected := *sts.Spec.Template.Spec.Volumes[0].DeepCopy()
	injected.Name = "undeclared"
	sts.Spec.Template.Spec.Volumes = append(sts.Spec.Template.Spec.Volumes, injected)
	if err := validRetainedTemplate(sts, f.data); err == nil {
		t.Fatal("undeclared ephemeral accepted")
	}
	sts = stamped.(*appsv1.StatefulSet).DeepCopy()
	sts.Spec.VolumeClaimTemplates = nil
	if err := validRetainedTemplate(sts, nil); err == nil {
		t.Fatal("undeclared ephemeral accepted without retained data")
	}
}

func TestPlatformClaimReceiptCannotBeSuppliedByDesiredMetadata(t *testing.T) {
	cr, scheme := controllerInput(), controllerScheme(t)
	slot := groupSlot{Role: "workers", Group: "default", Slot: slotStatefulset}
	desired := retirementObjects(t, cr, "default", 0)[0].(*appsv1.StatefulSet)
	desired.Annotations = map[string]string{platformClaimsAnnotation: `{}`}
	desired.OwnerReferences = nil
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cr).Build()
	if _, err := applyScopedObject(t.Context(), c, cr, desired, scheme, &slot, nil, nil, nil,
		platformStorageRuntime()); err == nil {
		t.Fatal("product/override metadata supplied controller platform proof")
	}
}
