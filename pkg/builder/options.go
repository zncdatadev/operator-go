package builder

import (
	"time"

	corev1 "k8s.io/api/core/v1"

	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
)

type Option struct {
	ClusterName   string
	RoleName      string
	RoleGroupName string
	Labels        map[string]string
	Annotations   map[string]string
}

type Options func(Option) Option

type WorkloadOptions struct {
	Option

	Affinity               *corev1.Affinity
	PodOverrides           *corev1.PodTemplateSpec
	EnvOverrides           map[string]string
	CommandOverrides       []string
	TerminationGracePeriod *time.Duration
	// Workload cpu and memory resource limits and requests
	Resource *commonsv1alpha1.ResourcesSpec
}
