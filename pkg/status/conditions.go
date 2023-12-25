package status

// ConditionType is the type of condition
const (
	ConditionTypeProgressing         string = "Progressing"
	ConditionTypeReconcile           string = "Reconcile"
	ConditionTypeAvailable           string = "Available"
	ConditionTypeReconcilePVC        string = "ReconcilePVC"
	ConditionTypeReconcileService    string = "ReconcileService"
	ConditionTypeReconcileIngress    string = "ReconcileIngress"
	ConditionTypeReconcileDeployment string = "ReconcileDeployment"
	ConditionTypeReconcileSecret     string = "ReconcileSecret"
	ConditionTypeReconcileDaemonSet  string = "ReconcileDaemonSet"
	ConditionTypeReconcileConfigMap  string = "ReconcileConfigMap"
)

// ConditionReason is the reason for the condition
const (
	ConditionReasonPreparing string = "Preparing"
	ConditionReasonRunning   string = "Running"
	ConditionReasonConfig    string = "Config"
	ConditionReasonReady     string = "Ready"
	ConditionReasonFail      string = "Fail"
)
