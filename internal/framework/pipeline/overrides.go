package pipeline

import (
	"fmt"
	"maps"
	"slices"

	corev1 "k8s.io/api/core/v1"
)

// CloneRuntime copies mutable declaration data while retaining stateless codec
// identities. Product callbacks transfer ownership of their returned values.
func CloneRuntime(in RuntimeDescription) RuntimeDescription {
	out := in
	out.Main = cloneProcess(in.Main)
	out.Initializers = slices.Clone(in.Initializers)
	for i := range in.Initializers {
		out.Initializers[i] = cloneProcess(in.Initializers[i])
	}
	out.Coordination = CloneInput(in.Coordination)
	out.Directories = CloneInput(in.Directories)
	if in.SharedGroup != nil {
		value := *in.SharedGroup
		out.SharedGroup = &value
	}
	out.Endpoints = slices.Clone(in.Endpoints)
	out.LogOutputs = slices.Clone(in.LogOutputs)
	out.Files = cloneDeclaredFiles(in.Files)
	return out
}

func cloneDeclaredFiles(files []File) []File {
	out := slices.Clone(files)
	for i, file := range out {
		switch content := file.Content.(type) {
		case KeyValues:
			content.Values = maps.Clone(content.Values)
			out[i].Content = content
		case Lines:
			out[i].Content = slices.Clone(content)
		}
	}
	return out
}

func applyProcessOverrides(process *Process, layers ...*Overrides) error {
	for _, layer := range layers {
		if layer == nil {
			continue
		}
		for _, name := range sortedKeys(layer.EnvOverrides) {
			// Replacing the entire entry removes any prior ValueFrom source.
			process.Env = slices.DeleteFunc(process.Env, func(env corev1.EnvVar) bool { return env.Name == name })
			process.Env = append(process.Env, corev1.EnvVar{Name: name, Value: layer.EnvOverrides[name]})
		}
		if layer.CLIOverrides != nil {
			if *layer.CLIOverrides == nil {
				return fmt.Errorf("cliOverrides must not be null")
			}
			process.Args = slices.Clone(*layer.CLIOverrides)
		}
	}
	return nil
}

func cloneProcess(in Process) Process {
	out := in
	out.Command = slices.Clone(in.Command)
	out.Args = slices.Clone(in.Args)
	out.Env = make([]corev1.EnvVar, len(in.Env))
	for i := range in.Env {
		in.Env[i].DeepCopyInto(&out.Env[i])
	}
	out.Identity = in.Identity.DeepCopy()
	out.Access = slices.Clone(in.Access)
	out.Lifecycle = in.Lifecycle.DeepCopy()
	out.StartupProbe = in.StartupProbe.DeepCopy()
	out.ReadinessProbe = in.ReadinessProbe.DeepCopy()
	out.LivenessProbe = in.LivenessProbe.DeepCopy()
	return out
}
