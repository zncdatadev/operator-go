package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/zncdatadev/operator-go/internal/framework/pipeline"
	"github.com/zncdatadev/operator-go/pkg/framework"

	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	kubernetesjson "sigs.k8s.io/json"
)

// RolePDBAnnotation is a controller-issued receipt for the one budget slot of a
// role. Products and input metadata cannot supply this deletion authority.
const RolePDBAnnotation = "framework.kubedoop.dev/role-pdb"
const rolePDBKind = "pdb"

var errRolePDBTerminating = errors.New("waiting for terminating role PodDisruptionBudget")

type rolePDBSlot struct {
	Role string `json:"role"`
	Slot string `json:"slot"`
}

func (s rolePDBSlot) object(owner client.Object) *policyv1.PodDisruptionBudget {
	role := framework.RoleIdentity{ClusterIdentity: framework.ClusterIdentity{
		Name: owner.GetName(), Namespace: owner.GetNamespace()}, Name: s.Role}
	return &policyv1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{
		Name: role.PodDisruptionBudgetName(), Namespace: role.Namespace}}
}

func decodeRolePDB(object client.Object) (*rolePDBSlot, error) {
	raw, present := object.GetAnnotations()[RolePDBAnnotation]
	if !present {
		return nil, nil
	}
	var slot rolePDBSlot
	strict, err := kubernetesjson.UnmarshalStrict([]byte(raw), &slot)
	if err != nil || len(strict) != 0 || len(validation.IsDNS1123Label(slot.Role)) != 0 || slot.Slot != rolePDBKind {
		return nil, fmt.Errorf("invalid role PDB receipt on %T %s", object, object.GetName())
	}
	return &slot, nil
}

func stampRolePDB(desired, owner client.Object, slot rolePDBSlot) (client.Object, error) {
	if _, ok := desired.(*policyv1.PodDisruptionBudget); !ok ||
		client.ObjectKeyFromObject(desired) != client.ObjectKeyFromObject(slot.object(owner)) ||
		len(validation.IsDNS1123Label(slot.Role)) != 0 || slot.Slot != rolePDBKind {
		return nil, fmt.Errorf("role PDB slot does not match its desired resource")
	}
	next := desired.DeepCopyObject().(client.Object)
	annotations := maps.Clone(next.GetAnnotations())
	if annotations == nil {
		annotations = map[string]string{}
	}
	raw, err := json.Marshal(slot)
	if err != nil {
		return nil, err
	}
	annotations[RolePDBAnnotation] = string(raw)
	next.SetAnnotations(annotations)
	return next, nil
}

func checkRolePDBObject(owner, object client.Object, kind schema.GroupVersionKind, expected rolePDBSlot) error {
	copy := object.DeepCopyObject().(client.Object)
	copy.SetDeletionTimestamp(nil)
	if err := checkOwnership(owner, copy, kind); err != nil {
		return err
	}
	actual, err := decodeRolePDB(object)
	if err != nil {
		return err
	}
	if actual == nil || *actual != expected {
		return fmt.Errorf("role PDB %s has a missing or mismatched role receipt", object.GetName())
	}
	if _, ok := object.(*policyv1.PodDisruptionBudget); !ok ||
		client.ObjectKeyFromObject(object) != client.ObjectKeyFromObject(expected.object(owner)) {
		return fmt.Errorf("role PDB %s has a mismatched kind, name or namespace", object.GetName())
	}
	labels := object.GetLabels()
	if labels["app.kubernetes.io/instance"] != owner.GetName() || labels["app.kubernetes.io/component"] != expected.Role {
		return fmt.Errorf("role PDB %s has damaged identity labels", object.GetName())
	}
	record, err := decodeManagedMetadata(object)
	if err != nil {
		return err
	}
	if !slices.Contains(record.Object.Annotations, RolePDBAnnotation) {
		return fmt.Errorf("role PDB %s is absent from the managed metadata record", object.GetName())
	}
	return nil
}

