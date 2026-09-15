// Command manager runs the Trino reference operator through the generated binding.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/zncdatadev/operator-go/examples/trino-operator/api/v1alpha1/registration"
	"github.com/zncdatadev/operator-go/examples/trino-operator/internal/product"
	"github.com/zncdatadev/operator-go/pkg/framework"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

func main() {
	var materializerImage, vectorImage, namespace, metricsAddress, probeAddress, electionNamespace string
	var leaderElection bool
	var refreshInterval time.Duration
	flag.StringVar(&materializerImage, "materializer-image", "", "co-released materialization helper image (required)")
	flag.StringVar(&vectorImage, "vector-image", "", "Vector image for enabled file collection (required)")
	flag.StringVar(&namespace, "namespace", "", "watch one namespace; empty watches all namespaces")
	flag.StringVar(&metricsAddress, "metrics-bind-address", "0", "metrics address; 0 disables metrics")
	flag.StringVar(&probeAddress, "health-probe-bind-address", ":8081", "manager health endpoint address")
	flag.StringVar(&electionNamespace, "leader-election-namespace", os.Getenv("POD_NAMESPACE"),
		"leader election namespace")
	flag.BoolVar(&leaderElection, "leader-elect", false, "enable leader election")
	flag.DurationVar(&refreshInterval, "fact-refresh-interval", 30*time.Second, "external fact refresh interval")
	logging := zap.Options{}
	logging.BindFlags(flag.CommandLine)
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&logging)))
	if err := start(materializerImage, vectorImage, namespace, metricsAddress, probeAddress, electionNamespace,
		leaderElection, refreshInterval); err != nil {
		ctrl.Log.Error(err, "Trino operator stopped")
		os.Exit(1)
	}
}

func start(materializerImage, vectorImage, namespace, metricsAddress, probeAddress, electionNamespace string,
	leaderElection bool, refreshInterval time.Duration,
) error {
	if materializerImage == "" || vectorImage == "" {
		return fmt.Errorf("--materializer-image and --vector-image are required")
	}
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return err
	}
	options := ctrl.Options{Scheme: scheme, Metrics: metricsserver.Options{BindAddress: metricsAddress},
		HealthProbeBindAddress: probeAddress, LeaderElection: leaderElection,
		LeaderElectionID: "trino.operator.kubedoop.dev", LeaderElectionNamespace: electionNamespace}
	if namespace != "" {
		options.Cache = cache.Options{DefaultNamespaces: map[string]cache.Config{namespace: {}}}
	}
	configuration, err := ctrl.GetConfig()
	if err != nil {
		return err
	}
	manager, err := ctrl.NewManager(configuration, options)
	if err != nil {
		return err
	}
	uid, nonRoot := int64(1001), true
	if err := registration.Register(manager, product.Definition(), registration.Options[product.TrinoFacts]{
		Facts: product.BaseFacts(), ResolveFacts: product.ResolveFacts, FactRefreshInterval: refreshInterval,
		Assembly: framework.AssemblyOptions{MaterializerImage: materializerImage, VectorImage: vectorImage,
			HelperIdentity: &corev1.SecurityContext{RunAsUser: &uid, RunAsGroup: &uid, RunAsNonRoot: &nonRoot}},
	}); err != nil {
		return err
	}
	if err := manager.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return err
	}
	if err := manager.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return err
	}
	return manager.Start(ctrl.SetupSignalHandler())
}
