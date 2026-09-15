package controller

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/zncdatadev/operator-go/internal/framework/pipeline"
	"github.com/zncdatadev/operator-go/pkg/framework"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestVectorRuntimeDependenciesAndSingleGeneration(t *testing.T) {
	ctx := context.Background()
	cr, scheme := controllerInput(), controllerScheme(t)
	reads := 0
	c := retirementClient(scheme, cr, nil, interceptor.Funcs{
		Get: func(ctx context.Context, underlying client.WithWatch, key client.ObjectKey, object client.Object,
			options ...client.GetOption,
		) error {
			if key.Name == "destination" {
				reads++
			}
			return underlying.Get(ctx, key, object, options...)
		},
	})
	r := newTestReconciler(c, scheme, testFacts{})
	role := r.Definition.Roles["workers"]
	role.Config.Common.Logging.EnableVectorAgent = true
	r.Definition.Roles["workers"] = role
	r.Assembly = framework.AssemblyOptions{MaterializerImage: "example.invalid/materializer:test",
		VectorImage: "example.invalid/vector:test"}
	generated := map[string]int{}
	r.Definition.GenerateGroup = func(in framework.EffectiveInput[testConfig, testClusterConfig, testFacts]) (
		framework.RuntimeDescription, error,
	) {
		generated[in.Group.Name]++
		out := framework.RuntimeDescription{ConfigDirectory: "config", Main: framework.Process{Name: "main",
			Command: []string{"run"}, Access: []framework.DirectoryAccess{
				{Directory: "config", MountPath: "/config", ReadOnly: true}, {Directory: "logs", MountPath: "/logs"}}},
			Directories: []framework.Directory{{Name: "config"}, {Name: "logs"}},
			Files:       []framework.File{{Directory: "config", Path: "application.conf", Content: framework.Text("ready")}}}
		if in.Group.Name != "nofiles" {
			out.LogOutputs = []framework.LogOutput{{Container: "main", Directory: "logs", RelativePath: "server.log"}}
		}
		return out, nil
	}
	source := pipeline.SourceSnapshot[testFacts]{
		Cluster:       framework.ClusterIdentity{Name: cr.Name, Namespace: cr.Namespace},
		ClusterConfig: json.RawMessage(`{"vectorAgentConfigMap":"destination"}`),
		Roles:         []pipeline.RoleSource{{Name: "workers"}}, Groups: []pipeline.GroupSource{
			{Role: "workers", Name: "enabled", Replicas: 1},
			{Role: "workers", Name: "disabled", Replicas: 1, Config: json.RawMessage(`{"logging":{"enableVectorAgent":false}}`)},
			{Role: "workers", Name: "nofiles", Replicas: 1},
		}}
	plan, err := r.buildGroupPlan(ctx, source, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, group := range plan.Groups {
		name := group.Outcome.Group.Name
		if generated[name] != 1 {
			t.Fatalf("%s generated %d times", name, generated[name])
		}
		if name == "enabled" {
			if group.Resources != nil || group.Outcome.Facts == nil || group.Outcome.Facts.State != framework.FactsPending {
				t.Fatalf("enabled collector did not wait: %+v", group.Outcome)
			}
		} else if group.Resources == nil || group.Outcome.Facts != nil {
			t.Fatalf("unselected consumer acquired a dependency: %+v", group.Outcome)
		}
	}
	if reads != 1 {
		t.Fatalf("destination reads = %d, want one per pass", reads)
	}
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "destination", Namespace: cr.Namespace,
		UID: "destination-uid"},
		Data: map[string]string{"ADDRESS": "receiver-a.test.svc:6000"}}
	if err := c.Create(ctx, cm); err != nil {
		t.Fatal(err)
	}
	first := vectorPlanAddress(t, r, source, cm.ResourceVersion)
	cm.Data["ADDRESS"] = "receiver-b.test.svc:6000"
	if err := c.Update(ctx, cm); err != nil {
		t.Fatal(err)
	}
	second := vectorPlanAddress(t, r, source, cm.ResourceVersion)
	if first != "receiver-a.test.svc:6000" || second != "receiver-b.test.svc:6000" {
		t.Fatalf("live discovery was cached across passes: %q -> %q", first, second)
	}
	reads = 0
	source.Groups = source.Groups[1:]
	if _, err := r.buildGroupPlan(ctx, source, nil); err != nil {
		t.Fatal(err)
	}
	if reads != 0 {
		t.Fatal("groups without an actual selected collector still read the destination")
	}
}

func vectorPlanAddress[CR client.Object](t *testing.T,
	r *Reconciler[CR, testConfig, testClusterConfig, testFacts], source pipeline.SourceSnapshot[testFacts], version string,
) string {
	t.Helper()
	plan, err := r.buildGroupPlan(context.Background(), source, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, group := range plan.Groups {
		if group.Outcome.Group.Name != "enabled" {
			continue
		}
		if group.Resources == nil || group.Outcome.Facts == nil || group.Outcome.Facts.State != framework.FactsResolved ||
			len(group.Outcome.Facts.Observed) != 1 || group.Outcome.Facts.Observed[0].UID != "destination-uid" ||
			group.Outcome.Facts.Observed[0].ResourceVersion != version {
			t.Fatalf("resolved destination/provenance missing: %+v", group.Outcome)
		}
		for _, container := range group.Resources.StatefulSet.Spec.Template.Spec.Containers {
			if container.Name == "vector" {
				return container.Env[0].Value
			}
		}
	}
	t.Fatal("no selected Vector")
	return ""
}
