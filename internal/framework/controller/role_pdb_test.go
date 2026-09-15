package controller

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/zncdatadev/operator-go/internal/framework/pipeline"
	"github.com/zncdatadev/operator-go/pkg/framework"
	"github.com/zncdatadev/operator-go/pkg/framework/input"

	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	generatedtrino "github.com/zncdatadev/operator-go/internal/framework/pipeline/testinput"
)

func roleBudgetFixture(t *testing.T, intercept interceptor.Funcs,
) (*Reconciler[*generatedtrino.TrinoCluster, testConfig, testClusterConfig, testFacts],
	*generatedtrino.TrinoCluster, pipeline.BuiltRole,
) {
	t.Helper()
	cr := controllerInput()
	c := fake.NewClientBuilder().WithScheme(controllerScheme(t)).WithObjects(cr).
		WithStatusSubresource(cr, &policyv1.PodDisruptionBudget{}).WithInterceptorFuncs(intercept).Build()
	r := newTestReconciler(c, c.Scheme(), testFacts{})
	source := pipeline.SourceSnapshot[testFacts]{
		Cluster: framework.ClusterIdentity{Name: cr.Name, Namespace: cr.Namespace},
		Roles:   []pipeline.RoleSource{{Name: "workers"}},
		Groups: []pipeline.GroupSource{{Role: "workers", Name: "a", Replicas: 2},
			{Role: "workers", Name: "b", Replicas: 3}},
	}
	roles, err := pipeline.BuildRoleResources(r.Definition, source)
	if err != nil || len(roles) != 1 || roles[0].Error != "" || roles[0].PodDisruptionBudget == nil {
		t.Fatalf("build role budget: roles=%+v err=%v", roles, err)
	}
	return r, cr, roles[0]
}

func readRoleBudget(t *testing.T, c client.Client, role pipeline.BuiltRole) *policyv1.PodDisruptionBudget {
	t.Helper()
	live := role.PodDisruptionBudget.DeepCopy()
	applyTestGet(t, c, live)
	return live
}

func TestRolePDBApplyPreservesStatusAndNoop(t *testing.T) {
	persisted := 0
	r, cr, role := roleBudgetFixture(t, interceptor.Funcs{Update: func(ctx context.Context, c client.WithWatch,
		object client.Object, options ...client.UpdateOption) error {
		if len((&client.UpdateOptions{}).ApplyOptions(options).DryRun) == 0 {
			persisted++
		}
		return c.Update(ctx, object, options...)
	}})
	if pending, err := r.applyBuiltRole(t.Context(), cr, role); err != nil || pending {
		t.Fatalf("create role budget pending=%t err=%v", pending, err)
	}
	live := readRoleBudget(t, r.Client, role)
	if live.Spec.MinAvailable.IntVal != 4 || live.Spec.MaxUnavailable != nil ||
		len(live.Spec.Selector.MatchLabels) != 2 || live.Spec.Selector.MatchLabels["role-group"] != "" {
		t.Fatalf("unexpected cross-group budget: %+v", live.Spec)
	}
	live.Status.DisruptionsAllowed, live.Status.ObservedGeneration = 2, 7
	if err := r.Client.Status().Update(t.Context(), live); err != nil {
		t.Fatal(err)
	}
	live = readRoleBudget(t, r.Client, role)
	live.Annotations["another.controller/keep"] = "external"
	if err := r.Client.Update(t.Context(), live); err != nil {
		t.Fatal(err)
	}
	live = readRoleBudget(t, r.Client, role)
	version := live.ResourceVersion
	persisted = 0
	if _, err := r.applyBuiltRole(t.Context(), cr, role); err != nil {
		t.Fatal(err)
	}
	if got := readRoleBudget(t, r.Client, role); got.ResourceVersion != version || persisted != 0 {
		t.Fatal("identical role budget caused a persisted update")
	}
	minimum := intstr.FromInt32(3)
	role.PodDisruptionBudget.Spec.MinAvailable = &minimum
	if _, err := r.applyBuiltRole(t.Context(), cr, role); err != nil {
		t.Fatal(err)
	}
	got := readRoleBudget(t, r.Client, role)
	if persisted != 1 || got.Spec.MinAvailable.IntVal != 3 || got.Status.DisruptionsAllowed != 2 ||
		got.Status.ObservedGeneration != 7 || got.Annotations["another.controller/keep"] != "external" {
		t.Fatalf("role budget apply lost independent state: updates=%d object=%+v", persisted, got)
	}
}