func checkApplyRolePDB(owner, live client.Object, kind schema.GroupVersionKind, expected *rolePDBSlot) error {
	actual, err := decodeRolePDB(live)
	if err != nil {
		return err
	}
	if expected == nil {
		record, recordErr := decodeManagedMetadata(live)
		candidate := false
		if pdb, ok := live.(*policyv1.PodDisruptionBudget); ok {
			candidate = looksLikeRolePDB(owner, pdb)
		}
		if actual != nil || candidate || (recordErr == nil && slices.Contains(record.Object.Annotations, RolePDBAnnotation)) {
			return fmt.Errorf("%T %s belongs to a role PDB slot", live, live.GetName())
		}
		return nil
	}
	if err := checkRolePDBObject(owner, live, kind, *expected); err != nil {
		return err
	}
	if !live.GetDeletionTimestamp().IsZero() {
		return errRolePDBTerminating
	}
	return nil
}

func (r *Reconciler[CR, C, S, F]) readRolePDB(ctx context.Context, cr CR, slot rolePDBSlot,
) (*policyv1.PodDisruptionBudget, error) {
	object := slot.object(cr)
	if err := r.Client.Get(ctx, client.ObjectKeyFromObject(object), object); err != nil {
		return nil, client.IgnoreNotFound(err)
	}
	kind, err := apiutil.GVKForObject(cr, r.Scheme)
	if err != nil {
		return nil, err
	}
	if err := checkRolePDBObject(cr, object, kind, slot); err != nil {
		return nil, err
	}
	return object, nil
}

// Deletion observes the exact current slot again after every conflict. Neither
// an earlier list nor a status entry authorizes deleting a replacement object.
func (r *Reconciler[CR, C, S, F]) deleteRolePDB(ctx context.Context, cr CR, slot rolePDBSlot) (bool, error) {
	pending := false
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := r.currentInput(ctx, cr); err != nil {
			return err
		}
		live, err := r.readRolePDB(ctx, cr, slot)
		if err != nil || live == nil {
			pending = false
			return err
		}
		pending = true
		if !live.DeletionTimestamp.IsZero() {
			return nil
		}
		if err := r.currentInput(ctx, cr); err != nil {
			return err
		}
		uid, version := live.UID, live.ResourceVersion
		return client.IgnoreNotFound(r.Client.Delete(ctx, live, &client.DeleteOptions{
			Preconditions:     &metav1.Preconditions{UID: &uid, ResourceVersion: &version},
			PropagationPolicy: deletionPropagation()}))
	})
	return pending, err
}

// Only controlled, receipted role slots are inventory. A custom PDB with no
// receipt is untouched; damage to a recognizable framework slot remains visible.
func (r *Reconciler[CR, C, S, F]) rolePDBInventory(ctx context.Context, cr CR) ([]rolePDBSlot, []error) {
	var list policyv1.PodDisruptionBudgetList
	if err := r.Client.List(ctx, &list, client.InNamespace(cr.GetNamespace())); err != nil {
		return nil, []error{err}
	}
	kind, err := apiutil.GVKForObject(cr, r.Scheme)
	if err != nil {
		return nil, []error{err}
	}
	slices.SortFunc(list.Items, func(a, b policyv1.PodDisruptionBudget) int { return strings.Compare(a.Name, b.Name) })
	var slots []rolePDBSlot
	var failures []error
	for i := range list.Items {
		object := &list.Items[i]
		controller := metav1.GetControllerOf(object)
		if controller == nil || controller.UID != cr.GetUID() {
			continue
		}
		slot, err := decodeRolePDB(object)
		if err == nil && slot == nil && looksLikeRolePDB(cr, object) {
			err = fmt.Errorf("controlled role PDB candidate %s is missing its role receipt", object.Name)
		}
		if err == nil && slot != nil {
			err = checkRolePDBObject(cr, object, kind, *slot)
		}
		if err != nil {
			failures = append(failures, err)
		} else if slot != nil {
			slots = append(slots, *slot)
		}
	}
	slices.SortFunc(slots, func(a, b rolePDBSlot) int { return strings.Compare(a.Role, b.Role) })
	return slots, failures
}

func looksLikeRolePDB(owner client.Object, object *policyv1.PodDisruptionBudget) bool {
	record, err := decodeManagedMetadata(object)
	if err == nil && slices.Contains(record.Object.Annotations, RolePDBAnnotation) {
		return true
	}
	role := object.Labels["app.kubernetes.io/component"]
	return role != "" && object.Name == (rolePDBSlot{Role: role, Slot: rolePDBKind}).object(owner).Name
}

