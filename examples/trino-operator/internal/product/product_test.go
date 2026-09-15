package product

import (
	"slices"
	"strings"
	"testing"

	"github.com/zncdatadev/operator-go/pkg/framework"
)

func effectiveInput() framework.EffectiveInput[TrinoConfig, TrinoClusterConfig, TrinoFacts] {
	definition := Definition()
	cluster := framework.ClusterIdentity{Name: "sample", Namespace: "test"}
	coordinator := framework.GroupIdentity{ClusterIdentity: cluster, Role: trinoCoordinatorRole, Name: "default", Replicas: 1}
	worker := framework.GroupIdentity{ClusterIdentity: cluster, Role: trinoWorkerRole, Name: "default", Replicas: 2}
	coordinatorConfig, workerConfig := definition.Roles[trinoCoordinatorRole].Config, definition.Roles[trinoWorkerRole].Config
	return framework.EffectiveInput[TrinoConfig, TrinoClusterConfig, TrinoFacts]{
		Group: worker, Config: workerConfig, ClusterConfig: definition.ClusterConfigDefaults, Facts: BaseFacts(),
		Image: framework.ResolvedImage{Reference: "quay.io/zncdatadev/trino:476-kubedoop0.0.0-dev"},
		Topology: []framework.ResolvedGroup[TrinoConfig]{
			{Group: coordinator, Config: &coordinatorConfig}, {Group: worker, Config: &workerConfig},
		},
	}
}

func TestNativeDefinitionUsesPodUIDAndExplicitImageIdentity(t *testing.T) {
	input := effectiveInput()
	definition := Definition()
	if err := definition.ValidateInput(input); err != nil {
		t.Fatal(err)
	}
	description, err := definition.GenerateGroup(input)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(description.Main.Command, []string{trinoLauncher}) ||
		*description.Main.Identity.RunAsUser != 1001 || *description.SharedGroup != 1001 {
		t.Fatalf("Trino image execution contract changed: %+v", description.Main)
	}
	if len(description.Main.Env) != 1 || description.Main.Env[0].Name != "TRINO_NODE_ID" ||
		description.Main.Env[0].ValueFrom.FieldRef.FieldPath != "metadata.uid" {
		t.Fatal("default process no longer supplies the Pod UID to native environment interpolation")
	}
	node := findFile(description.Files, trinoConfigDirectory, trinoNodeFile)
	id, known := literalProperty(node, trinoNodeIDKey)
	if !known || id != "${ENV:TRINO_NODE_ID}" {
		t.Fatalf("default node identity is not native Pod UID interpolation: %q", id)
	}
	config := findFile(description.Files, trinoConfigDirectory, "config.properties")
	uri, known := literalProperty(config, "discovery.uri")
	if !known || uri != "http://sample-coordinators-default.test.svc:8080" {
		t.Fatalf("discovery did not consume complete topology: %q", uri)
	}
	if len(description.LogOutputs) != 1 || findFile(description.Files, trinoConfigDirectory, "catalog/tpch.properties") == nil {
		t.Fatal("native file outputs or bundled catalog missing")
	}
	input.Config.Common.Logging.Containers[trinoName] = framework.ContainerLogging{
		Console: framework.Logger{Level: trinoInfoLevel}, File: framework.Logger{Level: trinoOffLevel},
		Loggers: map[string]framework.Logger{trinoRootLogger: {Level: trinoInfoLevel}},
	}
	description, err = definition.GenerateGroup(input)
	if err != nil || len(description.LogOutputs) != 0 {
		t.Fatalf("OFF native file sink must withdraw its actual output: %v", err)
	}
	if _, known := literalProperty(findFile(description.Files, trinoConfigDirectory, "config.properties"), "log.path"); known {
		t.Fatal("OFF native file sink retained log.path")
	}
}

func TestNativeLoggingThresholdsAndUnsupportedValues(t *testing.T) {
	for _, tc := range []struct {
		name, console, file, logger, root string
		wantError                         bool
	}{
		{"file-default", trinoOffLevel, trinoTraceLevel, trinoDebugLevel, trinoInfoLevel, false},
		{"single-clamp", trinoWarnLevel, trinoOffLevel, trinoInfoLevel, trinoWarnLevel, false},
		{"equal-sinks", trinoErrorLevel, trinoErrorLevel, trinoDebugLevel, trinoErrorLevel, false},
		{"unequal-sinks", trinoInfoLevel, trinoWarnLevel, trinoInfoLevel, "", true},
		{"fatal", trinoOffLevel, "FATAL", trinoInfoLevel, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logging := framework.Logging{Containers: map[string]framework.ContainerLogging{trinoName: {
				Console: framework.Logger{Level: tc.console}, File: framework.Logger{Level: tc.file},
				Loggers: map[string]framework.Logger{trinoRootLogger: {Level: trinoInfoLevel}, trinoLoggerName: {Level: tc.logger}},
			}}}
			plan, err := resolveTrinoLogging(logging)
			if (err != nil) != tc.wantError {
				t.Fatalf("unexpected native mapping: %+v %v", plan, err)
			}
			if err == nil && plan.Levels[""] != framework.Literal(tc.root) {
				t.Fatalf("native empty root key has incorrect threshold: %+v", plan.Levels)
			}
		})
	}
}

func TestDiscoveryWaitsForDeclaredCoordinatorAndWithdrawsAbsentRole(t *testing.T) {
	in := framework.ClusterOutputInput[TrinoClusterConfig, TrinoFacts]{Cluster: effectiveInput().Group.ClusterIdentity}
	output, err := trinoDiscovery(in)
	if err != nil || output.State != framework.ClusterOutputReady || len(output.ConfigMaps) != 0 {
		t.Fatal("no declared coordinator should explicitly withdraw shared discovery")
	}
	in.Groups = []framework.GroupOutcome{{Group: effectiveInput().Topology[0].Group, Error: "invalid input"}}
	output, err = trinoDiscovery(in)
	if err != nil || output.State != framework.ClusterOutputPending || output.Reason == "" || len(output.ConfigMaps) != 0 {
		t.Fatal("failed declared coordinator should keep shared output pending")
	}
	in.Groups[0].Error = ""
	in.Groups[0].GeneratedEndpoints = []framework.Endpoint{{Name: trinoHTTPEndpoint, Port: 8080}}
	output, err = trinoDiscovery(in)
	if err != nil || output.State != framework.ClusterOutputReady || len(output.ConfigMaps) != 1 ||
		!strings.HasSuffix(output.ConfigMaps[0].Name, "-discovery") {
		t.Fatalf("generated discovery is incomplete: %+v %v", output, err)
	}
}
