package pipeline

import (
	"fmt"
	"maps"
	"path"
	"reflect"
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

func relativeFile(p string) bool {
	return p != "" && p != "." && p != ".." && !path.IsAbs(p) &&
		path.Clean(p) == p && !strings.HasPrefix(p, "../") && !strings.ContainsRune(p, '\x00')
}

func nilCodec(codec PropertyCodec) bool {
	if codec == nil {
		return true
	}
	v := reflect.ValueOf(codec)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

func validateContent(content FileContent) error {
	switch c := content.(type) {
	case KeyValues:
		if nilCodec(c.Codec) {
			return fmt.Errorf("key values require a codec")
		}
		for _, key := range slices.Sorted(maps.Keys(c.Values)) {
			switch c.Values[key].(type) {
			case Literal, PodNameBinding:
			default:
				return fmt.Errorf("key %q has an unsupported or nil value", key)
			}
		}
	case Lines:
		for _, line := range c {
			if strings.ContainsAny(line, "\r\n") {
				return fmt.Errorf("lines cannot contain CR or LF")
			}
		}
	case Text:
	default:
		return fmt.Errorf("unsupported or nil file content; use a content value")
	}
	return nil
}

func validateProcess(p Process, sharedGroup *int64) error {
	if len(validation.IsDNS1123Label(p.Name)) != 0 || strings.TrimSpace(p.Image) == "" {
		return fmt.Errorf("main process requires a valid container name and image")
	}
	if len(p.Command) > 0 && p.Command[0] == "" {
		return fmt.Errorf("main process command cannot start with an empty executable")
	}
	if sharedGroup != nil && *sharedGroup < 0 {
		return fmt.Errorf("shared group cannot be negative")
	}
	if id := p.Identity; id != nil {
		if (id.RunAsUser != nil && *id.RunAsUser < 0) || (id.RunAsGroup != nil && *id.RunAsGroup < 0) {
			return fmt.Errorf("process UID and GID cannot be negative")
		}
		if id.RunAsNonRoot != nil && *id.RunAsNonRoot && id.RunAsUser != nil && *id.RunAsUser == 0 {
			return fmt.Errorf("process declares UID 0 with runAsNonRoot")
		}
	}
	return nil
}

func validateEndpoints(endpoints []Endpoint) error {
	ports := map[string]bool{}
	for _, endpoint := range endpoints {
		if len(validation.IsValidPortName(endpoint.Name)) != 0 || ports[endpoint.Name] ||
			endpoint.Port < 1 || endpoint.Port > 65535 {
			return fmt.Errorf("invalid or duplicate endpoint %q", endpoint.Name)
		}
		ports[endpoint.Name] = true
	}
	return nil
}

// ValidateRuntime checks the declaration itself, before user overrides. Success
// proves neither actual filesystem permissions nor product startup or health.
func ValidateRuntime(r RuntimeDescription) error {
	if err := validateRuntimeDomains(r); err != nil {
		return err
	}
	dirs, mounts, writable := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, dir := range r.Directories {
		if len(validation.IsDNS1123Label(dir.Name)) != 0 || dirs[dir.Name] {
			return fmt.Errorf("invalid or duplicate directory %q", dir.Name)
		}
		dirs[dir.Name] = true
	}
	if r.ConfigDirectory != "" && !dirs[r.ConfigDirectory] {
		return fmt.Errorf("config override directory %q is not declared", r.ConfigDirectory)
	}
	for _, access := range r.Main.Access {
		p := access.MountPath
		if !dirs[access.Directory] || !path.IsAbs(p) || path.Clean(p) != p || strings.ContainsRune(p, '\x00') {
			return fmt.Errorf("invalid mount %q or unknown directory %q", p, access.Directory)
		}
		if mounts[p] {
			return fmt.Errorf("duplicate mount path %q", p)
		}
		mounts[p] = true
		writable[access.Directory] = writable[access.Directory] || !access.ReadOnly
	}
	files := make([]string, 0, len(r.Files))
	for _, file := range r.Files {
		if !dirs[file.Directory] || !relativeFile(file.Path) {
			return fmt.Errorf("invalid file path %q or unknown directory %q", file.Path, file.Directory)
		}
		name := file.Directory + "/" + file.Path
		for _, other := range files {
			if name == other || strings.HasPrefix(name, other+"/") || strings.HasPrefix(other, name+"/") {
				return fmt.Errorf("file paths %q and %q collide", name, other)
			}
		}
		if err := validateContent(file.Content); err != nil {
			return fmt.Errorf("file %q: %w", name, err)
		}
		files = append(files, name)
	}
	if err := validateEndpoints(r.Endpoints); err != nil {
		return err
	}
	logs := map[string]bool{}
	for _, log := range r.LogOutputs {
		name := log.Directory + "/" + log.RelativePath
		if log.Container != r.Main.Name || !dirs[log.Directory] || !relativeFile(log.RelativePath) ||
			!writable[log.Directory] || logs[name] {
			return fmt.Errorf("invalid log output %q: producer, writable directory, path or uniqueness", name)
		}
		logs[name] = true
	}
	return nil
}

func validateRuntimeDomains(r RuntimeDescription) error {
	if err := validateProcess(r.Main, r.SharedGroup); err != nil {
		return err
	}
	if err := validateLifecycle(r); err != nil {
		return err
	}
	if err := validatePlatformVolumes(r); err != nil {
		return err
	}
	if err := validateRetainedData(r); err != nil {
		return err
	}
	return nil
}
