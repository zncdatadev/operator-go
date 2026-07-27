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

package reconciler_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
)

var _ = Describe("EventManager", func() {
	var eventManager *reconciler.EventManager
	var events chan string
	var testPod *corev1.Pod

	// A drained recorder of its own, so each spec reads exactly the events it emitted. The suite's
	// shared `recorder` is never drained, which is why the specs here used to assert nothing:
	// "we can't easily verify the exact content with fake recorder" was never true of
	// record.FakeRecorder — it publishes every event on a channel.
	BeforeEach(func() {
		fake := record.NewFakeRecorder(16)
		events = fake.Events
		eventManager = reconciler.NewEventManager(fake, testScheme)
		Expect(eventManager).NotTo(BeNil())

		testPod = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		}
	})

	// emitted returns the single event the spec produced, failing if there is none.
	emitted := func() string {
		var got string
		Eventually(events).Should(Receive(&got))
		return got
	}

	Describe("NewEventManager", func() {
		It("should create an EventManager with recorder", func() {
			Expect(eventManager.Recorder).NotTo(BeNil())
		})
	})

	Describe("resource events", func() {
		It("names the object's Kind, which the typed object itself does not carry", func() {
			// Everything the framework applies is a typed struct from pkg/builder round-tripped
			// through the typed client, so TypeMeta is empty and the message used to read
			// "Created  default/test-pod" — a hole exactly where the disambiguator belongs
			// between a Service, its headless Service and its metrics Service.
			Expect(testPod.GetObjectKind().GroupVersionKind().Kind).To(BeEmpty())

			eventManager.EmitCreateEvent("test-cluster", testPod)
			Expect(emitted()).To(ContainSubstring("Created Pod default/test-pod for cluster test-cluster"))
		})

		It("names the Kind on update and delete too", func() {
			eventManager.EmitUpdateEvent("test-cluster", testPod)
			Expect(emitted()).To(ContainSubstring("Updated Pod default/test-pod"))

			eventManager.EmitDeleteEvent("test-cluster", testPod)
			Expect(emitted()).To(ContainSubstring("Deleted Pod default/test-pod"))
		})

		It("falls back to the bare Go type name for an object the scheme does not know", func() {
			// A product may ship an extra resource whose type is not in the reconciler's scheme;
			// an empty kind is worse than an approximate one. The fallback is the bare type name —
			// "Pod", not "*v1.Pod" — because the star and the package qualifier are noise in a
			// message a human scans, and the bare name is what the scheme would have answered.
			fake := record.NewFakeRecorder(4)
			unknown := reconciler.NewEventManager(fake, runtime.NewScheme())
			unknown.EmitCreateEvent("test-cluster", testPod)

			var got string
			Eventually(fake.Events).Should(Receive(&got))
			Expect(got).To(ContainSubstring("Created Pod default/test-pod"))
			Expect(got).NotTo(ContainSubstring("Created  "), "an empty kind is the original bug")
			Expect(got).NotTo(ContainSubstring("*"))
		})
	})

	Describe("EmitErrorEvent", func() {
		It("should emit an error event naming the cluster and the cause", func() {
			eventManager.EmitErrorEvent("test-cluster", testPod, errors.New("boom"))
			Expect(emitted()).To(ContainSubstring("Reconciliation failed for cluster test-cluster: boom"))
		})
	})

	Describe("EmitWarningEvent", func() {
		It("should emit a warning event", func() {
			eventManager.EmitWarningEvent(testPod, "TestWarning", "This is a warning")
			Expect(emitted()).To(ContainSubstring("Warning TestWarning This is a warning"))
		})
	})

	Describe("EmitNormalEvent", func() {
		It("should emit a normal event", func() {
			eventManager.EmitNormalEvent(testPod, "TestNormal", "This is normal")
			Expect(emitted()).To(ContainSubstring("Normal TestNormal This is normal"))
		})
	})

	// LogAndEmitError/LogAndEmitInfo are product-facing helpers the framework never calls itself.
	Describe("LogAndEmitError", func() {
		It("should log and emit an error event", func() {
			eventManager.LogAndEmitError(context.Background(), testPod, errors.New("boom"), "Operation failed")
			Expect(emitted()).To(ContainSubstring("Warning Error Operation failed: boom"))
		})
	})

	Describe("LogAndEmitInfo", func() {
		It("should log and emit an info event", func() {
			eventManager.LogAndEmitInfo(context.Background(), testPod, "TestReason", "Operation succeeded")
			Expect(emitted()).To(ContainSubstring("Normal TestReason Operation succeeded"))
		})
	})
})
