package product

import (
	"os/exec"
	"slices"
	"testing"
)

func TestTrinoLifecycleConsumesTypedInputWithoutDefaultManagementGrant(t *testing.T) {
	in := effectiveInput()
	runtime, err := generateTrino(in)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Main.Lifecycle != nil || len(runtime.Initializers) != 1 || runtime.Main.StartupProbe == nil || runtime.Main.ReadinessProbe == nil || runtime.Coordination == nil {
		t.Fatal("default native initialization/probes or explicit shutdown boundary missing")
	}
	in.Config.Product.ShutdownCredentialsSecret = "admin-credentials"
	runtime, err = generateTrino(in)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Main.Lifecycle == nil || !slices.Contains(runtime.Main.Lifecycle.PreStop.Exec.Command, "/kubedoop/shutdown-credentials") {
		t.Fatal("secret-based shutdown is not consumed")
	}
	found := false
	for _, dir := range runtime.Directories {
		if dir.Secret != nil && dir.Secret.SecretName == "admin-credentials" {
			found = true
		}
	}
	if !found {
		t.Fatal("credentials secret was not mounted")
	}
	in.Group.Role = trinoCoordinatorRole
	runtime, err = generateTrino(in)
	if err != nil || runtime.Main.Lifecycle != nil || runtime.Coordination.ShutdownPriority != 100 {
		t.Fatal("coordinator must stop after workers without invoking a worker-only shutdown protocol")
	}
}

func TestTrinoLifecycleScriptsCompile(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal("Python is required to verify the actual image protocol scripts", err)
	}
	for name, script := range map[string]string{"initialize": trinoInitialize, "ready": trinoReady, "shutdown": trinoShutdown} {
		command := exec.Command(python, "-c", "import sys; compile(sys.argv[1],sys.argv[2],'exec')", script, name)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v %s", name, err, output)
		}
	}
}
