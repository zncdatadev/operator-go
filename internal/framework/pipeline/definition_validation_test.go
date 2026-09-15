package pipeline

import (
	"strings"
	"testing"

	"github.com/zncdatadev/operator-go/pkg/framework"
)

func validationDefinition[C, S, F any]() framework.ProductDefinition[C, S, F] {
	return framework.ProductDefinition[C, S, F]{
		Name: "fixture", Roles: map[string]framework.RoleDefinition[C]{"workers": {}},
		GenerateGroup: func(framework.EffectiveInput[C, S, F]) (RuntimeDescription, error) {
			return RuntimeDescription{}, nil
		},
	}
}

func TestDefinitionValidationDoesNotEvaluateDefaultsOrCallbacks(t *testing.T) {
	definition := validationDefinition[testProductConfig, clusterConfigFixture, struct{}]()
	definition.Roles["workers"] = framework.RoleDefinition[testProductConfig]{RoleConfig: RoleConfig{
		PodDisruptionBudget: PodDisruptionBudgetConfig{Enabled: true, MaxUnavailable: -1}}}
	definition.ValidateInput = func(framework.EffectiveInput[testProductConfig, clusterConfigFixture, struct{}]) error {
		t.Fatal("static definition validation invoked product validation")
		return nil
	}
	definition.GenerateGroup = func(framework.EffectiveInput[testProductConfig, clusterConfigFixture, struct{}]) (
		RuntimeDescription, error,
	) {
		t.Fatal("static definition validation invoked generation")
		return RuntimeDescription{}, nil
	}
	if err := ValidateDefinition(definition); err != nil {
		t.Fatalf("static validation rejected defaults that higher user layers can repair: %v", err)
	}
}

func TestDefinitionValidationRejectsMissingContractAndTypeConflicts(t *testing.T) {
	for _, mutate := range []func(*framework.ProductDefinition[struct{}, struct{}, struct{}]){
		func(d *framework.ProductDefinition[struct{}, struct{}, struct{}]) { d.Name = "" },
		func(d *framework.ProductDefinition[struct{}, struct{}, struct{}]) { d.Roles = nil },
		func(d *framework.ProductDefinition[struct{}, struct{}, struct{}]) { d.GenerateGroup = nil },
	} {
		definition := validationDefinition[struct{}, struct{}, struct{}]()
		mutate(&definition)
		if err := ValidateDefinition(definition); err == nil {
			t.Fatal("incomplete static product contract accepted")
		}
	}
	for _, err := range []error{
		ValidateDefinition(validationDefinition[struct{ Value *string }, struct{}, struct{}]()),
		ValidateDefinition(validationDefinition[struct{ Resources string }, struct{}, struct{}]()),
		ValidateDefinition(validationDefinition[struct{}, struct{ Stopped string }, struct{}]()),
		ValidateDefinition(validationDefinition[struct{}, struct{}, any]()),
	} {
		if err == nil {
			t.Fatal("unsupported config/facts or colliding operation field accepted")
		}
	}
	_, err := ResolveConfig(framework.Config[struct {
		Resources string `json:"productResources"`
	}]{}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "collides") {
		t.Fatalf("direct resolver bypassed generated Go field collision protection: %v", err)
	}
}