func TestRolePDBConvergesDespiteGroupBuildFailure(t *testing.T) {
	r, cr := factsTestReconciler(t, nil)
	r.Definition.ValidateInput = func(framework.EffectiveInput[testConfig, testClusterConfig, testFacts]) error {
		return errors.New("deliberate product validation failure")
	}
	_, err := r.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(cr)})
	if err == nil || !strings.Contains(err.Error(), "deliberate product validation failure") {
		t.Fatalf("missing group pipeline error: %v", err)
	}
	live := &policyv1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{
		Name: cr.Name + "-workers-pdb", Namespace: cr.Namespace}}
	applyTestGet(t, r.Client, live)
	if live.Spec.MinAvailable.IntVal != 1 {
		t.Fatalf("failed groups were excluded from desired replicas: %+v", live.Spec)
	}
	current := cr.DeepCopy()
	applyTestGet(t, r.Client, current)
	if !meta.IsStatusConditionTrue(current.Status.Conditions, "RoleResourcesApplied") ||
		meta.IsStatusConditionTrue(current.Status.Conditions, "Applied") {
		t.Fatalf("role and workload outcomes were conflated: %+v", current.Status)
	}
}

func TestRolePDBPendingFactsKeepCompleteBudget(t *testing.T) {
	r, cr := factsTestReconciler(t, nil)
	r.ResolveFacts = func(context.Context, framework.FactsReader,
		framework.FactInput[testConfig, testClusterConfig, testFacts],
	) (
		framework.FactResult[testFacts], error) {
		return framework.FactResult[testFacts]{Diagnostic: framework.FactDiagnostic{
			State: framework.FactsPending, Reason: "Missing", Message: "waiting"}}, nil
	}
	result, err := r.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(cr)})
	if err != nil || result.RequeueAfter != retirementPoll {
		t.Fatalf("pending facts lost cadence: %v %v", result, err)
	}
	live := &policyv1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{
		Name: cr.Name + "-workers-pdb", Namespace: cr.Namespace}}
	applyTestGet(t, r.Client, live)
	if live.Spec.MinAvailable.IntVal != 1 {
		t.Fatal("pending facts reduced role budget")
	}
}

func TestRolePDBDisabledWaitsForDeletionAndReadd(t *testing.T) {
	r, cr, role := roleBudgetFixture(t, interceptor.Funcs{})
	if _, err := r.applyBuiltRole(t.Context(), cr, role); err != nil {
		t.Fatal(err)
	}
	live := readRoleBudget(t, r.Client, role)
	live.Finalizers = []string{"fixture.design/hold"}
	if err := r.Client.Update(t.Context(), live); err != nil {
		t.Fatal(err)
	}
	disabled := role
	disabled.Config = input.Clone(role.Config)
	disabled.Config.PodDisruptionBudget.Enabled = false
	disabled.PodDisruptionBudget = nil
	if pending, err := r.applyBuiltRole(t.Context(), cr, disabled); err != nil || !pending {
		t.Fatalf("disabled budget was not pending deletion: %t %v", pending, err)
	}
	live = readRoleBudget(t, r.Client, role)
	if live.DeletionTimestamp.IsZero() {
		t.Fatal("disabled role did not issue deletion")
	}
	version := live.ResourceVersion
	if pending, err := r.applyBuiltRole(t.Context(), cr, role); err != nil || !pending {
		t.Fatalf("re-add did not wait for accepted deletion: %t %v", pending, err)
	}
	if readRoleBudget(t, r.Client, role).ResourceVersion != version {
		t.Fatal("re-add rewrote terminating budget")
	}
}

func TestRolePDBRetirementUsesCompleteInventoryAndRetainsInvalidConfig(t *testing.T) {
	r, cr, role := roleBudgetFixture(t, interceptor.Funcs{})
	if _, err := r.applyBuiltRole(t.Context(), cr, role); err != nil {
		t.Fatal(err)
	}
	invalid := pipeline.BuiltRole{Role: role.Role, Error: "invalid management config"}
	status := framework.ReconcileStatus{}
	if _, errs := r.reconcileRoleResources(t.Context(), cr, []pipeline.BuiltRole{invalid},
		[]framework.RoleIdentity{role.Role}, nil, nil, &status); len(errs) == 0 {
		t.Fatal("invalid role config was accepted")
	}
	readRoleBudget(t, r.Client, role)
	status = framework.ReconcileStatus{}
	inventoryErr := errors.New("incomplete role inventory")
	if _, errs := r.reconcileRoleResources(t.Context(), cr, nil, nil,
		inventoryErr, inventoryErr, &status); len(errs) == 0 {
		t.Fatal("incomplete inventory was accepted")
	}
	readRoleBudget(t, r.Client, role)
	status = framework.ReconcileStatus{}
	pending, errs := r.reconcileRoleResources(t.Context(), cr, nil, nil, nil, nil, &status)
	if len(errs) != 0 || !pending {
		t.Fatalf("removed role did not retire: %t %v", pending, errs)
	}
	if err := r.Client.Get(t.Context(), client.ObjectKeyFromObject(role.PodDisruptionBudget),
		&policyv1.PodDisruptionBudget{}); !apierrors.IsNotFound(err) {
		t.Fatalf("removed role budget remains: %v", err)
	}
}

