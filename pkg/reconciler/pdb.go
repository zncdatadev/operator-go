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

package reconciler

import (
	"context"

	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/client"
)

var _ ResourceReconciler[builder.PodDisruptionBudgetBuilder] = &PDB{}

type PDB struct {
	GenericResourceReconciler[builder.PodDisruptionBudgetBuilder]
}

func NewPDBReconciler(
	client *client.Client,
	name string,
	options ...builder.PDBBuilderOption,
) (*PDB, error) {
	b, err := builder.NewDefaultPDBBuilder(
		client,
		name,
		options...,
	)
	if err != nil {
		return nil, err
	}
	return &PDB{
		GenericResourceReconciler: *NewGenericResourceReconciler[builder.PodDisruptionBudgetBuilder](
			client,
			b,
		),
	}, nil
}

func (r *PDB) Reconcile(ctx context.Context) (ctrl.Result, error) {
	logger.V(5).Info("Building resource", "namespace", r.GetNamespace(), "cluster", r.GetName(), "name", r.GetName())
	resource, err := r.GetBuilder().Build(ctx)

	if err != nil {
		return ctrl.Result{}, err
	}

	logger.V(1).Info("Reconciling pdb resource", "namespace", r.GetNamespace(), "cluster", r.GetName(), "name", resource.GetName())
	logExtraValues := []any{
		"name", resource.GetName(),
		"namespace", resource.GetNamespace(),
		"cluster", r.GetName(),
	}

	obj := &policyv1.PodDisruptionBudget{}
	if err := r.GetClient().Client.Get(ctx, ctrlclient.ObjectKey{Namespace: resource.GetNamespace(), Name: resource.GetName()}, obj); err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(1).Info("Resource pdb not found, will create", logExtraValues...)
			if err := r.GetClient().Client.Create(ctx, resource); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: r.RequeueAfter}, nil
		}
		logger.Error(err, "Failed to fetch resource", logExtraValues...)
		return ctrl.Result{}, err
	}

	logger.V(1).Info("Updating pdb resource", logExtraValues...)

	newPdb := resource.(*policyv1.PodDisruptionBudget).DeepCopy()
	objDeepCopy := obj.DeepCopy()
	objDeepCopy.Spec = newPdb.Spec
	objDeepCopy.Labels = newPdb.Labels
	objDeepCopy.Annotations = newPdb.Annotations

	if err := r.GetClient().Client.Patch(ctx, objDeepCopy, ctrlclient.MergeFrom(obj)); err != nil {
		logger.Error(err, "Failed to update resource", logExtraValues...)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
