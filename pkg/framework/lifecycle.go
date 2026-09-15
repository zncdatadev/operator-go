package framework

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// WorkloadCoordination opts into ordered workload transitions. Kubernetes owns
// init execution and one-at-a-time OrderedReady rolling replacement; the framework
// serializes scale-down ordinals and persists progress deadlines. Timeout reports
// failure without forcing deletion. It is not a guarantee of successful product
// shutdown: kubelet may kill a process when its termination budget expires.
type WorkloadCoordination struct {
	ProgressDeadline metav1.Duration `json:"progressDeadline"`
	// Lower priorities finish stopping before higher priorities begin. Equal
	// priorities are independent. This ordering also applies to withdrawn groups.
	ShutdownPriority int32 `json:"shutdownPriority"`
}
