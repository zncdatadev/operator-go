package product

import (
	"reflect"
	"slices"

	"github.com/zncdatadev/operator-go/pkg/framework"

	corev1 "k8s.io/api/core/v1"
)

func findContainer(pod corev1.PodTemplateSpec, name string) *corev1.Container {
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == name {
			return &pod.Spec.Containers[i]
		}
	}
	return nil
}

func findFile(files []framework.File, directory, name string) *framework.File {
	for i := range files {
		if files[i].Directory == directory && files[i].Path == name {
			return &files[i]
		}
	}
	return nil
}

func processPremise(process framework.Process, container *corev1.Container) bool {
	return container != nil && len(container.EnvFrom) == 0 && container.WorkingDir == "" &&
		process.Image == container.Image && slices.Equal(process.Command, container.Command) &&
		slices.Equal(process.Args, container.Args) && slices.EqualFunc(process.Env, container.Env,
		func(a, b corev1.EnvVar) bool { return reflect.DeepEqual(a, b) })
}
