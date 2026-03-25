package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SmartScalerSpec defines the desired state of SmartScaler
type SmartScalerSpec struct {

	// The deployment this scaler will manage
	TargetDeployment string `json:"targetDeployment"`

	// Minimum number of replicas
	MinReplicas int32 `json:"minReplicas"`

	// Maximum number of replicas
	MaxReplicas int32 `json:"maxReplicas"`

	// Metric source configuration
	Metric MetricSpec `json:"metric"`

	// Scaling thresholds and cooldown
	ScalingPolicy ScalingPolicySpec `json:"scalingPolicy"`
}

type MetricSpec struct {
	// Type of metric source: "prometheus", "kafka", "rabbitmq"
	Source string `json:"source"`

	// The metric endpoint or connection URL
	Endpoint string `json:"endpoint"`

	// Metric query (e.g Prometheus PromQL query or Kafka topic name)
	Query string `json:"query"`
}

type ScalingPolicySpec struct {
	// Scale UP when metric value per replica exceeds this
	ScaleUpThreshold int64 `json:"scaleUpThreshold"`

	// Scale DOWN when metric value per replica drops below this
	ScaleDownThreshold int64 `json:"scaleDownThreshold"`

	// Seconds to wait before scaling again (prevents flapping)
	CooldownSeconds int64 `json:"cooldownSeconds"`
}

// SmartScalerStatus defines the observed state of SmartScaler
type SmartScalerStatus struct {
	// Current number of replicas
	CurrentReplicas int32 `json:"currentReplicas,omitempty"`

	// Last time a scaling action was taken
	LastScaleTime *metav1.Time `json:"lastScaleTime,omitempty"`

	// Human readable status message
	Message string `json:"message,omitempty"`

	// Current metric value being tracked
	CurrentMetricValue int64 `json:"currentMetricValue,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Target",type="string",JSONPath=".spec.targetDeployment"
// +kubebuilder:printcolumn:name="MinReplicas",type="integer",JSONPath=".spec.minReplicas"
// +kubebuilder:printcolumn:name="MaxReplicas",type="integer",JSONPath=".spec.maxReplicas"
// +kubebuilder:printcolumn:name="CurrentReplicas",type="integer",JSONPath=".status.currentReplicas"
// +kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.message"

type SmartScaler struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SmartScalerSpec   `json:"spec,omitempty"`
	Status SmartScalerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

type SmartScalerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SmartScaler `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SmartScaler{}, &SmartScalerList{})
}
