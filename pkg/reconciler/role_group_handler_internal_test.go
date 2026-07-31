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

package reconciler

import "testing"

// TestAffinityFieldsCoverCorev1Affinity guards mergeAffinity against corev1.Affinity growing a
// member. affinityFields drives the per-member merge, so a member missing from it would silently
// stop being inheritable from the role level — the exact class of bug this merge exists to fix,
// reintroduced by a dependency bump rather than by an edit here.
//
// It lives in an internal test file because the check reads an unexported symbol; every other spec
// in this package is external (package reconciler_test) and deliberately stays that way.
func TestAffinityFieldsCoverCorev1Affinity(t *testing.T) {
	if err := affinityFieldsAreComplete(); err != nil {
		t.Fatal(err)
	}
}
