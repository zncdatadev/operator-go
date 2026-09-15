package operator

import (
	"context"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/zncdatadev/operator-go/pkg/framework"
	"github.com/zncdatadev/operator-go/pkg/framework/input"
)

type registrationConfig struct {
	Labels map[string]string `json:"labels"`
}
type registrationCluster struct {
	Names []string `json:"names"`
}
type registrationFacts struct {
	Catalogs map[string]string `json:"catalogs"`
}
type registrationCR struct {
	metav1.TypeMeta
	metav1.ObjectMeta
	Status framework.ReconcileStatus
}

func (cr *registrationCR) DeepCopyObject() runtime.Object {
	if cr == nil {
		return nil
	}
	out := *cr
	cr.DeepCopyInto(&out.ObjectMeta)
	cr.Status.DeepCopyInto(&out.Status)
	return &out
}
func registrationBinding() input.Binding[*registrationCR] {
	return input.Binding[*registrationCR]{Version: input.ContractVersion, Roles: []string{"workers"},
		AddToScheme: func(*runtime.Scheme) error { return nil },
		NewObject:   func() *registrationCR { return &registrationCR{} },
		Operation:   func(*registrationCR) framework.ClusterOperation { return framework.ClusterOperation{} },
		Project:     func(*registrationCR) (input.Projection, error) { return input.Projection{}, nil },
		Status:      func(cr *registrationCR) *framework.ReconcileStatus { return &cr.Status },
	}
}
func registrationDefinition() framework.ProductDefinition[registrationConfig, registrationCluster, registrationFacts] {
	return framework.ProductDefinition[registrationConfig, registrationCluster, registrationFacts]{Name: "registration",
		ClusterConfigDefaults: registrationCluster{Names: []string{"default"}},
		Roles: map[string]framework.RoleDefinition[registrationConfig]{"workers": {
			Config: framework.Config[registrationConfig]{Product: registrationConfig{Labels: map[string]string{"key": "value"}},
				Common: framework.CommonConfig{Resources: framework.Resources{Memory: framework.Memory{Limit: resource.MustParse("1Gi")}}}},
		}},
		GenerateGroup: func(framework.EffectiveInput[registrationConfig, registrationCluster, registrationFacts]) (
			framework.RuntimeDescription, error) {
			return framework.RuntimeDescription{}, nil
		},
	}
}

type inaccessibleManager struct{ ctrl.Manager }

func TestRegistrationRejectsStaticErrorsBeforeManagerAccess(t *testing.T) {
	for _, scenario := range []string{"version", "missing-role", "extra-role", "duplicate-role", "missing-generator",
		"negative-refresh", "missing-project", "missing-operation", "missing-scheme", "missing-constructor", "missing-status"} {
		t.Run(scenario, func(t *testing.T) {
			definition, binding := registrationDefinition(), registrationBinding()
			options := Options[registrationConfig, registrationCluster, registrationFacts]{}
			switch scenario {
			case "version":
				binding.Version++
			case "missing-role":
				delete(definition.Roles, "workers")
			case "extra-role":
				definition.Roles["another"] = definition.Roles["workers"]
			case "duplicate-role":
				binding.Roles = append(binding.Roles, "workers")
			case "missing-generator":
				definition.GenerateGroup = nil
			case "negative-refresh":
				options.FactRefreshInterval = -time.Second
			case "missing-project":
				binding.Project = nil
			case "missing-operation":
				binding.Operation = nil
			case "missing-scheme":
				binding.AddToScheme = nil
			case "missing-constructor":
				binding.NewObject = nil
			case "missing-status":
				binding.Status = nil
			}
			if err := Register(inaccessibleManager{}, definition, options, binding); err == nil {
				t.Fatal("invalid registration accepted")
			}
		})
	}
}

func TestRegistrationOwnsDataWithoutCallingProducts(t *testing.T) {
	definition, binding := registrationDefinition(), registrationBinding()
	definition.ImageDefaults.Custom = "invalid but reparable by user input"
	definition.GenerateGroup = func(framework.EffectiveInput[registrationConfig, registrationCluster, registrationFacts]) (
		framework.RuntimeDescription, error) {
		t.Fatal("registration generated product resources")
		return framework.RuntimeDescription{}, nil
	}
	definition.ValidateInput = func(framework.EffectiveInput[registrationConfig, registrationCluster, registrationFacts]) error {
		t.Fatal("registration validated business defaults")
		return nil
	}
	uid := int64(1001)
	options := Options[registrationConfig, registrationCluster, registrationFacts]{
		Facts:    registrationFacts{Catalogs: map[string]string{"catalog": "original"}},
		Assembly: framework.AssemblyOptions{HelperIdentity: &corev1.SecurityContext{RunAsUser: &uid}},
		ResolveFacts: func(context.Context, framework.FactsReader,
			framework.FactInput[registrationConfig, registrationCluster, registrationFacts]) (framework.FactResult[registrationFacts], error) {
			t.Fatal("registration read external facts")
			return framework.FactResult[registrationFacts]{}, nil
		},
	}
	r, err := prepareRegistration(definition, options, binding)
	if err != nil {
		t.Fatal(err)
	}
	definition.ClusterConfigDefaults.Names[0] = "changed"
	role := definition.Roles["workers"]
	role.Config.Product.Labels["key"] = "changed"
	role.Config.Common.Resources.Memory.Limit.Add(resource.MustParse("1Gi"))
	definition.Roles["workers"] = role
	options.Facts.Catalogs["catalog"] = "changed"
	binding.Roles[0] = "changed"
	uid = 0
	owned := r.Definition.Roles["workers"]
	if r.Definition.ClusterConfigDefaults.Names[0] != "default" || owned.Config.Product.Labels["key"] != "value" ||
		owned.Config.Common.Resources.Memory.Limit.String() != "1Gi" || *r.Assembly.HelperIdentity.RunAsUser != 1001 ||
		r.Facts.Catalogs["catalog"] != "original" || r.Binding.Roles[0] != "workers" {
		t.Fatal("registration retained caller-owned mutable data")
	}
	if err := Register(nil, definition, options, binding); err == nil {
		t.Fatal("nil manager accepted")
	}
}

func TestRegistrationRejectsOpaqueFactsBeforeCopy(t *testing.T) {
	type opaque struct{ Unsupported chan string }
	definition := framework.ProductDefinition[registrationConfig, registrationCluster, opaque]{Name: "opaque",
		Roles: registrationDefinition().Roles,
		GenerateGroup: func(framework.EffectiveInput[registrationConfig, registrationCluster, opaque]) (
			framework.RuntimeDescription, error) {
			return framework.RuntimeDescription{}, nil
		},
	}
	err := Register(inaccessibleManager{}, definition, Options[registrationConfig, registrationCluster, opaque]{}, registrationBinding())
	if err == nil || !strings.Contains(err.Error(), "shared facts") {
		t.Fatalf("opaque facts accepted: %v", err)
	}
}
