/*
Copyright 2024 ZNCDataDev.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
