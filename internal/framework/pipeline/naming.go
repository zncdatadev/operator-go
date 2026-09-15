package pipeline

import (
	"fmt"
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

// SourceGroupIdentities validates the complete desired identity inventory before
// configuration resolution or product callbacks. A failed build is still desired.
func SourceGroupIdentities[F any](source SourceSnapshot[F]) ([]GroupIdentity, error) {
	if len(validation.IsDNS1123Label(source.Cluster.Name)) != 0 ||
		len(validation.IsDNS1123Label(source.Cluster.Namespace)) != 0 {
		return nil, fmt.Errorf("cluster name and namespace must be valid DNS labels")
	}
	groups := slices.Clone(source.Groups)
	slices.SortFunc(groups, func(a, b GroupSource) int {
		if a.Role != b.Role {
			return strings.Compare(a.Role, b.Role)
		}
		return strings.Compare(a.Name, b.Name)
	})
	identities := make([]GroupIdentity, 0, len(groups))
	names := make(map[string]string, len(groups)*4)
	for i, group := range groups {
		if group.Name == "" || group.Replicas < 0 ||
			(i > 0 && groups[i-1].Role == group.Role && groups[i-1].Name == group.Name) {
			return nil, fmt.Errorf("invalid or duplicate role group %s/%s", group.Role, group.Name)
		}
		identity := GroupIdentity{ClusterIdentity: CloneInput(source.Cluster),
			Role: group.Role, Name: group.Name, Replicas: group.Replicas}
		if len(validation.IsDNS1123Label(group.Role)) != 0 || len(validation.IsDNS1123Label(group.Name)) != 0 {
			return nil, fmt.Errorf("role group %s/%s must use valid DNS labels", group.Role, group.Name)
		}
		if err := checkGroupNames(identity, names); err != nil {
			return nil, err
		}
		identities = append(identities, identity)
	}
	return identities, nil
}

// Check every fixed resource name in the complete declared topology, including
// the headless suffix and ordinary/headless Service collisions between groups.
// These checks run before config/facts can remove any buildable group outputs.
func checkGroupNames(group GroupIdentity, names map[string]string) error {
	base := group.ServiceName()
	owner := group.Role + "/" + group.Name
	for _, item := range []struct {
		kind, name, description string
		validate                func(string) []string
	}{
		{kindService, base, kindService, validation.IsDNS1035Label},
		{kindService, base + "-headless", "headless Service", validation.IsDNS1035Label},
		{kindStatefulSet, base, kindStatefulSet, validation.IsDNS1123Subdomain},
		{kindConfigMap, base, kindConfigMap, validation.IsDNS1123Subdomain},
	} {
		if len(item.validate(item.name)) != 0 {
			return fmt.Errorf("role group %s cannot produce a valid %s name %q", owner, item.description, item.name)
		}
		key := item.kind + "/" + group.Namespace + "/" + item.name
		if previous, exists := names[key]; exists {
			return fmt.Errorf("role groups %s and %s produce the same %s name %q", previous, owner, item.kind, item.name)
		}
		names[key] = owner
	}
	return nil
}

// SourceRoleIdentities checks the complete CR inventory without product callbacks
// or configuration validation. Roles are never inferred from surviving groups.
func SourceRoleIdentities[F any](source SourceSnapshot[F]) ([]RoleIdentity, error) {
	if _, err := SourceGroupIdentities(source); err != nil {
		return nil, err
	}
	roles := slices.Clone(source.Roles)
	slices.SortFunc(roles, func(a, b RoleSource) int { return strings.Compare(a.Name, b.Name) })
	identities := make([]RoleIdentity, 0, len(roles))
	known := make(map[string]bool, len(roles))
	for _, role := range roles {
		if len(validation.IsDNS1123Label(role.Name)) != 0 || known[role.Name] {
			return nil, fmt.Errorf("invalid or duplicate role %q", role.Name)
		}
		identity := RoleIdentity{ClusterIdentity: CloneInput(source.Cluster), Name: role.Name}
		if len(validation.IsDNS1123Subdomain(identity.PodDisruptionBudgetName())) != 0 {
			return nil, fmt.Errorf("role %q cannot produce a valid PodDisruptionBudget name", role.Name)
		}
		known[role.Name] = true
		identities = append(identities, identity)
	}
	for _, group := range source.Groups {
		if !known[group.Role] {
			return nil, fmt.Errorf("role group %s/%s references a role absent from the source", group.Role, group.Name)
		}
	}
	return identities, nil
}
