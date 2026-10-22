package generator

import (
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/gsid-nl/kdef/internal/types"
)

// GenerateCSIDriver creates a CSIDriver from a CSIDriverConfig.
func GenerateCSIDriver(d types.CSIDriverConfig) *storagev1.CSIDriver {
	spec := storagev1.CSIDriverSpec{
		AttachRequired:    &d.AttachRequired,
		PodInfoOnMount:    &d.PodInfoOnMount,
		RequiresRepublish: &d.RequiresRepublish,
	}
	if d.FSGroupPolicy != "" {
		p := storagev1.FSGroupPolicy(d.FSGroupPolicy)
		spec.FSGroupPolicy = &p
	}
	for _, m := range d.LifecycleModes {
		spec.VolumeLifecycleModes = append(spec.VolumeLifecycleModes, storagev1.VolumeLifecycleMode(m))
	}
	return &storagev1.CSIDriver{
		TypeMeta: metav1.TypeMeta{APIVersion: "storage.k8s.io/v1", Kind: "CSIDriver"},
		ObjectMeta: metav1.ObjectMeta{
			Name:   d.Name,
			Labels: kdefLabels(map[string]string{"app.kubernetes.io/name": d.Name}),
		},
		Spec: spec,
	}
}
