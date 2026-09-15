package dataops

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EnsureAsset records a fully checked binding independently of the product CR.
// Its immutable initial identity is never synthesized from a replacement claim.
func EnsureAsset(ctx context.Context, c client.Client, claim *corev1.PersistentVolumeClaim, owner ClusterRef) error {
	var source Source
	var binding Binding
	if err := json.Unmarshal([]byte(claim.Annotations[SourceAnnotation]), &source); err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(claim.Annotations[BindingAnnotation]), &binding); err != nil {
		return err
	}
	if owner.UID != source.CRUID || owner.APIVersion == "" || owner.Kind == "" || owner.Name == "" || source.CRUID == "" || binding.PVCUID != claim.UID || binding.PVUID == "" {
		return fmt.Errorf("incomplete retained identity")
	}
	identity := DataIdentity{Cluster: owner, ClaimName: claim.Name, Source: source, Binding: binding}
	name := claim.Annotations[AssetAnnotation]
	if name == "" {
		name = "data-" + string(claim.UID)
	}
	asset := &DataAsset{}
	err := c.Get(ctx, client.ObjectKey{Namespace: claim.Namespace, Name: name}, asset)
	if apierrors.IsNotFound(err) {
		if claim.Annotations[AssetAnnotation] != "" {
			return fmt.Errorf("data asset %s was lost", name)
		}
		asset = &DataAsset{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: claim.Namespace}, Spec: identity}
		if err = c.Create(ctx, asset); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	current := asset.Spec
	if asset.Status.Current != nil {
		current = *asset.Status.Current
	}
	if asset.Status.Destroyed || current != identity {
		return fmt.Errorf("data asset identity differs from retained binding")
	}
	if claim.Annotations[AssetAnnotation] == name {
		return nil
	}
	next := claim.DeepCopy()
	next.Annotations[AssetAnnotation] = name
	return c.Update(ctx, next)
}

// CheckHistory prevents a disappeared recorded ordinal from being recreated as fresh data.
func CheckHistory(ctx context.Context, c client.Client, namespace string, source Source, claims []corev1.PersistentVolumeClaim) error {
	var assets DataAssetList
	if err := c.List(ctx, &assets, client.InNamespace(namespace)); err != nil {
		return err
	}
	observed := map[string]corev1.PersistentVolumeClaim{}
	for _, claim := range claims {
		observed[claim.Name] = claim
	}
	for _, asset := range assets.Items {
		current := asset.Spec
		if asset.Status.Current != nil {
			current = *asset.Status.Current
		}
		for _, previous := range append([]History{{From: asset.Spec}}, asset.Status.History...) {
			former := previous.From.Source
			if former.CRUID == source.CRUID && former.Role == source.Role && former.Group == source.Group &&
				(current.Source.CRUID != former.CRUID || current.Source.Role != former.Role || current.Source.Group != former.Group) {
				return fmt.Errorf("data asset %s moved to another owner; source group cannot invent replacement data", asset.Name)
			}
		}
		if current.Source.CRUID != source.CRUID || current.Source.Role != source.Role || current.Source.Group != source.Group {
			continue
		}
		if asset.Status.Destroyed {
			return fmt.Errorf("data asset %s was destroyed; a fresh data identity requires a new group", asset.Name)
		}
		claim, ok := observed[current.ClaimName]
		if !ok || claim.UID != current.Binding.PVCUID {
			return fmt.Errorf("recorded data asset %s lost its original PVC", asset.Name)
		}
		if asset.Annotations[LockAnnotation] != "" {
			return fmt.Errorf("data asset %s has an active explicit operation", asset.Name)
		}
	}
	return nil
}
