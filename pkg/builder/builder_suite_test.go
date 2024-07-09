package builder_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/scheme"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	cfg       *rest.Config
	k8sClient ctrlclient.Client
	testEnv   *envtest.Environment
)

func TestBuilder(t *testing.T) {

	testK8sVersion := os.Getenv("ENVTEST_K8S_VERSION")
	if testK8sVersion == "" {
		logf.Log.Info("ENVTEST_K8S_VERSION is not set, using default version")
		testK8sVersion = "1.28.3"
	}
	if asserts := os.Getenv("KUBEBUILDER_ASSETS"); asserts == "" {
		logf.Log.Info("KUBEBUILDER_ASSETS is not set, using default version " + testK8sVersion)
		os.Setenv("KUBEBUILDER_ASSETS", filepath.Join("..", "..", "bin", "k8s", fmt.Sprintf("%s-%s-%s", testK8sVersion, runtime.GOOS, runtime.GOARCH)))
	}

	RegisterFailHandler(Fail)
	RunSpecs(t, "Builder Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	By("bootstrapping test environment")

	testEnv = &envtest.Environment{
		ErrorIfCRDPathMissing: true,
	}

	var err error

	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	k8sClient, err = ctrlclient.New(cfg, ctrlclient.Options{Scheme: scheme.Scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())
})
