package pipeline

import (
	"encoding/json"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/zncdatadev/operator-go/pkg/framework"
)

func TestSharedConfigMapsRespectUnavailableGroupSlots(t *testing.T) {
	for _, unavailable := range []string{"invalid-config", "pending-facts"} {
		t.Run(unavailable, func(t *testing.T) {
			definition := TrinoDefinition()
			source := SourceSnapshot[TrinoFacts]{
				Cluster: ClusterIdentity{Name: "reserved", Namespace: "fixture"},
				Roles:   []RoleSource{{Name: trinoCoordinatorRole}, {Name: trinoWorkerRole}},
				Groups: []GroupSource{
					{Role: trinoCoordinatorRole, Name: "default", Replicas: 1},
					{Role: trinoWorkerRole, Name: "blocked", Replicas: 1},
				},
			}
			if unavailable == "invalid-config" {
				source.Groups[1].Config = json.RawMessage(`{"httpPort":"invalid"}`)
			}
			reservedName := "reserved-workers-blocked"
			definition.GenerateCluster = func(in framework.ClusterOutputInput[TrinoClusterConfig, TrinoFacts]) (
				ClusterOutput, error,
			) {
				return ClusterOutput{State: ClusterOutputReady, ConfigMaps: []corev1.ConfigMap{{
					ObjectMeta: metav1.ObjectMeta{Name: reservedName, Namespace: in.Cluster.Namespace},
					Data:       map[string]string{"shared": "must not occupy the unavailable group slot"},
				}}}, nil
			}
			prepared, err := PrepareInputs(definition, source)
			if err != nil {
				t.Fatal(err)
			}
			var facts map[GroupKey]framework.FactResult[TrinoFacts]
			if unavailable == "pending-facts" {
				facts = map[GroupKey]framework.FactResult[TrinoFacts]{
					{Role: trinoCoordinatorRole, Name: "default"}: {
						Value: &TrinoFacts{}, Diagnostic: FactDiagnostic{State: FactsResolved},
					},
					{Role: trinoWorkerRole, Name: "blocked"}: {
						Diagnostic: FactDiagnostic{State: FactsPending, Reason: "CatalogMissing"},
					},
				}
			}
			plan, err := BuildPreparedResources(definition, prepared, facts, assemblyBuildOptions())
			if err != nil {
				t.Fatalf("shared conflict must preserve independent group plans: %v", err)
			}
			if !strings.Contains(plan.ClusterError, "reserved ConfigMap slot") ||
				!strings.Contains(plan.ClusterError, reservedName) || len(plan.ClusterOutput.ConfigMaps) != 0 ||
				plan.ClusterOutput.State != "" {
				t.Fatalf("shared output occupied a desired group slot: output=%+v error=%q",
					plan.ClusterOutput, plan.ClusterError)
			}
			if len(plan.Groups) != 2 || plan.Groups[0].Resources == nil || plan.Groups[0].Outcome.Error != "" ||
				plan.Groups[1].Resources != nil || len(plan.Roles) != 2 {
				t.Fatalf("shared conflict changed independent group/role results: %+v", plan)
			}
			blocked := plan.Groups[1].Outcome
			if unavailable == "invalid-config" && blocked.Error == "" {
				t.Fatal("test did not exercise a failed configuration")
			}
			if unavailable == "pending-facts" && (blocked.Error != "" || blocked.Facts == nil ||
				blocked.Facts.State != FactsPending) {
				t.Fatalf("test lost the pending facts result: %+v", blocked)
			}
		})
	}
}
