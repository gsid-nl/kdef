package generator

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/gsid-nl/kdef/internal/types"
)

// GenerateSecret creates a Kubernetes Secret from a SecretConfig.
func GenerateSecret(s types.SecretConfig) *corev1.Secret {
	secretType := s.Type
	if secretType == "" {
		secretType = "Opaque"
	}

	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.Name,
			Namespace: s.Namespace,
			Labels:    kdefLabels(map[string]string{}),
		},
		Type:       corev1.SecretType(secretType),
		StringData: s.Data,
	}
}
