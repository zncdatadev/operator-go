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

package listener_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/listener"
	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("ServiceTypeFor", func() {
	It("maps each class to the Service type that realises it", func() {
		// Restored from v0.12.6's builder.ListenerClass2ServiceType. "unstable" is NodePort — the
		// address is tied to whichever node the pod lands on and changes when it is rescheduled;
		// the LoadBalancer is the STABLE class. The constant's doc comment said the opposite until
		// this change, which is why two downstream operators drew opposite conclusions.
		Expect(listener.ServiceTypeFor(listener.ListenerClassClusterInternal)).
			To(Equal(corev1.ServiceTypeClusterIP))
		Expect(listener.ServiceTypeFor(listener.ListenerClassExternalUnstable)).
			To(Equal(corev1.ServiceTypeNodePort))
		Expect(listener.ServiceTypeFor(listener.ListenerClassExternalStable)).
			To(Equal(corev1.ServiceTypeLoadBalancer))
	})

	It("falls back to the narrowest exposure for a class it does not know", func() {
		// An unrecognised value must not become an accidental public address.
		Expect(listener.ServiceTypeFor("")).To(Equal(corev1.ServiceTypeClusterIP))
		Expect(listener.ServiceTypeFor(listener.ListenerClass("something-new"))).
			To(Equal(corev1.ServiceTypeClusterIP))
	})
})
