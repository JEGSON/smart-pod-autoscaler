package controller_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	autoscalerv1alpha1 "github.com/JEGSON/smart-pod-autoscaler/api/v1alpha1"
)

// ── Helpers ──────────────────────────────────────────────────────────────────

func makeDeployment(name string) *appsv1.Deployment {
	const namespace = "default"
	replicas := int32(2)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": name},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "app", Image: "nginx"},
					},
				},
			},
		},
	}
}

func makeScaler(name, deployment string) *autoscalerv1alpha1.SmartScaler {
	const namespace = "default"
	return &autoscalerv1alpha1.SmartScaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: autoscalerv1alpha1.SmartScalerSpec{
			TargetDeployment: deployment,
			MinReplicas:      1,
			MaxReplicas:      10,
			Metric: autoscalerv1alpha1.MetricSpec{
				Source:   "prometheus",
				Endpoint: "http://localhost:9090",
				Query:    "sum(kube_pod_info)",
			},
			ScalingPolicy: autoscalerv1alpha1.ScalingPolicySpec{
				// stub returns 850
				// with 2 replicas: 850/2 = 425
				// 425 > 200 → scale UP fires   ✅
				// 425 > 50  → scale DOWN won't fire ✅
				ScaleUpThreshold:   200,
				ScaleDownThreshold: 50,
				CooldownSeconds:    10, // low so tests run fast
			},
		},
	}
}

// ── Test Suite ────────────────────────────────────────────────────────────────

