// Command dataops runs the explicit retained-data operation controller.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/zncdatadev/operator-go/pkg/framework/dataops"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	image := flag.String("worker-image", "", "Python 3 image for the fixed copy/erase worker")
	flag.Parse()
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{
		corev1.AddToScheme, appsv1.AddToScheme, storagev1.AddToScheme, dataops.AddToScheme,
	} {
		if err := add(scheme); err != nil {
			return err
		}
	}
	config, err := ctrl.GetConfig()
	if err != nil {
		return err
	}
	manager, err := ctrl.NewManager(config, ctrl.Options{Scheme: scheme, Metrics: server.Options{BindAddress: "0"},
		HealthProbeBindAddress: "0"})
	if err != nil {
		return err
	}
	if err = dataops.Register(manager, dataops.Options{WorkerImage: *image}); err != nil {
		return err
	}
	return manager.Start(ctrl.SetupSignalHandler())
}
