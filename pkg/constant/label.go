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

package constant

// Kubernetes recommended labels for applications.
// https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/
// https://kubernetes.io/docs/reference/labels-annotations-taints/
const (
	LabelKubernetesComponent = "app.kubernetes.io/component"
	LabelKubernetesInstance  = "app.kubernetes.io/instance"
	LabelKubernetesName      = "app.kubernetes.io/name"
	LabelKubernetesManagedBy = "app.kubernetes.io/managed-by"
	LabelKubernetesRoleGroup = "app.kubernetes.io/role-group"
	LabelKubernetesVersion   = "app.kubernetes.io/version"
)

// MatchingLabelsNames returns the list of label keys used for label selector matching.
func MatchingLabelsNames() []string {
	return []string{
		LabelKubernetesName,
		LabelKubernetesInstance,
		LabelKubernetesRoleGroup,
		LabelKubernetesComponent,
		LabelKubernetesManagedBy,
	}
}

// Enrichment labels for the enrichment controller.
// When a pod has the label `enrichment.kubedoop.dev/enable=true`,
// the enrichment controller will set the node address to the pod annotation when the pod is created.
const (
	LabelEnrichmentEnable      = "enrichment." + KubedoopDomain + "/enable"
	LabelEnrichmentEnableValue = "true"
	LabelEnrichmentNodeAddress = "enrichment." + KubedoopDomain + "/node-address"
)
