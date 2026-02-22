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

package util

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ExecUtil executes commands in pods.
type ExecUtil struct {
	Client    client.Client
	Config    *rest.Config
	ClientSet *kubernetes.Clientset
}

// NewExecUtil creates a new ExecUtil.
func NewExecUtil(client client.Client, config *rest.Config) (*ExecUtil, error) {
	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes clientset: %w", err)
	}

	return &ExecUtil{
		Client:    client,
		Config:    config,
		ClientSet: clientSet,
	}, nil
}

// ExecuteResult contains the result of a command execution.
type ExecuteResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Execute runs a command in a pod container.
func (e *ExecUtil) Execute(ctx context.Context, namespace, podName, containerName string, command []string) (*ExecuteResult, error) {
	return e.ExecuteWithTimeout(ctx, namespace, podName, containerName, command, 30*time.Second)
}

// ExecuteWithTimeout runs a command with a timeout.
func (e *ExecUtil) ExecuteWithTimeout(ctx context.Context, namespace, podName, containerName string, command []string, timeout time.Duration) (*ExecuteResult, error) {
	// Create timeout context
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Prepare the exec request
	req := e.ClientSet.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: containerName,
			Command:   command,
			Stdin:     false,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	// Create the executor
	exec, err := remotecommand.NewSPDYExecutor(e.Config, "POST", req.URL())
	if err != nil {
		return nil, fmt.Errorf("failed to create executor: %w", err)
	}

	// Capture output
	var stdout, stderr bytes.Buffer

	// Execute the command
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})

	result := &ExecuteResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}

	if err != nil {
		result.ExitCode = 1 // Non-zero exit code on error
		return result, fmt.Errorf("command execution failed: %w", err)
	}

	result.ExitCode = 0
	return result, nil
}

// ExecuteSimple runs a command and returns only stdout.
func (e *ExecUtil) ExecuteSimple(ctx context.Context, namespace, podName, containerName string, command []string) (string, error) {
	result, err := e.Execute(ctx, namespace, podName, containerName, command)
	if err != nil {
		return "", err
	}
	return result.Stdout, nil
}

// ExecuteInPod finds the first pod matching labels and executes a command.
func (e *ExecUtil) ExecuteInPod(ctx context.Context, namespace string, labels map[string]string, containerName string, command []string) (*ExecuteResult, error) {
	// Find pods matching labels
	podList := &corev1.PodList{}
	if err := e.Client.List(ctx, podList,
		client.InNamespace(namespace),
		client.MatchingLabels(labels),
	); err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	if len(podList.Items) == 0 {
		return nil, fmt.Errorf("no pods found matching labels %v", labels)
	}

	// Find a running pod
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			return e.Execute(ctx, namespace, pod.Name, containerName, command)
		}
	}

	return nil, fmt.Errorf("no running pods found matching labels %v", labels)
}

// CopyFromPod copies a file from a pod.
func (e *ExecUtil) CopyFromPod(ctx context.Context, namespace, podName, containerName, srcPath string) ([]byte, error) {
	// Use cat command to read file content
	command := []string{"cat", srcPath}
	result, err := e.Execute(ctx, namespace, podName, containerName, command)
	if err != nil {
		return nil, fmt.Errorf("failed to copy file from pod: %w", err)
	}
	return []byte(result.Stdout), nil
}

// PodIsReady checks if a pod is ready.
func (e *ExecUtil) PodIsReady(ctx context.Context, namespace, podName string) (bool, error) {
	pod := &corev1.Pod{}
	if err := e.Client.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      podName,
	}, pod); err != nil {
		return false, err
	}

	if pod.Status.Phase != corev1.PodRunning {
		return false, nil
	}

	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady {
			return cond.Status == corev1.ConditionTrue, nil
		}
	}

	return false, nil
}

// WaitForPodReady waits for a pod to be ready.
func (e *ExecUtil) WaitForPodReady(ctx context.Context, namespace, podName string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for pod %s/%s to be ready", namespace, podName)
		case <-ticker.C:
			ready, err := e.PodIsReady(ctx, namespace, podName)
			if err != nil {
				return err
			}
			if ready {
				return nil
			}
		}
	}
}

// ExecuteWithOutput executes a command and streams output to the provided writers.
func (e *ExecUtil) ExecuteWithOutput(ctx context.Context, namespace, podName, containerName string, command []string, stdout, stderr io.Writer) error {
	req := e.ClientSet.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: containerName,
			Command:   command,
			Stdin:     false,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(e.Config, "POST", req.URL())
	if err != nil {
		return fmt.Errorf("failed to create executor: %w", err)
	}

	return exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: stdout,
		Stderr: stderr,
	})
}
