/*
Copyright 2024 ZNCDataDev.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package builder

// cloneSlice returns an independent deep copy of src, using the generated DeepCopyInto of the
// element type. Builders use it in Build() so the returned Kubernetes object shares no backing
// array with the builder: callers routinely mutate a built object (the reconciler appends sidecar
// containers and volumes to the pod template), and a shared slice would let that mutation reach
// back into the builder — and therefore into every other object it builds.
//
// A nil source stays nil so the built object keeps the same "unset vs empty" distinction the
// builder holds.
func cloneSlice[T any, PT interface {
	*T
	DeepCopyInto(*T)
}](src []T) []T {
	if src == nil {
		return nil
	}
	out := make([]T, len(src))
	for i := range src {
		PT(&src[i]).DeepCopyInto(&out[i])
	}
	return out
}

// clonePtr returns an independent copy of a scalar pointer field, for the same reason cloneSlice
// copies slices: an optional Kubernetes field is a pointer, and handing the builder's own pointer
// to the built object lets a caller writing through it change the builder and every other object
// the builder has produced. A nil source stays nil so "unset" survives the copy.
func clonePtr[T any](src *T) *T {
	if src == nil {
		return nil
	}
	value := *src
	return &value
}
