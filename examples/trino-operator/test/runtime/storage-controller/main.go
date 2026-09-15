// Command storage-controller runs the retained-directory acceptance fixture through
// formal generated registration. Its process reads marker data; it is not Trino.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	generatedtrino "github.com/zncdatadev/operator-go/examples/trino-operator/api/v1alpha1"
	"github.com/zncdatadev/operator-go/examples/trino-operator/api/v1alpha1/registration"
	"github.com/zncdatadev/operator-go/examples/trino-operator/internal/product"
	api "github.com/zncdatadev/operator-go/pkg/framework"
)

const (
	workerRole   = "workers"
	defaultGroup = "default"
	processName  = "trino"
	dataSlot     = "data"
	dataPath     = "/data"
	httpPort     = int32(8080)
)

// No path in this program creates or modifies the marker. The harness writes
// and fsyncs it separately, after observing the controller's binding receipt.
const markerServer = `from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

MARKER_PATH = Path("/data/marker.json")
MAX_MARKER_BYTES = 65536

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == "/healthz":
            self.reply(200, b"ready\n", "text/plain")
            return
        if self.path != "/marker":
            self.reply(404, b"not found\n", "text/plain")
            return
        try:
            with MARKER_PATH.open("rb") as source:
                content = source.read(MAX_MARKER_BYTES + 1)
        except FileNotFoundError:
            self.reply(404, b"marker absent\n", "text/plain")
            return
        except OSError:
            self.reply(500, b"marker unreadable\n", "text/plain")
            return
        if len(content) > MAX_MARKER_BYTES:
            self.reply(413, b"marker too large\n", "text/plain")
            return
        self.reply(200, content, "application/json")

    def reply(self, status, content, content_type):
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(content)))
        self.end_headers()
        self.wfile.write(content)

if __name__ == "__main__":
    server = ThreadingHTTPServer(("0.0.0.0", 8080), Handler)
    print("STORAGE_MARKER_SERVER_READY port=8080 marker=/data/marker.json", flush=True)
    server.serve_forever()
`

type options struct {
	kubeconfig, namespace, image, storageClass string
	capacity                                   resource.Quantity
}

func main() {
	if err := run(os.Args[1:]); err != nil && !errors.Is(err, flag.ErrHelp) {
		_, _ = fmt.Fprintln(os.Stderr, "storage-controller:", err)
		os.Exit(1)
	}
}

func parseOptions(args []string) (options, error) {
	var out options
	flags := flag.NewFlagSet("storage-controller", flag.ContinueOnError)
	flags.StringVar(&out.kubeconfig, "kubeconfig", "", "explicit kubeconfig file (required)")
	flags.StringVar(&out.namespace, "namespace", "", "namespace to watch (required)")
	flags.StringVar(&out.image, "image", "", "fixed experiment image containing python3 (required)")
	flags.StringVar(&out.storageClass, "storage-class", "", "explicit retained storage class (required)")
	capacity := flags.String("capacity", "64Mi", "positive data claim capacity")
	if err := flags.Parse(args); err != nil {
		return out, err
	}
	for _, value := range []string{out.kubeconfig, out.namespace, out.image, out.storageClass} {
		if strings.TrimSpace(value) == "" {
			return out, fmt.Errorf("--kubeconfig, --namespace, --image and --storage-class are required")
		}
	}
	if flags.NArg() != 0 {
		return out, fmt.Errorf("no positional arguments are accepted")
	}
	if len(validation.IsDNS1123Label(out.namespace)) != 0 ||
		len(validation.IsDNS1123Subdomain(out.storageClass)) != 0 {
		return out, fmt.Errorf("namespace or storage class name is invalid")
	}
	var err error
	out.capacity, err = resource.ParseQuantity(*capacity)
	if err != nil || out.capacity.Sign() <= 0 {
		return out, fmt.Errorf("--capacity must be a positive Kubernetes quantity")
	}
	return out, nil
}

