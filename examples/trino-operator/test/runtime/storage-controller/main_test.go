package main

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/zncdatadev/operator-go/examples/trino-operator/internal/product"
	api "github.com/zncdatadev/operator-go/pkg/framework"
)

func TestStorageOptionsRequireExplicitScope(t *testing.T) {
	base := []string{"--kubeconfig=/not-read", "--namespace=experiment", "--image=fixed-image", "--storage-class=retained"}
	settings, err := parseOptions(base)
	if err != nil || settings.capacity.Cmp(resource.MustParse("64Mi")) != 0 {
		t.Fatalf("unexpected defaults: %+v, %v", settings, err)
	}
	for _, extra := range []string{"--capacity=0", "--capacity=-1Gi", "--capacity=nope", "--namespace=", "positional"} {
		if _, err := parseOptions(append(append([]string{}, base...), extra)); err == nil {
			t.Fatalf("accepted %q", extra)
		}
	}
	if _, err := parseOptions(nil); err == nil {
		t.Fatal("accepted implicit cluster scope")
	}
}

func TestStorageDefinitionDeclaresOnlyRetainedMarkerProcess(t *testing.T) {
	definition := storageDefinition("fixed-image", "retained", resource.MustParse("64Mi"))
	in := api.EffectiveInput[product.TrinoConfig, product.TrinoClusterConfig, product.TrinoFacts]{
		Group:  api.GroupIdentity{Role: workerRole, Name: defaultGroup, Replicas: 1},
		Config: definition.Roles[workerRole].Config,
	}
	if err := definition.ValidateInput(in); err != nil {
		t.Fatal(err)
	}
	run, err := definition.GenerateGroup(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Files) != 0 || len(run.LogOutputs) != 0 || len(run.Directories) != 1 || !run.Directories[0].Data {
		t.Fatal("marker fixture must declare only one retained data slot")
	}
	main := run.Main
	if main.Name != processName || main.Command[0] != "python3" || main.Args[2] != markerServer ||
		*main.Identity.RunAsUser != 1000 || *main.Identity.RunAsGroup != 1000 || *run.SharedGroup != 1000 ||
		len(main.Access) != 1 || main.Access[0].Directory != dataSlot || main.Access[0].MountPath != dataPath {
		t.Fatalf("unexpected marker process: %+v", main)
	}
	if in.Config.Common.Resources.Storage.StorageClassName != "retained" ||
		in.Config.Common.Resources.Storage.Capacity.Cmp(resource.MustParse("64Mi")) != 0 {
		t.Fatal("retained source request changed")
	}
}

func TestMarkerServerReadsWithoutInitializingData(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is unavailable; runtime evidence must still exercise the image")
	}
	const verify = `import http.client, os, pathlib, sys, threading
scope = {"__name__": "fixture_test"}
exec(compile(sys.stdin.read(), "marker_server", "exec"), scope)
marker = pathlib.Path(sys.argv[1]) / "marker.json"
scope["MARKER_PATH"] = marker
server = scope["ThreadingHTTPServer"](("127.0.0.1", 0), scope["Handler"])
thread = threading.Thread(target=server.serve_forever, daemon=True)
thread.start()
def request(method, path):
    connection = http.client.HTTPConnection("127.0.0.1", server.server_port, timeout=3)
    try:
        connection.request(method, path)
        response = connection.getresponse()
        return response.status, response.read()
    finally:
        connection.close()
try:
    assert request("GET", "/healthz") == (200, b"ready\n")
    assert request("GET", "/marker")[0] == 404
    assert request("POST", "/marker")[0] == 501
    assert not marker.exists(), "server initialized missing data"
    content = b'{"nonce":"test-only-existing-content"}\n'
    with marker.open("wb") as output:
        output.write(content)
        output.flush()
        os.fsync(output.fileno())
    before = marker.stat()
    assert request("GET", "/marker") == (200, content)
    assert request("GET", "/unknown")[0] == 404
    after = marker.stat()
    assert before.st_mtime_ns == after.st_mtime_ns and marker.read_bytes() == content
finally:
    server.shutdown()
    server.server_close()
    thread.join(timeout=3)
    assert not thread.is_alive()
`
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, python, "-c", verify, t.TempDir())
	command.Stdin = strings.NewReader(markerServer)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("marker server verification failed: %v\n%s", err, output)
	}
}

func TestStorageDefinitionRejectsUnsupportedWorkloads(t *testing.T) {
	definition := storageDefinition("fixed-image", "retained", resource.MustParse("64Mi"))
	for _, scenario := range []string{"coordinator", "group", "replicas", "port", "logging", "cluster"} {
		in := api.EffectiveInput[product.TrinoConfig, product.TrinoClusterConfig, product.TrinoFacts]{
			Group:  api.GroupIdentity{Role: workerRole, Name: defaultGroup, Replicas: 1},
			Config: definition.Roles[workerRole].Config,
		}
		switch scenario {
		case "coordinator":
			in.Group.Role = "coordinators"
		case "group":
			in.Group.Name = "other"
		case "replicas":
			in.Group.Replicas = 2
		case "port":
			in.Config.Product.HTTPPort = 9090
		case "logging":
			in.Config.Common.Logging.EnableVectorAgent = true
		case "cluster":
			in.ClusterConfig.NodeEnvironment = "production"
		}
		if err := definition.ValidateInput(in); err == nil {
			t.Fatalf("accepted unsupported %s", scenario)
		}
	}
}
