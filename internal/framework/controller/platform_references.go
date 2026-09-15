package controller

import (
	"github.com/zncdatadev/operator-go/pkg/framework"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type platformInputReference struct {
	object   framework.FactResource
	key      client.ObjectKey
	slot     string
	optional bool
}

// Environment dependencies come from the final Pod: a removed or replaced
// default env reference must not continue withholding its former consumer.
func platformReferences(namespace string, runtime *framework.RuntimeDescription, pod *corev1.PodSpec,
) []platformInputReference {
	var out []platformInputReference
	if runtime != nil {
		for _, d := range runtime.Directories {
			if !platformDirectory(d) {
				continue
			}
			object, key := platformReference(namespace, d)
			out = append(out, platformInputReference{object: object, key: key, slot: d.Name})
		}
	}
	if pod == nil {
		return out
	}
	add := func(slot, name string, optional *bool) {
		out = append(out, platformInputReference{object: &corev1.Secret{},
			key:  client.ObjectKey{Namespace: namespace, Name: name},
			slot: slot, optional: optional != nil && *optional})
	}
	for _, container := range append(append([]corev1.Container{}, pod.InitContainers...), pod.Containers...) {
		for _, env := range container.Env {
			if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
				ref := env.ValueFrom.SecretKeyRef
				add(container.Name+"/env/"+env.Name, ref.Name, ref.Optional)
			}
		}
		for _, env := range container.EnvFrom {
			if env.SecretRef != nil {
				add(container.Name+"/envFrom/"+env.Prefix, env.SecretRef.Name, env.SecretRef.Optional)
			}
		}
	}
	return out
}
