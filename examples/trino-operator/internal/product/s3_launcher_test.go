package product

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zncdatadev/operator-go/internal/framework/pipeline"
	"github.com/zncdatadev/operator-go/pkg/framework"
	"github.com/zncdatadev/operator-go/pkg/framework/input"
)

// Exercise the real internal pipeline with the actual product adapter: the
// regression crossed generation, inherited CLI overrides and final Pod patches.
func TestS3LauncherRetainsCommandAcrossOverrideChannels(t *testing.T) {
	roleArgs := []string{testTrinoEtcArgument, "-D", "operator.go.update=role", trinoRunArgument}
	groupArgs := []string{testTrinoEtcArgument, "-D", "operator.go.update=group", trinoRunArgument}
	emptyArgs := []string{}
	configured := effectiveInput()
	configured.Config.Product.Hive = HiveCatalog{MetastoreURI: "thrift://metastore.test:9083",
		S3: framework.S3Connection{Type: framework.S3Inline, Inline: framework.S3Endpoint{Host: "minio.test", Port: 9000,
			Credentials: framework.S3Credentials{SecretName: s3CredentialSlot}}}}
	resolved, err := ResolveFacts(t.Context(), nil, framework.FactInput[TrinoConfig, TrinoClusterConfig, TrinoFacts]{
		Group: configured.Group, Config: configured.Config, ClusterConfig: configured.ClusterConfig, Shared: configured.Facts})
	if err != nil || resolved.Value == nil {
		t.Fatalf("S3 fixture facts did not resolve: %v", err)
	}
	config, err := json.Marshal(map[string]any{"hive": map[string]any{
		"metastoreURI": configured.Config.Product.Hive.MetastoreURI,
		"s3":           map[string]any{"type": framework.S3Inline, "inline": configured.Config.Product.Hive.S3.Inline}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		group *[]string
		pod   json.RawMessage
		want  []string
	}{
		{name: "inherited-role-cli", want: roleArgs},
		{name: "group-cli", group: &groupArgs, want: groupArgs},
		{name: "explicit-empty-cli", group: &emptyArgs, want: emptyArgs},
		{name: "role-pod-above-group-cli", group: &groupArgs, want: []string{"--etc-dir=/pod/config", trinoRunArgument},
			pod: json.RawMessage(`{"spec":{"containers":[{"name":"trino","args":["--etc-dir=/pod/config","run"]}]}}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			projection := input.Projection{Cluster: framework.ClusterIdentity{Name: "sample", Namespace: "test"},
				Roles: []input.Role{
					{Name: trinoCoordinatorRole, Config: config, Groups: []input.Group{{Name: "primary"}}},
					{Name: trinoWorkerRole, Config: config,
						Overrides: &input.Overrides{CLIOverrides: &roleArgs, PodOverrides: tc.pod},
						Groups:    []input.Group{{Name: "primary", Overrides: &input.Overrides{CLIOverrides: tc.group}}}},
				}}
			plan, err := pipeline.Build(Definition(), projection, *resolved.Value, framework.AssemblyOptions{
				MaterializerImage: "helper:test", VectorImage: "vector:test"})
			if err != nil {
				t.Fatal(err)
			}
			for _, group := range plan.Groups {
				if group.Outcome.Group.Role != trinoWorkerRole {
					continue
				}
				if group.Resources == nil || group.Outcome.Error != "" {
					t.Fatalf("S3 and valid override composition failed: %s", group.Outcome.Error)
				}
				main := group.Resources.StatefulSet.Spec.Template.Spec.Containers[0]
				if !slices.Equal(main.Command, []string{s3Shell, s3ShellFlags, s3Launcher, trinoLauncher}) ||
					!slices.Equal(main.Args, tc.want) {
					t.Fatalf("launcher/argument boundary changed: command=%q args=%q", main.Command, main.Args)
				}
				runS3Launcher(t, main.Command, main.Args)
				return
			}
			t.Fatal("worker was not generated")
		})
	}
}

func runS3Launcher(t *testing.T, command, args []string) {
	t.Helper()
	directory := t.TempDir()
	// Only fixed credential reads are replaced by fixture output. The actual
	// wrapper executes in /bin/sh and must forward all options without evaluation.
	cat := `#!/bin/sh
case "$1" in
 /kubedoop/s3-credentials/ACCESS_KEY) printf '%s' 'fixture-access' ;;
 /kubedoop/s3-credentials/SECRET_KEY) printf '%s' 'fixture-secret' ;;
 *) exit 1 ;;
esac
`
	launcher := `#!/bin/sh
test "$TRINO_S3_ACCESS_KEY" = fixture-access || exit 1
test "$TRINO_S3_SECRET_KEY" = fixture-secret || exit 1
printf '%s\n' "$@"
`
	for name, content := range map[string]string{"cat": cat, "launcher": launcher} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0700); err != nil {
			t.Fatal(err)
		}
	}
	argv := slices.Clone(command)
	argv[3] = filepath.Join(directory, "launcher") // replace the image-only executable with a local argument observer
	argv = append(argv, args...)
	process := exec.CommandContext(t.Context(), argv[0], argv[1:]...)
	process.Env = append(os.Environ(), "PATH="+directory+":"+os.Getenv("PATH"))
	output, err := process.CombinedOutput()
	if err != nil || string(output) != strings.Join(args, "\n")+"\n" {
		t.Fatalf("actual shell did not execute launcher with final CLI args: %v, output=%q", err, output)
	}
}
