package generator

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/gsid-nl/kdef/internal/types"
)

// GenerateConfigMap creates a ConfigMap from a ConfigMapConfig.
func GenerateConfigMap(cm types.ConfigMapConfig) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cm.Name,
			Namespace: cm.Namespace,
			Labels: kdefLabels(map[string]string{}),
		},
		Data: cm.Data,
	}
}
