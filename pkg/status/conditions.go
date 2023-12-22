package status

// ConditionType is the type of condition
const (
	ConditionTypeProgressing string = "Progressing"
	ConditionTypeReconcile   string = "Reconcile"
	ConditionTypeAvailable   string = "Available"
)

// ConditionReason is the reason for the condition
const (
	ConditionReasonPreparing           string = "Preparing"
	ConditionReasonRunning             string = "Running"
	ConditionReasonConfig              string = "Config"
	ConditionReasonReconcilePVC        string = "ReconcilePVC"
	ConditionReasonReconcileService    string = "ReconcileService"
	ConditionReasonReconcileIngress    string = "ReconcileIngress"
	ConditionReasonReconcileDeployment string = "ReconcileDeployment"
)
