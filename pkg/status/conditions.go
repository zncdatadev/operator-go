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

package status

// ConditionType is the type of condition
const (
	ConditionTypeProgressing          string = "Progressing"
	ConditionTypeReconcile            string = "Reconcile"
	ConditionTypeAvailable            string = "Available"
	ConditionTypeReconcilePVC         string = "ReconcilePVC"
	ConditionTypeReconcileService     string = "ReconcileService"
	ConditionTypeReconcileIngress     string = "ReconcileIngress"
	ConditionTypeReconcileDeployment  string = "ReconcileDeployment"
	ConditionTypeReconcileSecret      string = "ReconcileSecret"
	ConditionTypeReconcileDaemonSet   string = "ReconcileDaemonSet"
	ConditionTypeReconcileConfigMap   string = "ReconcileConfigMap"
	ConditionTypeReconcileStatefulSet string = "ReconcileStatefulSet"
)

// ConditionReason is the reason for the condition
const (
	ConditionReasonPreparing string = "Preparing"
	ConditionReasonRunning   string = "Running"
	ConditionReasonConfig    string = "Config"
	ConditionReasonReady     string = "Ready"
	ConditionReasonFail      string = "Fail"
)
