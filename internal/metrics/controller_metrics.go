package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// Counts how many scale up actions have happened
	ScaleUpTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "smartscaler_scale_up_total",
			Help: "Total number of scale up actions performed",
		},
		[]string{"namespace", "scaler", "deployment"},
	)

	// Counts how many scale down actions have happened
	ScaleDownTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "smartscaler_scale_down_total",
			Help: "Total number of scale down actions performed",
		},
		[]string{"namespace", "scaler", "deployment"},
	)

	// Tracks the current replica count per scaler
	CurrentReplicas = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "smartscaler_current_replicas",
			Help: "Current number of replicas managed by SmartScaler",
		},
		[]string{"namespace", "scaler", "deployment"},
	)

	// Tracks the raw metric value being used for scaling decisions
	CurrentMetricValue = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "smartscaler_current_metric_value",
			Help: "Current external metric value being tracked",
		},
		[]string{"namespace", "scaler", "source"},
	)

	// Counts reconcile errors
	ReconcileErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "smartscaler_reconcile_errors_total",
			Help: "Total number of reconcile errors",
		},
		[]string{"namespace", "scaler"},
	)
)

func init() {
	// Register all metrics with controller-runtime's registry
	// They will be served on the /metrics endpoint automatically
	metrics.Registry.MustRegister(
		ScaleUpTotal,
		ScaleDownTotal,
		CurrentReplicas,
		CurrentMetricValue,
		ReconcileErrors,
	)
}
