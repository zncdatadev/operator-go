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
)

var _ = Describe("EventManager", func() {
	var eventManager *reconciler.EventManager
	var testPod *corev1.Pod

	BeforeEach(func() {
		eventManager = reconciler.NewEventManager(recorder)
		Expect(eventManager).NotTo(BeNil())

		testPod = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		}
	})

	Describe("NewEventManager", func() {
		It("should create an EventManager with recorder", func() {
			Expect(eventManager.Recorder).To(Equal(recorder))
		})
	})

	Describe("EmitCreateEvent", func() {
		It("should emit a create event", func() {
			eventManager.EmitCreateEvent("test-cluster", testPod)
			// Event should be recorded (we can't easily verify the exact content with fake recorder)
		})
	})

	Describe("EmitUpdateEvent", func() {
		It("should emit an update event", func() {
			eventManager.EmitUpdateEvent("test-cluster", testPod)
		})
	})

	Describe("EmitDeleteEvent", func() {
		It("should emit a delete event", func() {
			eventManager.EmitDeleteEvent("test-cluster", testPod)
		})
	})

	Describe("EmitErrorEvent", func() {
		It("should emit an error event", func() {
			testErr := errors.New("test error")
			eventManager.EmitErrorEvent("test-cluster", testPod, testErr)
		})
	})

	Describe("EmitWarningEvent", func() {
		It("should emit a warning event", func() {
			eventManager.EmitWarningEvent(testPod, "TestWarning", "This is a warning")
		})
	})

	Describe("EmitNormalEvent", func() {
		It("should emit a normal event", func() {
			eventManager.EmitNormalEvent(testPod, "TestNormal", "This is normal")
		})
	})

	Describe("EmitProgressingEvent", func() {
		It("should emit a progressing event", func() {
			eventManager.EmitProgressingEvent(testPod, "Cluster is progressing")
		})
	})

	Describe("EmitAvailableEvent", func() {
		It("should emit an available event", func() {
			eventManager.EmitAvailableEvent(testPod, "test-cluster")
		})
	})

	Describe("EmitDegradedEvent", func() {
		It("should emit a degraded event", func() {
			eventManager.EmitDegradedEvent(testPod, "TestDegraded", "Cluster is degraded")
		})
	})

	Describe("LogAndEmitError", func() {
		It("should log and emit an error event", func() {
			ctx := context.Background()
			testErr := errors.New("test error")
			eventManager.LogAndEmitError(ctx, testPod, testErr, "Operation failed")
		})
	})

	Describe("LogAndEmitInfo", func() {
		It("should log and emit an info event", func() {
			ctx := context.Background()
			eventManager.LogAndEmitInfo(ctx, testPod, "TestReason", "Operation succeeded")
		})
	})
})
