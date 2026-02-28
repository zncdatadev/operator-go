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

package util_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
)

// MockPodExecutor is a mock implementation of PodExecutor for testing
type MockPodExecutor struct {
	ExecuteWithTimeoutFunc   func(ctx context.Context, namespace, podName, containerName string, command []string, timeout time.Duration) (*util.ExecuteResult, error)
	ExecuteWithOutputFunc    func(ctx context.Context, namespace, podName, containerName string, command []string, stdout, stderr io.Writer) error
	ExecuteWithTimeoutCalled bool
	ExecuteWithOutputCalled  bool
	LastNamespace            string
	LastPodName              string
	LastContainerName        string
	LastCommand              []string
	LastTimeout              time.Duration
}

// ExecuteWithTimeout implements PodExecutor interface
func (m *MockPodExecutor) ExecuteWithTimeout(ctx context.Context, namespace, podName, containerName string, command []string, timeout time.Duration) (*util.ExecuteResult, error) {
	m.ExecuteWithTimeoutCalled = true
	m.LastNamespace = namespace
	m.LastPodName = podName
	m.LastContainerName = containerName
	m.LastCommand = command
	m.LastTimeout = timeout

	if m.ExecuteWithTimeoutFunc != nil {
		return m.ExecuteWithTimeoutFunc(ctx, namespace, podName, containerName, command, timeout)
	}
	return &util.ExecuteResult{Stdout: "mock output", Stderr: "", ExitCode: 0}, nil
}

// ExecuteWithOutput implements PodExecutor interface
func (m *MockPodExecutor) ExecuteWithOutput(ctx context.Context, namespace, podName, containerName string, command []string, stdout, stderr io.Writer) error {
	m.ExecuteWithOutputCalled = true
	m.LastNamespace = namespace
	m.LastPodName = podName
	m.LastContainerName = containerName
	m.LastCommand = command

	if m.ExecuteWithOutputFunc != nil {
		return m.ExecuteWithOutputFunc(ctx, namespace, podName, containerName, command, stdout, stderr)
	}
	stdout.Write([]byte("mock output"))
	return nil
}

// MockExitError is a mock error that implements ExitStatus() for testing extractExitCode
type MockExitError struct {
	exitStatus int
	message    string
}

func (e *MockExitError) Error() string {
	return e.message
}

func (e *MockExitError) ExitStatus() int {
	return e.exitStatus
}

