package inputgen_test

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Generate and consume the SDK from outside its module path. This catches Go
// internal visibility and import dependencies that in-module fixtures conceal.
func TestExternalConsumerGenerationAndAPIRoundtrip(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the SDK checkout")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	consumer := t.TempDir()
	copyConsumerFixture(t, filepath.Join(filepath.Dir(file), "testdata", "consumer"), consumer)
	module, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	const sdk = "github.com/zncdatadev/operator-go"
	moduleText := strings.Replace(string(module), "module "+sdk, "module example.com/framework-consumer", 1)
	moduleText += "\nrequire " + sdk + " v0.0.0\nreplace " + sdk + " => " + strconv.Quote(root) + "\n"
	if err := os.WriteFile(filepath.Join(consumer, "go.mod"), []byte(moduleText), 0o600); err != nil {
		t.Fatal(err)
	}
	sums, err := os.ReadFile(filepath.Join(root, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(consumer, "go.sum"), sums, 0o600); err != nil {
		t.Fatal(err)
	}
	environment := externalConsumerEnvironment(root)
	runConsumerGo(t, consumer, environment, "run", "-mod=readonly", "./cmd/generate")
	runConsumerGo(t, consumer, environment, "test", "-mod=readonly", "-count=1", "-timeout=90s", "-v", "./...")
}

func copyConsumerFixture(t *testing.T, source, destination string) {
	t.Helper()
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func externalConsumerEnvironment(root string) []string {
	result := make([]string, 0, len(os.Environ())+5)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		switch key {
		case "GOWORK", "GOPROXY", "GOSUMDB", "GOFLAGS", "FRAMEWORK_ENVTEST_ASSETS":
			continue
		}
		result = append(result, entry)
	}
	assets := filepath.Join(root, "bin", "k8s", "1.35.0-"+runtime.GOOS+"-"+runtime.GOARCH)
	return append(result, "GOWORK=off", "GOPROXY=off", "GOSUMDB=off", "GOFLAGS=",
		"FRAMEWORK_ENVTEST_ASSETS="+assets)
}

func runConsumerGo(t *testing.T, directory string, environment []string, arguments ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "go", arguments...)
	command.Dir, command.Env = directory, environment
	command.WaitDelay = 5 * time.Second
	output, err := command.CombinedOutput()
	t.Logf("external consumer: go %s\n%s", strings.Join(arguments, " "), output)
	if err != nil {
		t.Fatalf("external consumer failed: %v (context=%v)", err, ctx.Err())
	}
}
