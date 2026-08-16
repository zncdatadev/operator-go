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

package vector

import (
	"strings"
	"testing"

	"github.com/zncdatadev/operator-go/pkg/productlogging"
	"github.com/zncdatadev/operator-go/pkg/sidecar"
	corev1 "k8s.io/api/core/v1"
)

// A producer declaration carries two names that may differ, and each drives a different thing:
// the mount follows the pod container, the pre-created directory follows the log dir. Conflating
// them is what forced a product's Vector event tag to equal its container name.
func TestProvider_Inject_MountFollowsContainerAndMkdirFollowsLogDir(t *testing.T) {
	p := NewVectorSidecarProvider("test-product:latest", WithProducers([]productlogging.ContainerLogging{
		{Container: "node", LogDirName: "superset", Framework: productlogging.LoggingFrameworkLogback},
	}))
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "node", Image: "node-image"}},
	}
	if err := p.Inject(podSpec, &sidecar.SidecarConfig{Enabled: true}); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	// mkdir must target the directory the appender writes to, or log4j 1.x and Python's
	// FileHandler fail to open their files (neither creates parent directories).
	script := vectorInitContainer(podSpec).Command[2]
	if !strings.Contains(script, "mkdir -p /kubedoop/log/superset") {
		t.Errorf("mkdir must follow the log directory, got %q", script)
	}
	if strings.Contains(script, "/kubedoop/log/node") {
		t.Errorf("mkdir must not use the container-derived directory, got %q", script)
	}

	// The mount must land on the real pod container, which is the one named "node".
	var mounted bool
	for _, m := range podSpec.Containers[0].VolumeMounts {
		if m.Name == VectorLogVolumeName {
			mounted = true
		}
	}
	if !mounted {
		t.Error("the shared log volume must be mounted on the pod container named by Container")
	}
}

// A producer naming no container in the assembled pod was skipped in silence. The pod then came
// up with a log directory nothing mounts: the appender writes into the container's own
// filesystem, Vector collects nothing, and every signal says healthy.
func TestProvider_Inject_RejectsProducerMatchingNoContainer(t *testing.T) {
	p := NewVectorSidecarProvider("test-product:latest", WithProducers([]productlogging.ContainerLogging{
		{Container: "ghost", Framework: productlogging.LoggingFrameworkLogback},
	}))
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "node", Image: "node-image"}},
	}

	err := p.Inject(podSpec, &sidecar.SidecarConfig{Enabled: true})
	if err == nil {
		t.Fatal("Inject() = nil, want a rejection for a producer that matches no container")
	}
	// Both remedies must be named: the two causes are "your container is called something else"
	// and "you meant to move the log tag, not the container".
	for _, want := range []string{"ghost", "MainContainerName", "LogDirName"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q must mention %q", err, want)
		}
	}
}

// An init container is a legitimate producer: a sidecar-injected one writes into the same shared
// volume, and the phase ordering guarantees it exists by the time Vector is injected.
func TestProvider_Inject_AcceptsInitContainerProducer(t *testing.T) {
	p := NewVectorSidecarProvider("test-product:latest", WithProducers([]productlogging.ContainerLogging{
		{Container: "exporter", Framework: productlogging.LoggingFrameworkLogback},
	}))
	podSpec := &corev1.PodSpec{
		Containers:     []corev1.Container{{Name: "node", Image: "node-image"}},
		InitContainers: []corev1.Container{{Name: "exporter", Image: "exporter-image"}},
	}
	if err := p.Inject(podSpec, &sidecar.SidecarConfig{Enabled: true}); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
}

// The provider is reachable directly by a product that drives it without the reconciler, so it
// re-checks what the framework path already validated — the value lands in its own shell command.
func TestProvider_Inject_RejectsUnusableLogDirName(t *testing.T) {
	p := NewVectorSidecarProvider("test-product:latest", WithProducers([]productlogging.ContainerLogging{
		{Container: "node", LogDirName: "x; touch /tmp/pwn", Framework: productlogging.LoggingFrameworkLogback},
	}))
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "node", Image: "node-image"}},
	}
	if err := p.Inject(podSpec, &sidecar.SidecarConfig{Enabled: true}); err == nil {
		t.Fatal("Inject() = nil, want a rejection: the value is concatenated into /bin/sh -c")
	}
}
