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

// WorkloadKind names the Kubernetes workload a role's role groups are built as. It is a per-ROLE
// choice, not per role group: every group of a role runs the same process, and the workload kind
// is a property of that process (does it hold per-pod state and identity?), not of a group's
// sizing. It is also reconcile-invariant — a product decides it when the operator is written, the
// way it decides ports and container names — which is why it lives on the handler
// (BaseRoleGroupHandler.RoleWorkloadKinds) rather than on the per-reconcile build context
// (see docs/architecture.md §4.1.4 for that split).
type WorkloadKind string

const (
	// WorkloadKindStatefulSet is the default: the role group is built as a StatefulSet with a
	// headless Service for per-pod DNS identity. Every role behaves this way unless the handler
	// says otherwise, so existing operators render byte-identical output.
	WorkloadKindStatefulSet WorkloadKind = "StatefulSet"

	// WorkloadKindDeployment builds the role group as a Deployment: interchangeable pods, no
	// headless Service (there is no stable pod identity for it to name), no data PVC
	// (StorageMountPath is ignored), and the standard RollingUpdate rollout. For the roles this
	// fits — a stateless UI, an API server in front of a stateful backend — a StatefulSet's
	// one-pod-at-a-time identity machinery is pure cost.
	WorkloadKindDeployment WorkloadKind = "Deployment"
)

// WorkloadKindProvider is implemented by handlers that choose a workload kind per role
// (BaseRoleGroupHandler and everything embedding it, via the promoted method). The framework
// type-asserts its handler against this interface wherever the kind matters outside the build —
// today the health manager, which must read the right object to judge availability. A handler
// that does not implement it is treated as all-StatefulSet, which is what every handler was
// before this interface existed.
type WorkloadKindProvider interface {
	// WorkloadKindFor returns the workload kind for a role. Implementations return
	// WorkloadKindStatefulSet for roles they have no explicit answer for.
	WorkloadKindFor(roleName string) WorkloadKind
}
