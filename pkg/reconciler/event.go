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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// EventManager handles Kubernetes events.
type EventManager struct {
	Recorder record.EventRecorder

	// scheme resolves the Kind of the objects named in resource events. See objectKind.
	scheme *runtime.Scheme
}

// NewEventManager creates a new EventManager. The scheme is optional but strongly recommended: it
// is what lets resource events name the kind of the object they are about (see objectKind).
func NewEventManager(recorder record.EventRecorder, scheme *runtime.Scheme) *EventManager {
	return &EventManager{Recorder: recorder, scheme: scheme}
}

// objectKind returns the Kind to name in an event message.
//
// It cannot come from the object itself: everything the framework applies is a typed struct built
// by pkg/builder (`&corev1.Service{ObjectMeta: ...}`) and round-tripped through controller-runtime's
// typed client, which does not populate TypeMeta — so GroupVersionKind().Kind is "" and the message
// read "Created  ns/kafka-broker-default", with a hole where the kind belongs. The framework applies
// a Service, a headless Service and a metrics Service under names differing only by suffix, so the
// kind is exactly the disambiguator someone reading `kubectl get events` after an incident needs.
//
// The scheme lookup is the same one applyResource uses, with the Go type name as the fallback for
// an object the scheme does not know (a product's unregistered extra resource).
func (e *EventManager) objectKind(obj client.Object) string {
	if kind := obj.GetObjectKind().GroupVersionKind().Kind; kind != "" {
		return kind
	}
	if e.scheme != nil {
		if gvks, _, err := e.scheme.ObjectKinds(obj); err == nil && len(gvks) > 0 {
			return gvks[0].Kind
		}
	}
	return fmt.Sprintf("%T", obj)
}

// EmitCreateEvent emits a resource creation event.
func (e *EventManager) EmitCreateEvent(clusterName string, obj client.Object) {
	e.Recorder.Eventf(obj, corev1.EventTypeNormal, "Created",
		"Created %s %s/%s for cluster %s",
		e.objectKind(obj),
		obj.GetNamespace(),
		obj.GetName(),
		clusterName)
}

// EmitUpdateEvent emits a resource update event.
func (e *EventManager) EmitUpdateEvent(clusterName string, obj client.Object) {
	e.Recorder.Eventf(obj, corev1.EventTypeNormal, "Updated",
		"Updated %s %s/%s for cluster %s",
		e.objectKind(obj),
		obj.GetNamespace(),
		obj.GetName(),
		clusterName)
}

// EmitDeleteEvent emits a resource deletion event.
func (e *EventManager) EmitDeleteEvent(clusterName string, obj client.Object) {
	e.Recorder.Eventf(obj, corev1.EventTypeNormal, "Deleted",
		"Deleted %s %s/%s for cluster %s",
		e.objectKind(obj),
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

// LogAndEmitError logs an error and emits an event. Provided for extensions and product code; the
// framework itself does not call it.
func (e *EventManager) LogAndEmitError(ctx context.Context, obj client.Object, err error, message string) {
	logger := log.FromContext(ctx)
	logger.Error(err, message)
	e.Recorder.Eventf(obj, corev1.EventTypeWarning, "Error", "%s: %v", message, err)
}

// LogAndEmitInfo logs an info message and emits an event. Provided for extensions and product
// code; the framework itself does not call it.
func (e *EventManager) LogAndEmitInfo(ctx context.Context, obj client.Object, reason, message string) {
	logger := log.FromContext(ctx)
	logger.Info(message, "reason", reason)
	e.Recorder.Event(obj, corev1.EventTypeNormal, reason, message)
}
