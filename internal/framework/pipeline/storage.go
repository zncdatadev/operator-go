package pipeline

import (
	"encoding/json"
	"fmt"

	"github.com/zncdatadev/operator-go/pkg/framework"
	"k8s.io/apimachinery/pkg/util/validation"
)

// Storage has a fixed discriminator rule. It is deliberately outside the
// generic object merger: changing type starts a fresh branch; keeping or
// omitting type inherits fields within the current branch.
func foldStorageLayer(base, layer map[string]json.RawMessage) error {
	var resources, inherited map[string]json.RawMessage
	if err := json.Unmarshal(layer["resources"], &resources); err != nil {
		if len(layer["resources"]) == 0 {
			return nil
		}
		return err
	}
	patch, present := resources["storage"]
	if !present {
		return nil
	}
	if err := json.Unmarshal(base["resources"], &inherited); err != nil {
		return err
	}
	var next, previous map[string]json.RawMessage
	if err := json.Unmarshal(patch, &next); err != nil {
		return err
	}
	if err := json.Unmarshal(inherited["storage"], &previous); err != nil {
		return err
	}
	if discriminator, present := next["type"]; present {
		var oldType, newType framework.StorageType
		if err := json.Unmarshal(discriminator, &newType); err != nil {
			return err
		}
		if err := json.Unmarshal(previous["type"], &oldType); err != nil {
			return err
		}
		if oldType == "" {
			oldType = framework.StorageEphemeral
		}
		if oldType != newType {
			previous = make(map[string]json.RawMessage)
		}
	}
	merged, err := json.Marshal(mergeObjects(previous, next))
	if err != nil {
		return err
	}
	inherited["storage"] = merged
	base["resources"], err = json.Marshal(inherited)
	if err != nil {
		return err
	}
	delete(resources, "storage")
	layer["resources"], err = json.Marshal(resources)
	return err
}

// Syntax and contradictory fields are checked for every layer, before another
// layer could hide them. Partial persistent inputs remain valid for inheritance.
func validateStorageLayer(data json.RawMessage, path string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return fmt.Errorf("%s: storage must be an object", path)
	}
	if raw, present := fields["type"]; present {
		var kind framework.StorageType
		if err := json.Unmarshal(raw, &kind); err != nil ||
			(kind != framework.StorageEphemeral && kind != framework.StoragePersistent) {
			return fmt.Errorf("%s.type: must be ephemeral or persistent", path)
		}
		if kind == framework.StorageEphemeral {
			if _, present := fields["storageClassName"]; present {
				return fmt.Errorf("%s: ephemeral storage cannot specify storageClassName", path)
			}
			if _, present := fields["capacity"]; present {
				return fmt.Errorf("%s: ephemeral storage cannot specify capacity", path)
			}
		}
	}
	return nil
}

func validateStorage(storage framework.Storage) error {
	switch storage.Type {
	case "", framework.StorageEphemeral:
		if storage.StorageClassName != "" || !storage.Capacity.IsZero() {
			return fmt.Errorf("config.resources.storage: ephemeral storage cannot have storageClassName or capacity")
		}
	case framework.StoragePersistent:
		if storage.StorageClassName == "" || len(validation.IsDNS1123Subdomain(storage.StorageClassName)) != 0 ||
			storage.Capacity.Sign() <= 0 {
			return fmt.Errorf("config.resources.storage: persistent storage requires " +
				"an explicit storage class and positive capacity")
		}
	default:
		return fmt.Errorf("config.resources.storage.type: must be ephemeral or persistent")
	}
	return nil
}
