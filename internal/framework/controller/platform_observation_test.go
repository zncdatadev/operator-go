package controller

import (
	"context"
	"testing"

	"github.com/zncdatadev/operator-go/internal/framework/pipeline"
	"github.com/zncdatadev/operator-go/pkg/framework"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func TestPlatformProducerObservationAndReferenceRefresh(t *testing.T) {
	ctx := context.Background()
	scheme := controllerScheme(t)
	class := platformObject("listeners.kubedoop.dev", "ListenerClass")
	class.SetName("external")
	class.SetUID("class-uid")
	class.SetGeneration(1)
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "password", Namespace: "test", UID: "secret-uid"},
		Data: map[string][]byte{"password.db": []byte("first")}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(class, secret).
		WithStatusSubresource(&appsv1.StatefulSet{}, &corev1.Pod{}, &corev1.PersistentVolumeClaim{}).Build()
	r := newTestReconciler(c, scheme, testFacts{})
	owner := controllerInput()
	owner.Namespace = "test"
	runtime := &framework.RuntimeDescription{Directories: []framework.Directory{
		{Name: "listener", Listener: &framework.ListenerVolume{Class: "external"}},
		{Name: "password", Secret: &framework.SecretVolume{SecretName: "password"}},
	}}
	preparing, first, err := r.preparePlatformVolumes(ctx, "test", runtime, nil)
	if err != nil || preparing.Diagnostic.State != framework.FactsResolved || first == "" {
		t.Fatalf("producer was withheld waiting for not-yet-created CSI results: %+v %v", preparing, err)
	}
	// A class status-only write changes RV but must not initiate an endless rollout.
	class.Object["status"] = map[string]any{"message": "status-only"}
	if err := c.Update(ctx, class); err != nil {
		t.Fatal(err)
	}
	_, second, err := r.preparePlatformVolumes(ctx, "test", runtime, nil)
	if err != nil || first != second {
		t.Fatalf("status-only reference update rolls producer: %v", err)
	}
	secret.Data["password.db"] = []byte("second")
	if err := c.Update(ctx, secret); err != nil {
		t.Fatal(err)
	}
	_, third, err := r.preparePlatformVolumes(ctx, "test", runtime, nil)
	if err != nil || third == second {
		t.Fatalf("Secret update did not change producer stamp: %v", err)
	}

	sts := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "demo-workers-default", Namespace: "test",
		UID: "sts-uid", Generation: 1},
		Spec:   appsv1.StatefulSetSpec{Replicas: ptr.To(int32(1))},
		Status: appsv1.StatefulSetStatus{ObservedGeneration: 1, UpdateRevision: "revision-1"}}
	if err := controllerutil.SetControllerReference(owner, sts, scheme); err != nil {
		t.Fatal(err)
	}
	if err := c.Create(ctx, sts); err != nil {
		t.Fatal(err)
	}
	group := framework.GroupIdentity{ClusterIdentity: framework.ClusterIdentity{Namespace: "test"}, Replicas: 1}
	resources := &pipeline.GroupResources{StatefulSet: *sts}
	out, err := r.observePlatform(ctx, owner, group, resources, runtime)
	if err != nil || out.Diagnostic.State != framework.FactsPending {
		t.Fatalf("missing producer observation: %+v %v", out, err)
	}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: sts.Name + "-0", Namespace: "test", UID: "pod-uid",
		Labels:          map[string]string{appsv1.ControllerRevisionHashLabelKey: "revision-1"},
		OwnerReferences: []metav1.OwnerReference{{UID: sts.UID, Controller: ptr.To(true)}}},
		Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}}}
	claim := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: pod.Name + "-listener", Namespace: "test",
		UID:             "claim-uid",
		OwnerReferences: []metav1.OwnerReference{{UID: pod.UID, Controller: ptr.To(true)}}},
		Spec:   corev1.PersistentVolumeClaimSpec{VolumeName: "listener-pv"},
		Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound}}
	pv := &corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "listener-pv", UID: "pv-uid"},
		Spec: corev1.PersistentVolumeSpec{ClaimRef: &corev1.ObjectReference{Name: claim.Name, Namespace: claim.Namespace,
			UID: claim.UID}}}
	listener := platformObject("listeners.kubedoop.dev", "Listener")
	listener.SetName(claim.Name)
	listener.SetNamespace("test")
	listener.SetUID("listener-uid")
	listener.SetOwnerReferences([]metav1.OwnerReference{{UID: pv.UID}})
	listener.Object["status"] = map[string]any{"ingressAddresses": []any{
		map[string]any{"address": "gateway.test", "ports": map[string]any{"http": int64(31080)}},
	}}
	for _, object := range []client.Object{pod, claim, pv, listener} {
		if err := c.Create(ctx, object); err != nil {
			t.Fatal(err)
		}
	}
	out, err = r.observePlatform(ctx, owner, group, resources, runtime)
	if err != nil || out.Diagnostic.State != framework.FactsResolved || len(out.Listeners) != 1 {
		t.Fatalf("current CSI results were not resolved: %+v %v", out, err)
	}
	if out.Listeners[0].Ports["http"] != 31080 {
		t.Fatal("generated port substituted for observed listener port")
	}
	// External address refresh needs neither a CR edit nor a Pod restart.

	listener.Object["status"] = map[string]any{"ingressAddresses": []any{
		map[string]any{"address": "gateway-new.test", "ports": map[string]any{"http": int64(31081)}},
	}}
	if err := c.Update(ctx, listener); err != nil {
		t.Fatal(err)
	}
	out, err = r.observePlatform(ctx, owner, group, resources, runtime)
	if err != nil || out.Listeners[0].Address != "gateway-new.test" {
		t.Fatalf("address update was cached: %+v %v", out, err)
	}
	// Reusing a former listener's name is not proof for the current volume.
	listener.SetOwnerReferences([]metav1.OwnerReference{{UID: "former-pv"}})
	if err := c.Update(ctx, listener); err != nil {
		t.Fatal(err)
	}
	out, err = r.observePlatform(ctx, owner, group, resources, runtime)
	if err == nil || out.Diagnostic.State != framework.FactsReadError || len(out.Listeners) != 0 {
		t.Fatalf("stale physical identity published addresses: %+v %v", out, err)
	}
}

func TestFinalSecretEnvironmentRefresh(t *testing.T) {
	ctx := context.Background()
	scheme := controllerScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := newTestReconciler(c, scheme, testFacts{})
	pod := &corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Env: []corev1.EnvVar{
		{Name: "TOKEN", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: "token"}, Key: "value", Optional: ptr.To(true),
		}}},
	}}}}
	out, missing, err := r.preparePlatformVolumes(ctx, "test", nil, pod)
	if err != nil || out.Diagnostic.State != framework.FactsResolved {
		t.Fatalf("optional Secret blocked: %+v %v", out, err)
	}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "token", Namespace: "test", UID: "token-uid"},
		Data: map[string][]byte{"value": []byte("private")}}
	if err := c.Create(ctx, secret); err != nil {
		t.Fatal(err)
	}
	out, present, err := r.preparePlatformVolumes(ctx, "test", nil, pod)
	if err != nil || out.Diagnostic.State != framework.FactsResolved || missing == present {
		t.Fatalf("env-only reference did not refresh: %+v %v", out, err)
	}
	pod.Containers[0].Env[0] = corev1.EnvVar{Name: "TOKEN", Value: "override"}
	out, stamp, err := r.preparePlatformVolumes(ctx, "test", nil, pod)
	if err != nil || out != nil || stamp != "" {
		t.Fatal("replaced SecretKeyRef remained a dependency")
	}
}
