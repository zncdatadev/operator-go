package pipeline

import (
	"fmt"
	"path"
	"reflect"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

const vectorTypeKey = "type"

// composeVector consumes the framework logging gate. Products declare actual
// files; every declared output is collected when enabled. Platform files join
// declarations before file overrides.
func composeVector(in RuntimeDescription, enabled bool, options AssemblyOptions) (
	RuntimeDescription, *corev1.Container, error,
) {
	r := CloneRuntime(in)
	if !enabled {
		return r, nil, nil
	}
	sources := map[string]any{}
	inputs := []string{}
	paths := map[string]string{}
	for index, output := range r.LogOutputs {
		mount := path.Join("/logs", output.Directory)
		paths[output.Directory] = mount
		name := fmt.Sprintf("source_%d", index)
		inputs = append(inputs, name)
		sources[name] = map[string]any{vectorTypeKey: "file", "include": []string{path.Join(mount, output.RelativePath)},
			"read_from": "beginning"}
	}
	if len(inputs) == 0 {
		return r, nil, nil
	}
	if r.ConfigDirectory == "" {
		return r, nil, fmt.Errorf("log collection requires an explicit config directory")
	}
	if strings.TrimSpace(options.VectorImage) == "" {
		return r, nil, fmt.Errorf("vector image is required when collection is enabled")
	}
	if r.Main.Name == vectorContainerName {
		return r, nil, fmt.Errorf("main container conflicts with Vector name")
	}
	const dataDirectory = "vector-data"
	const dataPath = "/var/lib/vector"
	sink := map[string]any{vectorTypeKey: "console", "inputs": inputs, "target": "stdout",
		"encoding": map[string]any{"codec": "json"}}
	if options.VectorDestination != nil {
		if err := options.VectorDestination.Validate(); err != nil {
			return r, nil, fmt.Errorf("vector destination: %w", err)
		}
		sink = map[string]any{vectorTypeKey: "vector", "inputs": inputs, "address": options.VectorDestination.Address}
	}
	config, err := yaml.Marshal(map[string]any{"data_dir": dataPath, "sources": sources,
		"sinks": map[string]any{"collected": sink}})
	if err != nil {
		return r, nil, err
	}
	r.Directories = append(r.Directories, Directory{Name: dataDirectory})
	r.Files = append(r.Files, File{Directory: r.ConfigDirectory, Path: vectorConfigFile, Content: Text(config)})
	vector := &corev1.Container{Name: vectorContainerName, Image: options.VectorImage,
		Command:         []string{vectorContainerName},
		Args:            []string{"--config", path.Join(vectorConfigPath, vectorConfigFile)},
		SecurityContext: options.HelperIdentity.DeepCopy(),
		VolumeMounts: []corev1.VolumeMount{{Name: r.ConfigDirectory, MountPath: vectorConfigPath, ReadOnly: true},
			{Name: dataDirectory, MountPath: dataPath}}}
	if options.VectorDestination != nil {
		// A changed discovery address must reach the running collector even
		// without a separate platform restarter. The literal env value is a
		// template trigger; generated YAML remains the only configuration source.
		vector.Env = []corev1.EnvVar{{Name: "FRAMEWORK_VECTOR_DESTINATION", Value: options.VectorDestination.Address}}
	}
	for _, directory := range sortedKeys(paths) {
		vector.VolumeMounts = append(vector.VolumeMounts, corev1.VolumeMount{
			Name: directory, MountPath: paths[directory], ReadOnly: true})
	}
	return r, vector, nil
}

// collectorKnown is a platform premise exposed to product final validation.
// A product needs the premise, not the helper container names or volume layout.
func collectorKnown(expected, actual GroupResources, generated RuntimeDescription, files []File) bool {
	if !filePreparationKnown(expected, actual, files) {
		return false
	}
	beforePod, afterPod := expected.StatefulSet.Spec.Template, actual.StatefulSet.Spec.Template
	before, after := findContainer(beforePod, vectorContainerName), findContainer(afterPod, vectorContainerName)
	// A user-added container is not evidence of a framework-selected collector.
	if before == nil || generated.Main.Name == vectorContainerName {
		return false
	}
	if !sameContainerExecution(before, after) || !reflect.DeepEqual(before.VolumeMounts, after.VolumeMounts) ||
		shadowsFile(after, vectorConfigPath, vectorConfigFile) {
		return false
	}
	for index := range before.VolumeMounts {
		mount := &before.VolumeMounts[index]
		if !sameMount(beforePod.Spec, afterPod.Spec, mount, findMount(after, mount.MountPath)) {
			return false
		}
	}
	original := findFile(generated.Files, generated.ConfigDirectory, vectorConfigFile)
	file := findFile(files, generated.ConfigDirectory, vectorConfigFile)
	return original != nil && file != nil && reflect.DeepEqual(original.Content, file.Content)
}
