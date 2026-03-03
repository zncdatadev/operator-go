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

package reconciler

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// EventManager handles Kubernetes events.
type EventManager struct {
	Recorder record.EventRecorder
}

// NewEventManager creates a new EventManager.
func NewEventManager(recorder record.EventRecorder) *EventManager {
	return &EventManager{Recorder: recorder}
}

// EmitCreateEvent emits a resource creation event.
func (e *EventManager) EmitCreateEvent(clusterName string, obj client.Object) {
	e.Recorder.Eventf(obj, corev1.EventTypeNormal, "Created",
		"Created %s %s/%s for cluster %s",
		obj.GetObjectKind().GroupVersionKind().Kind,
		obj.GetNamespace(),
		obj.GetName(),
		clusterName)
}

// EmitUpdateEvent emits a resource update event.
func (e *EventManager) EmitUpdateEvent(clusterName string, obj client.Object) {
	e.Recorder.Eventf(obj, corev1.EventTypeNormal, "Updated",
		"Updated %s %s/%s for cluster %s",
		obj.GetObjectKind().GroupVersionKind().Kind,
		obj.GetNamespace(),
		obj.GetName(),
		clusterName)
}

// EmitDeleteEvent emits a resource deletion event.
func (e *EventManager) EmitDeleteEvent(clusterName string, obj client.Object) {
	e.Recorder.Eventf(obj, corev1.EventTypeNormal, "Deleted",
		"Deleted %s %s/%s for cluster %s",
		obj.GetObjectKind().GroupVersionKind().Kind,
		obj.GetNamespace(),
		obj.GetName(),
		clusterName)
}

// EmitErrorEvent emits an error event.
func (e *EventManager) EmitErrorEvent(clusterName string, obj client.Object, err error) {
	e.Recorder.Eventf(obj, corev1.EventTypeWarning, "ReconcileError",
		"Reconciliation failed for cluster %s: %v",
		clusterName,
		err)
}

// EmitWarningEvent emits a warning event.
func (e *EventManager) EmitWarningEvent(obj client.Object, reason, message string) {
	e.Recorder.Event(obj, corev1.EventTypeWarning, reason, message)
}

// EmitNormalEvent emits a normal event.
func (e *EventManager) EmitNormalEvent(obj client.Object, reason, message string) {
	e.Recorder.Event(obj, corev1.EventTypeNormal, reason, message)
}

// EmitProgressingEvent emits a progressing event.
func (e *EventManager) EmitProgressingEvent(obj client.Object, message string) {
	e.Recorder.Event(obj, corev1.EventTypeNormal, "Progressing", message)
}

// EmitAvailableEvent emits an available event.
func (e *EventManager) EmitAvailableEvent(obj client.Object, clusterName string) {
	e.Recorder.Event(obj, corev1.EventTypeNormal, "Available",
		fmt.Sprintf("Cluster %s is available", clusterName))
}

// EmitDegradedEvent emits a degraded event.
func (e *EventManager) EmitDegradedEvent(obj client.Object, reason, message string) {
	e.Recorder.Event(obj, corev1.EventTypeWarning, reason, message)
}

// LogAndEmitError logs an error and emits an event.
func (e *EventManager) LogAndEmitError(ctx context.Context, obj client.Object, err error, message string) {
	logger := log.FromContext(ctx)
	logger.Error(err, message)
	e.Recorder.Eventf(obj, corev1.EventTypeWarning, "Error", "%s: %v", message, err)
}

// LogAndEmitInfo logs an info message and emits an event.
func (e *EventManager) LogAndEmitInfo(ctx context.Context, obj client.Object, reason, message string) {
	logger := log.FromContext(ctx)
	logger.Info(message, "reason", reason)
	e.Recorder.Event(obj, corev1.EventTypeNormal, reason, message)
}
