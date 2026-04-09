package generator

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/gsid-nl/kdef/internal/types"
)

// GenerateServiceAccount creates a ServiceAccount manifest.
func GenerateServiceAccount(sa types.ServiceAccountConfig, namespace string) *corev1.ServiceAccount {
	var pullSecrets []corev1.LocalObjectReference
	for _, s := range sa.ImagePullSecrets {
		pullSecrets = append(pullSecrets, corev1.LocalObjectReference{Name: s})
	}

	return &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      sa.Name,
			Namespace: namespace,
			Labels:    kdefLabels(map[string]string{}),
		},
		ImagePullSecrets: pullSecrets,
	}
}
