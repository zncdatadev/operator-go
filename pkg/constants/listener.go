package constants

const (
	ListenerAPIGroup     string = "listeners." + KubedoopDomain
	ListenerStorageClass string = ListenerAPIGroup

	listenerAPIGroupPrefix string = ListenerAPIGroup + "/"
)

func ListenerStorageClassPtr() *string {
	listenersStorageClass := ListenerStorageClass
	return &listenersStorageClass
}

// Kubeddoop defined annotations for PVCTemplate.
// Then csi driver can extract annotations from PVC to prepare the listener for pod.
const (
	// Specify which network listening rules to use, it is REQUIRED.
	// It can be one of the following values:
	//	- cluster-internal
	//	- external-unstable
	//	- external-stable
	//	- <other user defined class name>
	AnnotationListenersClass string = listenerAPIGroupPrefix + "class"
	// The listener name is used to identify the listener, it is OPTIONAL.
	// If not set, the listener name will be the same as the pod name.
	AnnotationListenerName string = listenerAPIGroupPrefix + "listenerName"
)

type ListenerClass string

const (
	// ClusterInternal is the default listener class.
	// cluster-internal --> k8s service with ClusterIP
	ClusterInternal ListenerClass = "cluster-internal"
	// external-unstable --> k8s service with NodePort
	ExternalUnstable ListenerClass = "external-unstable"
	// ExternalStable requires a k8s LoadBalancer
	// external-stable --> k8s service with LoadBalancer
	ExternalStable ListenerClass = "external-stable"
)
