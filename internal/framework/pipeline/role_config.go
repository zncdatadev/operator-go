package pipeline

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"

	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	kubernetesjson "sigs.k8s.io/json"

	"github.com/zncdatadev/operator-go/pkg/framework"
)

// BuildRoleResources consumes all desired replicas, including groups that cannot
// currently build or resolve facts. An integer minAvailable supports a role whose
// Pods are managed by several StatefulSets; no controller-scale inference is used.
func BuildRoleResources[C, S, F any](
	definition framework.ProductDefinition[C, S, F], source SourceSnapshot[F],
) ([]BuiltRole, error) {
	identities, err := SourceRoleIdentities(source)
	if err != nil {
		return nil, err
	}
	layers := make(map[string]json.RawMessage, len(source.Roles))
	for _, role := range source.Roles {
		layers[role.Name] = role.Config
	}
	totals := make(map[string]int64, len(identities))
	for _, group := range source.Groups {
		if totals[group.Role] > math.MaxInt64-int64(group.Replicas) {
			return nil, fmt.Errorf("desired replica count for role %q exceeds the supported range", group.Role)
		}
		totals[group.Role] += int64(group.Replicas)
	}
	built := make([]BuiltRole, 0, len(identities))
	for _, identity := range identities {
		role := BuiltRole{Role: identity}
		declaration, exists := definition.Roles[identity.Name]
		if !exists {
			role.Error = "role is not declared by the product"
		} else {
			config, err := resolveRoleConfig(declaration.RoleConfig, layers[identity.Name])
			if err != nil {
				role.Error = err.Error()
			} else {
				role.Config = &config
				role.PodDisruptionBudget, err = buildRolePDB(identity, config, totals[identity.Name])
				if err != nil {
					role.Error = err.Error()
				}
			}
		}
		built = append(built, role)
	}
	return built, nil
}

func resolveRoleConfig(defaults RoleConfig, layer json.RawMessage) (RoleConfig, error) {
	resolved := defaults
	if len(layer) != 0 {
		if err := validateJSON(layer, reflect.TypeFor[RoleConfig](), "role.roleConfig"); err != nil {
			return RoleConfig{}, err
		}
		strict, decodeErr := kubernetesjson.UnmarshalStrict(layer, &RoleConfig{}, kubernetesjson.DisallowDuplicateFields)
		if err := errors.Join(decodeErr, errors.Join(strict...)); err != nil {
			return RoleConfig{}, fmt.Errorf("role.roleConfig: %w", err)
		}
		data, err := json.Marshal(defaults)
		if err != nil {
			return RoleConfig{}, err
		}
		var base, patch map[string]json.RawMessage
		if err := json.Unmarshal(data, &base); err != nil {
			return RoleConfig{}, err
		}
		if err := json.Unmarshal(layer, &patch); err != nil {
			return RoleConfig{}, err
		}
		merged, err := json.Marshal(mergeObjects(base, patch))
		if err != nil {
			return RoleConfig{}, err
		}
		if err := json.Unmarshal(merged, &resolved); err != nil {
			return RoleConfig{}, err
		}
	}
	if resolved.PodDisruptionBudget.MaxUnavailable < 0 {
		return RoleConfig{}, fmt.Errorf("role.roleConfig.podDisruptionBudget.maxUnavailable must be nonnegative")
	}
	return resolved, nil
}

func buildRolePDB(role RoleIdentity, config RoleConfig, replicas int64) (*policyv1.PodDisruptionBudget, error) {
	if !config.PodDisruptionBudget.Enabled {
		return nil, nil
	}
	minimum := max(int64(0), replicas-int64(config.PodDisruptionBudget.MaxUnavailable))
	if minimum > math.MaxInt32 {
		return nil, fmt.Errorf("role %q minimum available replica count exceeds int32", role.Name)
	}
	selector := map[string]string{
		labelInstance:  role.ClusterIdentity.Name,
		labelComponent: role.Name,
	}
	minAvailable := intstr.FromInt32(int32(minimum))
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: role.PodDisruptionBudgetName(), Namespace: role.Namespace,
			Labels: resourceLabels(role.ClusterIdentity, selector)},
		Spec: policyv1.PodDisruptionBudgetSpec{MinAvailable: &minAvailable,
			Selector: &metav1.LabelSelector{MatchLabels: CloneInput(selector)}},
	}, nil
}
