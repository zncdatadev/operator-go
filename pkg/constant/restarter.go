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

// Restarter policy covers workload restarts and pod expiration.
//
// Workload restarter:
//
//	If a workload has the label `restarter.kubedoop.dev/enable=true`,
//	and a ConfigMap or Secret mounted as a volume in the pod is updated,
//	the restarter will update the annotations in the workload pod template.
//	The workload controller will then update all pods of the workload.
//
// Pod expiration:
//
//	When a workload mounts a secret-class managed by secret-operator, some secrets
//	are created and mounted for the pod by secret-operator, for example, Kerberos
//	and TLS secrets.
//	TLS and Kerberos secrets have expiration times. When the secrets are created,
//	secret-operator sets the expiration time in the pod annotation.
//	The restarter checks the expiration time in the pod annotation, and if it has
//	expired, the restarter restarts the pod.
const (
	LabelRestarterEnable      = "restarter." + KubedoopDomain + "/enable"
	LabelRestarterEnableValue = "true"

	AnnotationSecretRestarterPrefix    = "secret.restarter." + KubedoopDomain + "/"
	AnnotationConfigMapRestarterPrefix = "configmap.restarter." + KubedoopDomain + "/"

	LabelRestarterExpiresAtPrefix = "restarter." + KubedoopDomain + "/expires-at."
)
