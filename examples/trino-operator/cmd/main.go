/*
Copyright 2024 ZNCDataDev.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"crypto/tls"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	// Import operator-go SDK
	"github.com/zncdatadev/operator-go/pkg/common"
	"github.com/zncdatadev/operator-go/pkg/reconciler"

	// Import Trino Operator API
	trinov1alpha1 "github.com/zncdatadev/operator-go/examples/trino-operator/api/v1alpha1"

	// Import Trino Operator internal implementation
	"github.com/zncdatadev/operator-go/examples/trino-operator/internal/constants"
	trinocontroller "github.com/zncdatadev/operator-go/examples/trino-operator/internal/controller"
	"github.com/zncdatadev/operator-go/examples/trino-operator/internal/extensions"
	"github.com/zncdatadev/operator-go/examples/trino-operator/internal/product"
	webhookv1alpha1 "github.com/zncdatadev/operator-go/examples/trino-operator/internal/webhook/v1alpha1"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(trinov1alpha1.AddToScheme(scheme))
}

// managerFlags holds the command-line configuration of the manager process.
type managerFlags struct {
	metricsAddr          string
	metricsCertPath      string
	metricsCertName      string
	metricsCertKey       string
	webhookCertPath      string
	webhookCertName      string
	webhookCertKey       string
	probeAddr            string
	enableLeaderElection bool
	secureMetrics        bool
	enableHTTP2          bool
}

// registerManagerFlags declares every flag the manager accepts on fs.
//
// The deployment manifests under config/ pass these flags to the container, and an undeclared
// flag aborts flag parsing before the manager ever starts — so this set must stay a superset of
// the args in config/manager/manager.yaml and the config/default patches.
func registerManagerFlags(fs *flag.FlagSet) *managerFlags {
	f := &managerFlags{}
	fs.StringVar(&f.metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	fs.StringVar(&f.probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	fs.BoolVar(&f.enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	fs.BoolVar(&f.secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	fs.StringVar(&f.webhookCertPath, "webhook-cert-path", "", "The directory that contains the webhook certificate.")
	fs.StringVar(&f.webhookCertName, "webhook-cert-name", "tls.crt", "The name of the webhook certificate file.")
	fs.StringVar(&f.webhookCertKey, "webhook-cert-key", "tls.key", "The name of the webhook key file.")
	fs.StringVar(&f.metricsCertPath, "metrics-cert-path", "",
		"The directory that contains the metrics server certificate.")
	fs.StringVar(&f.metricsCertName, "metrics-cert-name", "tls.crt", "The name of the metrics server certificate file.")
	fs.StringVar(&f.metricsCertKey, "metrics-cert-key", "tls.key", "The name of the metrics server key file.")
	fs.BoolVar(&f.enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	return f
}

// buildManagerOptions turns the parsed flags into ctrl.Options. The returned cert watchers must
// be added to the manager so rotated certificates are picked up without a restart.
func buildManagerOptions(f *managerFlags) (ctrl.Options, []*certwatcher.CertWatcher, error) {
	// HTTP/2 is disabled by default: it is only needed for very specific use cases and keeping it
	// off avoids CVE-2023-44487 and CVE-2023-39325 (Rapid Reset).
	var tlsOpts []func(*tls.Config)
	if !f.enableHTTP2 {
		tlsOpts = append(tlsOpts, func(c *tls.Config) {
			c.NextProtos = []string{"http/1.1"}
		})
	}

	var watchers []*certwatcher.CertWatcher

	// Clone per server: the webhook and metrics servers each extend the shared base with their
	// own GetCertificate.
	webhookTLSOpts := slices.Clone(tlsOpts)
	if f.webhookCertPath != "" {
		webhookCertWatcher, err := certwatcher.New(
			filepath.Join(f.webhookCertPath, f.webhookCertName),
			filepath.Join(f.webhookCertPath, f.webhookCertKey),
		)
		if err != nil {
			return ctrl.Options{}, nil, err
		}
		watchers = append(watchers, webhookCertWatcher)
		webhookTLSOpts = append(webhookTLSOpts, func(c *tls.Config) {
			c.GetCertificate = webhookCertWatcher.GetCertificate
		})
	}

	metricsOptions := metricsserver.Options{
		BindAddress:   f.metricsAddr,
		SecureServing: f.secureMetrics,
		TLSOpts:       slices.Clone(tlsOpts),
	}
	if f.secureMetrics {
		// Protect the metrics endpoint with the Kubernetes token authn/authz the
		// config/rbac/metrics_auth_role.yaml ClusterRole grants.
		metricsOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}
	if f.metricsCertPath != "" {
		metricsCertWatcher, err := certwatcher.New(
			filepath.Join(f.metricsCertPath, f.metricsCertName),
			filepath.Join(f.metricsCertPath, f.metricsCertKey),
		)
		if err != nil {
			return ctrl.Options{}, nil, err
		}
		watchers = append(watchers, metricsCertWatcher)
		metricsOptions.TLSOpts = append(metricsOptions.TLSOpts, func(c *tls.Config) {
			c.GetCertificate = metricsCertWatcher.GetCertificate
		})
	}

	return ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsOptions,
		WebhookServer:          webhook.NewServer(webhook.Options{TLSOpts: webhookTLSOpts}),
		HealthProbeBindAddress: f.probeAddr,
		LeaderElection:         f.enableLeaderElection,
		LeaderElectionID:       "a3f6b8c9.kubedoop.dev",
	}, watchers, nil
}

// newExtensionRegistry builds the extension registry for TrinoCluster reconciliation.
//
// The registry is instantiated for the product's own CR type, which is what lets the extensions
// declare *TrinoCluster in their hooks instead of the SDK's wide ClusterInterface. It is handed
// to exactly one reconciler (GenericReconcilerConfig.ExtensionRegistry); an operator that manages
// several CR types builds one registry per type.
func newExtensionRegistry(scheme *runtime.Scheme) *common.ExtensionRegistry[*trinov1alpha1.TrinoCluster] {
	registry := common.NewExtensionRegistry[*trinov1alpha1.TrinoCluster]()

	// Register Catalog extension (demonstrates ClusterExtension)
	registry.RegisterClusterExtension(extensions.NewCatalogExtension())

	// Register Health extension (demonstrates RoleExtension)
	registry.RegisterRoleExtension(extensions.NewHealthExtension())

	// Register Discovery extension (demonstrates ClusterExtension PostReconcile +
	// reconciler.EnsureDiscoveryConfigMap): publishes the coordinator URI in a discovery
	// ConfigMap named after the cluster, the kubedoop pattern every product follows. It runs
	// after the catalog extension has refreshed the status, so it is registered at a lower
	// priority rather than relying on registration order alone.
	registry.RegisterClusterExtension(extensions.NewDiscoveryExtension(scheme), common.WithPriority(common.PriorityLow))

	return registry
}

func main() {
	flags := registerManagerFlags(flag.CommandLine)
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgrOpts, certWatchers, err := buildManagerOptions(flags)
	if err != nil {
		setupLog.Error(err, "unable to build manager options")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), mgrOpts)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	for _, watcher := range certWatchers {
		if err := mgr.Add(watcher); err != nil {
			setupLog.Error(err, "unable to add certificate watcher to manager")
			os.Exit(1)
		}
	}

	// ==================== Register Extensions ====================
	// This is the key to using operator-go SDK extension mechanism

	extensionRegistry := newExtensionRegistry(mgr.GetScheme())

	// ==================== Create GenericReconciler ====================
	// Use operator-go SDK's GenericReconciler instead of traditional Controller

	// Create RoleGroupHandler (embeds the SDK BaseRoleGroupHandler; the framework owns
	// resource orchestration).
	roleGroupHandler := trinocontroller.NewTrinoRoleGroupHandler(mgr.GetScheme())

	// Create GenericReconciler config
	reconcilerCfg := &reconciler.GenericReconcilerConfig[*trinov1alpha1.TrinoCluster]{
		Client: mgr.GetClient(),
		// Uncached: used to refresh the resourceVersion after a conflicting status write, which
		// the informer cache is by definition too stale to serve.
		APIReader: mgr.GetAPIReader(),
		Scheme:    mgr.GetScheme(),
		//nolint:staticcheck // TODO: migrate to GetEventRecorder when SDK supports new events API
		Recorder:         mgr.GetEventRecorderFor("trino-cluster-controller"),
		RoleGroupHandler: roleGroupHandler,
		// The handler also declares this product's roles, once per reconcile pass with the cr in
		// hand — ports, primary container name, log producers.
		RoleProvider: roleGroupHandler,
		// The product's derived config flows through the SDK merge pipeline as the lowest layer;
		// any CRD configOverrides always win over it.
		RoleGroupResolver: reconciler.RoleGroupResolverFunc[*trinov1alpha1.TrinoCluster](
			product.ComputeConfig),
		// Read every reconcile, so an operator upgrade moves existing clusters onto the co-released
		// product image. A mutating webhook cannot do this: its defaults are persisted at admission
		// and never recomputed, freezing kubedoopVersion at whatever version first admitted the CR.
		ImageResolution: reconciler.ImageResolution{
			ProductName: constants.ProductName,
			Defaults:    constants.ImageDefaults(),
		},
		HealthCheckInterval: 120 * time.Second,
		HealthCheckTimeout:  300 * time.Second,
		Prototype:           &trinov1alpha1.TrinoCluster{},
		// The reconciler owns its extensions; nothing outside this process-local registry can
		// inject a hook into a TrinoCluster reconcile.
		ExtensionRegistry: extensionRegistry,
	}

	// Create GenericReconciler
	trinoReconciler, err := reconciler.NewGenericReconciler(reconcilerCfg)
	if err != nil {
		setupLog.Error(err, "unable to create reconciler")
		os.Exit(1)
	}

	// Use GenericReconciler's SetupWithManager to register Controller
	if err := trinoReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "TrinoCluster")
		os.Exit(1)
	}

	// ==================== Register Webhooks ====================
	// Register TrinoCluster webhook for validation and defaulting
	if err := webhookv1alpha1.SetupTrinoClusterWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "TrinoCluster")
		os.Exit(1)
	}

	// ==================== Health Checks ====================

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
