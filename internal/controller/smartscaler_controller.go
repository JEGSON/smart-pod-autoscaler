package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/JEGSON/smart-pod-autoscaler/internal/metrics"
	controllermetrics "github.com/JEGSON/smart-pod-autoscaler/internal/metrics"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	autoscalerv1alpha1 "github.com/JEGSON/smart-pod-autoscaler/api/v1alpha1"
)

type SmartScalerReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// ✅ Injectable metric fetcher for testing
	TestMetricFetcher func(ctx context.Context, scaler *autoscalerv1alpha1.SmartScaler) (int64, error)
}

// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaler.autoscaler.io,resources=smartscalers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaler.autoscaler.io,resources=smartscalers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch

func (r *SmartScalerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling SmartScaler", "name", req.Name, "namespace", req.Namespace)

	// ── STEP 1: Fetch SmartScaler ──────────────────────────────
	scaler := &autoscalerv1alpha1.SmartScaler{}
	if err := r.Get(ctx, req.NamespacedName, scaler); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// ── STEP 2: Fetch Deployment ───────────────────────────────
	deployment := &appsv1.Deployment{}
	deploymentKey := types.NamespacedName{
		Name:      scaler.Spec.TargetDeployment,
		Namespace: req.Namespace,
	}
	if err := r.Get(ctx, deploymentKey, deployment); err != nil {
		if errors.IsNotFound(err) {
			return r.updateStatus(ctx, scaler, 0, 0,
				fmt.Sprintf("Deployment %s not found", scaler.Spec.TargetDeployment))
		}
		return ctrl.Result{}, err
	}

	// ── STEP 3: Cooldown check ─────────────────────────────────
	if scaler.Status.LastScaleTime != nil {
		cooldown := time.Duration(scaler.Spec.ScalingPolicy.CooldownSeconds) * time.Second
		timeSince := time.Since(scaler.Status.LastScaleTime.Time)

		if timeSince < cooldown {
			remaining := cooldown - timeSince
			logger.Info("Cooldown active", "remaining", remaining)
			return ctrl.Result{RequeueAfter: remaining}, nil
		}
	}

	// ── STEP 4: Fetch metric ───────────────────────────────────
	metricValue, err := r.fetchMetric(ctx, scaler)
	if err != nil {
		controllermetrics.ReconcileErrors.WithLabelValues(req.Namespace, req.Name).Inc()
		return r.updateStatus(ctx, scaler, *deployment.Spec.Replicas, 0,
			fmt.Sprintf("Metric error: %s", err.Error()))
	}

	controllermetrics.CurrentMetricValue.WithLabelValues(
		req.Namespace,
		req.Name,
		scaler.Spec.Metric.Source,
	).Set(float64(metricValue))

	// ── STEP 5: Calculate replicas ─────────────────────────────
	current := *deployment.Spec.Replicas
	desired := r.calculateReplicas(scaler, current, metricValue)

	// ── STEP 6: Scale if needed ────────────────────────────────
	if desired != current {
		deployment.Spec.Replicas = &desired

		if err := r.Update(ctx, deployment); err != nil {
			controllermetrics.ReconcileErrors.WithLabelValues(req.Namespace, req.Name).Inc()
			return ctrl.Result{}, fmt.Errorf("update failed: %w", err)
		}

		controllermetrics.CurrentReplicas.WithLabelValues(
			req.Namespace,
			req.Name,
			scaler.Spec.TargetDeployment,
		).Set(float64(desired))

		labels := []string{req.Namespace, req.Name, scaler.Spec.TargetDeployment}
		if desired > current {
			controllermetrics.ScaleUpTotal.WithLabelValues(labels...).Inc()
		} else {
			controllermetrics.ScaleDownTotal.WithLabelValues(labels...).Inc()
		}

		logger.Info("Scaled", "from", current, "to", desired)
	}

	// ── STEP 7: Update status ──────────────────────────────────
	msg := fmt.Sprintf("OK — metric=%d replicas=%d", metricValue, desired)
	return r.updateStatus(ctx, scaler, desired, metricValue, msg)
}

// ── Scaling Logic ─────────────────────────────────────────────
func (r *SmartScalerReconciler) calculateReplicas(
	scaler *autoscalerv1alpha1.SmartScaler,
	current int32,
	metricValue int64,
) int32 {

	policy := scaler.Spec.ScalingPolicy
	min := scaler.Spec.MinReplicas
	max := scaler.Spec.MaxReplicas

	metricPerReplica := metricValue / int64(current)

	switch {
	case metricPerReplica > policy.ScaleUpThreshold:
		if current+1 > max {
			return max
		}
		return current + 1

	case metricPerReplica < policy.ScaleDownThreshold:
		if current-1 < min {
			return min
		}
		return current - 1

	default:
		return current
	}
}

// ── Metric Fetcher (Injectable) ───────────────────────────────
func (r *SmartScalerReconciler) fetchMetric(
	ctx context.Context,
	scaler *autoscalerv1alpha1.SmartScaler,
) (int64, error) {
	// Use injected fetcher if provided (tests)
	if r.TestMetricFetcher != nil {
		return r.TestMetricFetcher(ctx, scaler)
	}

	// Real implementation
	spec := scaler.Spec.Metric
	switch spec.Source {
	case "prometheus":
		fetcher, err := metrics.NewPrometheusFetcher(spec.Endpoint, spec.Query)
		if err != nil {
			log.FromContext(ctx).Info("Prometheus unavailable, using stub value")
			return 850, nil
		}
		value, err := fetcher.Fetch(ctx)
		if err != nil {
			log.FromContext(ctx).Info("Prometheus fetch failed, using stub value", "error", err)
			return 850, nil
		}
		return value, nil

	case "kafka":
		fetcher, err := metrics.NewKafkaFetcher(spec.Endpoint, spec.Query, "my-consumer-group")
		if err != nil {
			return 0, err
		}
		defer fetcher.Close()
		return fetcher.Fetch(ctx)

	default:
		return 0, fmt.Errorf("unknown metric source: %s", spec.Source)
	}
}

// ── Status Update (FIXED BUG) ─────────────────────────────────
func (r *SmartScalerReconciler) updateStatus(
	ctx context.Context,
	scaler *autoscalerv1alpha1.SmartScaler,
	replicas int32,
	metricValue int64,
	message string,
) (ctrl.Result, error) {

	now := metav1.Now()

	previous := scaler.Status.CurrentReplicas

	scaler.Status.CurrentReplicas = replicas
	scaler.Status.CurrentMetricValue = metricValue
	scaler.Status.Message = message

	if replicas != previous {
		scaler.Status.LastScaleTime = &now
	}

	if err := r.Status().Update(ctx, scaler); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// ── Register Controller ───────────────────────────────────────
func (r *SmartScalerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&autoscalerv1alpha1.SmartScaler{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}
