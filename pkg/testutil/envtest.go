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

package testutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	stdruntime "runtime"
	"time"

	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func init() {
	// Set up logger for tests
	logf.SetLogger(zap.New(zap.WriteTo(os.Stdout), zap.UseDevMode(true)))
}

// TestEnv wraps envtest.Environment for easier testing.
// This follows the kubebuilder v4 testing pattern.
type TestEnv struct {
	Env     *envtest.Environment
	Cfg     *rest.Config
	Client  client.Client
	Scheme  *runtime.Scheme
	Ctx     context.Context
	Cancel  context.CancelFunc
	started bool
}

// TestEnvConfig contains configuration for creating a TestEnv.
type TestEnvConfig struct {
	// CRDDirectoryPaths is a list of paths to directories containing CRD definitions.
	CRDDirectoryPaths []string

	// WebhookInstallOptions for webhook testing.
	WebhookInstallOptions envtest.WebhookInstallOptions

	// ErrorIfCRDPathMissing determines whether to error if CRD path is missing.
	// Defaults to false for library projects without CRDs.
	ErrorIfCRDPathMissing bool

	// BinaryAssetsDirectory is the path to the kubebuilder binaries.
	// If empty, it will be auto-detected.
	BinaryAssetsDirectory string
}

// NewTestEnv creates a new TestEnv with default configuration.
// This follows the kubebuilder v4 pattern for test environment setup.
func NewTestEnv(cfg *TestEnvConfig) *TestEnv {
	if cfg == nil {
		cfg = &TestEnvConfig{}
	}

	// Get the project root directory
	_, filename, _, _ := stdruntime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "..", "..")

	// Default CRD paths if not specified
	if len(cfg.CRDDirectoryPaths) == 0 {
		cfg.CRDDirectoryPaths = []string{
			filepath.Join(projectRoot, "config", "crd", "bases"),
		}
	}

	env := &envtest.Environment{
		CRDDirectoryPaths:     cfg.CRDDirectoryPaths,
		WebhookInstallOptions: cfg.WebhookInstallOptions,
		ErrorIfCRDPathMissing: cfg.ErrorIfCRDPathMissing,
	}

	// Auto-detect binary assets directory (for IDE runs)
	if cfg.BinaryAssetsDirectory == "" {
		if binaryDir := getFirstFoundEnvTestBinaryDir(projectRoot); binaryDir != "" {
			env.BinaryAssetsDirectory = binaryDir
		}
	} else {
		env.BinaryAssetsDirectory = cfg.BinaryAssetsDirectory
	}

	scheme := runtime.NewScheme()
	ctx, cancel := context.WithCancel(context.Background())

	return &TestEnv{
		Env:    env,
		Scheme: scheme,
		Ctx:    ctx,
		Cancel: cancel,
	}
}

// Start starts the envtest environment.
// This follows the kubebuilder v4 pattern.
func (e *TestEnv) Start() error {
	if e.started {
		return nil
	}

	logf.Log.Info("Starting envtest environment")

	cfg, err := e.Env.Start()
	if err != nil {
		return fmt.Errorf("failed to start envtest: %w", err)
	}
	e.Cfg = cfg

	// Add standard Kubernetes types to scheme
	if err := corev1.AddToScheme(e.Scheme); err != nil {
		_ = e.Env.Stop()
		return fmt.Errorf("failed to add corev1 to scheme: %w", err)
	}

	// Add project-specific types to scheme
	if err := v1alpha1.AddToScheme(e.Scheme); err != nil {
		_ = e.Env.Stop()
		return fmt.Errorf("failed to add v1alpha1 to scheme: %w", err)
	}

	// Create client
	k8sClient, err := client.New(cfg, client.Options{Scheme: e.Scheme})
	if err != nil {
		_ = e.Env.Stop()
		return fmt.Errorf("failed to create client: %w", err)
	}
	e.Client = k8sClient

	e.started = true
	logf.Log.Info("envtest environment started successfully")
	return nil
}

