package pipeline

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/zncdatadev/operator-go/pkg/framework"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestLifecycleAssemblyOrdersInitializationAndPreservesOverrideAuthority(t *testing.T) {
	r := assemblyRuntimeFixture()
	r.Coordination = &framework.WorkloadCoordination{ProgressDeadline: metav1.Duration{Duration: time.Minute}}
	r.Initializers = []Process{{Name: "initialize", Image: r.Main.Image,
		Command: []string{"initialize"}, Access: r.Main.Access}}
	r.Main.Lifecycle = &corev1.Lifecycle{PreStop: &corev1.LifecycleHandler{
		Exec: &corev1.ExecAction{Command: []string{"drain"}}}}
	r.Main.StartupProbe = &corev1.Probe{ProbeHandler: corev1.ProbeHandler{
		Exec: &corev1.ExecAction{Command: []string{"ready"}}}}
	if err := ValidateRuntime(r); err != nil {
		t.Fatal(err)
	}
	identity := GroupIdentity{ClusterIdentity: ClusterIdentity{Name: "life", Namespace: "test"},
		Role: "workers", Name: "default", Replicas: 2}
	resources, _, checks, err := buildGroup(identity, CommonConfig{}, ResolvedImage{}, r, GroupSource{},
		AssemblyOptions{MaterializerImage: "helper:1"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(resources.Coordination, r.Coordination) {
		t.Fatal("final resource clone lost lifecycle coordination")
	}
	resources.Coordination.ShutdownPriority = 7
	if r.Coordination.ShutdownPriority != 0 {
		t.Fatal("final resource policy aliases runtime")
	}
	set := resources.StatefulSet
	if len(set.Spec.Template.Spec.InitContainers) != 2 ||
		set.Spec.Template.Spec.InitContainers[0].Name != materializerContainerName ||
		set.Spec.Template.Spec.InitContainers[1].Name != "initialize" ||
		set.Spec.Template.Spec.Containers[0].Lifecycle.PreStop.Exec.Command[0] != "drain" ||
		set.Spec.PodManagementPolicy != appsv1.OrderedReadyPodManagement ||
		set.Spec.UpdateStrategy.Type != appsv1.RollingUpdateStatefulSetStrategyType {
		t.Fatalf("initialization/lifecycle assembly missing: %+v", set.Spec)
	}
	for _, check := range checks {
		if check.Subject == lifecycleSubject && check.State != Consistent {
			t.Fatal("unchanged initialization incorrectly invalidated", check)
		}
	}
	if len(checks) == 0 {
		t.Fatal("no final checks")
	}
	source := GroupSource{Overrides: &Overrides{PodOverrides: json.RawMessage(`{"spec":{"containers":[
 {"name":"trino","lifecycle":{"preStop":{"exec":{"command":["custom"]}}}}]}}`)}}
	_, _, checks, err = buildGroup(identity, CommonConfig{}, ResolvedImage{}, r, source,
		AssemblyOptions{MaterializerImage: "helper:1"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, check := range checks {
		if check.Subject == lifecycleSubject && check.State == Unknown {
			found = true
		}
	}
	if !found {
		t.Fatal("override won without invalidating lifecycle premise")
	}
	cloned := CloneRuntime(r)
	cloned.Initializers[0].Command[0] = "changed"
	cloned.Main.Lifecycle.PreStop.Exec.Command[0] = "changed"
	cloned.Main.StartupProbe.Exec.Command[0] = "changed"
	cloned.Coordination.ProgressDeadline.Duration = time.Second
	if r.Initializers[0].Command[0] != "initialize" || r.Main.Lifecycle.PreStop.Exec.Command[0] != "drain" ||
		r.Main.StartupProbe.Exec.Command[0] != "ready" || r.Coordination.ProgressDeadline.Duration != time.Minute {
		t.Fatal("mutable lifecycle alias")
	}
}
