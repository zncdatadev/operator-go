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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/testutil"
)

func specRoles(names ...string) map[string]commonsv1alpha1.RoleSpec {
	out := map[string]commonsv1alpha1.RoleSpec{}
	for _, n := range names {
		out[n] = commonsv1alpha1.RoleSpec{}
	}
	return out
}

var _ = Describe("ValidateCatalog", func() {
	It("fails hard when the CR declares a role the product does not support", func() {
		// This is the typo case. Before, an unrecognised role name silently produced a workload
		// with no ports, no image override and no Service, and the reconcile reported success.
		_, err := reconciler.ValidateCatalog(
			reconciler.RoleCatalog{"server": {}},
			specRoles("server", "sever"),
		)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("sever"), "names the role the CR got wrong")
		Expect(err.Error()).To(ContainSubstring("server"), "and lists what it could have meant")
	})

	It("only WARNS when the product declares a role the CR does not use", func() {
		// The asymmetry is deliberate: a product may support more roles than a given cluster uses.
		unused, err := reconciler.ValidateCatalog(
			reconciler.RoleCatalog{"namenode": {}, "datanode": {}, "journalnode": {}},
			specRoles("namenode", "datanode"),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(unused).To(ConsistOf("journalnode"))
	})

	It("says nothing about a role the product marked Optional", func() {
		unused, err := reconciler.ValidateCatalog(
			reconciler.RoleCatalog{"server": {}, "observer": {Optional: true}},
			specRoles("server"),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(unused).To(BeEmpty())
	})

	It("reports unused roles in a stable order", func() {
		// The caller turns this into an event, and an unstable message would rewrite the CR every
		// pass with nothing to back the ping-pong off.
		unused, err := reconciler.ValidateCatalog(
			reconciler.RoleCatalog{"a": {}, "z": {}, "m": {}, "used": {}},
			specRoles("used"),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(unused).To(Equal([]string{"a", "m", "z"}))
	})

	It("accepts a catalog that matches the CR exactly", func() {
		unused, err := reconciler.ValidateCatalog(
			reconciler.RoleCatalog{"server": {}},
			specRoles("server"),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(unused).To(BeEmpty())
	})

	It("names every unsupported role, and both lists, in a stable order", func() {
		// The mirror of "reports unused roles in a stable order", which the hard-error path lacked:
		// both lists came from map iteration, and the existing specs only ever had one element each,
		// so removing either sort broke no test. This message lands in Degraded, where an unstable
		// text defeats updateStatus's no-op guard and reschedules the controller forever.
		for range 20 {
			_, err := reconciler.ValidateCatalog(
				reconciler.RoleCatalog{"worker": {}, "coordinator": {}, "gateway": {}},
				specRoles("worker", "zzz", "aaa"),
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.roles declares aaa, zzz"))
			Expect(err.Error()).To(ContainSubstring("known roles are coordinator, gateway, worker"))
		}
	})
})

var _ = Describe("RoleDeclaration.Validate", func() {
	It("accepts a realistic declaration", func() {
		decl := reconciler.RoleDeclaration{
			MainContainerName: "zookeeper",
			ContainerPorts:    []corev1.ContainerPort{{Name: "client", ContainerPort: 2282}},
			ServicePorts:      []corev1.ServicePort{{Name: "client", Port: 2282}},
			Command:           []string{"/bin/bash", "-c", "zkServer.sh start-foreground"},
			DataVolume:        &reconciler.DataVolume{MountPath: "/kubedoop/data"},
		}
		Expect(decl.Validate("server")).To(Succeed())
	})

	It("rejects a data volume with no mount path", func() {
		decl := reconciler.RoleDeclaration{DataVolume: &reconciler.DataVolume{}}
		err := decl.Validate("server")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("mountPath"))
	})

	It("rejects service ports with no container ports, which would target nothing", func() {
		decl := reconciler.RoleDeclaration{
			ServicePorts: []corev1.ServicePort{{Name: "client", Port: 2282}},
		}
		err := decl.Validate("server")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("target nothing"))
	})

	It("rejects an unnamed container port", func() {
		decl := reconciler.RoleDeclaration{
			ContainerPorts: []corev1.ContainerPort{{ContainerPort: 2282}},
		}
		err := decl.Validate("server")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no name"))
	})

	It("rejects a data volume named after a framework-owned pod volume", func() {
		// Kubernetes does NOT stop this: the API server accepts a StatefulSet whose claim template
		// shares a name with a pod volume, and the StatefulSet controller resolves the conflict by
		// dropping the pod volume. Named "config", the generated ConfigMap is then mounted nowhere
		// and the product starts on an empty config directory, with no event and no rejected write.
		decl := reconciler.RoleDeclaration{
			DataVolume: &reconciler.DataVolume{Name: "config", MountPath: "/kubedoop/data"},
		}
		err := decl.Validate("server")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("reserved"))
		Expect(err.Error()).To(ContainSubstring("ConfigMap volume"), "and says WHAT it would displace")
	})

	It("rejects a data volume named after the shared log volume", func() {
		decl := reconciler.RoleDeclaration{
			DataVolume: &reconciler.DataVolume{Name: "log", MountPath: "/kubedoop/data"},
		}
		Expect(decl.Validate("server")).NotTo(Succeed())
	})

	It("rejects an unparsable logVolumeSize, naming the value", func() {
		// Checked once per pass rather than where it is consumed, because the consumer could only
		// log and fall back to the 33Mi default — and an under-sized log emptyDir filling up is how
		// a chatty pod gets evicted by the kubelet with nothing naming the operator.
		decl := reconciler.RoleDeclaration{LogVolumeSize: "128MB"}
		err := decl.Validate("datanode")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("128MB"))
		Expect(err.Error()).To(ContainSubstring("datanode"))
	})

	It("accepts a logVolumeSize that is a real quantity", func() {
		Expect(reconciler.RoleDeclaration{LogVolumeSize: "128Mi"}.Validate("datanode")).To(Succeed())
	})

	It("rejects a config default whose affinity does not decode, naming the role", func() {
		// A bad DEFAULT must fail at the product's own call site. hdfs's composite affinity was
		// commented out with "Need to check the exact type", and nothing ever told anyone.
		decl := reconciler.RoleDeclaration{
			ConfigDefaults: &commonsv1alpha1.RoleGroupConfigSpec{
				Affinity: &k8sruntime.RawExtension{Raw: []byte(`{"nodeAffinty":{}}`)},
			},
		}
		err := decl.Validate("datanode")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("datanode"))
		Expect(err.Error()).To(ContainSubstring("nodeAffinty"))
	})
})

