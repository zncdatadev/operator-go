// Command logging-controller validates the native Python logging adapter and
// central Vector discovery through formal generated registration. It is a
// bounded acceptance fixture, not the production Trino executable.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	generated "github.com/zncdatadev/operator-go/examples/trino-operator/api/v1alpha1"
	"github.com/zncdatadev/operator-go/examples/trino-operator/api/v1alpha1/registration"
	"github.com/zncdatadev/operator-go/examples/trino-operator/internal/product"
	"github.com/zncdatadev/operator-go/pkg/framework"
	nativelogging "github.com/zncdatadev/operator-go/pkg/framework/logging"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

const (
	processName     = "python"
	configDirectory = "config"
	logsDirectory   = "logs"
)

const server = `import json,logging,logging.config
from http.server import BaseHTTPRequestHandler,ThreadingHTTPServer
from urllib.parse import urlparse,parse_qs
logging.config.dictConfig(json.load(open('/config/logging.json')))
class Handler(BaseHTTPRequestHandler):
 def do_GET(self):
  marker=parse_qs(urlparse(self.path).query).get('marker',['ready'])[0]
  logging.getLogger('product').debug(marker+'-debug')
  logging.getLogger('product').warning(marker+'-warning')
  self.send_response(200);self.end_headers();self.wfile.write(marker.encode())
 def log_message(self,*args): pass
ThreadingHTTPServer(('0.0.0.0',8080),Handler).serve_forever()
`

func main() {
	if err := run(os.Args[1:]); err != nil && !errors.Is(err, flag.ErrHelp) {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("logging-controller", flag.ContinueOnError)
	kubeconfig := flags.String("kubeconfig", "", "explicit experiment kubeconfig")
	namespace := flags.String("namespace", "", "isolated watched namespace")
	image := flags.String("image", "", "pinned image containing python3")
	helper := flags.String("materializer-image", "", "built framework materializer image")
	vector := flags.String("vector-image", "", "pinned Vector image")
	if err := flags.Parse(args); err != nil {
		return err
	}
	for _, value := range []*string{kubeconfig, namespace, image, helper, vector} {
		if strings.TrimSpace(*value) == "" {
			return fmt.Errorf("all five explicit experiment flags are required")
		}
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("positional arguments are unsupported")
	}
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		return err
	}
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return err
	}
	if err := generated.AddToScheme(scheme); err != nil {
		return err
	}
	ctrl.SetLogger(zap.New())
	manager, err := ctrl.NewManager(config, ctrl.Options{Scheme: scheme,
		Metrics: metricsserver.Options{BindAddress: "0"}, HealthProbeBindAddress: "0",
		Cache: cache.Options{DefaultNamespaces: map[string]cache.Config{*namespace: {}}}})
	if err != nil {
		return err
	}
	if err := registration.Register(manager, definition(*image), registration.Options[struct{}]{
		FactRefreshInterval: time.Second, Assembly: framework.AssemblyOptions{
			MaterializerImage: *helper, VectorImage: *vector},
	}); err != nil {
		return err
	}
	return manager.Start(ctrl.SetupSignalHandler())
}

func definition(image string) framework.ProductDefinition[product.TrinoConfig, product.TrinoClusterConfig, struct{}] {
	defaults := framework.Config[product.TrinoConfig]{Common: framework.CommonConfig{
		Resources: framework.Resources{CPU: framework.CPU{Min: resource.MustParse("50m"), Max: resource.MustParse("500m")},
			Memory: framework.Memory{Limit: resource.MustParse("128Mi")}},
		Logging: framework.Logging{EnableVectorAgent: true, Containers: map[string]framework.ContainerLogging{
			processName: {Console: framework.Logger{Level: "WARN"}, File: framework.Logger{Level: "DEBUG"},
				Loggers: map[string]framework.Logger{"ROOT": {Level: "INFO"}, "product": {Level: "DEBUG"}}}}},
	}}
	return framework.ProductDefinition[product.TrinoConfig, product.TrinoClusterConfig, struct{}]{
		Name: "logging-experiment", ImageDefaults: framework.ImageConfig{Custom: image, PullPolicy: corev1.PullIfNotPresent},
		Roles: map[string]framework.RoleDefinition[product.TrinoConfig]{
			"workers": {Config: defaults}, "coordinators": {Config: defaults},
		},
		GenerateGroup: func(in framework.EffectiveInput[product.TrinoConfig, product.TrinoClusterConfig, struct{}]) (
			framework.RuntimeDescription, error,
		) {
			logging, exists := in.Config.Common.Logging.Containers[processName]
			if !exists || len(in.Config.Common.Logging.Containers) != 1 {
				return framework.RuntimeDescription{}, fmt.Errorf("logging experiment consumes exactly the python container")
			}
			content, err := nativelogging.Python(logging, "/logs/server.log")
			if err != nil {
				return framework.RuntimeDescription{}, err
			}
			uid := int64(1000)
			out := framework.RuntimeDescription{ConfigDirectory: configDirectory, SharedGroup: &uid,
				Main: framework.Process{Name: processName, Command: []string{"python3"}, Args: []string{"-u", "-c", server},
					Access: []framework.DirectoryAccess{{Directory: configDirectory, MountPath: "/config", ReadOnly: true},
						{Directory: logsDirectory, MountPath: "/logs"}}},
				Directories: []framework.Directory{{Name: configDirectory}, {Name: logsDirectory}},
				Files:       []framework.File{{Directory: configDirectory, Path: "logging.json", Content: content}},
				Endpoints:   []framework.Endpoint{{Name: "http", Port: 8080}},
			}
			if logging.File.Level != "OFF" {
				out.LogOutputs = []framework.LogOutput{
					{Container: processName, Directory: logsDirectory, RelativePath: "server.log"},
				}
			}
			return out, nil
		},
	}
}
