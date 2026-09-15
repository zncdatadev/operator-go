package product

import (
	"fmt"
	"net"
	"strconv"

	"github.com/zncdatadev/operator-go/pkg/framework"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const trinoListenerDirectory = "listener"

func configureTrinoListener(r *framework.RuntimeDescription, in framework.EffectiveInput[TrinoConfig, TrinoClusterConfig, TrinoFacts]) {
	if in.ClusterConfig.ListenerClass == "" || in.Group.Role != trinoCoordinatorRole {
		return
	}
	r.Directories = append(r.Directories, framework.Directory{Name: trinoListenerDirectory, Listener: &framework.ListenerVolume{Class: in.ClusterConfig.ListenerClass}})
	r.Main.Access = append(r.Main.Access, framework.DirectoryAccess{Directory: trinoListenerDirectory, MountPath: "/kubedoop/listener", ReadOnly: true})
}
func trinoGroupDiscovery(cluster framework.ClusterIdentity, group framework.GroupOutcome) (framework.ClusterOutput, error) {
	if group.Platform != nil && (group.Platform.Phase != "Observing" || group.Platform.Diagnostic.State != framework.FactsResolved) {
		return framework.ClusterOutput{State: framework.ClusterOutputPending, Reason: "coordinator platform results unavailable"}, nil
	}
	scheme, port := trinoHTTPEndpoint, int32(0)
	for _, endpoint := range group.GeneratedEndpoints {
		if endpoint.Name == trinoHTTPEndpoint && port == 0 {
			port = endpoint.Port
		}
		if endpoint.Name == trinoHTTPSEndpoint {
			scheme, port = trinoHTTPSEndpoint, endpoint.Port
		}
	}
	if port == 0 {
		return framework.ClusterOutput{State: framework.ClusterOutputPending, Reason: "coordinator generated endpoint unavailable"}, nil
	}
	address := group.Group.ServiceDNS()
	if group.Platform != nil && len(group.Platform.Listeners) > 0 {
		found := false
		for _, listener := range group.Platform.Listeners {
			if listener.Directory != trinoListenerDirectory {
				continue
			}
			if externalPort, ok := listener.Ports[scheme]; ok {
				address, port, found = listener.Address, externalPort, true
				break
			}
		}
		if !found {
			return framework.ClusterOutput{State: framework.ClusterOutputPending, Reason: "coordinator listener endpoint unavailable"}, nil
		}
	}
	return framework.ClusterOutput{State: framework.ClusterOutputReady, ConfigMaps: []corev1.ConfigMap{{
		ObjectMeta: metav1.ObjectMeta{Name: cluster.Name + "-discovery", Namespace: cluster.Namespace},
		Data:       map[string]string{"TRINO_URI": fmt.Sprintf("%s://%s", scheme, net.JoinHostPort(address, strconv.Itoa(int(port))))},
	}}}, nil
}
