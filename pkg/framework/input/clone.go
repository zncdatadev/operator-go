package input

import (
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// Clone copies generated presence data, preserving nil versus empty maps/slices
// and native Quantity/Duration/Affinity values. It is not a general clone for
// clients, closures, codecs or arbitrary Go state. Generated root methods copy
// Kubernetes metadata with its own DeepCopyInto implementation.
func Clone[T any](value T) T {
	in := reflect.ValueOf(value)
	if !in.IsValid() {
		return value
	}
	return cloneValue(in).Interface().(T)
}

func cloneValue(value reflect.Value) reflect.Value {
	switch value.Type() {
	case affinityType:
		affinity := value.Interface().(corev1.Affinity)
		return reflect.ValueOf(*affinity.DeepCopy())
	case durationType:
		return value
	case quantityType:
		quantity := value.Interface().(resource.Quantity).DeepCopy()
		return reflect.ValueOf(quantity)
	}
	switch value.Kind() {
	case reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		out := reflect.New(value.Type().Elem())
		out.Elem().Set(cloneValue(value.Elem()))
		return out
	case reflect.Struct:
		out := reflect.New(value.Type()).Elem()
		for index := 0; index < value.NumField(); index++ {
			out.Field(index).Set(cloneValue(value.Field(index)))
		}
		return out
	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		out := reflect.MakeMapWithSize(value.Type(), value.Len())
		for iterator := value.MapRange(); iterator.Next(); {
			out.SetMapIndex(iterator.Key(), cloneValue(iterator.Value()))
		}
		return out
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		out := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for index := 0; index < value.Len(); index++ {
			out.Index(index).Set(cloneValue(value.Index(index)))
		}
		return out
	default:
		return value
	}
}