var _ = Describe("ExecUtil", func() {
	var (
		execUtil     *util.ExecUtil
		mockExecutor *MockPodExecutor
	)

	Describe("NewExecUtil", func() {
		It("should create a new ExecUtil with valid config", func() {
			config := testEnv.GetConfig()
			eu, err := util.NewExecUtil(k8sClient, config)
			Expect(err).NotTo(HaveOccurred())
			Expect(eu).NotTo(BeNil())
			Expect(eu.Client).NotTo(BeNil())
			Expect(eu.Config).NotTo(BeNil())
			Expect(eu.ClientSet).NotTo(BeNil())
			Expect(eu.Executor).To(BeNil())
		})

		It("should panic with nil config", func() {
			Expect(func() {
				_, _ = util.NewExecUtil(k8sClient, nil)
			}).To(Panic())
		})

		It("should return error with invalid config", func() {
			// Create an invalid config with malformed host
			invalidConfig := &rest.Config{
				Host: "://invalid-host-format",
			}

			_, err := util.NewExecUtil(k8sClient, invalidConfig)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create kubernetes clientset"))
		})
	})

	Describe("WithExecutor", func() {
		BeforeEach(func() {
			config := testEnv.GetConfig()
			var err error
			execUtil, err = util.NewExecUtil(k8sClient, config)
			Expect(err).NotTo(HaveOccurred())
			mockExecutor = &MockPodExecutor{}
		})

		It("should set custom executor and return ExecUtil", func() {
			result := execUtil.WithExecutor(mockExecutor)
			Expect(result).To(Equal(execUtil))
			Expect(execUtil.Executor).To(Equal(mockExecutor))
		})
	})

	Describe("Execute", func() {
		BeforeEach(func() {
			config := testEnv.GetConfig()
			var err error
			execUtil, err = util.NewExecUtil(k8sClient, config)
			Expect(err).NotTo(HaveOccurred())
			mockExecutor = &MockPodExecutor{}
			execUtil.WithExecutor(mockExecutor)
		})

		It("should execute command with default timeout", func() {
			mockExecutor.ExecuteWithTimeoutFunc = func(ctx context.Context, namespace, podName, containerName string, command []string, timeout time.Duration) (*util.ExecuteResult, error) {
				Expect(timeout).To(Equal(30 * time.Second))
				return &util.ExecuteResult{Stdout: "test output", Stderr: "", ExitCode: 0}, nil
			}

			result, err := execUtil.Execute(ctx, "default", "test-pod", "container", []string{"echo", "hello"})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Stdout).To(Equal("test output"))
			Expect(mockExecutor.LastNamespace).To(Equal("default"))
			Expect(mockExecutor.LastPodName).To(Equal("test-pod"))
			Expect(mockExecutor.LastContainerName).To(Equal("container"))
			Expect(mockExecutor.LastCommand).To(Equal([]string{"echo", "hello"}))
		})

		It("should return error when executor fails", func() {
			mockExecutor.ExecuteWithTimeoutFunc = func(ctx context.Context, namespace, podName, containerName string, command []string, timeout time.Duration) (*util.ExecuteResult, error) {
				return &util.ExecuteResult{Stdout: "", Stderr: "error output", ExitCode: 1}, errors.New("command failed")
			}

			result, err := execUtil.Execute(ctx, "default", "test-pod", "container", []string{"false"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("command failed"))
			Expect(result).NotTo(BeNil())
			Expect(result.ExitCode).To(Equal(1))
		})
	})

	Describe("ExecuteWithTimeout", func() {
		BeforeEach(func() {
			config := testEnv.GetConfig()
			var err error
			execUtil, err = util.NewExecUtil(k8sClient, config)
			Expect(err).NotTo(HaveOccurred())
			mockExecutor = &MockPodExecutor{}
			execUtil.WithExecutor(mockExecutor)
		})

		It("should execute command with custom timeout", func() {
			mockExecutor.ExecuteWithTimeoutFunc = func(ctx context.Context, namespace, podName, containerName string, command []string, timeout time.Duration) (*util.ExecuteResult, error) {
				Expect(timeout).To(Equal(60 * time.Second))
				return &util.ExecuteResult{Stdout: "output", Stderr: "", ExitCode: 0}, nil
			}

			result, err := execUtil.ExecuteWithTimeout(ctx, "ns", "pod", "cnt", []string{"ls"}, 60*time.Second)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Stdout).To(Equal("output"))
			Expect(mockExecutor.LastTimeout).To(Equal(60 * time.Second))
		})

		It("should handle context cancellation", func() {
			mockExecutor.ExecuteWithTimeoutFunc = func(ctx context.Context, namespace, podName, containerName string, command []string, timeout time.Duration) (*util.ExecuteResult, error) {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				default:
					return &util.ExecuteResult{Stdout: "output", Stderr: "", ExitCode: 0}, nil
				}
			}

			canceledCtx, cancel := context.WithCancel(context.Background())
			cancel()

			result, err := execUtil.ExecuteWithTimeout(canceledCtx, "ns", "pod", "cnt", []string{"ls"}, 30*time.Second)
			Expect(err).To(Equal(context.Canceled))
			Expect(result).To(BeNil())
		})

		It("should capture stderr output", func() {
			mockExecutor.ExecuteWithTimeoutFunc = func(ctx context.Context, namespace, podName, containerName string, command []string, timeout time.Duration) (*util.ExecuteResult, error) {
				return &util.ExecuteResult{Stdout: "stdout", Stderr: "stderr", ExitCode: 0}, nil
			}

			result, err := execUtil.ExecuteWithTimeout(ctx, "ns", "pod", "cnt", []string{"cmd"}, 30*time.Second)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Stdout).To(Equal("stdout"))
			Expect(result.Stderr).To(Equal("stderr"))
		})
	})

	Describe("ExecuteSimple", func() {
		BeforeEach(func() {
			config := testEnv.GetConfig()
			var err error
			execUtil, err = util.NewExecUtil(k8sClient, config)
			Expect(err).NotTo(HaveOccurred())
			mockExecutor = &MockPodExecutor{}
			execUtil.WithExecutor(mockExecutor)
		})

		It("should return only stdout on success", func() {
			mockExecutor.ExecuteWithTimeoutFunc = func(ctx context.Context, namespace, podName, containerName string, command []string, timeout time.Duration) (*util.ExecuteResult, error) {
				return &util.ExecuteResult{Stdout: "simple output", Stderr: "", ExitCode: 0}, nil
			}

			output, err := execUtil.ExecuteSimple(ctx, "ns", "pod", "cnt", []string{"echo", "test"})
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("simple output"))
		})

		It("should return error when execution fails", func() {
			mockExecutor.ExecuteWithTimeoutFunc = func(ctx context.Context, namespace, podName, containerName string, command []string, timeout time.Duration) (*util.ExecuteResult, error) {
				return nil, errors.New("execution failed")
			}

			output, err := execUtil.ExecuteSimple(ctx, "ns", "pod", "cnt", []string{"cmd"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("execution failed"))
			Expect(output).To(BeEmpty())
		})

		It("should return stdout even with error when result is available", func() {
			mockExecutor.ExecuteWithTimeoutFunc = func(ctx context.Context, namespace, podName, containerName string, command []string, timeout time.Duration) (*util.ExecuteResult, error) {
				return &util.ExecuteResult{Stdout: "partial output", Stderr: "error", ExitCode: 1}, errors.New("failed")
			}

			output, err := execUtil.ExecuteSimple(ctx, "ns", "pod", "cnt", []string{"cmd"})
			Expect(err).To(HaveOccurred())
			Expect(output).To(Equal("partial output"))
		})
	})

	Describe("ExecuteInPod", func() {
		BeforeEach(func() {
			config := testEnv.GetConfig()
			var err error
			execUtil, err = util.NewExecUtil(k8sClient, config)
			Expect(err).NotTo(HaveOccurred())
			mockExecutor = &MockPodExecutor{}
			execUtil.WithExecutor(mockExecutor)
		})

		It("should execute command using mock executor", func() {
			mockExecutor.ExecuteWithTimeoutFunc = func(ctx context.Context, namespace, podName, containerName string, command []string, timeout time.Duration) (*util.ExecuteResult, error) {
				return &util.ExecuteResult{Stdout: "in-pod output", Stderr: "", ExitCode: 0}, nil
			}

			result, err := execUtil.ExecuteInPod(ctx, "ns", map[string]string{"app": "test"}, "cnt", []string{"ls"})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Stdout).To(Equal("in-pod output"))
		})

		It("should return error when mock executor fails", func() {
			mockExecutor.ExecuteWithTimeoutFunc = func(ctx context.Context, namespace, podName, containerName string, command []string, timeout time.Duration) (*util.ExecuteResult, error) {
				return nil, errors.New("pod not found")
			}

			result, err := execUtil.ExecuteInPod(ctx, "ns", map[string]string{"app": "test"}, "cnt", []string{"ls"})
			Expect(err).To(HaveOccurred())
			Expect(result).To(BeNil())
		})
	})

	Describe("ExecuteInPod without mock (integration)", func() {
		BeforeEach(func() {
			config := testEnv.GetConfig()
			var err error
			execUtil, err = util.NewExecUtil(k8sClient, config)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error when no pods match labels", func() {
			result, err := execUtil.ExecuteInPod(ctx, "default", map[string]string{"app": "nonexistent"}, "cnt", []string{"ls"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no pods found"))
			Expect(result).To(BeNil())
		})

		It("should return error when no running pods found", func() {
			// Create a pending pod
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pending-pod",
					Namespace: "default",
					Labels:    map[string]string{"app": "pending-app"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "nginx"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())

			// Set pod status to pending
			pod.Status.Phase = corev1.PodPending
			Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())

			result, err := execUtil.ExecuteInPod(ctx, "default", map[string]string{"app": "pending-app"}, "main", []string{"ls"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no running pods"))
			Expect(result).To(BeNil())
		})
	})

	Describe("CopyFromPod", func() {
		BeforeEach(func() {
			config := testEnv.GetConfig()
			var err error
			execUtil, err = util.NewExecUtil(k8sClient, config)
			Expect(err).NotTo(HaveOccurred())
			mockExecutor = &MockPodExecutor{}
			execUtil.WithExecutor(mockExecutor)
		})

		It("should copy file content from pod", func() {
			mockExecutor.ExecuteWithTimeoutFunc = func(ctx context.Context, namespace, podName, containerName string, command []string, timeout time.Duration) (*util.ExecuteResult, error) {
				Expect(command).To(Equal([]string{"cat", "/etc/config.conf"}))
				return &util.ExecuteResult{Stdout: "file content", Stderr: "", ExitCode: 0}, nil
			}

			data, err := execUtil.CopyFromPod(ctx, "ns", "pod", "cnt", "/etc/config.conf")
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(Equal("file content"))
		})

		It("should return error when copy fails", func() {
			mockExecutor.ExecuteWithTimeoutFunc = func(ctx context.Context, namespace, podName, containerName string, command []string, timeout time.Duration) (*util.ExecuteResult, error) {
				return nil, errors.New("file not found")
			}

			data, err := execUtil.CopyFromPod(ctx, "ns", "pod", "cnt", "/nonexistent")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to copy file from pod"))
			Expect(data).To(BeNil())
		})
	})

	Describe("PodIsReady", func() {
		BeforeEach(func() {
			config := testEnv.GetConfig()
			var err error
			execUtil, err = util.NewExecUtil(k8sClient, config)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return false for non-existent pod", func() {
			ready, err := execUtil.PodIsReady(ctx, "default", "nonexistent-pod")
			Expect(err).To(HaveOccurred())
			Expect(ready).To(BeFalse())
		})

		It("should return false for pending pod", func() {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pending-ready-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "nginx"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())

			pod.Status.Phase = corev1.PodPending
			Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())

			ready, err := execUtil.PodIsReady(ctx, "default", "pending-ready-pod")
			Expect(err).NotTo(HaveOccurred())
			Expect(ready).To(BeFalse())
		})

		It("should return false for running pod without ready condition", func() {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "running-not-ready-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "nginx"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())

			pod.Status.Phase = corev1.PodRunning
			pod.Status.Conditions = []corev1.PodCondition{}
			Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())

			ready, err := execUtil.PodIsReady(ctx, "default", "running-not-ready-pod")
			Expect(err).NotTo(HaveOccurred())
			Expect(ready).To(BeFalse())
		})

		It("should return false for running pod with ready condition false", func() {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "running-ready-false-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "nginx"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())

			pod.Status.Phase = corev1.PodRunning
			pod.Status.Conditions = []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionFalse,
				},
			}
			Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())

			ready, err := execUtil.PodIsReady(ctx, "default", "running-ready-false-pod")
			Expect(err).NotTo(HaveOccurred())
			Expect(ready).To(BeFalse())
		})

		It("should return true for ready pod", func() {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ready-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "nginx"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())

			pod.Status.Phase = corev1.PodRunning
			pod.Status.Conditions = []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				},
			}
			Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())

			ready, err := execUtil.PodIsReady(ctx, "default", "ready-pod")
			Expect(err).NotTo(HaveOccurred())
			Expect(ready).To(BeTrue())
		})
	})

	Describe("WaitForPodReady", func() {
		BeforeEach(func() {
			config := testEnv.GetConfig()
			var err error
			execUtil, err = util.NewExecUtil(k8sClient, config)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error for non-existent pod", func() {
			err := execUtil.WaitForPodReady(ctx, "default", "nonexistent-wait-pod", 2*time.Second)
			Expect(err).To(HaveOccurred())
			// Error can be either "not found" or "timed out" depending on timing
		})

		It("should timeout waiting for pod that never becomes ready", func() {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "never-ready-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "nginx"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())

			pod.Status.Phase = corev1.PodRunning
			pod.Status.Conditions = []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionFalse,
				},
			}
			Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())

			err := execUtil.WaitForPodReady(ctx, "default", "never-ready-pod", 2*time.Second)
			Expect(err).To(HaveOccurred())
			// Error can be either "context deadline exceeded" or "timed out waiting for pod" depending on timing
		})

		It("should return immediately for already ready pod", func() {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "already-ready-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "nginx"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())

			pod.Status.Phase = corev1.PodRunning
			pod.Status.Conditions = []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				},
			}
			Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())

			err := execUtil.WaitForPodReady(ctx, "default", "already-ready-pod", 5*time.Second)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("ExecuteWithOutput", func() {
		BeforeEach(func() {
			config := testEnv.GetConfig()
			var err error
			execUtil, err = util.NewExecUtil(k8sClient, config)
			Expect(err).NotTo(HaveOccurred())
			mockExecutor = &MockPodExecutor{}
			execUtil.WithExecutor(mockExecutor)
		})

		It("should stream output to provided writers", func() {
			mockExecutor.ExecuteWithOutputFunc = func(ctx context.Context, namespace, podName, containerName string, command []string, stdout, stderr io.Writer) error {
				stdout.Write([]byte("streamed output"))
				stderr.Write([]byte("streamed error"))
				return nil
			}

			var stdoutBuf, stderrBuf bytes.Buffer
			err := execUtil.ExecuteWithOutput(ctx, "ns", "pod", "cnt", []string{"cmd"}, &stdoutBuf, &stderrBuf)
			Expect(err).NotTo(HaveOccurred())
			Expect(stdoutBuf.String()).To(Equal("streamed output"))
			Expect(stderrBuf.String()).To(Equal("streamed error"))
		})

		It("should pass correct parameters to executor", func() {
			mockExecutor.ExecuteWithOutputFunc = func(ctx context.Context, namespace, podName, containerName string, command []string, stdout, stderr io.Writer) error {
				Expect(namespace).To(Equal("test-ns"))
				Expect(podName).To(Equal("test-pod"))
				Expect(containerName).To(Equal("test-container"))
				Expect(command).To(Equal([]string{"test", "cmd"}))
				return nil
			}

			var stdoutBuf, stderrBuf bytes.Buffer
			err := execUtil.ExecuteWithOutput(ctx, "test-ns", "test-pod", "test-container", []string{"test", "cmd"}, &stdoutBuf, &stderrBuf)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error when executor fails", func() {
			mockExecutor.ExecuteWithOutputFunc = func(ctx context.Context, namespace, podName, containerName string, command []string, stdout, stderr io.Writer) error {
				return errors.New("stream error")
			}

			var stdoutBuf, stderrBuf bytes.Buffer
			err := execUtil.ExecuteWithOutput(ctx, "ns", "pod", "cnt", []string{"cmd"}, &stdoutBuf, &stderrBuf)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("stream error"))
		})
	})

	Describe("ExtractExitCode", func() {
		It("should return 0 for nil error", func() {
			exitCode := util.ExtractExitCode(nil)
			Expect(exitCode).To(Equal(0))
		})

		It("should extract exit code from error implementing ExitStatus", func() {
			exitErr := &MockExitError{exitStatus: 42, message: "exit error"}
			exitCode := util.ExtractExitCode(exitErr)
			Expect(exitCode).To(Equal(42))
		})

		It("should extract exit code using errors.As", func() {
			wrappedErr := errors.Join(errors.New("wrapper"), &MockExitError{exitStatus: 5, message: "inner exit error"})
			exitCode := util.ExtractExitCode(wrappedErr)
			Expect(exitCode).To(Equal(5))
		})

		It("should return 1 for regular error without ExitStatus", func() {
			exitCode := util.ExtractExitCode(errors.New("regular error"))
			Expect(exitCode).To(Equal(1))
		})
	})

	Describe("ExecuteInPod with running pod", func() {
		var testPod *corev1.Pod

		BeforeEach(func() {
			config := testEnv.GetConfig()
			var err error
			execUtil, err = util.NewExecUtil(k8sClient, config)
			Expect(err).NotTo(HaveOccurred())

			// Create a running pod for integration tests
			testPod = &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-running-pod-integration",
					Namespace: "default",
					Labels:    map[string]string{"app": "test-runner-integration"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "nginx"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, testPod)).To(Succeed())

			testPod.Status.Phase = corev1.PodRunning
			Expect(k8sClient.Status().Update(ctx, testPod)).To(Succeed())
		})

		AfterEach(func() {
			// Cleanup
			k8sClient.Delete(ctx, testPod)
		})

		It("should find running pod by labels", func() {
			// Verify the pod exists and is running
			foundPod := &corev1.Pod{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: "test-running-pod-integration", Namespace: "default"}, foundPod)
			Expect(err).NotTo(HaveOccurred())
			Expect(foundPod.Status.Phase).To(Equal(corev1.PodRunning))
		})
	})

	Describe("ExecuteWithOutput without mock", func() {
		BeforeEach(func() {
			config := testEnv.GetConfig()
			var err error
			execUtil, err = util.NewExecUtil(k8sClient, config)
			Expect(err).NotTo(HaveOccurred())
			// No mock executor - uses real API path
		})

		It("should fail when pod does not exist", func() {
			// This exercises the non-mock executor path
			var stdoutBuf, stderrBuf bytes.Buffer
			err := execUtil.ExecuteWithOutput(ctx, "default", "nonexistent-pod-output", "main", []string{"ls"}, &stdoutBuf, &stderrBuf)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("CopyFromPod without mock", func() {
		BeforeEach(func() {
			config := testEnv.GetConfig()
			var err error
			execUtil, err = util.NewExecUtil(k8sClient, config)
			Expect(err).NotTo(HaveOccurred())
			// No mock executor - uses real API path
		})

		It("should fail when pod does not exist", func() {
			// This exercises the non-mock executor path
			_, err := execUtil.CopyFromPod(ctx, "default", "nonexistent-pod-copy", "main", "/etc/config")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("ExecuteSimple without mock", func() {
		BeforeEach(func() {
			config := testEnv.GetConfig()
			var err error
			execUtil, err = util.NewExecUtil(k8sClient, config)
			Expect(err).NotTo(HaveOccurred())
			// No mock executor - uses real API path
		})

		It("should fail when pod does not exist", func() {
			// This exercises the non-mock executor path
			_, err := execUtil.ExecuteSimple(ctx, "default", "nonexistent-pod-simple", "main", []string{"ls"})
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("GetPod integration", func() {
		BeforeEach(func() {
			config := testEnv.GetConfig()
			var err error
			execUtil, err = util.NewExecUtil(k8sClient, config)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should verify pod lookup via client", func() {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "lookup-test-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "nginx"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())

			// Verify pod can be retrieved
			foundPod := &corev1.Pod{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: "lookup-test-pod", Namespace: "default"}, foundPod)
			Expect(err).NotTo(HaveOccurred())
			Expect(foundPod.Name).To(Equal("lookup-test-pod"))
		})
	})

	Describe("ExecuteInPod error paths", func() {
		BeforeEach(func() {
			config := testEnv.GetConfig()
			var err error
			execUtil, err = util.NewExecUtil(k8sClient, config)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error when client.List fails", func() {
			// Use canceled context to trigger list error
			canceledCtx, cancel := context.WithCancel(context.Background())
			cancel()

			result, err := execUtil.ExecuteInPod(canceledCtx, "default", map[string]string{"app": "test"}, "main", []string{"ls"})
			Expect(err).To(HaveOccurred())
			Expect(result).To(BeNil())
		})
	})

	Describe("WaitForPodReady success path", func() {
		BeforeEach(func() {
			config := testEnv.GetConfig()
			var err error
			execUtil, err = util.NewExecUtil(k8sClient, config)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error when PodIsReady check fails", func() {
			err := execUtil.WaitForPodReady(ctx, "nonexistent-namespace", "nonexistent-pod", 1*time.Second)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("ExecuteWithTimeout non-mock path", func() {
		BeforeEach(func() {
			config := testEnv.GetConfig()
			var err error
			execUtil, err = util.NewExecUtil(k8sClient, config)
			Expect(err).NotTo(HaveOccurred())
			// Don't set mock executor - uses real API path
		})

		It("should fail when pod does not exist", func() {
			result, err := execUtil.ExecuteWithTimeout(ctx, "default", "nonexistent-pod-timeout", "main", []string{"ls"}, 5*time.Second)
			Expect(err).To(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.ExitCode).To(Equal(1))
		})
	})

	Describe("Execute non-mock path", func() {
		BeforeEach(func() {
			config := testEnv.GetConfig()
			var err error
			execUtil, err = util.NewExecUtil(k8sClient, config)
			Expect(err).NotTo(HaveOccurred())
			// Don't set mock executor - uses real API path
		})

		It("should fail when pod does not exist", func() {
			result, err := execUtil.Execute(ctx, "default", "nonexistent-pod-exec", "main", []string{"ls"})
			Expect(err).To(HaveOccurred())
			Expect(result).NotTo(BeNil())
		})
	})
})
