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

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// The framework exports metrics for exactly one thing: the orphan cleanup state machine. That is
// deliberate, and the boundary is worth stating so this file does not grow into a second, worse
// copy of tools that already exist.
//
//   - controller-runtime already exports reconcile counts, error counts and durations
//     (controller_runtime_reconcile_*). Re-exporting those per cluster would add cardinality and no
//     information.
//   - kube-state-metrics already turns CR status conditions into metrics once it is configured for
//     the product's CRD. Available/Progressing/Degraded belong there, not here.
//
// What neither covers is the cleanup state machine, because it is internal to this SDK: it runs
// across many reconciles, records its progress in annotations on the objects it is retiring, and
// reports the rest through log lines. A role group stuck mid-teardown for three days produces no
// error, no failing reconcile and no condition transition — only an annotation nobody reads.
const (
	metricsNamespace = "operator_go"

	// Every series here is scoped to one cluster CR, so both labels are always this pair.
	metricLabelNamespace = "namespace"
	metricLabelCluster   = "cluster"
)

var (
	// OrphanCleanupPending is the number of role groups whose resources the framework has not
	// finished reclaiming, per cluster.
	//
	// The CR also carries this as the OrphanCleanupPending condition, which is the right place for a
	// human reading `kubectl describe`. The metric exists because turning a CR condition into an
	// alertable series needs kube-state-metrics configured for that CRD, which is a per-deployment
	// step an operator author cannot do on the user's behalf — and a teardown that never finishes is
	// exactly the state worth alerting on: the pods keep running, the PVCs keep costing, and nothing
	// else says so.
	OrphanCleanupPending = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Name:      "orphan_cleanup_pending",
		Help: "Number of role groups whose orphaned resources have not finished being reclaimed. " +
			"A value that stays above zero means a teardown is stuck.",
	}, []string{metricLabelNamespace, metricLabelCluster})

	// OrphanDrainTimeouts counts the times the framework gave up waiting for an orphaned
	// StatefulSet's ordered drain and deleted it with pods still terminating.
	//
	// This is the one event in the teardown with no other surface at all — today it is a single
	// log line. It also deserves one more than anything else the cleaner does: reaching it means a
	// stateful product was denied the ordered shutdown the scale-to-zero existed to give it, so a
	// pod was killed mid-flush. It is a counter rather than a gauge because the interesting question
	// is "did this ever happen", and the answer must survive the operator restarting.
	OrphanDrainTimeouts = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: metricsNamespace,
		Name:      "orphan_drain_timeouts_total",
		Help: "Number of orphaned StatefulSets deleted with pods still terminating, because the " +
			"ordered drain exceeded DrainTimeout.",
	}, []string{metricLabelNamespace, metricLabelCluster})
)

func init() {
	// The manager's registry, so these appear on the metrics endpoint an operator already serves
	// without any wiring in main.go.
	metrics.Registry.MustRegister(OrphanCleanupPending, OrphanDrainTimeouts)
}

// forgetClusterMetrics drops every series belonging to one cluster. Without it a deleted CR leaves
// its last gauge value published forever, and an alert on "cleanup pending" would fire for a
// cluster that no longer exists — the classic way a metric outlives the thing it measured.
func forgetClusterMetrics(namespace, cluster string) {
	labels := prometheus.Labels{metricLabelNamespace: namespace, metricLabelCluster: cluster}
	OrphanCleanupPending.Delete(labels)
	OrphanDrainTimeouts.Delete(labels)
}
