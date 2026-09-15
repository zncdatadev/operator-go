package pipeline

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/zncdatadev/operator-go/pkg/framework"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/utils/ptr"
)

const secretDriver = "secrets.kubedoop.dev"
const listenerDriver = "listeners.kubedoop.dev"

func validatePlatformVolumes(runtime RuntimeDescription) error {
	for _, d := range runtime.Directories {
		sources := 0
		if d.Data {
			sources++
		}
		if d.Secret != nil {
			sources++
		}
		if d.Listener != nil {
			sources++
		}
		if sources > 1 {
			return fmt.Errorf("directory %s declares multiple volume sources", d.Name)
		}
		if d.Secret == nil && d.Listener == nil {
			continue
		}
		if d.Name == runtime.ConfigDirectory {
			return fmt.Errorf("platform directory cannot be the generated config directory")
		}
		for _, file := range runtime.Files {
			if file.Directory == d.Name {
				return fmt.Errorf("generated files cannot write platform directory %s", d.Name)
			}
		}
		for _, log := range runtime.LogOutputs {
			if log.Directory == d.Name {
				return fmt.Errorf("logs cannot write platform directory %s", d.Name)
			}
		}
		accessed := false
		for _, process := range append([]Process{runtime.Main}, runtime.Initializers...) {
			for _, a := range process.Access {
				if a.Directory == d.Name {
					accessed = true
					if !a.ReadOnly {
						return fmt.Errorf("platform directory %s requires read-only access", d.Name)
					}
				}
			}
		}
		if !accessed {
			return fmt.Errorf("platform directory %s has no declared consumer", d.Name)
		}
		if err := validatePlatformSource(d); err != nil {
			return err
		}
	}
	return nil
}

