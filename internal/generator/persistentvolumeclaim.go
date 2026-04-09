package generator

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/gsid-nl/kdef/internal/types"
)

// GeneratePersistentVolumeClaim creates a PVC from a PersistentVolumeClaimConfig.
func GeneratePersistentVolumeClaim(pvc types.PersistentVolumeClaimConfig) *corev1.PersistentVolumeClaim {
	accessModes := make([]corev1.PersistentVolumeAccessMode, 0, len(pvc.AccessModes))
	for _, mode := range pvc.AccessModes {
		accessModes = append(accessModes, corev1.PersistentVolumeAccessMode(mode))
	}
	if len(accessModes) == 0 {
		accessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
	}

	result := &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "PersistentVolumeClaim",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvc.Name,
			Namespace: pvc.Namespace,
			Labels:    kdefLabels(map[string]string{}),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: accessModes,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(pvc.Storage),
				},
			},
		},
	}

	if pvc.StorageClass != "" {
		result.Spec.StorageClassName = &pvc.StorageClass
	}

	return result
}
