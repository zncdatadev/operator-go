package pipeline

import (
	"fmt"
	"path"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
)

const lifecycleSubject = "assembly.lifecycle"

func validateLifecycle(r RuntimeDescription) error {
	if c := r.Coordination; c != nil {
		if c.ProgressDeadline.Duration < time.Second || c.ProgressDeadline.Duration > time.Hour {
			return fmt.Errorf("coordination progressDeadline must be between 1s and 1h")
		}
	}
	if len(r.Initializers) > 0 && r.Coordination == nil {
		return fmt.Errorf("initializers require bounded workload coordination")
	}
	if err := validateProbes(r.Main); err != nil {
		return err
	}
	names := map[string]bool{r.Main.Name: true, materializerContainerName: true, vectorContainerName: true}
	dirs := map[string]bool{}
	for _, dir := range r.Directories {
		dirs[dir.Name] = true
	}
	for _, init := range r.Initializers {
		if err := validateProcess(init, r.SharedGroup); err != nil {
			return fmt.Errorf("initializer %q: %w", init.Name, err)
		}
		if names[init.Name] {
			return fmt.Errorf("initializer %q has a reserved or duplicate name", init.Name)
		}
		names[init.Name] = true
		if init.Lifecycle != nil || init.StartupProbe != nil || init.ReadinessProbe != nil || init.LivenessProbe != nil {
			return fmt.Errorf("initializer %q cannot declare lifecycle or probes", init.Name)
		}
		paths := map[string]bool{}
		for _, access := range init.Access {
			if !dirs[access.Directory] || !path.IsAbs(access.MountPath) || path.Clean(access.MountPath) != access.MountPath ||
				strings.ContainsRune(access.MountPath, '\x00') || paths[access.MountPath] {
				return fmt.Errorf("initializer %q has invalid directory access", init.Name)
			}
			paths[access.MountPath] = true
		}
	}
	return nil
}

func checkLifecycle(expected, actual GroupResources, runtime RuntimeDescription) []Check {
	if len(runtime.Initializers) == 0 && runtime.Main.Lifecycle == nil && runtime.Main.StartupProbe == nil &&
		runtime.Main.ReadinessProbe == nil && runtime.Main.LivenessProbe == nil && runtime.Coordination == nil {
		return nil
	}
	before, after := expected.StatefulSet.Spec.Template.Spec, actual.StatefulSet.Spec.Template.Spec
	main := findContainer(actual.StatefulSet.Spec.Template, runtime.Main.Name)
	baseline := findContainer(expected.StatefulSet.Spec.Template, runtime.Main.Name)
	check := Check{Subject: lifecycleSubject, State: Consistent,
		Reason: "declared initialization, probes and lifecycle are retained; execution success is separately observed"}
	if main == nil || !apiequality.Semantic.DeepEqual(main.Lifecycle, baseline.Lifecycle) ||
		!apiequality.Semantic.DeepEqual(main.StartupProbe, baseline.StartupProbe) ||
		!apiequality.Semantic.DeepEqual(main.ReadinessProbe, baseline.ReadinessProbe) ||
		!apiequality.Semantic.DeepEqual(main.LivenessProbe, baseline.LivenessProbe) ||
		!apiequality.Semantic.DeepEqual(before.TerminationGracePeriodSeconds, after.TerminationGracePeriodSeconds) {
		check.State, check.Reason = Unknown, "podOverrides changed the declared lifecycle, probes or termination budget"
	}
	// Ordinary overrides still win. Changed initializer execution invalidates the
	// product premise instead of being silently described as initialized.
	for _, init := range runtime.Initializers {
		var a, b *corev1.Container
		ai, bi := -1, -1
		for i := range before.InitContainers {
			if before.InitContainers[i].Name == init.Name {
				b = &before.InitContainers[i]
				bi = i
			}
		}
		for i := range after.InitContainers {
			if after.InitContainers[i].Name == init.Name {
				a = &after.InitContainers[i]
				ai = i
			}
		}
		if !apiequality.Semantic.DeepEqual(a, b) || ai != bi {
			check.State, check.Reason = Unknown, "podOverrides changed declared initializer execution or order"
		}
	}
	return []Check{check}
}

func validateProbes(main Process) error {
	for _, probe := range []*corev1.Probe{main.StartupProbe, main.ReadinessProbe, main.LivenessProbe} {
		if probe == nil {
			continue
		}
		n := 0
		if probe.Exec != nil {
			n++
			if len(probe.Exec.Command) == 0 {
				return fmt.Errorf("probe exec requires a command")
			}
		}
		if probe.HTTPGet != nil {
			n++
		}
		if probe.TCPSocket != nil {
			n++
		}
		if probe.GRPC != nil {
			n++
		}
		if n != 1 || probe.InitialDelaySeconds < 0 || probe.TimeoutSeconds < 0 ||
			probe.PeriodSeconds < 0 || probe.SuccessThreshold < 0 || probe.FailureThreshold < 0 {
			return fmt.Errorf("probe requires exactly one action and nonnegative thresholds")
		}
	}
	return nil
}
