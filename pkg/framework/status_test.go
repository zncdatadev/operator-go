package framework_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/zncdatadev/operator-go/pkg/framework"
)

func TestStatusDeepCopyIsolatesFixedMutableFields(t *testing.T) {
	count := int32(0)
	original := framework.ReconcileStatus{
		ObservedGeneration: 7,
		Conditions: []metav1.Condition{{Type: "Stopped", Status: metav1.ConditionTrue, ObservedGeneration: 7,
			LastTransitionTime: metav1.NewTime(time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC))}},
		Roles: []framework.RoleReconcileStatus{{Name: "workers", Applied: true}},
		Groups: []framework.GroupReconcileStatus{{Name: "default", Role: "workers", ExecutionReplicas: &count,
			Checks: []framework.Check{{Subject: "files", State: framework.Unknown}},
			Facts: &framework.FactDiagnostic{State: framework.FactsResolved, Observed: []framework.FactObject{{
				APIVersion: "v1", Kind: "ConfigMap", Name: "catalogs", UID: "source-uid", ResourceVersion: "8"}}},
		}},
	}
	before, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	copy := original.DeepCopy()
	copy.Conditions[0].ObservedGeneration = 99
	copy.Conditions[0].LastTransitionTime = metav1.NewTime(time.Now())
	copy.Roles[0].Name = "changed"
	copy.Groups[0].Name = "changed"
	*copy.Groups[0].ExecutionReplicas = 4
	copy.Groups[0].Checks[0].Reason = "changed"
	copy.Groups[0].Facts.State = framework.FactsReadError
	copy.Groups[0].Facts.Observed[0].UID = "another-object"
	after, err := json.Marshal(original)
	if err != nil || string(before) != string(after) {
		t.Fatalf("copy mutation changed original status: %s, %v", after, err)
	}
	for _, fragment := range []string{`"executionReplicas":0`, `"desiredReplicas":0`, `"readyReplicas":0`,
		`"applied":false`, `"observedGeneration":7`, `"state":"unknown"`, `"resourceVersion":"8"`} {
		if !strings.Contains(string(before), fragment) {
			t.Fatalf("status wire contract lost %s", fragment)
		}
	}
}

func TestStatusDeepCopyPreservesNilAndEmptyCollections(t *testing.T) {
	if (*framework.ReconcileStatus)(nil).DeepCopy() != nil {
		t.Fatal("nil status became a non-nil value")
	}
	for _, original := range []framework.ReconcileStatus{
		{}, {Conditions: []metav1.Condition{}, Roles: []framework.RoleReconcileStatus{},
			Groups: []framework.GroupReconcileStatus{}},
		{Groups: []framework.GroupReconcileStatus{{Checks: []framework.Check{},
			Facts: &framework.FactDiagnostic{Observed: []framework.FactObject{}}}}},
	} {
		if !reflect.DeepEqual(original, *original.DeepCopy()) {
			t.Fatalf("status copy changed collection presence: %+v", original)
		}
	}
}

// The public facts constraint accepts ordinary Kubernetes API objects without
// requiring a write client or an import of the framework's controller package.
var _ framework.FactResource = (*corev1.ConfigMap)(nil)
