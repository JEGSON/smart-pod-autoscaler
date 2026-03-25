package v1alpha1

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	autoscalerv1alpha1 "github.com/JEGSON/smart-pod-autoscaler/api/v1alpha1"
)

var webhookLogger = log.Log.WithName("smartscaler-webhook")

func SetupSmartScalerWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &autoscalerv1alpha1.SmartScaler{}).
		WithDefaulter(&SmartScalerCustomDefaulter{}).
		WithValidator(&SmartScalerCustomValidator{}).
		Complete()
}

// ── DEFAULTING WEBHOOK ──────────────────────────────────────────────

type SmartScalerCustomDefaulter struct{}

var _ admission.Defaulter[*autoscalerv1alpha1.SmartScaler] = &SmartScalerCustomDefaulter{}

func (d *SmartScalerCustomDefaulter) Default(ctx context.Context, scaler *autoscalerv1alpha1.SmartScaler) error {
	webhookLogger.Info("Applying defaults", "name", scaler.Name)

	if scaler.Spec.MinReplicas == 0 {
		scaler.Spec.MinReplicas = 1
	}
	if scaler.Spec.MaxReplicas == 0 {
		scaler.Spec.MaxReplicas = 10
	}
	if scaler.Spec.ScalingPolicy.CooldownSeconds == 0 {
		scaler.Spec.ScalingPolicy.CooldownSeconds = 60
	}
	if scaler.Spec.Metric.Source == "" {
		scaler.Spec.Metric.Source = "prometheus"
	}

	return nil
}

// ── VALIDATING WEBHOOK ─────────────────────────────────────────────

type SmartScalerCustomValidator struct{}

var _ admission.Validator[*autoscalerv1alpha1.SmartScaler] = &SmartScalerCustomValidator{}

func (v *SmartScalerCustomValidator) ValidateCreate(ctx context.Context, scaler *autoscalerv1alpha1.SmartScaler) (admission.Warnings, error) {
	webhookLogger.Info("Validating create", "name", scaler.Name)
	return validate(scaler)
}

func (v *SmartScalerCustomValidator) ValidateUpdate(ctx context.Context, oldScaler, newScaler *autoscalerv1alpha1.SmartScaler) (admission.Warnings, error) {
	webhookLogger.Info("Validating update", "name", newScaler.Name)
	return validate(newScaler)
}

func (v *SmartScalerCustomValidator) ValidateDelete(ctx context.Context, scaler *autoscalerv1alpha1.SmartScaler) (admission.Warnings, error) {
	return nil, nil
}

// ── SHARED VALIDATION ──────────────────────────────────────────────

func validate(scaler *autoscalerv1alpha1.SmartScaler) (admission.Warnings, error) {
	var warnings admission.Warnings

	if scaler.Spec.TargetDeployment == "" {
		return nil, fmt.Errorf("targetDeployment must be set")
	}
	if scaler.Spec.MinReplicas < 1 {
		return nil, fmt.Errorf("minReplicas must be at least 1, got %d", scaler.Spec.MinReplicas)
	}
	if scaler.Spec.MaxReplicas <= scaler.Spec.MinReplicas {
		return nil, fmt.Errorf(
			"maxReplicas (%d) must be greater than minReplicas (%d)",
			scaler.Spec.MaxReplicas, scaler.Spec.MinReplicas,
		)
	}
	if scaler.Spec.MaxReplicas > 100 {
		return nil, fmt.Errorf("maxReplicas cannot exceed 100, got %d", scaler.Spec.MaxReplicas)
	}

	validSources := map[string]bool{"prometheus": true, "kafka": true}
	if !validSources[scaler.Spec.Metric.Source] {
		return nil, fmt.Errorf(
			"unsupported metric source %q — must be one of: prometheus, kafka",
			scaler.Spec.Metric.Source,
		)
	}
	if scaler.Spec.Metric.Endpoint == "" {
		return nil, fmt.Errorf("metric.endpoint must be set")
	}
	if scaler.Spec.Metric.Query == "" {
		return nil, fmt.Errorf("metric.query must be set")
	}
	if scaler.Spec.ScalingPolicy.ScaleUpThreshold <= scaler.Spec.ScalingPolicy.ScaleDownThreshold {
		return nil, fmt.Errorf(
			"scaleUpThreshold (%d) must be greater than scaleDownThreshold (%d)",
			scaler.Spec.ScalingPolicy.ScaleUpThreshold,
			scaler.Spec.ScalingPolicy.ScaleDownThreshold,
		)
	}
	if scaler.Spec.ScalingPolicy.CooldownSeconds < 10 {
		warnings = append(warnings, "cooldownSeconds is very low (<10s) — may cause replica flapping")
	}

	return warnings, nil
}
