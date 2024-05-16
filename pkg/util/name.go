package util

type ResourceNameGenerator struct {
	InstanceName string
	RoleName     string
	GroupName    string
}

// NewResourceNameGenerator new a ResourceNameGenerator
func NewResourceNameGenerator(instanceName, roleName, groupName string) *ResourceNameGenerator {
	return &ResourceNameGenerator{
		InstanceName: instanceName,
		RoleName:     roleName,
		GroupName:    groupName,
	}
}

// NewResourceNameGeneratorOneRole new a ResourceNameGenerator without roleName
func NewResourceNameGeneratorOneRole(instanceName, groupName string) *ResourceNameGenerator {
	return &ResourceNameGenerator{
		InstanceName: instanceName,
		GroupName:    groupName,
	}
}

// GenerateResourceName generate resource Name
func (r *ResourceNameGenerator) GenerateResourceName(extraSuffix string) string {
	var res string
	if r.InstanceName != "" {
		res = r.InstanceName + "-"
	}
	if r.GroupName != "" {
		res = res + r.GroupName + "-"
	}
	if r.RoleName != "" {
		res = res + r.RoleName
	} else {
		res = res[:len(res)-1]
	}
	if extraSuffix != "" {
		return res + "-" + extraSuffix
	}
	return res
}
