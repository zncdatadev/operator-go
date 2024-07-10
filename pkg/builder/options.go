package builder

import (
	"time"

	corev1 "k8s.io/api/core/v1"

	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
)

type RoleGroupInfo struct {
	ClusterName   string
	RoleName      string
	RoleGroupName string
}

func (i *RoleGroupInfo) String() string {
	return i.ClusterName + "-" + i.RoleName + "-" + i.RoleGroupName
}

type ResourceOptions struct {
	Labels        map[string]string
	Annotations   map[string]string
	RoleGroupInfo *RoleGroupInfo
}

type WorkloadOptions struct {
	Labels                 map[string]string
	Annotations            map[string]string
	Affinity               *corev1.Affinity
	PodOverrides           *corev1.PodTemplateSpec
	EnvOverrides           map[string]string
	CommandOverrides       []string
	TerminationGracePeriod *time.Duration
	// Workload cpu and memory resource limits and requests
	Resource      *commonsv1alpha1.ResourcesSpec
	RoleGroupInfo *RoleGroupInfo
}