func platformVolume(d Directory) (corev1.Volume, bool) {
	volume := corev1.Volume{Name: d.Name}
	if d.Secret == nil && d.Listener == nil {
		return volume, false
	}
	annotations := map[string]string{}
	class := listenerDriver
	if s := d.Secret; s != nil {
		if s.SecretName != "" {
			volume.Secret = &corev1.SecretVolumeSource{SecretName: s.SecretName, DefaultMode: ptr.To(int32(0440))}
			return volume, true
		}
		class = secretDriver
		annotations[secretDriver+"/class"] = s.SecretClass
		if s.Format != "" {
			annotations[secretDriver+"/format"] = s.Format
		}
		if len(s.Scope) > 0 {
			annotations[secretDriver+"/scope"] = strings.Join(s.Scope, ",")
		}
		if len(s.KerberosServiceNames) > 0 {
			annotations[secretDriver+"/kerberosServiceNames"] = strings.Join(s.KerberosServiceNames, ",")
		}
	} else {
		if d.Listener.Class != "" {
			annotations[listenerDriver+"/class"] = d.Listener.Class
		}
		if d.Listener.Name != "" {
			annotations[listenerDriver+"/listenerName"] = d.Listener.Name
		}
	}
	volume.Ephemeral = &corev1.EphemeralVolumeSource{VolumeClaimTemplate: &corev1.PersistentVolumeClaimTemplate{
		ObjectMeta: metav1.ObjectMeta{Annotations: annotations}, Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}, StorageClassName: &class,
			VolumeMode: ptr.To(corev1.PersistentVolumeFilesystem), Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Mi")}}}}}
	return volume, true
}

func checkPlatformVolumes(expected, actual GroupResources, runtime RuntimeDescription) []framework.Check {
	var checks []framework.Check
	for _, d := range runtime.Directories {
		if d.Secret == nil && d.Listener == nil {
			continue
		}
		check := framework.Check{Subject: "platform.directory[" + d.Name + "]", State: framework.Consistent,
			Reason: "platform source and consumer mounts preserved"}
		before := findPodVolume(expected.StatefulSet.Spec.Template.Spec, d.Name)
		after := findPodVolume(actual.StatefulSet.Spec.Template.Spec, d.Name)
		if !reflect.DeepEqual(before, after) {
			check.State = framework.Conflict
			check.Reason = "podOverrides changed a declared platform volume"
		}
		for _, p := range append([]Process{runtime.Main}, runtime.Initializers...) {
			container := findContainer(actual.StatefulSet.Spec.Template, p.Name)
			if container == nil {
				for i := range actual.StatefulSet.Spec.Template.Spec.InitContainers {
					c := &actual.StatefulSet.Spec.Template.Spec.InitContainers[i]
					if c.Name == p.Name {
						container = c
					}
				}
			}
			for _, a := range p.Access {
				if a.Directory != d.Name {
					continue
				}
				m := findMount(container, a.MountPath)
				if m == nil || m.Name != d.Name || !m.ReadOnly || m.SubPath != "" || m.SubPathExpr != "" ||
					platformMountMasked(container, a.MountPath) {
					check.State = framework.Conflict
					check.Reason = "podOverrides displaced a platform consumer mount"
				}
			}
		}
		checks = append(checks, check)
	}
	return checks
}

func validatePlatformSource(d Directory) error {
	if d.Secret != nil {
		return validateSecretSource(d.Name, d.Secret)
	}
	if l := d.Listener; l != nil {
		if (l.Class == "") == (l.Name == "") {
			return fmt.Errorf("listener directory requires exactly one class or name")
		}
		for _, name := range []string{l.Class, l.Name} {
			if name != "" && len(validation.IsDNS1123Subdomain(name)) != 0 {
				return fmt.Errorf("invalid listener reference")
			}
		}
	}
	return nil
}

func validateSecretSource(directory string, s *framework.SecretVolume) error {

	if (s.SecretName == "") == (s.SecretClass == "") {
		return fmt.Errorf("secret directory %s requires exactly one Secret or SecretClass", directory)
	}
	for _, name := range []string{s.SecretName, s.SecretClass} {
		if name != "" && len(validation.IsDNS1123Subdomain(name)) != 0 {
			return fmt.Errorf("invalid secret reference")
		}
	}
	if s.SecretName != "" && (s.Format != "" || len(s.Scope) > 0 || len(s.KerberosServiceNames) > 0) {
		return fmt.Errorf("native Secret cannot specify CSI options")
	}
	if s.Format != "" && s.Format != "tls-pem" && s.Format != "tls-p12" && s.Format != "kerberos" {
		return fmt.Errorf("unsupported SecretClass format")
	}
	for _, scope := range s.Scope {
		if scope == framework.SecretScopePod || scope == framework.SecretScopeNode {
			continue
		}
		key, value, ok := strings.Cut(scope, "=")
		if !ok || (key != "service" && key != "listener-volume") || len(validation.IsDNS1123Subdomain(value)) != 0 {
			return fmt.Errorf("invalid SecretClass scope")
		}
	}
	for _, name := range s.KerberosServiceNames {
		if len(validation.IsDNS1123Label(name)) != 0 {
			return fmt.Errorf("invalid Kerberos service name")
		}
	}

	return nil
}

func platformMountMasked(container *corev1.Container, root string) bool {
	if container == nil {
		return true
	}
	for _, mount := range container.VolumeMounts {
		if strings.HasPrefix(mount.MountPath, strings.TrimSuffix(root, "/")+"/") {
			return true
		}
	}
	return false
}

// DeclaredPlatformClaims reconstructs only typed platform ephemeral sources.
// Controllers use these declarations to distinguish platform-owned temporary
// claims from arbitrary Pod override storage during later retirement.
func DeclaredPlatformClaims(runtime *framework.RuntimeDescription) []corev1.Volume {
	var out []corev1.Volume
	if runtime == nil {
		return out
	}
	for _, d := range runtime.Directories {
		if volume, ok := platformVolume(d); ok && volume.Ephemeral != nil {
			out = append(out, volume)
		}
	}
	return out
}
