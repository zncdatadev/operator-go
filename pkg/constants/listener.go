package constants

const (
	ListenerOperatorGroup string = "listeners." + ZncdataDomain
	ListenerStorageClass  string = ListenerOperatorGroup

	listenerOperatorGroupPrefix string = ListenerOperatorGroup + "/"
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
	AnnotationListenersClass string = listenerOperatorGroupPrefix + "class"
	// The listener name is used to identify the listener, it is OPTIONAL.
	// If not set, the listener name will be the same as the pod name.
	AnnotationListenerName string = listenerOperatorGroupPrefix + "listenerName"
)