func (r *Reconciler[CR, C, S, F]) applyBuiltRole(ctx context.Context, cr CR, role pipeline.BuiltRole,
) (bool, error) {
	if role.Error != "" {
		return false, errors.New(role.Error)
	}
	if role.Config == nil {
		return false, fmt.Errorf("role did not produce resolved management configuration")
	}
	slot := rolePDBSlot{Role: role.Role.Name, Slot: rolePDBKind}
	if !role.Config.PodDisruptionBudget.Enabled {
		return r.deleteRolePDB(ctx, cr, slot)
	}
	if role.PodDisruptionBudget == nil {
		return false, fmt.Errorf("enabled role did not produce a PodDisruptionBudget")
	}
	if err := r.currentInput(ctx, cr); err != nil {
		return false, err
	}
	_, err := applyScopedObject(ctx, r.Client, cr, role.PodDisruptionBudget, r.Scheme, nil, nil, &slot, nil, nil)
	if errors.Is(err, errRolePDBTerminating) {
		return true, nil
	}
	return false, err
}

func (r *Reconciler[CR, C, S, F]) reconcileRoleResources(ctx context.Context, cr CR, roles []pipeline.BuiltRole,
	desired []framework.RoleIdentity, buildErr, inventoryErr error, status *framework.ReconcileStatus,
) (bool, []error) {
	var failures []error
	pending := false
	if buildErr != nil {
		failures = append(failures, buildErr)
		setCondition(status, "Built", false, "BuildFailed", buildErr.Error())
	}
	for _, role := range roles {
		waiting, err := r.applyBuiltRole(ctx, cr, role)
		pending = pending || waiting
		observation := framework.RoleReconcileStatus{Name: role.Role.Name, Applied: err == nil && !waiting}
		if err != nil {
			observation.Message = err.Error()
			failures = append(failures, fmt.Errorf("role %s: %w", role.Role.Name, err))
		} else if waiting {
			observation.Message = "Waiting for role PodDisruptionBudget deletion before convergence"
		}
		status.Roles = append(status.Roles, observation)
		if role.Config == nil || role.Error != "" {
			setCondition(status, "Built", false, "BuildFailed", "Some role management configurations could not be built")
		}
	}
	if inventoryErr == nil {
		waiting, errs := r.retireRolePDBs(ctx, cr, desired, status)
		pending = pending || waiting
		failures = append(failures, errs...)
	} else {
		failures = append(failures, inventoryErr)
	}
	slices.SortFunc(status.Roles, func(a, b framework.RoleReconcileStatus) int { return strings.Compare(a.Name, b.Name) })
	complete := !pending && len(failures) == 0
	setCondition(status, "RoleResourcesApplied", complete,
		choose(complete, "RoleResourcesApplied", "RoleResourcesIncomplete"),
		choose(complete, "Desired role budgets match and removed role budget slots are absent",
			choose(len(failures) != 0, errorMessage(failures), "Waiting for role PodDisruptionBudget deletion")))
	if !complete {
		setCondition(status, "Applied", false, "ApplyIncomplete", "Role resources have not converged")
	}
	return pending, failures
}

func (r *Reconciler[CR, C, S, F]) retireRolePDBs(ctx context.Context, cr CR, desired []framework.RoleIdentity,
	status *framework.ReconcileStatus,
) (bool, []error) {
	wanted := make(map[string]bool, len(desired))
	for _, role := range desired {
		wanted[role.Name] = true
	}
	slots, failures := r.rolePDBInventory(ctx, cr)
	pending := false
	for _, slot := range slots {
		if wanted[slot.Role] {
			continue
		}
		waiting, err := r.deleteRolePDB(ctx, cr, slot)
		pending = pending || waiting
		observation := framework.RoleReconcileStatus{Name: slot.Role, Applied: err == nil && !waiting}
		if err != nil {
			observation.Message = err.Error()
			failures = append(failures, fmt.Errorf("retire role %s: %w", slot.Role, err))
		} else if waiting {
			observation.Message = "Waiting for removed role PodDisruptionBudget to disappear"
		}
		status.Roles = append(status.Roles, observation)
		if errors.Is(err, errSuperseded) {
			break
		}
	}
	return pending, failures
}