// Stop stops the envtest environment.
// This follows the kubebuilder v4 pattern with Eventually for graceful shutdown.
func (e *TestEnv) Stop() error {
	if !e.started {
		return nil
	}

	logf.Log.Info("Stopping envtest environment")

	if e.Cancel != nil {
		e.Cancel()
	}

	if e.Env != nil {
		// Use Eventually pattern from kubebuilder for graceful shutdown
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		var lastErr error
		for attempt := 0; ; attempt++ {
			select {
			case <-ctx.Done():
				if lastErr != nil {
					return fmt.Errorf("timed out stopping envtest: %w", lastErr)
				}
				return fmt.Errorf("timed out stopping envtest")
			default:
				if err := e.Env.Stop(); err != nil {
					lastErr = err
					// Exponential backoff: 100ms, 200ms, 400ms, etc., capped at 5s
					backoff := time.Duration(100*(1<<attempt)) * time.Millisecond
					backoff = min(backoff, 5*time.Second)
					select {
					case <-ctx.Done():
						return fmt.Errorf("timed out stopping envtest: %w", lastErr)
					case <-time.After(backoff):
						continue
					}
				}
				e.started = false
				logf.Log.Info("envtest environment stopped successfully")
				return nil
			}
		}
	}

	e.started = false
	return nil
}

// CreateNamespace creates a namespace for testing.
func (e *TestEnv) CreateNamespace(name string) (*corev1.Namespace, error) {
	ns := &corev1.Namespace{}
	ns.Name = name
	if err := e.Client.Create(e.Ctx, ns); err != nil {
		return nil, fmt.Errorf("failed to create namespace: %w", err)
	}
	return ns, nil
}

// DeleteNamespace deletes a namespace by name.
func (e *TestEnv) DeleteNamespace(name string) error {
	ns := &corev1.Namespace{}
	ns.Name = name
	if err := e.Client.Delete(e.Ctx, ns); err != nil {
		return fmt.Errorf("failed to delete namespace: %w", err)
	}
	return nil
}

// GetClient returns the Kubernetes client.
func (e *TestEnv) GetClient() client.Client {
	return e.Client
}

// GetScheme returns the runtime scheme.
func (e *TestEnv) GetScheme() *runtime.Scheme {
	return e.Scheme
}

// GetConfig returns the rest config.
func (e *TestEnv) GetConfig() *rest.Config {
	return e.Cfg
}

// GetContext returns the context.
func (e *TestEnv) GetContext() context.Context {
	return e.Ctx
}

// IsStarted returns whether the test environment is started.
func (e *TestEnv) IsStarted() bool {
	return e.started
}

// getFirstFoundEnvTestBinaryDir locates the first binary directory in the specified path.
// ENVTEST-based tests depend on specific binaries, usually located in paths set by
// controller-runtime. When running tests directly (e.g., via an IDE) without using
// Makefile targets, the 'BinaryAssetsDirectory' must be explicitly configured.
//
// This function streamlines the process by finding the required binaries, similar to
// setting the 'KUBEBUILDER_ASSETS' environment variable. To ensure the binaries are
// properly set up, run 'make setup-envtest' beforehand.
func getFirstFoundEnvTestBinaryDir(projectRoot string) string {
	// Check KUBEBUILDER_ASSETS environment variable first
	if kbAssets := os.Getenv("KUBEBUILDER_ASSETS"); kbAssets != "" {
		if _, err := os.Stat(kbAssets); err == nil {
			return kbAssets
		}
	}

	// Check standard kubebuilder installation paths
	standardPaths := []string{
		"/usr/local/kubebuilder/bin",
		"/usr/local/bin/kubebuilder",
	}

	for _, p := range standardPaths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	// Check project-local bin directory (from make setup-envtest)
	basePath := filepath.Join(projectRoot, "bin", "k8s")
	entries, err := os.ReadDir(basePath)
	if err != nil {
		logf.Log.V(1).Info("Failed to read bin/k8s directory", "path", basePath, "error", err)
		return ""
	}

	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join(basePath, entry.Name())
		}
	}

	return ""
}

// NewFakeClient creates a fake client with the default scheme.
// This is useful for unit tests that don't need envtest.
func NewFakeClient() client.Client {
	s := scheme.Scheme
	_ = corev1.AddToScheme(s)
	_ = v1alpha1.AddToScheme(s)
	return fake.NewClientBuilder().WithScheme(s).Build()
}

// NewFakeClientWithObjects creates a fake client with pre-populated objects.
func NewFakeClientWithObjects(objs ...client.Object) client.Client {
	s := scheme.Scheme
	_ = corev1.AddToScheme(s)
	_ = v1alpha1.AddToScheme(s)
	return fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()
}
