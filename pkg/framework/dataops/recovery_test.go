package dataops

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestDestroyMissingVolumeRequiresPersistedReclamation(t *testing.T) {
	for _, phase := range []string{phaseDeleteVolume, phaseReclaimVolume} {
		t.Run(phase, func(t *testing.T) {
			r, op, _ := testController(t, ActionDestroy)
			pv := &corev1.PersistentVolume{}
			if err := r.Client.Get(t.Context(), client.ObjectKey{Name: op.Spec.Source.Binding.VolumeName}, pv); err != nil {
				t.Fatal(err)
			}
			if err := r.Client.Delete(t.Context(), pv); err != nil {
				t.Fatal(err)
			}
			op.Status.Phase = phase
			err := r.destroy(t.Context(), op)
			if phase == phaseReclaimVolume {
				if err != nil || op.Status.Phase != phaseRecord {
					t.Fatalf("persisted reclamation should finish: %s, %v", op.Status.Phase, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "completion is unknown") || op.Status.Phase != phase {
				t.Fatalf("lost Retain PV invented completion: %s, %v", op.Status.Phase, err)
			}
		})
	}
}

func TestWorkerRejectsContainerIdentityAndSecurityOverrides(t *testing.T) {
	r, op, _ := testController(t, ActionDestroy)
	mutations := map[string]func(*corev1.SecurityContext){
		"root identity":        func(s *corev1.SecurityContext) { value := int64(0); s.RunAsUser = &value },
		"root group":           func(s *corev1.SecurityContext) { value := int64(0); s.RunAsGroup = &value },
		"privilege escalation": func(s *corev1.SecurityContext) { value := true; s.AllowPrivilegeEscalation = &value },
		"writable root":        func(s *corev1.SecurityContext) { value := false; s.ReadOnlyRootFilesystem = &value },
		"capabilities":         func(s *corev1.SecurityContext) { s.Capabilities.Add = []corev1.Capability{"SYS_ADMIN"} },
	}
	if err := checkWorkerSpec(op, jobFor(op, r.WorkerImage), r.WorkerImage); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			job := jobFor(op, r.WorkerImage)
			mutate(job.Spec.Template.Spec.Containers[0].SecurityContext)
			if err := checkWorkerSpec(op, job, r.WorkerImage); err == nil {
				t.Fatal("worker accepted container override of approved identity or security restrictions")
			}
		})
	}
}
