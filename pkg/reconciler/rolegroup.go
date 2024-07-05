package reconciler

type RoleGroupName struct {
	Name        string
	RoleName    string
	ClusterName string
}

func NewRoleGroupName(name, roleName, clusterName string) *RoleGroupName {
	return &RoleGroupName{
		Name:        name,
		RoleName:    roleName,
		ClusterName: clusterName,
	}
}

func (r *RoleGroupName) String() string {
	return r.ClusterName + "-" + r.RoleName + "-" + r.Name
}
