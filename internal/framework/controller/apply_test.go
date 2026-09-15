package controller

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func applyTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	for _, register := range []func(*runtime.Scheme) error{corev1.AddToScheme, appsv1.AddToScheme} {
		if err := register(scheme); err != nil {
			t.Fatal(err)
		}
	}
	return scheme
}

func applyTestOwner() *corev1.ConfigMap {
	return &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "owner", Namespace: "test", UID: "owner-uid"}}
}

func applyTestConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "workload", Namespace: "test",
		Labels: map[string]string{"owned": "v1"}, Annotations: map[string]string{"owned-note": "v1"}},
		Data: map[string]string{"keep": "old", "withdraw": "old"}, BinaryData: map[string][]byte{"old": {1, 2}}}
}

func applyTestStatefulSet() *appsv1.StatefulSet {
	return &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "workload", Namespace: "test"},
		Spec: appsv1.StatefulSetSpec{ServiceName: "headless", Selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{"app": "test"}}, Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test", "withdraw": "old"},
				Annotations: map[string]string{"withdraw": "old"}},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "example.invalid/test:1",
				ReadinessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{
					TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt32(8080)}}}}}},
		}}}
}

func applyTestGet(t *testing.T, c client.Client, object client.Object) {
	t.Helper()
	if err := c.Get(t.Context(), client.ObjectKeyFromObject(object), object); err != nil {
		t.Fatal(err)
	}
}

func applyTestChanged(t *testing.T, c client.Client, desired client.Object, scheme *runtime.Scheme, want bool) {
	t.Helper()
	changed, err := ApplyObject(t.Context(), c, applyTestOwner(), desired, scheme)
	if err != nil || changed != want {
		t.Fatalf("apply changed=%v want=%v error=%v", changed, want, err)
	}
}

func TestApplyConfigMapOwnershipMetadataAndFieldWithdrawal(t *testing.T) {
	scheme := applyTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	desired := applyTestConfigMap()
	before := desired.DeepCopy()
	applyTestChanged(t, c, desired, scheme, true)
	if !reflect.DeepEqual(desired, before) {
		t.Fatal("apply mutated the desired object")
	}
	live := desired.DeepCopy()
	applyTestGet(t, c, live)
	if !metav1.IsControlledBy(live, applyTestOwner()) {
		t.Fatal("create did not set the controller reference")
	}
	live.Labels["foreign"] = "keep"
	live.Annotations["foreign"] = "keep"
	live.Finalizers = []string{"foreign/finalizer"}
	if err := c.Update(t.Context(), live); err != nil {
		t.Fatal(err)
	}
	desired.Labels = nil
	desired.Annotations = map[string]string{"foreign": "desired-wins"}
	desired.Data = map[string]string{"keep": "new"}
	desired.BinaryData = nil
	applyTestChanged(t, c, desired, scheme, true)
	applyTestGet(t, c, live)
	if live.Labels["foreign"] != "keep" || live.Labels["owned"] != "" ||
		live.Annotations["owned-note"] != "" || live.Annotations["foreign"] != "desired-wins" ||
		len(live.Data) != 1 || live.Data["keep"] != "new" || len(live.BinaryData) != 0 ||
		!reflect.DeepEqual(live.Finalizers, []string{"foreign/finalizer"}) {
		t.Fatalf("withdrawal or foreign metadata preservation failed: %+v", live)
	}
	rv := live.ResourceVersion
	applyTestChanged(t, c, desired, scheme, false)
	applyTestGet(t, c, live)
	if live.ResourceVersion != rv {
		t.Fatal("no-op apply persisted another resource version")
	}
}

// This is a fake admission boundary, not a reimplementation used by apply.
// Real API defaulting and unchanged resourceVersion are covered by envtest.
func applyFakeDefaults(object client.Object) {
	if item, ok := object.(*appsv1.StatefulSet); ok {
		if item.Spec.PodManagementPolicy == "" {
			item.Spec.PodManagementPolicy = appsv1.OrderedReadyPodManagement
		}
		if item.Spec.Replicas == nil {
			value := int32(1)
			item.Spec.Replicas = &value
		}
		if item.Spec.Template.Spec.DNSPolicy == "" {
			item.Spec.Template.Spec.DNSPolicy = corev1.DNSClusterFirst
		}
		for i := range item.Spec.Template.Spec.Containers {
			if item.Spec.Template.Spec.Containers[i].TerminationMessagePath == "" {
				item.Spec.Template.Spec.Containers[i].TerminationMessagePath = "/dev/termination-log"
			}
		}
	}
}

func applyDryRun(options []client.UpdateOption) bool {
	settings := &client.UpdateOptions{}
	settings.ApplyOptions(options)
	return slices.Contains(settings.DryRun, metav1.DryRunAll)
}

