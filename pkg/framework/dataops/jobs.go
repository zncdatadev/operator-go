package dataops

import (
	_ "embed"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//go:embed worker.py
var workerScript string

func jobFor(op *DataOperation, image string) *batchv1.Job {
	zero, deadline := int32(0), int64(1800)
	yes, no := true, false
	uid, gid := op.Spec.WorkerIdentity.UID, op.Spec.WorkerIdentity.GID
	volumes := []corev1.Volume{{Name: "source", VolumeSource: corev1.VolumeSource{
		PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
			ClaimName: op.Spec.Source.ClaimName, ReadOnly: op.Spec.Action == ActionMigrate,
		},
	}}}
	mounts := []corev1.VolumeMount{{Name: "source", MountPath: "/source", ReadOnly: op.Spec.Action == ActionMigrate}}
	if op.Spec.Action == ActionMigrate {
		volumes = append(volumes, corev1.Volume{Name: "target", VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: op.Spec.Target.ClaimName},
		}})
		mounts = append(mounts, corev1.VolumeMount{Name: "target", MountPath: "/target"})
	}
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: jobName(op), Namespace: op.Namespace, Labels: map[string]string{LockAnnotation: string(op.UID)},
			OwnerReferences: []metav1.OwnerReference{{APIVersion: GroupVersion.String(), Kind: "DataOperation",
				Name: op.Name, UID: op.UID, Controller: &yes}},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &zero, ActiveDeadlineSeconds: &deadline,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{LockAnnotation: string(op.UID)}},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever, AutomountServiceAccountToken: &no,
					SecurityContext: &corev1.PodSecurityContext{RunAsNonRoot: &yes, RunAsUser: &uid, RunAsGroup: &gid, FSGroup: &gid},
					Volumes:         volumes,
					Containers: []corev1.Container{{Name: workerContainer, Image: image,
						Command: []string{"python3", "-c", workerScript, op.Spec.Action, string(op.UID)}, VolumeMounts: mounts,
						SecurityContext: &corev1.SecurityContext{AllowPrivilegeEscalation: &no, ReadOnlyRootFilesystem: &yes,
							Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}},
					}},
				},
			},
		},
	}
}
func jobName(op *DataOperation) string { return fmt.Sprintf("data-%s-%d", op.UID, op.Status.Attempt) }
