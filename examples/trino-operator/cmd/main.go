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
	"flag"
	"os"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	// Import operator-go SDK
	"github.com/zncdatadev/operator-go/pkg/common"
	"github.com/zncdatadev/operator-go/pkg/reconciler"

	// Import Trino Operator API
	trinov1alpha1 "github.com/zncdatadev/operator-go/examples/trino-operator/api/v1alpha1"

	// Import Trino Operator internal implementation
	trinocontroller "github.com/zncdatadev/operator-go/examples/trino-operator/internal/controller"
	"github.com/zncdatadev/operator-go/examples/trino-operator/internal/extensions"
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

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "a3f6b8c9.kubedoop.dev",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// ==================== Register Extensions ====================
	// This is the key to using operator-go SDK extension mechanism

	// Register Catalog extension (demonstrates ClusterExtension)
	catalogExt := extensions.NewCatalogExtension()
	common.GetExtensionRegistry().RegisterClusterExtension(catalogExt)

	// Register Health extension (demonstrates RoleExtension)
	healthExt := extensions.NewHealthExtension()
	common.GetExtensionRegistry().RegisterRoleExtension(healthExt)

	// ==================== Create GenericReconciler ====================
	// Use operator-go SDK's GenericReconciler instead of traditional Controller

	// Create RoleGroupHandler
	roleGroupHandler := trinocontroller.NewTrinoRoleGroupHandler()

	// Create GenericReconciler config
	reconcilerCfg := &reconciler.GenericReconcilerConfig[*trinov1alpha1.TrinoCluster]{
		Client:              mgr.GetClient(),
		Scheme:              mgr.GetScheme(),
		Recorder:            mgr.GetEventRecorderFor("trino-cluster-controller"),
		RoleGroupHandler:    roleGroupHandler,
		HealthCheckInterval: 120 * time.Second,
		HealthCheckTimeout:  300 * time.Second,
		Prototype:           &trinov1alpha1.TrinoCluster{},
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