func TestRolePDBCannotForgeOrAdoptSlot(t *testing.T) {
	r, cr, role := roleBudgetFixture(t, interceptor.Funcs{})
	for _, metadata := range []string{"annotation", "label"} {
		desired := role.PodDisruptionBudget.DeepCopy()
		if metadata == "annotation" {
			desired.Annotations = map[string]string{RolePDBAnnotation: `{"role":"workers","slot":"pdb"}`}
		} else {
			desired.Labels[RolePDBAnnotation] = "forged"
		}
		if changed, err := ApplyObject(t.Context(), r.Client, cr, desired, r.Scheme); err == nil || changed {
			t.Fatalf("public apply accepted forged %s", metadata)
		}
	}
	if _, err := r.applyBuiltRole(t.Context(), cr, role); err != nil {
		t.Fatal(err)
	}
	if changed, err := ApplyObject(t.Context(), r.Client, cr, role.PodDisruptionBudget, r.Scheme); err == nil || changed {
		t.Fatal("public apply captured role slot")
	}
	live := readRoleBudget(t, r.Client, role)
	delete(live.Annotations, RolePDBAnnotation)
	if err := r.Client.Update(t.Context(), live); err != nil {
		t.Fatal(err)
	}
	if changed, err := ApplyObject(t.Context(), r.Client, cr, role.PodDisruptionBudget, r.Scheme); err == nil || changed {
		t.Fatal("public apply captured role slot after its receipt was removed")
	}
	if _, err := r.applyBuiltRole(t.Context(), cr, role); err == nil {
		t.Fatal("role apply adopted damaged slot")
	}
	status := framework.ReconcileStatus{}
	if _, errs := r.retireRolePDBs(t.Context(), cr, nil, &status); len(errs) == 0 {
		t.Fatal("damaged removed slot silently disappeared from inventory")
	}
	readRoleBudget(t, r.Client, role)
}

func TestRolePDBDeletionConflictRechecksCurrentInput(t *testing.T) {
	deletes := 0
	r, cr, role := roleBudgetFixture(t, interceptor.Funcs{Delete: func(ctx context.Context, c client.WithWatch,
		object client.Object, options ...client.DeleteOption) error {
		deletes++
		preconditions := (&client.DeleteOptions{}).ApplyOptions(options).Preconditions
		if preconditions == nil || preconditions.UID == nil || preconditions.ResourceVersion == nil ||
			*preconditions.UID != object.GetUID() || *preconditions.ResourceVersion != object.GetResourceVersion() {
			t.Fatal("role deletion omitted exact UID/RV preconditions")
		}
		current := controllerInput()
		if err := c.Get(ctx, client.ObjectKeyFromObject(current), current); err != nil {
			return err
		}
		current.Generation++
		if err := c.Update(ctx, current); err != nil {
			return err
		}
		return apierrors.NewConflict(schema.GroupResource{Resource: "poddisruptionbudgets"},
			object.GetName(), errors.New("competing input change"))
	}})
	if _, err := r.applyBuiltRole(t.Context(), cr, role); err != nil {
		t.Fatal(err)
	}
	_, err := r.deleteRolePDB(t.Context(), cr, rolePDBSlot{Role: role.Role.Name, Slot: rolePDBKind})
	if !errors.Is(err, errSuperseded) || deletes != 1 {
		t.Fatalf("delete retried superseded intent: deletes=%d err=%v", deletes, err)
	}
	readRoleBudget(t, r.Client, role)
}

func TestRolePDBCustomResourceIsNotRetirementInventory(t *testing.T) {
	r, cr, role := roleBudgetFixture(t, interceptor.Funcs{})
	custom := role.PodDisruptionBudget.DeepCopy()
	custom.Name = "custom-budget"
	if _, err := ApplyObject(t.Context(), r.Client, cr, custom, r.Scheme); err != nil {
		t.Fatal(err)
	}
	status := framework.ReconcileStatus{}
	if pending, errs := r.retireRolePDBs(t.Context(), cr, nil, &status); pending || len(errs) != 0 {
		t.Fatalf("custom budget became a role slot: %t %v", pending, errs)
	}
	applyTestGet(t, r.Client, custom)
	record, err := decodeManagedMetadata(custom)
	if err != nil || slices.Contains(record.Object.Annotations, RolePDBAnnotation) {
		t.Fatalf("custom budget received a role receipt: %+v %v", record, err)
	}
}