var _ = Describe("SmartScaler Controller", func() {
	const (
		namespace = "default"
		timeout   = 20 * time.Second
		interval  = 500 * time.Millisecond
	)

	ctx := context.Background()

	BeforeEach(func() {
		// Ensure we use a predictable metric value for all tests
		reconciler.TestMetricFetcher = func(ctx context.Context, scaler *autoscalerv1alpha1.SmartScaler) (int64, error) {
			return 850, nil
		}
	})

	// ── TEST 1: Scale up when metric exceeds threshold ────────────────────
	Describe("Scaling behaviour", func() {
		It("should scale up the deployment when metric exceeds scaleUpThreshold", func() {
			deployName := "scale-up-test"
			scalerName := "scaler-scale-up"

			// Create the target deployment with 2 replicas
			deploy := makeDeployment(deployName)
			Expect(k8sClient.Create(ctx, deploy)).To(Succeed())

			// Create the SmartScaler
			scaler := makeScaler(scalerName, deployName)
			Expect(k8sClient.Create(ctx, scaler)).To(Succeed())

			// Wait for controller to scale the deployment up
			// stub metric=850, replicas=2, per-replica=425 > threshold=200 → scale up
			deployKey := types.NamespacedName{Name: deployName, Namespace: namespace}
			Eventually(func() int32 {
				d := &appsv1.Deployment{}
				if err := k8sClient.Get(ctx, deployKey, d); err != nil {
					return -1
				}
				return *d.Spec.Replicas
			}, timeout, interval).Should(BeNumerically(">", 2))

			// Cleanup
			Expect(k8sClient.Delete(ctx, scaler)).To(Succeed())
			Expect(k8sClient.Delete(ctx, deploy)).To(Succeed())
		})
	})

	// ── TEST 2: Never scale below minReplicas ─────────────────────────────
	Describe("Min/Max replica bounds", func() {
		It("should never scale below minReplicas", func() {
			deployName := "min-replicas-test"
			scalerName := "scaler-min"

			deploy := makeDeployment(deployName)
			Expect(k8sClient.Create(ctx, deploy)).To(Succeed())

			scaler := makeScaler(scalerName, deployName)
			scaler.Spec.MinReplicas = 2
			// Push scaleDownThreshold very high so scale down always wants to fire
			// but minReplicas should block it
			scaler.Spec.ScalingPolicy.ScaleDownThreshold = 99999
			scaler.Spec.ScalingPolicy.ScaleUpThreshold = 100000 // prevent scale up too
			Expect(k8sClient.Create(ctx, scaler)).To(Succeed())

			deployKey := types.NamespacedName{Name: deployName, Namespace: namespace}
			Consistently(func() int32 {
				d := &appsv1.Deployment{}
				if err := k8sClient.Get(ctx, deployKey, d); err != nil {
					return -1
				}
				return *d.Spec.Replicas
			}, 6*time.Second, interval).Should(BeNumerically(">=", 2))

			Expect(k8sClient.Delete(ctx, scaler)).To(Succeed())
			Expect(k8sClient.Delete(ctx, deploy)).To(Succeed())
		})

		// ── TEST 3: Never scale above maxReplicas ─────────────────────────
		It("should never scale above maxReplicas", func() {
			deployName := "max-replicas-test"
			scalerName := "scaler-max"

			deploy := makeDeployment(deployName)
			Expect(k8sClient.Create(ctx, deploy)).To(Succeed())

			scaler := makeScaler(scalerName, deployName)
			scaler.Spec.MaxReplicas = 3
			scaler.Spec.ScalingPolicy.ScaleUpThreshold = 1 // always trigger scale up
			Expect(k8sClient.Create(ctx, scaler)).To(Succeed())

			deployKey := types.NamespacedName{Name: deployName, Namespace: namespace}

			// Wait until it hits the ceiling
			Eventually(func() int32 {
				d := &appsv1.Deployment{}
				if err := k8sClient.Get(ctx, deployKey, d); err != nil {
					return -1
				}
				return *d.Spec.Replicas
			}, timeout, interval).Should(Equal(int32(3)))

			// Confirm it stays at max and never exceeds it
			Consistently(func() int32 {
				d := &appsv1.Deployment{}
				if err := k8sClient.Get(ctx, deployKey, d); err != nil {
					return -1
				}
				return *d.Spec.Replicas
			}, 6*time.Second, interval).Should(BeNumerically("<=", 3))

			Expect(k8sClient.Delete(ctx, scaler)).To(Succeed())
			Expect(k8sClient.Delete(ctx, deploy)).To(Succeed())
		})
	})

	// ── TEST 4: Handles missing deployment gracefully ─────────────────────
	Describe("Error handling", func() {
		It("should update status with error message when deployment not found", func() {
			scalerName := "scaler-no-deploy"

			// Point scaler at a deployment that doesn't exist
			scaler := makeScaler(scalerName, "non-existent-deployment")
			Expect(k8sClient.Create(ctx, scaler)).To(Succeed())

			scalerKey := types.NamespacedName{Name: scalerName, Namespace: namespace}
			Eventually(func() string {
				s := &autoscalerv1alpha1.SmartScaler{}
				if err := k8sClient.Get(ctx, scalerKey, s); err != nil {
					return ""
				}
				return s.Status.Message
			}, timeout, interval).Should(ContainSubstring("not found"))

			Expect(k8sClient.Delete(ctx, scaler)).To(Succeed())
		})
	})

	// ── TEST 5: Status is written back after reconcile ────────────────────
	Describe("Status updates", func() {
		It("should update currentReplicas in status after reconcile", func() {
			deployName := "status-test-deploy"
			scalerName := "scaler-status"

			deploy := makeDeployment(deployName)
			Expect(k8sClient.Create(ctx, deploy)).To(Succeed())

			scaler := makeScaler(scalerName, deployName)
			Expect(k8sClient.Create(ctx, scaler)).To(Succeed())

			scalerKey := types.NamespacedName{Name: scalerName, Namespace: namespace}
			Eventually(func() int32 {
				s := &autoscalerv1alpha1.SmartScaler{}
				if err := k8sClient.Get(ctx, scalerKey, s); err != nil {
					return -1
				}
				return s.Status.CurrentReplicas
			}, timeout, interval).Should(BeNumerically(">", 0))

			Expect(k8sClient.Delete(ctx, scaler)).To(Succeed())
			Expect(k8sClient.Delete(ctx, deploy)).To(Succeed())
		})

		It("should record the current metric value in status", func() {
			deployName := "metric-status-deploy"
			scalerName := "scaler-metric-status"

			deploy := makeDeployment(deployName)
			Expect(k8sClient.Create(ctx, deploy)).To(Succeed())

			scaler := makeScaler(scalerName, deployName)
			Expect(k8sClient.Create(ctx, scaler)).To(Succeed())

			scalerKey := types.NamespacedName{Name: scalerName, Namespace: namespace}
			// stub always returns 850 — status should reflect that
			Eventually(func() int64 {
				s := &autoscalerv1alpha1.SmartScaler{}
				if err := k8sClient.Get(ctx, scalerKey, s); err != nil {
					return -1
				}
				return s.Status.CurrentMetricValue
			}, timeout, interval).Should(Equal(int64(850)))

			Expect(k8sClient.Delete(ctx, scaler)).To(Succeed())
			Expect(k8sClient.Delete(ctx, deploy)).To(Succeed())
		})
	})

	// ── TEST 6: Clean deletion ────────────────────────────────────────────
	Describe("Deletion", func() {
		It("should handle SmartScaler deletion without errors", func() {
			deployName := "delete-test-deploy"
			scalerName := "scaler-delete"

			deploy := makeDeployment(deployName)
			Expect(k8sClient.Create(ctx, deploy)).To(Succeed())

			scaler := makeScaler(scalerName, deployName)
			Expect(k8sClient.Create(ctx, scaler)).To(Succeed())

			// Delete and confirm it's gone
			Expect(k8sClient.Delete(ctx, scaler)).To(Succeed())

			scalerKey := types.NamespacedName{Name: scalerName, Namespace: namespace}
			Eventually(func() bool {
				s := &autoscalerv1alpha1.SmartScaler{}
				err := k8sClient.Get(ctx, scalerKey, s)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			Expect(k8sClient.Delete(ctx, deploy)).To(Succeed())
		})
	})
})