var _ = Describe("RoleProviderFunc", func() {
	It("adapts a plain function, and receives the cr so a declaration can depend on it", func() {
		// The whole reason the per-call escape hatches can die: a port that moves because the CR
		// enabled TLS is computed here, from THIS cr, not written into process-wide handler state
		// that the next cluster inherits.
		var provider reconciler.RoleProvider[*testutil.MockCluster] = reconciler.RoleProviderFunc[*testutil.MockCluster](
			func(_ context.Context, _ client.Client, cr *testutil.MockCluster) (reconciler.RoleCatalog, error) {
				port := int32(2181)
				if cr.GetName() == "secure" {
					port = 2281
				}
				return reconciler.RoleCatalog{"server": {
					ContainerPorts: []corev1.ContainerPort{{Name: "client", ContainerPort: port}},
				}}, nil
			})

		plain, err := provider.DeclareRoles(context.Background(), nil, testutil.NewMockCluster("plain", "ns"))
		Expect(err).NotTo(HaveOccurred())
		Expect(plain["server"].ContainerPorts[0].ContainerPort).To(Equal(int32(2181)))

		secure, err := provider.DeclareRoles(context.Background(), nil, testutil.NewMockCluster("secure", "ns"))
		Expect(err).NotTo(HaveOccurred())
		Expect(secure["server"].ContainerPorts[0].ContainerPort).To(Equal(int32(2281)))
	})
})
