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

package v1alpha1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zncdatadev/operator-go/pkg/testutil"
)

// This is the whole product-side cost of the guard, and it is what every operator built on this
// SDK should copy. No envtest, no CR fixture, no cluster — the defect is visible in the generated
// schema, so the check reads the output of `make manifests`.
//
// What it prevents: a `+kubebuilder:default` on a field inside a role or role group `config` block.
// That block is folded Role -> RoleGroup, and structural defaulting fills a leaf as soon as its
// enclosing object exists, so the default lands in every role group that declared the enclosing
// object for any reason — after which "the group did not set this" and "the group asked for the
// default" are the same bytes, and the role's value can never win. Defaults for these fields go at
// consumption time instead.
var _ = Describe("Generated CRDs", func() {
	It("declare no default inside a role or role group config block", func() {
		Expect("../../config/crd/bases/*.yaml").To(testutil.HaveNoInheritedConfigDefaults())
	})
})
