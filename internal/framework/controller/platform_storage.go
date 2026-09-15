package controller

import (
	"encoding/json"
	"fmt"

	"github.com/zncdatadev/operator-go/internal/framework/pipeline"
	"github.com/zncdatadev/operator-go/pkg/framework"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const platformClaimsAnnotation = "framework.kubedoop.dev/platform-claims"

type platformClaimsReceipt struct {
	Version int             `json:"version"`
	CRUID   types.UID       `json:"crUID"`
	Role    string          `json:"role"`
	Group   string          `json:"group"`
	Volumes []corev1.Volume `json:"volumes"`
}

// This receipt is written only after reserved-key validation and reconstruction
// from the typed declaration. It survives CR edits and controller restarts.
func stampPlatformClaims(owner client.Object, object client.Object, slot *groupSlot,
	runtime *framework.RuntimeDescription,
) (client.Object, error) {
	sts, ok := object.(*appsv1.StatefulSet)
	if !ok {
		return object, nil
	}
	volumes := pipeline.DeclaredPlatformClaims(runtime)
	if len(volumes) == 0 {
		return object, nil
	}
	if slot == nil || owner.GetUID() == "" {
		return nil, fmt.Errorf("platform claims require an authenticated group")
	}
	receipt := platformClaimsReceipt{Version: 1, CRUID: owner.GetUID(), Role: slot.Role,
		Group: slot.Group, Volumes: volumes}
	next := sts.DeepCopy()
	if next.Annotations == nil {
		next.Annotations = map[string]string{}
	}
	encoded, err := json.Marshal(receipt)
	if err != nil {
		return nil, err
	}
	next.Annotations[platformClaimsAnnotation] = string(encoded)
	if _, err := platformClaimNames(next); err != nil {
		return nil, err
	}
	return next, nil
}

func platformClaimNames(sts *appsv1.StatefulSet) (map[string]bool, error) {
	names := map[string]bool{}
	text := sts.Annotations[platformClaimsAnnotation]
	if text == "" {
		return names, nil
	}
	var receipt platformClaimsReceipt
	if err := strictReceipt(text, &receipt); err != nil {
		return nil, err
	}
	if receipt.Version != 1 || receipt.CRUID == "" || receipt.Role == "" || receipt.Group == "" ||
		len(receipt.Volumes) == 0 {
		return nil, fmt.Errorf("invalid platform claim declaration receipt")
	}
	for _, declared := range receipt.Volumes {
		if declared.Name == "" || declared.Ephemeral == nil || names[declared.Name] {
			return nil, fmt.Errorf("invalid platform claim slot")
		}
		found := false
		for _, actual := range sts.Spec.Template.Spec.Volumes {
			if actual.Name == declared.Name {
				if found || !apiequality.Semantic.DeepEqual(actual, declared) {
					return nil, fmt.Errorf("platform claim %s differs from its declaration receipt", declared.Name)
				}
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("platform claim %s is missing", declared.Name)
		}
		names[declared.Name] = true
	}
	return names, nil
}

func validatePlatformClaimOwner(sts *appsv1.StatefulSet, owner client.Object, group groupSlot) error {
	if _, err := platformClaimNames(sts); err != nil {
		return err
	}
	text := sts.Annotations[platformClaimsAnnotation]
	if text == "" {
		return nil
	}
	var receipt platformClaimsReceipt
	if err := strictReceipt(text, &receipt); err != nil {
		return err
	}
	if receipt.CRUID != owner.GetUID() || receipt.Role != group.Role || receipt.Group != group.Group {
		return fmt.Errorf("platform claim declaration belongs to another CR or group")
	}
	return nil
}
