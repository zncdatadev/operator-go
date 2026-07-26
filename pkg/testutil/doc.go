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

// Package testutil provides testing utilities for operator-go tests.
// It includes envtest environment management, mock implementations,
// Gomega matchers, and test data builders.
//
// The mock cluster resources in this package carry kubebuilder markers so that `make manifests`
// generates REAL CRD schemas for them into config/crd/bases. That matters more than it looks:
// envtest installs those CRDs, so a schema-free CRD would mean the API server performs no
// defaulting, no validation and no pruning for any test in the repository — the entire suite
// would exercise merge and defaulting logic in a world that does not exist in production.
//
// The package carries no +kubebuilder:object:generate=true marker on purpose: that would opt
// EVERY struct here into deep-copy generation, including the mock handlers and extensions whose
// fields are funcs, which controller-gen cannot copy. Deep copies are generated for the types
// marked +kubebuilder:object:root=true and whatever they reference, which is exactly the set that
// needs them.
//
// +groupName=test.zncdata.dev
// +versionName=v1alpha1
package testutil
