package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zncdatadev/operator-go/internal/framework/pipeline"
	"github.com/zncdatadev/operator-go/pkg/framework"
)

func commandPlan(t *testing.T) string {
	t.Helper()
	plan, err := pipeline.PrepareMaterialization([]framework.File{
		{Directory: "config", Path: "node.properties", Content: framework.KeyValues{
			Codec: framework.PropertiesCodec{}, Values: map[string]framework.PropertyValue{
				"node.id": framework.PodNameBinding{}, "literal": framework.Literal("${POD_NAME}; $(touch NEVER)"),
			},
		}},
		{Directory: "config", Path: "catalog/custom", Content: framework.Text("connector.name=blackhole\n")},
		{Directory: "config", Path: "empty", Content: framework.Lines{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := pipeline.EncodeMaterializationPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(t.TempDir(), "plan.json")
	if err := os.WriteFile(name, data, 0600); err != nil {
		t.Fatal(err)
	}
	return name
}

func TestCommandMaterializesFilesAndReportsFailures(t *testing.T) {
	workspace := t.TempDir()
	binary := filepath.Join(workspace, "materialize")
	build := exec.Command("go", "build", "-mod=readonly", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build materializer: %s: %v", output, err)
	}
	planPath := commandPlan(t)
	outputRoot := filepath.Join(workspace, "output")
	t.Setenv("POD_NAME", "cluster-workers-default-0")
	for range 2 {
		command := exec.Command(binary, "--plan="+planPath, "--root="+outputRoot)
		command.Dir = workspace
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("execute materializer: %s: %v", output, err)
		}
	}
	for name, want := range map[string]string{
		"node.properties": "literal=${POD_NAME}; $(touch NEVER)\nnode.id=cluster-workers-default-0\n",
		"catalog/custom":  "connector.name=blackhole\n",
		"empty":           "",
	} {
		got, err := os.ReadFile(filepath.Join(outputRoot, "config", name))
		if err != nil || string(got) != want {
			t.Fatalf("file %q: got=%q want=%q error=%v", name, got, want, err)
		}
	}
	if _, err := os.Stat(filepath.Join(workspace, "NEVER")); !os.IsNotExist(err) {
		t.Fatalf("literal shell content executed: %v", err)
	}
	t.Setenv("POD_NAME", "invalid name")
	failedRoot := filepath.Join(workspace, "failed")
	command := exec.Command(binary, "--plan="+planPath, "--root="+failedRoot)
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "valid POD_NAME") {
		t.Fatalf("failed binding did not produce a nonzero process exit: output=%q err=%v", output, err)
	}
	if _, err := os.Stat(failedRoot); !os.IsNotExist(err) {
		t.Fatalf("failed binding wrote output: %v", err)
	}
}

func TestCommandRejectsIncompleteInputsBeforeWriting(t *testing.T) {
	planPath := commandPlan(t)
	for _, args := range [][]string{
		nil, {"--plan=" + planPath}, {"--root=unused"}, {"--unknown=true"},
		{"--plan=" + planPath, "--root=unused", "positional"},
	} {
		if err := run(args); err == nil {
			t.Fatalf("invalid command accepted: %v", args)
		}
	}
	root := filepath.Join(t.TempDir(), "not-created")
	badPlan := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(badPlan, []byte(`{"version":"v1","files":[],"unknown":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{badPlan, filepath.Join(t.TempDir(), "missing.json")} {
		if err := run([]string{"--plan=" + name, "--root=" + root}); err == nil {
			t.Fatalf("invalid or missing plan accepted: %s", name)
		}
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("invalid plan wrote output: %v", err)
	}
}
