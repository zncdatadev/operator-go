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

// Restarter policy has workload restart and pod expiration.
//
// Workload restarter:
//
//	If a workload has the label `restarter.kubedoop.dev/enable=true`,
//	 and a configmap or secret is updated when mounted as a volume in the pod,
//	 the restarter will update the annotations in the workload podTemplate.
//	 The workload controller will update all the pods of the workload.
//
// Pod expiration:
//
//	When workload mount with secret-class of secret-operator, some secrets will be
//	 created and mount for the pod by the secret-operator. Eg: kerberos, tls, etc.
//	 Tls and kerberos secrets have expiration time, when the secrets is created,
//	 secret-operator will set the expiration time in the pod annotation.
//	 The restarter will check the expiration time in the pod annotation, if the expiration time is expired,
//	 the restarter will restart the pod.
const (
	LabelRestarterEnable      = "restarter." + KubedoopDomain + "/enable"
	LabelRestarterEnableValue = "true"

	AnnotationSecretRestarterPrefix    = "secret.restarter." + KubedoopDomain + "/"
	AnnotationConfigmapRestarterPrefix = "configmap.restarter." + KubedoopDomain + "/"

	PrefixLabelRestarterExpiresAt = "restarter." + KubedoopDomain + "/expires-at."
)
