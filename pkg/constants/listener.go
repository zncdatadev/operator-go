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

// Zncdata defined annotations for PVCTemplate.
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
