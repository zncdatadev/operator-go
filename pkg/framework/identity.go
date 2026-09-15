package framework

type ClusterIdentity struct {
	Name      string
	Namespace string
	// Labels are cluster metadata, not inherited product configuration. Each
	// callback receives its own data copy from the framework.
	Labels map[string]string
}

type GroupIdentity struct {
	ClusterIdentity `json:"cluster"`
	Role            string
	Name            string
	Replicas        int32
}

// ServiceName returns the canonical group name used by resource assembly. Name
// validity and collisions are checked by the pipeline; this helper never truncates.
func (g GroupIdentity) ServiceName() string {
	return g.ClusterIdentity.Name + "-" + g.Role + "-" + g.Name
}

// ServiceDNS describes a declared endpoint, not observed service availability.
func (g GroupIdentity) ServiceDNS() string {
	return g.ServiceName() + "." + g.Namespace + ".svc"
}

type RoleIdentity struct {
	ClusterIdentity `json:"cluster"`
	Name            string
}

func (r RoleIdentity) PodDisruptionBudgetName() string {
	return r.ClusterIdentity.Name + "-" + r.Name + "-pdb"
}
