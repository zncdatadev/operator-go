package reconciler_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/scheme"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	commonsapiv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
)

var (
	cfg       *rest.Config
	k8sClient ctrlclient.Client
	testEnv   *envtest.Environment
)

func TestReconciler(t *testing.T) {

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
	RunSpecs(t, "Reconciler Suite")
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

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

type GiteaClusterSpec struct {
	Image            *ImageSpec                               `json:"image,omitempty"`
	ClusterConfig    *ClusterConfigSpec                       `json:"clusterConfig,omitempty"`
	ClusterOperation *commonsapiv1alpha1.ClusterOperationSpec `json:"clusterOperation,omitempty"`
	Gitea            *GiteaSpec                               `json:"gitea,omitempty"`
}

type ImageSpec struct {
	Custom         string `json:"custom,omitempty"`
	Repo           string `json:"repo,omitempty"`
	StackVersion   string `json:"stackVersion,omitempty"`
	ProductVersion string `json:"productVersion,omitempty"`
}

type ClusterConfigSpec struct {
	ListenerClass string `json:"listenerClass,omitempty"`
}

type GiteaSpec struct {
	RoleGroups       map[string]GiteaRoleGroupSpec `json:"roleGroups,omitempty"`
	Config           *GiteaConfigSpec              `json:"config,omitempty"`
	CommandOverrides []string                      `json:"commandOverrides,omitempty"`
	EnvOverrides     []corev1.EnvVar               `json:"envOverrides,omitempty"`
	ConfigOverrides  map[string]string             `json:"configOverrides,omitempty"`
}

type GiteaRoleGroupSpec struct {
	Replicas         int32             `json:"replicas,omitempty"`
	Config           *GiteaConfigSpec  `json:"config,omitempty"`
	CommandOverrides []string          `json:"commandOverrides,omitempty"`
	EnvOverrides     []corev1.EnvVar   `json:"envOverrides,omitempty"`
	ConfigOverrides  map[string]string `json:"configOverrides,omitempty"`
}

type GiteaConfigSpec struct {
	Affinity                *corev1.Affinity                            `json:"affinity,omitempty"`
	PodDisruptionBudget     *commonsapiv1alpha1.PodDisruptionBudgetSpec `json:"podDisruptionBudget,omitempty"`
	GracefulShutdownTimeout *string                                     `json:"gracefulShutdownTimeout,omitempty"`
	Logging                 *commonsapiv1alpha1.LoggingConfigSpec       `json:"logging,omitempty"`
	Resources               *commonsapiv1alpha1.ResourcesSpec           `json:"resources,omitempty"`
}