func TestApplyCanonicalDefaultsPreserveStatusAndRemovePodOverrides(t *testing.T) {
	scheme := applyTestScheme(t)
	dryRuns, writes := 0, 0
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&appsv1.StatefulSet{}).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, object client.Object, opts ...client.CreateOption) error {
				applyFakeDefaults(object)
				return c.Create(ctx, object, opts...)
			},
			Update: func(ctx context.Context, c client.WithWatch, object client.Object, opts ...client.UpdateOption) error {
				applyFakeDefaults(object)
				if applyDryRun(opts) {
					dryRuns++
				} else {
					writes++
				}
				return c.Update(ctx, object, opts...)
			},
		}).Build()
	desired := applyTestStatefulSet()
	applyTestChanged(t, c, desired, scheme, true)
	live := desired.DeepCopy()
	applyTestGet(t, c, live)
	rv := live.ResourceVersion
	applyTestChanged(t, c, desired, scheme, false)
	applyTestGet(t, c, live)
	if live.ResourceVersion != rv || dryRuns != 1 || writes != 0 {
		t.Fatal("API defaults must not cause repeated persisted updates")
	}
	live.Spec.Template.Labels["foreign"] = "keep"
	live.Spec.Template.Annotations["restarter.example/change"] = "keep"
	if err := c.Update(t.Context(), live); err != nil {
		t.Fatal(err)
	}
	live.Status.ReadyReplicas = 7
	if err := c.Status().Update(t.Context(), live); err != nil {
		t.Fatal(err)
	}
	delete(desired.Spec.Template.Labels, "withdraw")
	desired.Spec.Template.Annotations = nil
	desired.Spec.Template.Spec.Containers[0].ReadinessProbe = nil
	desired.Status.ReadyReplicas = 999
	applyTestChanged(t, c, desired, scheme, true)
	applyTestGet(t, c, live)
	if live.Status.ReadyReplicas != 7 || live.Spec.Template.Spec.Containers[0].ReadinessProbe != nil ||
		live.Spec.Template.Labels["withdraw"] != "" || live.Spec.Template.Labels["foreign"] != "keep" ||
		live.Spec.Template.Annotations["withdraw"] != "" ||
		live.Spec.Template.Annotations["restarter.example/change"] != "keep" {
		t.Fatalf("owned withdrawal or status/foreign preservation failed: %+v", live)
	}
}

func TestApplyPreservesServiceAllocations(t *testing.T) {
	scheme := applyTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&corev1.Service{}).Build()
	desired := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "workload", Namespace: "test"},
		Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer,
			ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyLocal,
			Ports:                 []corev1.ServicePort{{Name: "http", Port: 8080, TargetPort: intstr.FromInt32(8080)}}}}
	applyTestChanged(t, c, desired, scheme, true)
	live := desired.DeepCopy()
	applyTestGet(t, c, live)
	policy := corev1.IPFamilyPolicySingleStack
	live.Spec.ClusterIP, live.Spec.ClusterIPs = "10.0.0.42", []string{"10.0.0.42"}
	live.Spec.IPFamilies, live.Spec.IPFamilyPolicy = []corev1.IPFamily{corev1.IPv4Protocol}, &policy
	live.Spec.HealthCheckNodePort, live.Spec.Ports[0].NodePort = 32042, 31042
	if err := c.Update(t.Context(), live); err != nil {
		t.Fatal(err)
	}
	live.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{Hostname: "allocated.example"}}
	if err := c.Status().Update(t.Context(), live); err != nil {
		t.Fatal(err)
	}
	applyTestChanged(t, c, desired, scheme, false)
	desired.Spec.Selector = map[string]string{"changed": "yes"}
	desired.Spec.Ports[0].Port = 9090
	applyTestChanged(t, c, desired, scheme, true)
	applyTestGet(t, c, live)
	if live.Spec.ClusterIP != "10.0.0.42" || !reflect.DeepEqual(live.Spec.ClusterIPs, []string{"10.0.0.42"}) ||
		live.Spec.IPFamilyPolicy == nil || *live.Spec.IPFamilyPolicy != policy ||
		!reflect.DeepEqual(live.Spec.IPFamilies, []corev1.IPFamily{corev1.IPv4Protocol}) ||
		live.Spec.Ports[0].NodePort != 31042 || live.Spec.HealthCheckNodePort != 32042 ||
		len(live.Status.LoadBalancer.Ingress) != 1 {
		t.Fatalf("Service allocations or status changed: %+v", live)
	}
}

