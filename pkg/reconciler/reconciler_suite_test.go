package reconciler_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/scheme"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
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
		err := os.Setenv("KUBEBUILDER_ASSETS", filepath.Join("..", "..", "bin", "k8s", fmt.Sprintf("%s-%s-%s", testK8sVersion, runtime.GOOS, runtime.GOARCH)))
		if err != nil {
			t.Errorf("Failed to set KUBEBUILDER_ASSETS")
		}
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

// =================================================================================================
// Test data structures

type TrinoClusterSpec struct {
	Image            *ImageSpec                            `json:"image,omitempty"`
	ClusterConfig    *ClusterConfigSpec                    `json:"clusterConfig,omitempty"`
	ClusterOperation *commonsv1alpha1.ClusterOperationSpec `json:"clusterOperation,omitempty"`
	Coordinator      *TrinoCoordinatorSpec                 `json:"Coordinator,omitempty"`
	Worker           *TrinoWorkerSpec                      `json:"worker,omitempty"`
}

type ImageSpec struct {
	Custom          string `json:"custom,omitempty"`
	Repository      string `json:"repository,omitempty"`
	KubedoopVersion string `json:"kubedoopVersion,omitempty"`
	ProductVersion  string `json:"productVersion,omitempty"`
	PullPolicy      string `json:"pullPolicy,omitempty"`
	PullSecretName  string `json:"pullSecretName,omitempty"`
}

type ClusterConfigSpec struct {
	ListenerClass string `json:"listenerClass,omitempty"`
}

type TrinoCoordinatorSpec struct {
	RoleGroups map[string]TrinoRoleGroupSpec   `json:"roleGroups,omitempty"`
	Config     *TrinoConfigSpec                `json:"config,omitempty"`
	RoleConfig *commonsv1alpha1.RoleConfigSpec `json:"roleConfig,omitempty"`

	commonsv1alpha1.OverridesSpec `json:",inline"`
}

type TrinoWorkerSpec struct {
	RoleGroups map[string]TrinoRoleGroupSpec   `json:"roleGroups,omitempty"`
	Config     *TrinoConfigSpec                `json:"config,omitempty"`
	RoleConfig *commonsv1alpha1.RoleConfigSpec `json:"roleConfig,omitempty"`

	commonsv1alpha1.OverridesSpec `json:",inline"`
}

type TrinoRoleGroupSpec struct {
	commonsv1alpha1.OverridesSpec `json:",inline"`

	Replicas *int32           `json:"replicas,omitempty"`
	Config   *TrinoConfigSpec `json:"config,omitempty"`
}

type TrinoConfigSpec struct {
	commonsv1alpha1.RoleGroupConfigSpec `json:",inline"`
	QueryMaxMemory                      string `json:"queryMaxMemory,omitempty"`
	QueryMaxMemoryPerNode               string `json:"queryMaxMemoryPerNode,omitempty"`
}

// end of Test data structures
// =================================================================================================
