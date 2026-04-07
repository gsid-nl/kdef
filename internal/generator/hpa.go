package generator

import (
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/gsid-nl/kdef/internal/types"
)

// GenerateHPA creates a HorizontalPodAutoscaler for an app.
// Returns nil if no autoscale block is defined.
func GenerateHPA(app types.AppConfig) *autoscalingv2.HorizontalPodAutoscaler {
	if app.Autoscale == nil {
		return nil
	}

	as := app.Autoscale

	var metrics []autoscalingv2.MetricSpec

	if as.CPUPercent != nil {
		metrics = append(metrics, autoscalingv2.MetricSpec{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceCPU,
				Target: autoscalingv2.MetricTarget{
					Type:               autoscalingv2.UtilizationMetricType,
					AverageUtilization: as.CPUPercent,
				},
			},
		})
	}

	if as.MemPercent != nil {
		metrics = append(metrics, autoscalingv2.MetricSpec{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceMemory,
				Target: autoscalingv2.MetricTarget{
					Type:               autoscalingv2.UtilizationMetricType,
					AverageUtilization: as.MemPercent,
				},
			},
		})
	}

	// Default to CPU 80% if no metrics specified
	if len(metrics) == 0 {
		defaultCPU := int32(80)
		metrics = append(metrics, autoscalingv2.MetricSpec{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceCPU,
				Target: autoscalingv2.MetricTarget{
					Type:               autoscalingv2.UtilizationMetricType,
					AverageUtilization: &defaultCPU,
				},
			},
		})
	}

	// Use custom selector if provided
	selector := map[string]string{
		"app.kubernetes.io/name": app.Name,
	}
	if len(app.Selector) > 0 {
		selector = app.Selector
	}
	_ = selector // selector not used in HPA, but kept for consistency

	return &autoscalingv2.HorizontalPodAutoscaler{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "autoscaling/v2",
			Kind:       "HorizontalPodAutoscaler",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      app.Name,
			Namespace: app.Namespace,
			Labels:    kdefLabels(map[string]string{"app.kubernetes.io/name": app.Name}),
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       app.Name,
			},
			MinReplicas: &as.Min,
			MaxReplicas: as.Max,
			Metrics:     metrics,
		},
	}
}