func TestApplyRefusesAdoptionAndImmutableChanges(t *testing.T) {
	scheme := applyTestScheme(t)
	controller := true
	for _, reference := range []*metav1.OwnerReference{
		nil,
		{APIVersion: "v1", Kind: "ConfigMap", Name: "other", UID: "other", Controller: &controller},
		{APIVersion: "v1", Kind: "ConfigMap", Name: "owner", UID: "owner-uid"},
		{APIVersion: "v1", Kind: "Service", Name: "owner", UID: "owner-uid", Controller: &controller},
		{APIVersion: "wrong/v1", Kind: "ConfigMap", Name: "owner", UID: "owner-uid", Controller: &controller},
	} {
		live := applyTestConfigMap()
		if reference != nil {
			live.OwnerReferences = []metav1.OwnerReference{*reference}
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(live).Build()
		changed, err := ApplyObject(t.Context(), c, applyTestOwner(), applyTestConfigMap(), scheme)
		if err == nil || changed || !strings.Contains(err.Error(), "refusing to adopt") {
			t.Fatalf("unowned/foreign object was adopted: changed=%v error=%v", changed, err)
		}
	}
	for field, mutate := range map[string]func(*appsv1.StatefulSet){
		"spec.serviceName": func(s *appsv1.StatefulSet) { s.Spec.ServiceName = "other" },
		"spec.selector": func(s *appsv1.StatefulSet) {
			s.Spec.Selector.MatchLabels["app"] = "other"
		},
		"spec.podManagementPolicy": func(s *appsv1.StatefulSet) { s.Spec.PodManagementPolicy = appsv1.ParallelPodManagement },
		"spec.volumeClaimTemplates": func(s *appsv1.StatefulSet) {
			s.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{{ObjectMeta: metav1.ObjectMeta{Name: "data"}}}
		},
	} {
		t.Run(field, func(t *testing.T) {
			c := fake.NewClientBuilder().WithScheme(scheme).Build()
			desired := applyTestStatefulSet()
			applyTestChanged(t, c, desired, scheme, true)
			mutate(desired)
			changed, err := ApplyObject(t.Context(), c, applyTestOwner(), desired, scheme)
			if err == nil || changed || !strings.Contains(err.Error(), field) {
				t.Fatalf("immutable change was not explicit: changed=%v error=%v", changed, err)
			}
		})
	}
}

func TestApplyRetriesFreshReadsAndRechecksOwnership(t *testing.T) {
	for _, changeOwner := range []bool{false, true} {
		scheme := applyTestScheme(t)
		base := fake.NewClientBuilder().WithScheme(scheme).Build()
		desired := applyTestConfigMap()
		applyTestChanged(t, base, desired, scheme, true)
		attempts, gets := 0, 0
		c := interceptor.NewClient(base, interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object,
				opts ...client.GetOption) error {
				gets++
				return c.Get(ctx, key, obj, opts...)
			},
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				if applyDryRun(opts) {
					return c.Update(ctx, obj, opts...)
				}
				attempts++
				if attempts == 1 {
					concurrent := applyTestConfigMap()
					if err := c.Get(ctx, client.ObjectKeyFromObject(obj), concurrent); err != nil {
						return err
					}
					concurrent.Annotations["concurrent"] = "preserve"
					if changeOwner {
						concurrent.OwnerReferences[0].UID = types.UID("replacement-owner")
					}
					if err := c.Update(ctx, concurrent); err != nil {
						return err
					}
					return apierrors.NewConflict(schema.GroupResource{Resource: "configmaps"}, obj.GetName(), errors.New("race"))
				}
				return c.Update(ctx, obj, opts...)
			},
		})
		desired.Data["keep"] = "new"
		changed, err := ApplyObject(t.Context(), c, applyTestOwner(), desired, scheme)
		if gets != 2 || changed == changeOwner || (err != nil) != changeOwner {
			t.Fatalf("retry failed: changed=%v gets=%d changeOwner=%v error=%v", changed, gets, changeOwner, err)
		}
		live := applyTestConfigMap()
		applyTestGet(t, base, live)
		want := "new"
		if changeOwner {
			want = "old"
		}
		if live.Data["keep"] != want || live.Annotations["concurrent"] != "preserve" {
			t.Fatal("retry ignored a newer object or overwrote its metadata")
		}
	}
}

func TestApplyRejectsDamagedMetadataRecordsAndReservedKeys(t *testing.T) {
	scheme := applyTestScheme(t)
	for _, record := range []string{
		`missing`, `null`, `{}`, `{"object":null,"template":{}}`, `{"object":{"labels":null},"template":{}}`,
		`{"object":{"unknown":[]},"template":{}}`, `{"object":{},"object":{},"template":{}}`,
		`{"object":{"labels":["owned","owned"]},"template":{}}`,
		`{"object":{},"template":{}} {}`, `{"object":{"labels":["bad key"]},"template":{}}`,
	} {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		desired := applyTestConfigMap()
		applyTestChanged(t, c, desired, scheme, true)
		live := desired.DeepCopy()
		applyTestGet(t, c, live)
		if record == "missing" {
			delete(live.Annotations, ManagedMetadataAnnotation)
		} else {
			live.Annotations[ManagedMetadataAnnotation] = record
		}
		if err := c.Update(t.Context(), live); err != nil {
			t.Fatal(err)
		}
		if changed, err := ApplyObject(t.Context(), c, applyTestOwner(), desired, scheme); err == nil || changed {
			t.Fatalf("damaged record accepted: %s changed=%v error=%v", record, changed, err)
		}
	}
	for _, template := range []bool{false, true} {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		desired := applyTestStatefulSet()
		if template {
			desired.Spec.Template.Annotations[ManagedMetadataAnnotation] = "user-data"
		} else {
			desired.Annotations = map[string]string{ManagedMetadataAnnotation: "user-data"}
		}
		if changed, err := ApplyObject(t.Context(), c, applyTestOwner(), desired, scheme); err == nil || changed ||
			!strings.Contains(err.Error(), "reserved") {
			t.Fatalf("reserved key accepted: changed=%v error=%v", changed, err)
		}
	}
}