func run(args []string) error {
	settings, err := parseOptions(args)
	if err != nil {
		return err
	}
	configuration, err := clientcmd.BuildConfigFromFlags("", settings.kubeconfig)
	if err != nil {
		return err
	}
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{
		clientgoscheme.AddToScheme, storagev1.AddToScheme, generatedtrino.AddToScheme,
	} {
		if err := add(scheme); err != nil {
			return err
		}
	}
	ctrl.SetLogger(zap.New())
	manager, err := ctrl.NewManager(configuration, ctrl.Options{Scheme: scheme,
		Metrics: metricsserver.Options{BindAddress: "0"}, HealthProbeBindAddress: "0",
		Cache: cache.Options{DefaultNamespaces: map[string]cache.Config{settings.namespace: {}}},
	})
	if err != nil {
		return err
	}
	if err := registration.Register(manager, storageDefinition(settings.image, settings.storageClass, settings.capacity),
		registration.Options[product.TrinoFacts]{}); err != nil {
		return err
	}
	return manager.Start(ctrl.SetupSignalHandler())
}

func storageDefinition(
	image, class string, capacity resource.Quantity,
) api.ProductDefinition[product.TrinoConfig, product.TrinoClusterConfig, product.TrinoFacts] {
	defaults := api.Config[product.TrinoConfig]{Common: api.CommonConfig{
		Resources: api.Resources{CPU: api.CPU{Min: resource.MustParse("50m"), Max: resource.MustParse("500m")},
			Memory:  api.Memory{Limit: resource.MustParse("128Mi")},
			Storage: api.Storage{Type: api.StoragePersistent, StorageClassName: class, Capacity: capacity.DeepCopy()}},
		Logging: api.Logging{Containers: map[string]api.ContainerLogging{}},
	}, Product: product.TrinoConfig{HTTPPort: httpPort}}
	return api.ProductDefinition[product.TrinoConfig, product.TrinoClusterConfig, product.TrinoFacts]{
		ImageDefaults: api.ImageConfig{Custom: image, PullPolicy: corev1.PullIfNotPresent},
		Name:          "storage-experiment",
		Roles: map[string]api.RoleDefinition[product.TrinoConfig]{
			"coordinators": {Config: defaults}, workerRole: {Config: defaults},
		},
		ValidateInput: func(
			in api.EffectiveInput[product.TrinoConfig, product.TrinoClusterConfig, product.TrinoFacts],
		) error {
			if in.ClusterConfig != (product.TrinoClusterConfig{}) {
				return fmt.Errorf("storage experiment does not consume Trino cluster configuration")
			}
			if in.Group.Role != workerRole || in.Group.Name != defaultGroup || in.Group.Replicas > 1 {
				return fmt.Errorf("storage experiment supports only workers/default with zero or one replica")
			}
			if in.Config.Product.HTTPPort != httpPort || in.Config.Common.Logging.EnableVectorAgent ||
				len(in.Config.Common.Logging.Containers) != 0 {
				return fmt.Errorf("storage experiment requires HTTP 8080 with Vector and product logging disabled")
			}
			return nil
		},
		GenerateGroup: func(
			in api.EffectiveInput[product.TrinoConfig, product.TrinoClusterConfig, product.TrinoFacts],
		) (api.RuntimeDescription, error) {
			uid, nonRoot := int64(1000), true
			return api.RuntimeDescription{
				Main: api.Process{Name: processName,
					Command: []string{"python3"}, Args: []string{"-u", "-c", markerServer},
					Identity: &corev1.SecurityContext{RunAsUser: &uid, RunAsGroup: &uid, RunAsNonRoot: &nonRoot},
					Access:   []api.DirectoryAccess{{Directory: dataSlot, MountPath: dataPath}}},
				Directories: []api.Directory{{Name: dataSlot, Data: true}},
				SharedGroup: &uid, Endpoints: []api.Endpoint{{Name: "http", Port: httpPort}},
			}, nil
		},
	}
}
