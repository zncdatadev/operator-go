package pipeline

import (
	"fmt"
	"reflect"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func validateRetainedData(r RuntimeDescription) error {
	slot := ""
	for _, directory := range r.Directories {
		if !directory.Data {
			continue
		}
		if slot != "" {
			return fmt.Errorf("one data directory is supported")
		}
		slot = directory.Name
		if r.ConfigDirectory == slot {
			return fmt.Errorf("data directory cannot be the configuration directory")
		}
		for _, file := range r.Files {
			if file.Directory == slot {
				return fmt.Errorf("generated files cannot write into the data directory")
			}
		}
		for _, log := range r.LogOutputs {
			if log.Directory == slot {
				return fmt.Errorf("log outputs require a separate ephemeral directory")
			}
		}
		writable := false
		for _, access := range r.Main.Access {
			writable = writable || (access.Directory == slot && !access.ReadOnly)
		}
		if !writable {
			return fmt.Errorf("data directory requires writable access by the main process")
		}
	}
	return nil
}

func retainedClaimTemplate(slot RetainedDataSlot) corev1.PersistentVolumeClaim {
	mode, class := corev1.PersistentVolumeFilesystem, slot.StorageClassName
	return corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: slot.Name},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			VolumeMode:  &mode, StorageClassName: &class,
			Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{
				corev1.ResourceStorage: slot.Capacity.DeepCopy(),
			}},
		}}
}

// These checks describe the final mount relation. They never rewrite overrides
// or infer that the program actually uses the mounted data.
func checkRetainedData(expected, actual GroupResources, runtime RuntimeDescription) []Check {
	slot := expected.RetainedData
	if slot == nil {
		return nil
	}
	check := Check{Subject: "assembly.retainedData[" + slot.Name + "]", State: Consistent,
		Reason: "declared data slot and writable mounts are retained"}
	if !reflect.DeepEqual(expected.StatefulSet.Spec.VolumeClaimTemplates, actual.StatefulSet.Spec.VolumeClaimTemplates) ||
		!reflect.DeepEqual(expected.StatefulSet.Spec.PersistentVolumeClaimRetentionPolicy,
			actual.StatefulSet.Spec.PersistentVolumeClaimRetentionPolicy) ||
		!reflect.DeepEqual(slot, actual.RetainedData) {
		check.State, check.Reason = Conflict, "retained declaration or claim policy changed during assembly"
		return []Check{check}
	}
	main := findContainer(actual.StatefulSet.Spec.Template, runtime.Main.Name)
	for _, access := range runtime.Main.Access {
		if access.Directory != slot.Name {
			continue
		}
		mount := findMount(main, access.MountPath)
		if mount == nil || mount.Name != slot.Name || mount.ReadOnly != access.ReadOnly ||
			mount.SubPath != "" || mount.SubPathExpr != "" {
			check.State, check.Reason = Conflict, "podOverrides displaced or changed a declared retained data mount"
			break
		}
		for _, other := range main.VolumeMounts {
			if strings.HasPrefix(other.MountPath, strings.TrimSuffix(access.MountPath, "/")+"/") {
				check.State, check.Reason = Conflict, "podOverrides added a mount inside the retained data directory"
			}
		}
	}
	return []Check{check}
}
